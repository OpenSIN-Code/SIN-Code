// SPDX-License-Identifier: MIT
// Purpose: GoalStore is a high-level wrapper around Queue that exposes only
// the operations needed by the todo<->goal bridge (issue #317). It keeps the
// bridge decoupled from SQLite details and is safe for concurrent use (M7)
// because Queue serializes access through database transactions.
package autonomy

import (
	"context"
	"fmt"
	"time"
)

// GoalStore wraps an open Queue, exposing the subset of operations the
// todo<->goal bridge needs. A nil GoalStore is safe to call — every method
// returns a descriptive error instead of panicking. A nil context is
// silently replaced with context.Background() so callers can omit it.
type GoalStore struct {
	queue *Queue
}

func ensureCtx(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// NewGoalStore wraps an open Queue. Returns nil if q is nil so callers can
// detect a missing autonomy backend without a separate error path.
func NewGoalStore(q *Queue) *GoalStore {
	if q == nil {
		return nil
	}
	return &GoalStore{queue: q}
}

// AddGoal inserts a goal and returns its new ID. Empty workspace defaults to
// "." and max_retries <= 0 defaults to 3.
func (s *GoalStore) AddGoal(ctx context.Context, g *Goal) (int64, error) {
	if s == nil || s.queue == nil {
		return 0, fmt.Errorf("autonomy: goal store not initialized")
	}
	if g == nil {
		return 0, fmt.Errorf("autonomy: nil goal")
	}
	ws := g.Workspace
	if ws == "" {
		ws = "."
	}
	maxRetries := g.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	return s.queue.AddWithContract(ensureCtx(ctx), g.Prompt, ws, g.Priority, maxRetries, g.Contract)
}

// GetGoal returns a goal by ID, or (nil, nil) if not found.
func (s *GoalStore) GetGoal(ctx context.Context, id int64) (*Goal, error) {
	if s == nil || s.queue == nil {
		return nil, fmt.Errorf("autonomy: goal store not initialized")
	}
	return s.queue.Get(ensureCtx(ctx), id)
}

// CompleteGoal marks a goal verified (honouring child-tree finalization).
func (s *GoalStore) CompleteGoal(ctx context.Context, id int64, sessionID string) error {
	if s == nil || s.queue == nil {
		return fmt.Errorf("autonomy: goal store not initialized")
	}
	return s.queue.Complete(ensureCtx(ctx), id, sessionID)
}

// FailGoal records a failure; the goal returns to pending until its retry
// budget is spent, then becomes exhausted.
func (s *GoalStore) FailGoal(ctx context.Context, id int64, sessionID, errMsg string) error {
	if s == nil || s.queue == nil {
		return fmt.Errorf("autonomy: goal store not initialized")
	}
	return s.queue.Fail(ensureCtx(ctx), id, sessionID, errMsg)
}

// LeaseGoal atomically claims the highest-priority leasable goal. Returns
// (nil, nil) when no goal is available.
func (s *GoalStore) LeaseGoal(ctx context.Context, leaseDur time.Duration) (*Goal, error) {
	if s == nil || s.queue == nil {
		return nil, fmt.Errorf("autonomy: goal store not initialized")
	}
	return s.queue.Lease(ensureCtx(ctx), leaseDur)
}

// List returns goals filtered by status ("" = all), newest first.
func (s *GoalStore) List(ctx context.Context, status GoalStatus) ([]Goal, error) {
	if s == nil || s.queue == nil {
		return nil, fmt.Errorf("autonomy: goal store not initialized")
	}
	return s.queue.List(ensureCtx(ctx), status)
}

// Close releases the underlying queue.
func (s *GoalStore) Close() error {
	if s == nil || s.queue == nil {
		return nil
	}
	return s.queue.Close()
}
