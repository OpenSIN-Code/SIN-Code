// SPDX-License-Identifier: MIT
// Purpose: Tests for the semantic session ledger.
// Docs: ledger.doc.md
package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	db := filepath.Join(t.TempDir(), "ledger.db")
	s, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestOpenCreatesDefaultPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Record(context.Background(), Entry{SessionID: "s1", Type: TypeUserPrompt, Data: map[string]any{"content": "hi"}}); err != nil {
		t.Fatal(err)
	}
}

func TestRecordAndList(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	sid := "session-1"

	id, err := s.Record(ctx, Entry{SessionID: sid, Type: TypeUserPrompt, Data: map[string]any{"content": "hello"}, Summary: "user greeting"})
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}

	if _, err := s.Record(ctx, Entry{SessionID: sid, Type: TypeToolCall, Data: map[string]any{"tool": "sin_read"}, Summary: "read a file"}); err != nil {
		t.Fatal(err)
	}

	entries, err := s.List(ctx, sid, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Type != TypeToolCall {
		t.Fatalf("expected newest first, got %s", entries[0].Type)
	}
	if entries[0].Data["tool"] != "sin_read" {
		t.Fatalf("tool data mismatch: %v", entries[0].Data)
	}
}

func TestQueryByType(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	sid := "s2"
	for _, typ := range []EntryType{TypeUserPrompt, TypeToolCall, TypeToolCall, TypeVerifyPass} {
		if _, err := s.Record(ctx, Entry{SessionID: sid, Type: typ, Data: map[string]any{"x": 1}}); err != nil {
			t.Fatal(err)
		}
	}
	res, err := s.QueryByType(ctx, sid, TypeToolCall, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(res))
	}
}

func TestSessions(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for _, sid := range []string{"a", "b", "c"} {
		if _, err := s.Record(ctx, Entry{SessionID: sid, Type: TypeUserPrompt, Data: map[string]any{}}); err != nil {
			t.Fatal(err)
		}
	}
	sessions, err := s.Sessions(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(sessions))
	}
}

func TestDataRoundtrip(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	id, err := s.Record(ctx, Entry{
		SessionID: "s3",
		Type:      TypeToolCall,
		Data:      map[string]any{"tool": "edit", "args": map[string]any{"path": "foo.go"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := s.List(ctx, "s3", 1)
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].ID != id {
		t.Fatal("id mismatch")
	}
	if args := entries[0].Data["args"].(map[string]any); args["path"] != "foo.go" {
		t.Fatalf("nested data mismatch: %v", args)
	}
}

func TestSQLiteDriverRegistered(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sqlite driver not available: %v", err)
	}
	_ = db.Close()
}

func TestRace(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		i := i
		t.Run("parallel", func(t *testing.T) {
			t.Parallel()
			if _, err := s.Record(ctx, Entry{SessionID: "race", Type: TypeToolCall, Data: map[string]any{"i": i}}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSessionsOrdering(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	base := time.Now().UTC()
	// Record in chronological order: a first, then b, then c.
	for i, sid := range []string{"a", "b", "c"} {
		if _, err := s.Record(ctx, Entry{SessionID: sid, Type: TypeUserPrompt, Data: map[string]any{}, CreatedAt: base.Add(time.Duration(i) * time.Second)}); err != nil {
			t.Fatal(err)
		}
	}
	// Add a newer entry to session a so it should be first.
	if _, err := s.Record(ctx, Entry{SessionID: "a", Type: TypeToolCall, Data: map[string]any{}, CreatedAt: base.Add(10 * time.Second)}); err != nil {
		t.Fatal(err)
	}
	sessions, err := s.Sessions(ctx, 10)
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

func TestSessionsBackfill(t *testing.T) {
	ctx := context.Background()
	db := filepath.Join(t.TempDir(), "ledger.db")
	// First open: create schema and insert rows without session_index support (simulated by direct insert).
	s1, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s1.Record(ctx, Entry{SessionID: "old", Type: TypeUserPrompt, Data: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}
	// Second open triggers migration/backfill.
	s2, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	sessions, err := s2.Sessions(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0] != "old" {
		t.Fatalf("expected [old], got %v", sessions)
	}
}

func TestSessionsRace(t *testing.T) {
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
		t.Run("sessions", func(t *testing.T) {
			t.Parallel()
			if _, err := s.Sessions(ctx, 10); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSessionsPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skip performance test in short mode")
	}
	s := testStore(t)
	ctx := context.Background()
	base := time.Now().UTC()
	const sessions = 20
	const entriesPerSession = 50
	for i := 0; i < sessions; i++ {
		sid := fmt.Sprintf("session-%04d", i)
		for j := 0; j < entriesPerSession; j++ {
			if _, err := s.Record(ctx, Entry{
				SessionID: sid,
				Type:      TypeToolCall,
				Data:      map[string]any{},
				CreatedAt: base.Add(time.Duration(i*entriesPerSession+j) * time.Millisecond),
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	start := time.Now()
	got, err := s.Sessions(ctx, 1000)
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if len(got) != sessions {
		t.Fatalf("expected %d sessions, got %d", sessions, len(got))
	}
	if elapsed > 50*time.Millisecond {
		t.Fatalf("Sessions took %s, expected <50ms", elapsed)
	}
}
