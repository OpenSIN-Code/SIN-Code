// SPDX-License-Identifier: MIT
// Purpose: queue regression tests (mandate: bounded autonomy).
package autonomy

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func openTestQueue(t *testing.T) *Queue {
	t.Helper()
	q, err := Open(filepath.Join(t.TempDir(), "g.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { q.Close() })
	return q
}

func TestAddAssignsID(t *testing.T) {
	q := openTestQueue(t)
	ctx := context.Background()
	id, err := q.Add(ctx, "test prompt", "/tmp/ws", 5, 3)
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}
}

func TestLeaseClaimsHighestPriority(t *testing.T) {
	q := openTestQueue(t)
	ctx := context.Background()
	_, _ = q.Add(ctx, "low", "/tmp", 1, 1)
	_, _ = q.Add(ctx, "high", "/tmp", 10, 1)
	_, _ = q.Add(ctx, "mid", "/tmp", 5, 1)
	g, err := q.Lease(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if g == nil {
		t.Fatal("expected a lease, got nil")
	}
	if g.Priority != 10 {
		t.Fatalf("expected priority 10, got %d", g.Priority)
	}
	if g.Status != StatusRunning {
		t.Fatalf("expected status running, got %q", g.Status)
	}
}

func TestLeaseReturnsNilWhenEmpty(t *testing.T) {
	q := openTestQueue(t)
	g, err := q.Lease(context.Background(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if g != nil {
		t.Fatalf("expected nil, got %+v", g)
	}
}

func TestLeaseReclaimsStaleRunning(t *testing.T) {
	q := openTestQueue(t)
	ctx := context.Background()
	_, _ = q.Add(ctx, "stale", "/tmp", 0, 5)
	g, _ := q.Lease(ctx, time.Minute)
	if g == nil {
		t.Fatal("expected first lease")
	}
	// Backdate lease_until to a year ago to simulate a crashed worker.
	_, err := q.db.Exec(`UPDATE goals SET lease_until = ? WHERE id = ?`,
		time.Now().UTC().Add(-365*24*time.Hour).Format(time.RFC3339), g.ID)
	if err != nil {
		t.Fatal(err)
	}
	g2, _ := q.Lease(ctx, time.Minute)
	if g2 == nil || g2.ID != g.ID {
		t.Fatalf("expected reclaim of stale lease, got %+v", g2)
	}
	if g2.Attempts != g.Attempts+1 {
		t.Fatalf("attempts should have incremented: was %d, now %d", g.Attempts, g2.Attempts)
	}
}

func TestCompleteSetsVerified(t *testing.T) {
	q := openTestQueue(t)
	id, _ := q.Add(context.Background(), "task", "/tmp", 1, 1)
	g, _ := q.Lease(context.Background(), time.Minute)
	if err := q.Complete(context.Background(), g.ID, "sess-1"); err != nil {
		t.Fatal(err)
	}
	goals, _ := q.List(context.Background(), StatusVerified)
	if len(goals) != 1 || goals[0].ID != id || goals[0].SessionID != "sess-1" {
		t.Fatalf("verification not recorded: %+v", goals)
	}
}

func TestFailExhaustsAfterMaxRetries(t *testing.T) {
	q := openTestQueue(t)
	id, _ := q.Add(context.Background(), "task", "/tmp", 0, 2)
	// max_retries=2 means attempts 1+2 are leaseable; on the 2nd Fail,
	// attempts(2) >= max_retries(2) → exhausted. So 2 leases are enough.
	for i := 0; i < 2; i++ {
		g, _ := q.Lease(context.Background(), time.Minute)
		if g == nil {
			t.Fatalf("attempt %d: expected lease", i)
		}
		_ = q.Fail(context.Background(), g.ID, "", "boom")
	}
	goals, _ := q.List(context.Background(), "")
	if len(goals) != 1 || goals[0].ID != id {
		t.Fatal("goal lost")
	}
	if goals[0].Status != StatusExhausted {
		t.Fatalf("expected exhausted after retries, got %q", goals[0].Status)
	}
}

func TestListFiltersByStatus(t *testing.T) {
	q := openTestQueue(t)
	ctx := context.Background()
	_, _ = q.Add(ctx, "a", "/tmp", 1, 1)
	_, _ = q.Add(ctx, "b", "/tmp", 1, 1)
	_, _ = q.Add(ctx, "c", "/tmp", 1, 1)
	// Verify one
	g, _ := q.Lease(ctx, time.Minute)
	_ = q.Complete(ctx, g.ID, "")

	pending, _ := q.List(ctx, StatusPending)
	verified, _ := q.List(ctx, StatusVerified)
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending, got %d", len(pending))
	}
	if len(verified) != 1 {
		t.Fatalf("expected 1 verified, got %d", len(verified))
	}
}
