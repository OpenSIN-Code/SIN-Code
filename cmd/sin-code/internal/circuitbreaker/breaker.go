// SPDX-License-Identifier: MIT
// Purpose: circuit breaker for external HTTP callers. State machine with
// three states (Closed / Open / HalfOpen), thread-safe, stdlib-only
// (sync, time, errors, fmt — no math, no third-party).
//
// Design rationale:
//   - One sync.Mutex owns ALL state. Reads/writes of state, counters, and
//     the opened-at timestamp are SHORT (single-digit ns) and dwarfed
//     by the network call inside Execute(fn). Splitting into multiple
//     atomics + locks would invite subtle ordering bugs (state vs.
//     counter visibility across CPUs) for negligible throughput gain.
//     The simplicity tax is worth paying.
//   - The mutex is RELEASED while fn() runs (Execute takes the lock only
//     for the bookkeeping above and below the call).
//   - Open → HalfOpen is a TIME-driven transition: any IsAllowed() call
//     after openedAt + OpenDuration finds the breaker eligible for a
//     probe and admits at most HalfOpenProbes concurrent ones.
//   - HalfOpen → Closed / Open is OUTCOME-driven. Successes accumulate
//     until SuccessThreshold; one failure re-opens immediately.
//
// Docs: breaker.doc.md
package circuitbreaker

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// State is the current state of a Breaker. Render to string for logs,
// metrics, and /debug endpoints.
type State int32

const (
	// StateClosed: traffic flows normally. Failure counter accumulates.
	StateClosed State = iota
	// StateOpen: all IsAllowed() return ErrBreakerOpen until openedAt
	// + OpenDuration has elapsed.
	StateOpen
	// StateHalfOpen: a probe window. Up to HalfOpenProbes concurrent
	// calls are admitted; the first failure re-opens, SuccessThreshold
	// consecutive successes close the breaker again.
	StateHalfOpen
)

// String renders a State. Useful for slog/log fields, metrics labels,
// and the ceo-audit-readable /debug/breaker endpoint.
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return fmt.Sprintf("state(%d)", int(s))
	}
}

// ErrBreakerOpen is returned by IsAllowed() and Execute(fn) when the
// breaker is rejecting calls without invoking the protected function.
// Always use errors.Is(err, ErrBreakerOpen) — never string match.
var ErrBreakerOpen = errors.New("circuitbreaker: breaker is open")

// Config parameterizes a Breaker. Zero values fall back to safe
// defaults inside New() so callers only specify what they care about.
type Config struct {
	// FailureThreshold is the number of CONSECUTIVE failures (returned
	// error or recovered panic) that trips Closed → Open. Default 5.
	FailureThreshold int

	// OpenDuration is the wall-clock window during which the breaker
	// rejects all calls in Open state. After it elapses, the next
	// IsAllowed() transitions Open → HalfOpen. Default 10s.
	OpenDuration time.Duration

	// HalfOpenProbes is the maximum number of CONCURRENT probes allowed
	// in HalfOpen state. Excess callers get ErrBreakerOpen until one
	// or more probes complete (success or failure). Default 1.
	HalfOpenProbes int

	// SuccessThreshold is the number of CONSECUTIVE successes required
	// in HalfOpen to transition HalfOpen → Closed. Default 1.
	SuccessThreshold int

	// Now is overridable for deterministic tests. nil → time.Now.
	Now func() time.Time

	// Name is optional metadata that surfaces via String(). Useful when
	// one process owns many breakers (one per upstream) so logs can
	// distinguish them. Defaults to "breaker".
	Name string
}

// String renders the Config's identifying metadata. The numeric
// thresholds are intentionally NOT printed here so log lines stay
// scannable — those belong in /debug/status.
func (c *Config) String() string {
	if c == nil {
		return "breaker(none)"
	}
	if c.Name == "" {
		return "breaker(unnamed)"
	}
	return "breaker(" + c.Name + ")"
}

