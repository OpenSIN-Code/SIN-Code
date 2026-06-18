// SPDX-License-Identifier: MIT
// Purpose: tests for synchronous sub-goal delegation (issue #385)
// and the spawn_subgoal tool. Four core cases:
//
//  1. TestSpawnSubgoalSyncSuccess — happy path, parent depth 0,
//     simulated worker Completes the sub-goal within the timeout.
//  2. TestSpawnSubgoalSyncTimeout — sub-goal never terminates,
//     Timeout fires and the returned error wraps ErrSubgoalTimeout.
//  3. TestSpawnSubgoalDepthLimit — parentDepth 2 with maxDepth 2 is
//     rejected with ErrSubgoalDepthExceeded BEFORE enqueue.
//  4. TestSpawnSubgoalMaxDepthFlag — same shape, but the ceiling
//     comes from a config-style default — exercises the
//     defaultMaxDepth parameter path.
//
// Race-safe per mandate M7: all tests run with t.Parallel off; the
// queue is opened per-test, no shared state. The simulated worker in
// the success test uses raw SQL UPDATEs so it doesn't fight
// SpawnSubgoal's polling read for the single SQLite connection
// (modernc.org/sqlite is busy on concurrent writes from the same
// process — the API path is reserved for full-stack tests).
package agentloop

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
)

// openSpawnSubgoalQueue opens a fresh autonomy queue in a temp dir
// for the duration of the test. The closure flushes via t.Cleanup.
func openSpawnSubgoalQueue(t *testing.T) *autonomy.Queue {
	t.Helper()
	q, err := autonomy.Open(filepath.Join(t.TempDir(), "g.db"))
	if err != nil {
		t.Fatalf("autonomy.Open: %v", err)
	}
	t.Cleanup(func() { _ = q.Close() })
	return q
}

// childrenIDs returns the IDs of every direct child of parentID.
// Used by the success test to discover the just-spawned sub-goal.
func childrenIDs(t *testing.T, q *autonomy.Queue, parentID int64) []int64 {
	t.Helper()
	goals, err := q.Children(context.Background(), parentID)
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	out := make([]int64, 0, len(goals))
	for _, g := range goals {
		out = append(out, g.ID)
	}
	return out
}

// TestSpawnSubgoalSyncSuccess — happy path: a simulated worker
// flips the just-enqueued sub-goal's status to `verified` via the
// queue's Complete() API inside a goroutine; SpawnSubgoal polls
// and observes the terminal transition.
//
// Modernc.org/sqlite + WAL (set in autonomy.Open) lets the polling
// reader (SpawnSubgoal) and the writer (simulated worker) share
// the SQLite file without BUSY contention; we still retry on the
// rare transient to keep the test deterministic.
func TestSpawnSubgoalSyncSuccess(t *testing.T) {
	q := openSpawnSubgoalQueue(t)
	ctx := context.Background()

	parentID, err := q.Add(ctx, "parent", t.TempDir(), 0, 3)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})

	// Simulated worker: wait for the sub-goal to be enqueued
	// (under SpawnSubgoal); then drive it to verified via the
	// queue's Complete() with a retry loop for SQLite BUSY.
	go func() {
		defer close(done)
		deadline := time.Now().Add(3 * time.Second)
		var childID int64
		for time.Now().Before(deadline) {
			ids := childrenIDs(t, q, parentID)
			if len(ids) > 0 {
				childID = ids[0]
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if childID == 0 {
			t.Error("child goal never appeared in queue")
			return
		}
		const maxRetries = 100
		const retrySleep = 20 * time.Millisecond
		for i := 0; i < maxRetries; i++ {
			cerr := q.Complete(context.Background(), childID, "test-session")
			if cerr == nil {
				return
			}
			if !isTransientAutonomyErr(cerr) {
				t.Errorf("Complete: %v", cerr)
				return
			}
			time.Sleep(retrySleep)
		}
		t.Error("driveGoalToTerminal: timed out retrying Complete")
	}()

	res, err := SpawnSubgoal(ctx, q, parentID, 0, 2, SpawnSubgoalRequest{
		Description: "sub-task",
		Timeout:     5 * time.Second,
		Poll:        25 * time.Millisecond,
	})
	<-done
	if err != nil {
		t.Fatalf("SpawnSubgoal: %v", err)
	}
	if !res.Verified {
		t.Fatalf("expected verified=true, got %+v", res)
	}
	if res.Status != autonomy.StatusVerified {
		t.Fatalf("expected status=verified, got %q", res.Status)
	}
	if res.SessionID != "test-session" {
		t.Fatalf("expected session_id=test-session, got %q", res.SessionID)
	}
}

// isTransientAutonomyErr returns true for errors that benefit from
// a brief retry (SQLite BUSY / SQLITE_BUSY).
func isTransientAutonomyErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	for _, frag := range []string{"database is locked", "SQLITE_BUSY"} {
		if stringContains(s, frag) {
			return true
		}
	}
	return false
}

