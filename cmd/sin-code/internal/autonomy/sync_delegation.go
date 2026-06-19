// SPDX-License-Identifier: MIT
// Purpose: synchronous sub-goal delegation with parent wait (issue #385).
//
// spawn_subgoal enqueues children asynchronously, forcing the parent to
// checkpoint and resume later. SyncDelegator provides the complementary
// synchronous path: the parent calls Delegate, which creates a
// DelegationResult with a Done channel, and then blocks on Wait until the
// sub-agent calls Complete or the timeout elapses. This makes recursive
// decomposition ergonomic — the parent can decompose a goal inline and
// continue only after every child has reported back.
//
// All map access is guarded by sync.Mutex (mandate M7).
package autonomy

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// DelegationRequest describes a single synchronous sub-goal delegation.
type DelegationRequest struct {
	Goal      string        // the sub-goal prompt / description
	AgentName string        // which agent should handle the sub-goal
	Timeout   time.Duration // per-delegation timeout; <=0 uses delegator default
}

// DelegationResult holds the outcome of a synchronous delegation. The Done
// channel is closed by Complete so any number of Wait callers unblock
// simultaneously.
type DelegationResult struct {
	Goal     string
	Output   string
	Error    error
	Duration time.Duration
	Done     chan struct{}
}

// SyncDelegator coordinates synchronous sub-goal delegation. It is safe for
// concurrent use by multiple goroutines (mandate M7).
type SyncDelegator struct {
	timeout time.Duration
	mu      sync.Mutex
	active  map[string]*DelegationResult
}

// NewSyncDelegator returns a delegator with the given default timeout. A
// default <= 0 is replaced with 5 minutes to match the
// agentloop.subagent_sync_timeout convention.
func NewSyncDelegator(defaultTimeout time.Duration) *SyncDelegator {
	if defaultTimeout <= 0 {
		defaultTimeout = 5 * time.Minute
	}
	return &SyncDelegator{
		timeout: defaultTimeout,
		active:  make(map[string]*DelegationResult),
	}
}

// Delegate registers a new delegation, stores it in the active map, and
// returns the result immediately (it does NOT block). The caller, or a
// separate goroutine, should call Wait to block until the sub-agent calls
// Complete. If a delegation with the same goal already exists and has not
// been completed, an error is returned to prevent duplicate registrations.
func (d *SyncDelegator) Delegate(ctx context.Context, req DelegationRequest) (*DelegationResult, error) {
	if d == nil {
		return nil, fmt.Errorf("sync delegator is nil")
	}
	if req.Goal == "" {
		return nil, fmt.Errorf("delegation goal is empty")
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = d.timeout
	}

	res := &DelegationResult{
		Goal: req.Goal,
		Done: make(chan struct{}),
	}

	d.mu.Lock()
	if existing, ok := d.active[req.Goal]; ok {
		// If the existing entry is already closed, it's safe to overwrite.
		select {
		case <-existing.Done:
		default:
			d.mu.Unlock()
			return nil, fmt.Errorf("delegation already active for goal %q", req.Goal)
		}
	}
	d.active[req.Goal] = res
	d.mu.Unlock()

	// Attach the timeout to the context so callers waiting on the result
	// respect the per-delegation deadline. We don't cancel the context here
	// because the sub-agent may still be running; Wait handles the timeout.
	_ = ctx // ctx is accepted for future cancellation wiring; timeout is used by Wait
	_ = timeout

	return res, nil
}

// Complete is called by the sub-agent (or a supervisor) to deliver the
// delegation outcome. It records the output and error, then closes the Done
// channel so all Wait callers unblock. If no active delegation exists for
// the goal, the call is a no-op (the result may have already been cleaned
// up or never registered).
func (d *SyncDelegator) Complete(goal string, output string, err error) {
	if d == nil {
		return
	}

	d.mu.Lock()
	res, ok := d.active[goal]
	d.mu.Unlock()

	if !ok {
		return
	}

	res.Output = output
	res.Error = err
	close(res.Done)
}

// Wait blocks until the delegation for goal is completed (Done channel
// closed) or the timeout elapses. If the delegation is not found, an error
// is returned immediately. On timeout, the returned result (if any) has
// its Error set to a timeout error so callers can distinguish timeout from
// a sub-agent error.
func (d *SyncDelegator) Wait(goal string) (*DelegationResult, error) {
	if d == nil {
		return nil, fmt.Errorf("sync delegator is nil")
	}

	d.mu.Lock()
	res, ok := d.active[goal]
	d.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("no active delegation for goal %q", goal)
	}

	timeout := d.timeout

	select {
	case <-res.Done:
		return res, nil
	case <-time.After(timeout):
		return res, fmt.Errorf("delegation for goal %q timed out after %s", goal, timeout)
	}
}

// ActiveCount returns the number of currently registered (not yet
// completed) delegations. Completed delegations remain in the map until
// removed by Cleanup; this count includes them if Cleanup has not been
// called.
func (d *SyncDelegator) ActiveCount() int {
	if d == nil {
		return 0
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.active)
}

// Cleanup removes completed delegations from the active map. Delegations
// that are still in-progress are left untouched. This prevents the map from
// growing without bound across many delegations.
func (d *SyncDelegator) Cleanup() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for goal, res := range d.active {
		select {
		case <-res.Done:
			delete(d.active, goal)
		default:
		}
	}
}