// Breaker is the public, concurrent-safe FSM. Construct with New().
// The zero value of *Breaker is NOT usable — call New().
type Breaker struct {
	cfg Config

	// All fields below this line are protected by mu.
	mu            sync.Mutex
	state         State
	openedAt      time.Time
	consecFails   int
	halfInFlight  int
	halfSuccesses int
}

// New builds a Breaker from cfg. A nil cfg produces a Breaker with the
// package defaults (FailureThreshold=5, OpenDuration=10s, …). cfg is
// copied by value so subsequent mutations to the caller's struct are
// not observed.
func New(cfg *Config) *Breaker {
	if cfg == nil {
		cfg = &Config{}
	}
	// Mutate a local copy so we don't surprise the caller.
	c := *cfg
	c.applyDefaults()
	if c.Name == "" {
		c.Name = "unnamed"
	}
	return &Breaker{cfg: c, state: StateClosed}
}

// State returns the current state. Acquires and releases the mutex; safe
// for concurrent inspection (metrics, /metrics, /healthz).
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// IsAllowed is the "can I call right now?" predicate. Returns nil if
// the breaker would allow the call; returns ErrBreakerOpen otherwise.
//
// IsAllowed does NOT run any function. The caller is responsible for
// invoking RecordSuccess / RecordFailure if they want the FSM to learn
// from the call. For the common path of "run fn guarded by breaker",
// prefer Execute(fn) which handles both admission and bookkeeping.
//
// IsAllowed MAY admit a probe in HalfOpen state (incrementing
// halfInFlight). If the caller decides not to actually issue the probe,
// they must call RecordFailure (or RecordSuccess for an unrelated
// admission) so the probe slot is returned. Execute handles this
// correctly; manual IsAllowed users should match.
func (b *Breaker) IsAllowed() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.maybeAdmitLocked() {
		return ErrBreakerOpen
	}
	return nil
}

// RecordSuccess marks a successful call. Outside Execute(fn), this is
// a no-op in Open state (nothing was admitted) and clears the failure
// counter in Closed state. In HalfOpen it advances the success ladder.
func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.recordSuccessLocked()
}

// RecordFailure marks a failed call (or a previously-admitted probe
// that the caller chose not to issue). nil err is TREATED AS SUCCESS —
// callers should pass the real error or use RecordSuccess for a clean
// no-error outcome.
func (b *Breaker) RecordFailure(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	_ = err
	b.recordFailureLocked()
}

// Stats is a snapshot of the breaker's observable counters. Returned
// by Stats() under the lock, so the fields are consistent.
type Stats struct {
	Name              string
	State             State
	ConsecutiveFails  int
	HalfOpenInFlight  int
	HalfOpenSuccesses int
}

// Stats returns a consistent snapshot of observable counters.
func (b *Breaker) Stats() Stats {
	b.mu.Lock()
	defer b.mu.Unlock()
	return Stats{
		Name:              b.cfg.Name,
		State:             b.state,
		ConsecutiveFails:  b.consecFails,
		HalfOpenInFlight:  b.halfInFlight,
		HalfOpenSuccesses: b.halfSuccesses,
	}
}

// Execute runs fn under the breaker. If the current state rejects the
// call, returns ErrBreakerOpen WITHOUT running fn. Otherwise:
//   - fn runs with the breaker mutex RELEASED.
//   - on returned error OR panic (recovered), the breaker records a
//     failure and the original error is returned to the caller.
//   - on a nil error, the breaker records a success and nil is
//     returned to the caller.
//
// Execute is the required primary API. IsAllowed / RecordFailure /
// RecordSuccess exist for cases where the caller needs finer control
// (e.g. they want to issue an HTTP request themselves and map a 5xx
// to a failure explicitly).
func (b *Breaker) Execute(fn func() error) error {
	if err := b.IsAllowed(); err != nil {
		return err
	}

	// Run fn outside the lock; recover panics so a buggy upstream
	// cannot crash the agent loop. The recovered error is counted as
	// a failure.
	var funcErr error
	var panicked bool
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
				funcErr = fmt.Errorf("circuitbreaker: panic in protected fn: %v", r)
			}
		}()
		funcErr = fn()
	}()

	b.mu.Lock()
	defer b.mu.Unlock()
	if panicked || funcErr != nil {
		b.recordFailureLocked()
	} else {
		b.recordSuccessLocked()
	}
	return funcErr
}

