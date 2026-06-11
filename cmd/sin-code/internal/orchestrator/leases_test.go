// SPDX-License-Identifier: MIT
// Purpose: Hermetic unit tests for the Lease Coordinator (leases.go).
// Docs: leases.go
package orchestrator

import (
	"strings"
	"testing"
	"time"
)

// newTestTable returns a LeaseTable with the given TTL and registers a
// t.Cleanup to restore zero global state. Kept inline (not a helper file)
// because only these tests need it.
func newTestTable(t *testing.T, ttl time.Duration) *LeaseTable {
	t.Helper()
	lt := NewLeaseTable()
	lt.DefaultTTL = ttl
	return lt
}

func TestAcquireIsAllOrNothing(t *testing.T) {
	lt := newTestTable(t, 15*time.Minute)

	// 1) first acquire of "internal/tui/*" succeeds for agentA.
	l1, conflicts, err := lt.Acquire("agentA", "task-1", "refactor tui", []string{"internal/tui/*"})
	if err != nil {
		t.Fatalf("first acquire: unexpected error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("first acquire: expected no conflicts, got %d", len(conflicts))
	}
	if l1 == nil || l1.ID == 0 {
		t.Fatalf("first acquire: expected non-nil lease with ID, got %+v", l1)
	}

	// 2) second acquire by agentB with overlap must conflict AND must not
	//    leave agentB holding anything (verify by Count==1 afterwards).
	_, conflicts, err = lt.Acquire("agentB", "task-2", "edit tui view", []string{"internal/tui/view.go"})
	if err != nil {
		t.Fatalf("second acquire: unexpected error: %v", err)
	}
	if len(conflicts) == 0 {
		t.Fatalf("second acquire: expected conflicts, got none")
	}
	if got, want := lt.Count(), 1; got != want {
		t.Fatalf("after failed acquire, Count = %d, want %d (acquire must be all-or-nothing)", got, want)
	}

	// 3) a third agent can take a non-overlapping glob, proving agentB held nothing.
	_, conflicts, err = lt.Acquire("agentC", "task-3", "touch lsp", []string{"internal/lsp/client.go"})
	if err != nil {
		t.Fatalf("third acquire: unexpected error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("third acquire: expected no conflicts (only agentA's tui lease should exist), got %d", len(conflicts))
	}
	if got, want := lt.Count(), 2; got != want {
		t.Fatalf("after third acquire, Count = %d, want %d", got, want)
	}
}

func TestReleaseFreesPathsAndIsIdempotent(t *testing.T) {
	lt := newTestTable(t, 15*time.Minute)

	l, _, err := lt.Acquire("agentA", "task-1", "edit", []string{"internal/tui/*"})
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	// First release succeeds (no error expected for correct agent).
	if err := lt.Release(l.ID, "agentA"); err != nil {
		t.Fatalf("first release: %v", err)
	}
	if got, want := lt.Count(), 0; got != want {
		t.Fatalf("after release, Count = %d, want %d", got, want)
	}

	// Second release is idempotent (no error) — ID no longer exists.
	if err := lt.Release(l.ID, "agentA"); err != nil {
		t.Fatalf("second release (idempotent): expected nil error, got %v", err)
	}

	// Re-acquire with the same globs must succeed now that the path is free.
	_, conflicts, err := lt.Acquire("agentA", "task-2", "re-edit", []string{"internal/tui/*"})
	if err != nil {
		t.Fatalf("re-acquire: %v", err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("re-acquire: expected no conflicts, got %d", len(conflicts))
	}
	if got, want := lt.Count(), 1; got != want {
		t.Fatalf("after re-acquire, Count = %d, want %d", got, want)
	}
}

func TestReleaseRejectsWrongAgent(t *testing.T) {
	lt := newTestTable(t, 15*time.Minute)

	l, _, err := lt.Acquire("agentA", "task-1", "edit", []string{"internal/tui/*"})
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	if err := lt.Release(l.ID, "agentB"); err == nil {
		t.Fatalf("release with wrong agent: expected error, got nil")
	}
	// Lease must still be held.
	if got, want := lt.Count(), 1; got != want {
		t.Fatalf("after wrong-agent release, Count = %d, want %d (lease must remain)", got, want)
	}
}

func TestRenewExtendsTtl(t *testing.T) {
	lt := newTestTable(t, 50*time.Millisecond)
	lt.DefaultTTL = 50 * time.Millisecond // explicit for clarity

	l, _, err := lt.Acquire("agentA", "task-1", "edit", []string{"internal/tui/*"})
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	originalExpiry := l.ExpiresAt

	// Sleep past the original TTL so a renew-less check would expire.
	time.Sleep(80 * time.Millisecond)

	if err := lt.Renew(l.ID, "agentA"); err != nil {
		t.Fatalf("renew with correct agent: %v", err)
	}
	if !l.ExpiresAt.After(originalExpiry) {
		t.Fatalf("renew did not extend expiry: original=%v, after=%v", originalExpiry, l.ExpiresAt)
	}
}

func TestRenewRejectsForeign(t *testing.T) {
	lt := newTestTable(t, 15*time.Minute)

	l, _, err := lt.Acquire("agentA", "task-1", "edit", []string{"internal/tui/*"})
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	originalExpiry := l.ExpiresAt

	if err := lt.Renew(l.ID, "agentB"); err == nil {
		t.Fatalf("renew with wrong agent: expected error, got nil")
	}
	// Expiry must not have been touched.
	if !l.ExpiresAt.Equal(originalExpiry) {
		t.Fatalf("foreign renew must not change expiry: was=%v, now=%v", originalExpiry, l.ExpiresAt)
	}
}

func TestGlobsOverlapIsConservative(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"directory glob vs file inside", "internal/tui/*", "internal/tui/view.go", true},
		{"directory glob vs nested file", "internal/tui/*", "internal/tui/widgets/list.go", true},
		{"directory glob vs file elsewhere", "internal/tui/*", "internal/lsp/client.go", false},
		{"identical filename", "go.sum", "go.sum", true},
		{"sibling directory globs", "cmd/*", "internal/*", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// globsOverlap is symmetric — assert both directions to lock that in.
			if got := globsOverlap(tc.a, tc.b); got != tc.want {
				t.Errorf("globsOverlap(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
			if got := globsOverlap(tc.b, tc.a); got != tc.want {
				t.Errorf("globsOverlap(%q, %q) = %v, want %v (symmetric)", tc.b, tc.a, got, tc.want)
			}
		})
	}
}

func TestBoardShowsIntents(t *testing.T) {
	lt := newTestTable(t, 15*time.Minute)

	intent := "refactor tui widgets"
	if _, _, err := lt.Acquire("agentA", "task-1", intent, []string{"internal/tui/*"}); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	board := lt.Board()
	if board == "" {
		t.Fatalf("Board(): expected non-empty output after acquire, got empty")
	}
	if !strings.Contains(board, intent) {
		t.Errorf("Board() must contain intent %q\n--got--\n%s", intent, board)
	}
	if !strings.Contains(board, "agentA") {
		t.Errorf("Board() must contain agentA\n--got--\n%s", board)
	}
	if !strings.Contains(board, "task-1") {
		t.Errorf("Board() must contain task-1\n--got--\n%s", board)
	}
}

func TestBoardEmptyWhenNoLeases(t *testing.T) {
	lt := newTestTable(t, 15*time.Minute)

	if got := lt.Board(); got != "" {
		t.Fatalf("Board() on empty table = %q, want empty string", got)
	}
}

func TestAcquireRejectsEmptyGlobs(t *testing.T) {
	lt := newTestTable(t, 15*time.Minute)

	l, conflicts, err := lt.Acquire("agentA", "task-1", "noop", []string{})
	if err == nil {
		t.Fatalf("Acquire with empty globs: expected error, got nil (lease=%+v conflicts=%v)", l, conflicts)
	}
	if l != nil {
		t.Fatalf("Acquire with empty globs: expected nil lease, got %+v", l)
	}
	if len(conflicts) != 0 {
		t.Fatalf("Acquire with empty globs: expected nil conflicts, got %d", len(conflicts))
	}
	if got, want := lt.Count(), 0; got != want {
		t.Fatalf("Acquire with empty globs must not store anything, Count = %d, want %d", got, want)
	}
}

func TestLeaseExpiry(t *testing.T) {
	// DefaultTTL set to 1ms so the lease is virtually born expired.
	lt := newTestTable(t, time.Millisecond)

	l, _, err := lt.Acquire("agentA", "task-1", "edit", []string{"internal/tui/*"})
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if l == nil {
		t.Fatalf("acquire: expected non-nil lease")
	}

	// Wait long enough for the lease to expire (Acquire now reads timeNow
	// lazily, so any sleep > DefaultTTL works).
	time.Sleep(20 * time.Millisecond)

	// A new agent must be able to acquire the same glob — the original
	// lease must have been GC'd by expireLocked on entry to Acquire.
	_, conflicts, err := lt.Acquire("agentB", "task-2", "steal", []string{"internal/tui/*"})
	if err != nil {
		t.Fatalf("re-acquire after expiry: %v", err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("re-acquire after expiry: expected no conflicts, got %d", len(conflicts))
	}
	if got, want := lt.Count(), 1; got != want {
		t.Fatalf("after expiry + re-acquire, Count = %d, want %d (old lease must be evicted, new one present)", got, want)
	}
}
