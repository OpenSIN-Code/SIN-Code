// SPDX-License-Identifier: MIT
// Purpose: tests for recursive goal trees, continuation budget, and
// discovery dedup (loop-engineering extensions) — AGENTS.md §8.
package autonomy

import (
	"context"
	"testing"
	"time"
)

func TestSubGoalDrainsDepthFirst(t *testing.T) {
	q := openTestQueue(t)
	ctx := context.Background()
	parent, err := q.Add(ctx, "parent", "/tmp/ws", 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	child, err := q.AddSub(ctx, parent, "child", 6, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	// The deeper child must be leased before the parent even though both are
	// pending, because the parent has an unverified child.
	g, _ := q.Lease(ctx, time.Minute)
	if g == nil || g.ID != child {
		t.Fatalf("expected child %d to lease first, got %+v", child, g)
	}
	// Parent is not leasable until the child verifies.
	if g2, _ := q.Lease(ctx, time.Minute); g2 != nil {
		t.Fatalf("parent should not lease while child unverified, got %+v", g2)
	}
}

func TestParentBlocksUntilChildVerified(t *testing.T) {
	q := openTestQueue(t)
	ctx := context.Background()
	parent, _ := q.Add(ctx, "parent", "/tmp/ws", 5, 1)
	child, _ := q.AddSub(ctx, parent, "child", 6, 1, "")

	// Complete the parent's own work first: it should go to blocked, not
	// verified, because the child is still pending.
	if err := q.Complete(ctx, parent, "sess-parent"); err != nil {
		t.Fatal(err)
	}
	pg, _ := q.Get(ctx, parent)
	if pg.Status != StatusBlocked {
		t.Fatalf("parent should be blocked, got %q", pg.Status)
	}

	// Now verify the child — the parent should auto-finalize via bubbleUp.
	if err := q.Complete(ctx, child, "sess-child"); err != nil {
		t.Fatal(err)
	}
	pg, _ = q.Get(ctx, parent)
	if pg.Status != StatusVerified {
		t.Fatalf("parent should finalize after child verifies, got %q", pg.Status)
	}
}

func TestChildInheritsWorkspaceAndDepth(t *testing.T) {
	q := openTestQueue(t)
	ctx := context.Background()
	parent, _ := q.Add(ctx, "parent", "/tmp/special-ws", 0, 1)
	childID, _ := q.AddSub(ctx, parent, "child", 1, 1, "")
	child, _ := q.Get(ctx, childID)
	if child.Workspace != "/tmp/special-ws" {
		t.Fatalf("child should inherit workspace, got %q", child.Workspace)
	}
	if child.Depth != 1 {
		t.Fatalf("child depth should be 1, got %d", child.Depth)
	}
	if child.ParentID != parent {
		t.Fatalf("child parent_id wrong: %d", child.ParentID)
	}
}

func TestAddSubMissingParent(t *testing.T) {
	q := openTestQueue(t)
	if _, err := q.AddSub(context.Background(), 9999, "orphan", 0, 1, ""); err == nil {
		t.Fatal("expected error for missing parent")
	}
}

func TestContinueRefundsAttemptAndBumpsCounter(t *testing.T) {
	q := openTestQueue(t)
	ctx := context.Background()
	id, _ := q.Add(ctx, "long", "/tmp", 0, 1)
	g, _ := q.Lease(ctx, time.Minute) // attempts -> 1
	if g.Attempts != 1 {
		t.Fatalf("expected 1 attempt after lease, got %d", g.Attempts)
	}
	n, err := q.Continue(ctx, id, "sess-1", "checkpoint")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected continuation count 1, got %d", n)
	}
	after, _ := q.Get(ctx, id)
	if after.Attempts != 0 {
		t.Fatalf("continue should refund the attempt, got %d", after.Attempts)
	}
	if after.Status != StatusPending {
		t.Fatalf("continued goal should be pending, got %q", after.Status)
	}
	// It must be leasable again (a long task resumes, never abandoned).
	if g2, _ := q.Lease(ctx, time.Minute); g2 == nil || g2.ID != id {
		t.Fatal("continued goal should be leasable again")
	}
}

func TestContractPersistsThroughLease(t *testing.T) {
	q := openTestQueue(t)
	ctx := context.Background()
	contract := `{"semantic_criteria":["docs updated"]}`
	id, err := q.AddWithContract(ctx, "task", "/tmp", 0, 1, contract)
	if err != nil {
		t.Fatal(err)
	}
	g, _ := q.Lease(ctx, time.Minute)
	if g == nil || g.ID != id {
		t.Fatal("expected lease")
	}
	if g.Contract != contract {
		t.Fatalf("contract not preserved through lease: %q", g.Contract)
	}
}

func TestAddDiscoveredDedup(t *testing.T) {
	q := openTestQueue(t)
	ctx := context.Background()
	key := "todo:foo.go:fix this"
	id1, added1, err := q.AddDiscovered(ctx, "fix this", "/tmp", key, 0, 3, "")
	if err != nil {
		t.Fatal(err)
	}
	if !added1 || id1 == 0 {
		t.Fatal("first discovery should be enqueued")
	}
	// Same key while the goal is still live → not re-enqueued.
	_, added2, err := q.AddDiscovered(ctx, "fix this", "/tmp", key, 0, 3, "")
	if err != nil {
		t.Fatal(err)
	}
	if added2 {
		t.Fatal("duplicate discovery should be skipped while goal is live")
	}

	// Once the goal is verified, the same key may be enqueued again (the
	// marker could legitimately reappear later).
	if err := q.Complete(ctx, id1, ""); err != nil {
		t.Fatal(err)
	}
	_, added3, err := q.AddDiscovered(ctx, "fix this", "/tmp", key, 0, 3, "")
	if err != nil {
		t.Fatal(err)
	}
	if !added3 {
		t.Fatal("after verification, a re-discovered item should enqueue again")
	}
}

func TestAddDiscoveredEmptyKeyAlwaysAdds(t *testing.T) {
	q := openTestQueue(t)
	ctx := context.Background()
	_, a1, _ := q.AddDiscovered(ctx, "x", "/tmp", "", 0, 3, "")
	_, a2, _ := q.AddDiscovered(ctx, "x", "/tmp", "", 0, 3, "")
	if !a1 || !a2 {
		t.Fatal("empty dedup key should never dedupe")
	}
}
