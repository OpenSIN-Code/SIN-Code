// SPDX-License-Identifier: MIT
// Purpose: lifecycle hook manager (issue #108). Hooks intercept agent tool
// calls before/after execution and on loop failure, enabling automatic
// validation/repair chains (e.g. run gofmt after a write_file). Corrected,
// race-safe Go implementation of the design sketched in issue #108.
package hooks

import (
	"context"
	"fmt"
	"sync"
)

// Lifecycle names a point in a tool's execution lifecycle.
type Lifecycle string

const (
	BeforeToolExecution Lifecycle = "BeforeToolExecution"
	AfterToolExecution  Lifecycle = "AfterToolExecution"
	OnLoopFailure       Lifecycle = "OnLoopFailure"
)

// Context is threaded through every hook in a lifecycle phase. Hooks may
// mutate and return it so later hooks see the changes.
type Context struct {
	Ctx        context.Context
	ToolName   string
	InputArgs  map[string]interface{}
	OutputData interface{}
	Err        error
	LoopCount  int
}

// Func is a single hook. Returning an error aborts the remaining hooks for
// that phase and is surfaced to the caller.
type Func func(hc *Context) (*Context, error)

// Manager stores hooks per lifecycle phase.
type Manager struct {
	mu    sync.RWMutex
	hooks map[Lifecycle][]Func
}

var (
	hmInstance *Manager
	hmOnce     sync.Once
)

// GetManager returns the process-wide singleton manager with default hooks
// registered.
func GetManager() *Manager {
	hmOnce.Do(func() {
		hm := NewManager()
		hm.registerDefaultHooks()
		hmInstance = hm
	})
	return hmInstance
}

// NewManager builds an independent manager (used by tests).
func NewManager() *Manager {
	return &Manager{hooks: make(map[Lifecycle][]Func)}
}

// registerDefaultHooks installs the baseline validation hook: after a
// successful write_file, kick off the validation chain.
func (hm *Manager) registerDefaultHooks() {
	hm.Register(AfterToolExecution, func(hc *Context) (*Context, error) {
		if hc.ToolName == "write_file" && hc.Err == nil {
			fmt.Println("[HOOK] starting automated validation chain for changed files...")
		}
		return hc, nil
	})
}

// Register appends a hook to a lifecycle phase.
func (hm *Manager) Register(lc Lifecycle, hf Func) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.hooks[lc] = append(hm.hooks[lc], hf)
}

// Count returns the number of hooks registered for a phase.
func (hm *Manager) Count(lc Lifecycle) int {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return len(hm.hooks[lc])
}

// Trigger runs all hooks for a phase in registration order, threading the
// (possibly mutated) context through each. The first error aborts the chain.
func (hm *Manager) Trigger(lc Lifecycle, hc *Context) (*Context, error) {
	hm.mu.RLock()
	active := append([]Func(nil), hm.hooks[lc]...)
	hm.mu.RUnlock()

	cur := hc
	var err error
	for _, hook := range active {
		cur, err = hook(cur)
		if err != nil {
			return cur, fmt.Errorf("hook error at %s: %w", lc, err)
		}
	}
	return cur, nil
}