// stringContains is a tiny inline helper to avoid pulling strings
// into a test that doesn't otherwise need it.
func stringContains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestSpawnSubgoalSyncTimeout — sub-goal never terminates; the
// synchronous wait exceeds Timeout and ErrSubgoalTimeout comes back.
func TestSpawnSubgoalSyncTimeout(t *testing.T) {
	q := openSpawnSubgoalQueue(t)
	ctx := context.Background()

	parentID, err := q.Add(ctx, "parent", t.TempDir(), 0, 3)
	if err != nil {
		t.Fatal(err)
	}

	// No worker goroutine — the sub-goal stays pending forever; the
	// timeout must fire.
	_, err = SpawnSubgoal(ctx, q, parentID, 0, 2, SpawnSubgoalRequest{
		Description: "sub-task",
		Timeout:     150 * time.Millisecond,
		Poll:        25 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !errors.Is(err, ErrSubgoalTimeout) {
		t.Fatalf("expected ErrSubgoalTimeout wrapping, got %v", err)
	}
}

// TestSpawnSubgoalDepthLimit — parentDepth=2 with maxDepth=2 means
// the spawned child WOULD land at depth=3, exceeding the ceiling.
// SpawnSubgoal rejects BEFORE enqueue with ErrSubgoalDepthExceeded,
// so nothing is written to the queue.
func TestSpawnSubgoalDepthLimit(t *testing.T) {
	q := openSpawnSubgoalQueue(t)
	ctx := context.Background()
	parentID, err := q.Add(ctx, "parent at depth 2", t.TempDir(), 0, 3)
	if err != nil {
		t.Fatal(err)
	}

	_, err = SpawnSubgoal(ctx, q, parentID, 2, 2, SpawnSubgoalRequest{
		Description: "deep grandchild",
		Timeout:     time.Second,
		Poll:        50 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected depth-exceeded error, got nil")
	}
	if !errors.Is(err, ErrSubgoalDepthExceeded) {
		t.Fatalf("expected ErrSubgoalDepthExceeded wrapping, got %v", err)
	}

	// Verify nothing got enqueued.
	children, err := q.Children(ctx, parentID)
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 0 {
		t.Fatalf("depth-exceeded should be a pre-enqueue check, got children=%+v", children)
	}
}

// TestSpawnSubgoalMaxDepthFlag — same shape, but the ceiling comes
// from a config-style default (defaultMaxDepth=1) and the parent is
// at depth=1 (so child would land at depth=2, exceeding the
// ceiling).
func TestSpawnSubgoalMaxDepthFlag(t *testing.T) {
	q := openSpawnSubgoalQueue(t)
	ctx := context.Background()
	parentID, err := q.Add(ctx, "parent at depth 1", t.TempDir(), 0, 3)
	if err != nil {
		t.Fatal(err)
	}

	_, err = SpawnSubgoal(ctx, q, parentID, 1, 1, SpawnSubgoalRequest{
		Description: "child under tight ceiling",
		Timeout:     time.Second,
		Poll:        50 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected depth-exceeded error, got nil")
	}
	if !errors.Is(err, ErrSubgoalDepthExceeded) {
		t.Fatalf("expected ErrSubgoalDepthExceeded wrapping, got %v", err)
	}
}

// TestSpawnSubgoalNilQueue — defensive: nil queue must return
// ErrSubgoalQueue, never panic, never silently "succeed".
func TestSpawnSubgoalNilQueue(t *testing.T) {
	_, err := SpawnSubgoal(context.Background(), nil, 0, 0, 2, SpawnSubgoalRequest{
		Description: "x",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrSubgoalQueue) {
		t.Fatalf("expected ErrSubgoalQueue, got %v", err)
	}
}

// TestSpawnSubgoalEmptyDescription — description is required.
func TestSpawnSubgoalEmptyDescription(t *testing.T) {
	q := openSpawnSubgoalQueue(t)
	_, err := SpawnSubgoal(context.Background(), q, 0, 0, 2, SpawnSubgoalRequest{
		Description: "",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() == "" {
		t.Fatalf("expected description-required error, got %v", err)
	}
}

// TestEffectiveMaxDepth — small unit test for the override semantics.
func TestEffectiveMaxDepth(t *testing.T) {
	if got := effectiveMaxDepth(0, 0); got != SpawnSubgoalDefaultMaxDepth {
		t.Fatalf("effectiveMaxDepth(0,0): want %d, got %d", SpawnSubgoalDefaultMaxDepth, got)
	}
	if got := effectiveMaxDepth(0, 5); got != 5 {
		t.Fatalf("effectiveMaxDepth(0,5): want 5, got %d", got)
	}
	if got := effectiveMaxDepth(3, 5); got != 3 {
		t.Fatalf("effectiveMaxDepth(3,5): want 3, got %d", got)
	}
	if got := effectiveMaxDepth(3, 0); got != 3 {
		t.Fatalf("effectiveMaxDepth(3,0): want 3, got %d", got)
	}
}
