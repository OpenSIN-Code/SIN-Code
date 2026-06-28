// SPDX-License-Identifier: MIT
// Purpose: tests for GoalStore — the high-level wrapper around Queue used by
// the todo<->goal bridge (issue #317). Covers nil-safety, context fallback,
// and the full CRUD lifecycle.
package autonomy

import (
	"context"
	"testing"
	"time"
)

func TestGoalStore_NewGoalStore(t *testing.T) {
	q := openTestQueue(t)

	store := NewGoalStore(q)
	if store == nil {
		t.Fatal("NewGoalStore returned nil for non-nil queue")
	}

	// nil queue → nil store
	nilStore := NewGoalStore(nil)
	if nilStore != nil {
		t.Fatal("NewGoalStore(nil) should return nil")
	}
}

func TestGoalStore_AddAndGet(t *testing.T) {
	q := openTestQueue(t)
	store := NewGoalStore(q)

	id, err := store.AddGoal(context.Background(), &Goal{
		Prompt:    "test goal",
		Workspace: "test-ws",
		Priority:  5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}

	goal, err := store.GetGoal(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if goal == nil {
		t.Fatal("GetGoal returned nil for existing id")
	}
	if goal.Prompt != "test goal" {
		t.Errorf("prompt = %q, want %q", goal.Prompt, "test goal")
	}
	if goal.Workspace != "test-ws" {
		t.Errorf("workspace = %q, want %q", goal.Workspace, "test-ws")
	}
}

func TestGoalStore_AddGoal_Defaults(t *testing.T) {
	q := openTestQueue(t)
	store := NewGoalStore(q)

	// Empty workspace defaults to "."; max_retries <= 0 defaults to 3
	id, err := store.AddGoal(context.Background(), &Goal{
		Prompt: "defaults test",
	})
	if err != nil {
		t.Fatal(err)
	}

	goal, _ := store.GetGoal(context.Background(), id)
	if goal.Workspace != "." {
		t.Errorf("workspace = %q, want %q", goal.Workspace, ".")
	}
	if goal.MaxRetries != 3 {
		t.Errorf("max_retries = %d, want 3", goal.MaxRetries)
	}
}

func TestGoalStore_AddGoal_NilGoal(t *testing.T) {
	q := openTestQueue(t)
	store := NewGoalStore(q)

	_, err := store.AddGoal(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil goal")
	}
}

func TestGoalStore_GetGoal_NotFound(t *testing.T) {
	q := openTestQueue(t)
	store := NewGoalStore(q)

	goal, err := store.GetGoal(context.Background(), 9999)
	if err != nil {
		t.Fatal(err)
	}
	if goal != nil {
		t.Fatalf("expected nil for non-existent goal, got %+v", goal)
	}
}

func TestGoalStore_LeaseGoal(t *testing.T) {
	q := openTestQueue(t)
	store := NewGoalStore(q)

	_, err := store.AddGoal(context.Background(), &Goal{
		Prompt:    "leaseable goal",
		Workspace: "ws",
		Priority:  10,
	})
	if err != nil {
		t.Fatal(err)
	}

	goal, err := store.LeaseGoal(context.Background(), 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if goal == nil {
		t.Fatal("expected non-nil leased goal")
	}
	if goal.Prompt != "leaseable goal" {
		t.Errorf("prompt = %q, want %q", goal.Prompt, "leaseable goal")
	}
	if goal.Status != StatusRunning {
		t.Errorf("status = %q, want %q", goal.Status, StatusRunning)
	}
}

func TestGoalStore_LeaseGoal_Empty(t *testing.T) {
	q := openTestQueue(t)
	store := NewGoalStore(q)

	goal, err := store.LeaseGoal(context.Background(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if goal != nil {
		t.Fatalf("expected nil for empty queue, got %+v", goal)
	}
}

func TestGoalStore_CompleteGoal(t *testing.T) {
	q := openTestQueue(t)
	store := NewGoalStore(q)

	id, _ := store.AddGoal(context.Background(), &Goal{
		Prompt: "completable",
	})

	err := store.CompleteGoal(context.Background(), id, "session-1")
	if err != nil {
		t.Fatal(err)
	}

	goal, _ := store.GetGoal(context.Background(), id)
	if goal.Status != StatusVerified {
		t.Errorf("status = %q, want %q", goal.Status, StatusVerified)
	}
	if goal.SessionID != "session-1" {
		t.Errorf("session_id = %q, want %q", goal.SessionID, "session-1")
	}
}

func TestGoalStore_FailGoal(t *testing.T) {
	q := openTestQueue(t)
	store := NewGoalStore(q)

	id, _ := store.AddGoal(context.Background(), &Goal{
		Prompt:     "failable",
		MaxRetries: 5,
	})

	// Lease first so attempts increments
	_, _ = store.LeaseGoal(context.Background(), time.Minute)

	err := store.FailGoal(context.Background(), id, "sess", "test failure")
	if err != nil {
		t.Fatal(err)
	}

	goal, _ := store.GetGoal(context.Background(), id)
	if goal.Status != StatusPending {
		t.Errorf("status = %q, want %q (should retry)", goal.Status, StatusPending)
	}
	if goal.LastError != "test failure" {
		t.Errorf("last_error = %q, want %q", goal.LastError, "test failure")
	}
}

func TestGoalStore_FailGoal_Exhausted(t *testing.T) {
	q := openTestQueue(t)
	store := NewGoalStore(q)

	id, _ := store.AddGoal(context.Background(), &Goal{
		Prompt:     "exhaust me",
		MaxRetries: 1,
	})

	// Lease + fail once: attempts=1, max_retries=1 → exhausted
	_, _ = store.LeaseGoal(context.Background(), time.Minute)
	_ = store.FailGoal(context.Background(), id, "", "boom")

	goal, _ := store.GetGoal(context.Background(), id)
	if goal.Status != StatusExhausted {
		t.Errorf("status = %q, want %q", goal.Status, StatusExhausted)
	}
}

func TestGoalStore_List(t *testing.T) {
	q := openTestQueue(t)
	store := NewGoalStore(q)

	_, _ = store.AddGoal(context.Background(), &Goal{Prompt: "goal 1"})
	_, _ = store.AddGoal(context.Background(), &Goal{Prompt: "goal 2"})

	goals, err := store.List(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(goals) != 2 {
		t.Errorf("expected 2 goals, got %d", len(goals))
	}

	// Filter by pending status
	pending, err := store.List(context.Background(), StatusPending)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Errorf("expected 2 pending, got %d", len(pending))
	}
}

func TestGoalStore_List_FilterByStatus(t *testing.T) {
	q := openTestQueue(t)
	store := NewGoalStore(q)

	id1, _ := store.AddGoal(context.Background(), &Goal{Prompt: "g1"})
	_, _ = store.AddGoal(context.Background(), &Goal{Prompt: "g2"})

	_ = store.CompleteGoal(context.Background(), id1, "s")

	verified, _ := store.List(context.Background(), StatusVerified)
	if len(verified) != 1 {
		t.Errorf("expected 1 verified, got %d", len(verified))
	}
	pending, _ := store.List(context.Background(), StatusPending)
	if len(pending) != 1 {
		t.Errorf("expected 1 pending, got %d", len(pending))
	}
}

func TestGoalStore_Close(t *testing.T) {
	q := openTestQueue(t)
	store := NewGoalStore(q)

	if err := store.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestGoalStore_NilStoreSafety(t *testing.T) {
	var store *GoalStore

	// All methods on a nil store should return errors, not panic
	if _, err := store.AddGoal(context.Background(), &Goal{Prompt: "x"}); err == nil {
		t.Error("AddGoal on nil store should error")
	}
	if _, err := store.GetGoal(context.Background(), 1); err == nil {
		t.Error("GetGoal on nil store should error")
	}
	if err := store.CompleteGoal(context.Background(), 1, ""); err == nil {
		t.Error("CompleteGoal on nil store should error")
	}
	if err := store.FailGoal(context.Background(), 1, "", ""); err == nil {
		t.Error("FailGoal on nil store should error")
	}
	if _, err := store.LeaseGoal(context.Background(), time.Minute); err == nil {
		t.Error("LeaseGoal on nil store should error")
	}
	if _, err := store.List(context.Background(), ""); err == nil {
		t.Error("List on nil store should error")
	}
	// Close on nil store is a no-op (returns nil)
	if err := store.Close(); err != nil {
		t.Errorf("Close on nil store should return nil, got %v", err)
	}
}

func TestGoalStore_NilContextFallback(t *testing.T) {
	q := openTestQueue(t)
	store := NewGoalStore(q)

	// Passing nil context should not panic — ensureCtx replaces it
	id, err := store.AddGoal(nil, &Goal{Prompt: "nil ctx"})
	if err != nil {
		t.Fatalf("AddGoal with nil ctx: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id with nil ctx")
	}

	goal, err := store.GetGoal(nil, id)
	if err != nil {
		t.Fatalf("GetGoal with nil ctx: %v", err)
	}
	if goal == nil || goal.Prompt != "nil ctx" {
		t.Fatalf("unexpected goal: %+v", goal)
	}
}

func TestEnsureCtx(t *testing.T) {
	// nil → non-nil background context
	ctx := ensureCtx(nil)
	if ctx == nil {
		t.Fatal("ensureCtx(nil) returned nil")
	}

	// non-nil → same context returned
	bg := context.Background()
	ctx2 := ensureCtx(bg)
	if ctx2 != bg {
		t.Fatal("ensureCtx should return the same context when non-nil")
	}
}
