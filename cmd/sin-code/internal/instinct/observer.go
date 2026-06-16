// SPDX-License-Identifier: MIT
// Purpose: session-scoped observation buffer + flush to manager. The
// Observer is what the hook dispatcher (internal/learning) calls into.
// Docs: observer.doc.md
package instinct

import (
	"context"
	"sync"
)

// Observer buffers observations during a session and flushes them
// into instincts. Wire `Record` into your PostToolUse hooks and
// `Flush` into the Stop / SessionEnd hook.
type Observer struct {
	mu        sync.Mutex
	buf       []Observation
	mgr       *Manager
	extractor Extractor
}

// NewObserver returns a thread-safe Observer.
func NewObserver(mgr *Manager, ex Extractor) *Observer {
	if ex == nil {
		ex = HeuristicExtractor{MinRepeats: 2}
	}
	return &Observer{mgr: mgr, extractor: ex}
}

// Record captures a single tool/hook event. Cheap; never blocks on I/O.
func (o *Observer) Record(obs Observation) {
	o.mu.Lock()
	o.buf = append(o.buf, obs)
	o.mu.Unlock()
}

// Pending returns the number of buffered observations (for tests + CLI).
func (o *Observer) Pending() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.buf)
}

// Flush extracts candidates from the buffered observations and folds
// them into the store. Returns counts for telemetry.
func (o *Observer) Flush(ctx context.Context) (created, reinforced int, err error) {
	o.mu.Lock()
	batch := o.buf
	o.buf = nil
	o.mu.Unlock()
	if len(batch) == 0 {
		return 0, 0, nil
	}
	candidates, err := o.extractor.Extract(ctx, batch)
	if err != nil {
		return 0, 0, err
	}
	for _, c := range candidates {
		isNew, err := o.mgr.Observe(c)
		if err != nil {
			return created, reinforced, err
		}
		if isNew {
			created++
		} else {
			reinforced++
		}
	}
	return created, reinforced, nil
}
