// SPDX-License-Identifier: MIT
// Purpose: additional coverage tests for lessons package to reach 100% statement coverage.
package lessons

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestOpen_DefaultPath(t *testing.T) {
	s, err := Open("")
	if err != nil {
		t.Fatalf("Open with empty path: %v", err)
	}
	_ = s.Close()
}

func TestOpen_DefaultPathXDG(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	p := DefaultPath()
	if p != filepath.Join(dir, "sin-code", "lessons.db") {
		t.Fatalf("DefaultPath = %q, want %q", p, filepath.Join(dir, "sin-code", "lessons.db"))
	}
}

func TestOpen_PragmaError(t *testing.T) {
	testOpenPragmaErr = errTestOpen
	defer func() { testOpenPragmaErr = nil }()
	if _, err := Open(filepath.Join(t.TempDir(), "l.db")); err != errTestOpen {
		t.Fatalf("expected injected pragma error, got %v", err)
	}
}

func TestOpen_SchemaError(t *testing.T) {
	testOpenSchemaErr = errTestOpen
	defer func() { testOpenSchemaErr = nil }()
	if _, err := Open(filepath.Join(t.TempDir(), "l.db")); err != errTestOpen {
		t.Fatalf("expected injected schema error, got %v", err)
	}
}

func TestOpen_DBError(t *testing.T) {
	testOpenDBErr = errTestOpen
	defer func() { testOpenDBErr = nil }()
	if _, err := Open(filepath.Join(t.TempDir(), "l.db")); err != errTestOpen {
		t.Fatalf("expected injected db error, got %v", err)
	}
}

func TestRecord_ContextMarshalError(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	err := s.Record(ctx, Entry{Type: TypeToolError, Workspace: "/tmp", Context: map[string]any{"bad": make(chan int)}, Lesson: "x"})
	if err == nil {
		t.Fatal("expected context marshal error")
	}
}

func TestQuery_AfterClose(t *testing.T) {
	s := openTest(t)
	_ = s.Close()
	if _, err := s.Query(context.Background(), "/tmp", 10); err == nil {
		t.Fatal("expected query error after close")
	}
}

func TestDelete_AfterClose(t *testing.T) {
	s := openTest(t)
	_ = s.Close()
	if err := s.Delete(context.Background(), "id"); err == nil {
		t.Fatal("expected delete error after close")
	}
}

func TestPrune_AfterClose(t *testing.T) {
	s := openTest(t)
	_ = s.Close()
	if _, err := s.Prune(context.Background(), 30); err == nil {
		t.Fatal("expected prune error after close")
	}
}

func TestClose_NilDB(t *testing.T) {
	s := &Store{}
	if err := s.Close(); err != nil {
		t.Fatalf("Close with nil db should return nil, got %v", err)
	}
}

func TestFingerprint_MarshalError(t *testing.T) {
	ctx := map[string]any{"bad": make(chan int)}
	id := Fingerprint(TypeConstraint, "/tmp", ctx)
	if id == "" {
		t.Fatal("expected non-empty id even on marshal error")
	}
}

func TestBriefing_DefaultLimits(t *testing.T) {
	entries := []Entry{{Type: TypeConstraint, Occurrences: 2, Lesson: "x"}}
	b := Briefing(entries, 0, 0)
	if !containsSub(b, "WORKSPACE KNOWLEDGE") || !containsSub(b, "x") {
		t.Fatalf("expected default-limits briefing, got %q", b)
	}
}

func TestBriefing_ByteCap(t *testing.T) {
	entries := []Entry{{Type: TypeConstraint, Occurrences: 2, Lesson: "this is a very long lesson that exceeds the cap"}}
	b := Briefing(entries, 10, 50)
	if !containsSub(b, "WORKSPACE KNOWLEDGE") {
		t.Fatal("expected header")
	}
	// The lesson is too long to fit, so the body should be just the header.
	if containsSub(b, "very long lesson") {
		t.Fatal("expected byte cap to trim the lesson")
	}
}

func TestBriefing_LessonCap(t *testing.T) {
	entries := make([]Entry, 5)
	for i := range entries {
		entries[i] = Entry{Type: TypeConstraint, Occurrences: 2, Lesson: "lesson" + string(rune('A'+i))}
	}
	b := Briefing(entries, 2, 2048)
	if !containsSub(b, "lessonA") || !containsSub(b, "lessonB") {
		t.Fatalf("expected first two lessons, got %q", b)
	}
	if containsSub(b, "lessonC") {
		t.Fatal("expected lesson cap at 2")
	}
}

func TestPrune_NoMatches(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_ = s.Record(ctx, Entry{Type: TypeConstraint, Workspace: "/tmp", Lesson: "recent"})
	n, err := s.Prune(ctx, 90)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected 0 pruned, got %d", n)
	}
}

func TestQuery_ScanUnmarshalError(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	old := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	_, err := s.db.Exec(`INSERT INTO lessons (id, type, workspace, context, lesson, occurrences, first_seen, last_seen) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"bad", TypeConstraint, "/tmp", "not valid json", "x", 1, old, old)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Query(ctx, "/tmp", 10); err == nil {
		t.Fatal("expected unmarshal error during scan")
	}
}

func TestQuery_DefaultLimit(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_ = s.Record(ctx, Entry{Type: TypeConstraint, Workspace: "/tmp", Lesson: "x"})
	entries, err := s.Query(ctx, "/tmp", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected default limit to return 1 entry, got %d", len(entries))
	}
}

func TestQuery_ScanError(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	old := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	// Insert an invalid value for occurrences (a string) so Scan into int fails.
	_, err := s.db.Exec(`INSERT INTO lessons (id, type, workspace, context, lesson, occurrences, first_seen, last_seen) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"bad", TypeConstraint, "/tmp", "{}", "x", "not-an-int", old, old)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Query(ctx, "/tmp", 10); err == nil {
		t.Fatal("expected scan error")
	}
}

func TestRecord_ExecError(t *testing.T) {
	s := openTest(t)
	_ = s.Close()
	if err := s.Record(context.Background(), Entry{Type: TypeConstraint, Workspace: "/tmp", Lesson: "x"}); err == nil {
		t.Fatal("expected record error after close")
	}
}

func containsSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