// ── internal helpers (must be called with b.mu held) ─────────────────

// maybeAdmitLocked runs the time-driven Open → HalfOpen transition
// (if any), then evaluates whether the caller may proceed. On admit
// it bumps halfInFlight. Returns true/false; never errors.
//
// Single source of truth for "is this call allowed?" semantics so
// IsAllowed() and the bookkeeping path stay in lock-step.
func (b *Breaker) maybeAdmitLocked() bool {
	b.maybeTransitionOpenToHalfOpen()
	switch b.state {
	case StateClosed:
		return true
	case StateHalfOpen:
		if b.halfInFlight >= b.cfg.HalfOpenProbes {
			return false
		}
		b.halfInFlight++
		return true
	case StateOpen:
		return false
	}
	return false
}

// maybeTransitionOpenToHalfOpen is the time-driven transition: if the
// breaker has been Open for longer than OpenDuration, flip it to
// HalfOpen and clear all half-open bookkeeping so the probe wave
// starts clean. Idempotent.
func (b *Breaker) maybeTransitionOpenToHalfOpen() {
	if b.state != StateOpen {
		return
	}
	if b.cfg.Now().Sub(b.openedAt) >= b.cfg.OpenDuration {
		b.state = StateHalfOpen
		b.halfInFlight = 0
		b.halfSuccesses = 0
	}
}

// recordSuccessLocked handles a successful call under the lock.
// Behavior depends on current state.
func (b *Breaker) recordSuccessLocked() {
	switch b.state {
	case StateClosed:
		b.consecFails = 0
	case StateHalfOpen:
		if b.halfInFlight > 0 {
			b.halfInFlight--
		}
		b.halfSuccesses++
		if b.halfSuccesses >= b.cfg.SuccessThreshold {
			b.state = StateClosed
			b.consecFails = 0
			b.halfSuccesses = 0
			b.halfInFlight = 0
		}
	case StateOpen:
		// Defensive: success for a call that should have been
		// rejected. This means a caller admitted outside the
		// breaker's contract (e.g. raw HTTP without the wrapped
		// transport). Surface it as a half-open debt return so the
		// in-flight accounting stays sane.
		if b.halfInFlight > 0 {
			b.halfInFlight--
		}
	}
}

// recordFailureLocked handles a failed call under the lock.
func (b *Breaker) recordFailureLocked() {
	switch b.state {
	case StateClosed:
		b.consecFails++
		if b.consecFails >= b.cfg.FailureThreshold {
			b.state = StateOpen
			b.openedAt = b.cfg.Now()
		}
	case StateHalfOpen:
		if b.halfInFlight > 0 {
			b.halfInFlight--
		}
		// Any probe failure re-opens immediately and clears the
		// success ladder — one failure is enough to bail.
		b.state = StateOpen
		b.openedAt = b.cfg.Now()
		b.halfSuccesses = 0
	case StateOpen:
		// Defensive: failure for a call that should have been
		// rejected. Same accounting correction as success.
		if b.halfInFlight > 0 {
			b.halfInFlight--
		}
	}
}

// applyDefaults fills zero-valued fields with safe defaults. The
// defaults are tuned for "general external HTTP upstreams": 5
// consecutive failures, 10 seconds of rejection, 1 concurrent probe.
func (c *Config) applyDefaults() {
	if c.FailureThreshold <= 0 {
		c.FailureThreshold = 5
	}
	if c.OpenDuration <= 0 {
		c.OpenDuration = 10 * time.Second
	}
	if c.HalfOpenProbes <= 0 {
		c.HalfOpenProbes = 1
	}
	if c.SuccessThreshold <= 0 {
		c.SuccessThreshold = 1
	}
	if c.Now == nil {
		c.Now = time.Now
	}
}
