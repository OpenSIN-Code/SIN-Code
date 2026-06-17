// SPDX-License-Identifier: MIT
// Purpose: Tests and benchmarks for DistinctSessions() and the
// idx_ledger_session_id index (issues #338/#339).
package ledger

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// TestDistinctSessions_IndexExists verifies that the idx_ledger_session_id
// index is created in the schema.
func TestDistinctSessions_IndexExists(t *testing.T) {
	s := testStore(t)
	var name string
	err := s.db.QueryRow(`
		SELECT name FROM sqlite_master
		WHERE type = 'index' AND name = 'idx_ledger_session_id'
	`).Scan(&name)
	if err != nil {
		t.Fatalf("idx_ledger_session_id index not found: %v", err)
	}
	if name != "idx_ledger_session_id" {
		t.Fatalf("expected idx_ledger_session_id, got %q", name)
	}
}

// TestDistinctSessions_QueryWorks verifies that DistinctSessions returns
// the correct set of session IDs.
func TestDistinctSessions_QueryWorks(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for _, sid := range []string{"a", "b", "c"} {
		if _, err := s.Record(ctx, Entry{SessionID: sid, Type: TypeUserPrompt, Data: map[string]any{}}); err != nil {
			t.Fatal(err)
		}
	}
	sessions, err := s.DistinctSessions(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 3 {
		t.Fatalf("expected 3 distinct sessions, got %d: %v", len(sessions), sessions)
	}
}

// TestDistinctSessions_Ordering verifies that sessions are ordered by
// latest activity (MAX(created_at) DESC).
func TestDistinctSessions_Ordering(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	base := time.Now().UTC()
	for i, sid := range []string{"a", "b", "c"} {
		if _, err := s.Record(ctx, Entry{
			SessionID: sid, Type: TypeUserPrompt, Data: map[string]any{},
			CreatedAt: base.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Record(ctx, Entry{
		SessionID: "a", Type: TypeToolCall, Data: map[string]any{},
		CreatedAt: base.Add(10 * time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	sessions, err := s.DistinctSessions(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "c", "b"}
	if len(sessions) != len(want) {
		t.Fatalf("expected %d sessions, got %d: %v", len(want), len(sessions), sessions)
	}
	for i, w := range want {
		if sessions[i] != w {
			t.Fatalf("expected sessions[%d]=%s, got %s (full %v)", i, w, sessions[i], sessions)
		}
	}
}

// TestDistinctSessions_MigrationIdempotent verifies that opening a database
// multiple times does not corrupt the index or produce errors.
func TestDistinctSessions_MigrationIdempotent(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "ledger.db")

	s1, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := s1.Record(ctx, Entry{
			SessionID: fmt.Sprintf("sess-%d", i), Type: TypeUserPrompt, Data: map[string]any{},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := s2.DistinctSessions(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 5 {
		t.Fatalf("expected 5 sessions after re-open, got %d", len(sessions))
	}

	if err := s2.Close(); err != nil {
		t.Fatal(err)
	}
	s3, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s3.Close()
	sessions, err = s3.DistinctSessions(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 5 {
		t.Fatalf("expected 5 sessions after third open, got %d", len(sessions))
	}
}

// TestDistinctSessions_Limit verifies that the limit parameter is respected.
func TestDistinctSessions_Limit(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		if _, err := s.Record(ctx, Entry{
			SessionID: fmt.Sprintf("s-%d", i), Type: TypeUserPrompt, Data: map[string]any{},
		}); err != nil {
			t.Fatal(err)
		}
	}
	sessions, err := s.DistinctSessions(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions with limit, got %d", len(sessions))
	}
}

// TestDistinctSessions_RaceSafety verifies concurrent Record + DistinctSessions
// calls do not corrupt data (mandate M7).
func TestDistinctSessions_RaceSafety(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		i := i
		t.Run("record", func(t *testing.T) {
			t.Parallel()
			sid := fmt.Sprintf("race-%d", i%5)
			if _, err := s.Record(ctx, Entry{SessionID: sid, Type: TypeToolCall, Data: map[string]any{"i": i}}); err != nil {
				t.Fatal(err)
			}
		})
	}
	for i := 0; i < 10; i++ {
		t.Run("distinct", func(t *testing.T) {
			t.Parallel()
			if _, err := s.DistinctSessions(ctx, 100); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// --- Benchmarks ---

// seedForBench inserts nEntries across nSessions into the store.
func seedForBench(b *testing.B, s *Store, nSessions, nEntries int) {
	b.Helper()
	ctx := context.Background()
	base := time.Now().UTC()
	for i := 0; i < nSessions; i++ {
		sid := fmt.Sprintf("bench-session-%04d", i)
		for j := 0; j < nEntries; j++ {
			if _, err := s.Record(ctx, Entry{
				SessionID: sid,
				Type:      TypeToolCall,
				Data:      map[string]any{},
				CreatedAt: base.Add(time.Duration(i*nEntries+j) * time.Millisecond),
			}); err != nil {
				b.Fatal(err)
			}
		}
	}
}

// BenchmarkDistinctSessions_WithIndex measures DistinctSessions() performance
// with the idx_ledger_session_id index present (the default schema).
func BenchmarkDistinctSessions_WithIndex(b *testing.B) {
	s, err := Open(filepath.Join(b.TempDir(), "bench.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	seedForBench(b, s, 50, 200) // 10k entries across 50 sessions

	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := s.DistinctSessions(ctx, 1000); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDistinctSessions_NoIndex measures DistinctSessions() performance
// after dropping the session_id indexes, simulating the pre-#338 full-scan
// scenario. This proves the index provides a measurable speedup.
func BenchmarkDistinctSessions_NoIndex(b *testing.B) {
	s, err := Open(filepath.Join(b.TempDir(), "bench_no_idx.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	seedForBench(b, s, 50, 200)

	// Drop the indexes to simulate the pre-optimization scenario.
	if _, err := s.db.Exec(`DROP INDEX IF EXISTS idx_ledger_session_id`); err != nil {
		b.Fatal(err)
	}
	if _, err := s.db.Exec(`DROP INDEX IF EXISTS idx_ledger_session`); err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := s.DistinctSessions(ctx, 1000); err != nil {
			b.Fatal(err)
		}
	}
}
