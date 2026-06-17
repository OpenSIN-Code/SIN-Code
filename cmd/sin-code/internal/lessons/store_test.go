// SPDX-License-Identifier: MIT
// Purpose: lesson-store regression tests.
package lessons

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "l.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRecordIncrementsOccurrences(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	e := Entry{
		Type: TypeFailedVerification, Workspace: "/tmp/ws",
		Context: map[string]any{"mode": "poc"},
		Lesson:  "test failed: foo",
	}
	for i := 0; i < 3; i++ {
		if err := s.Record(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := s.Query(ctx, "/tmp/ws", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Occurrences != 3 {
		t.Fatalf("expected 1 entry with 3 occurrences, got %+v", entries)
	}
}

func TestQueryFiltersByWorkspace(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_ = s.Record(ctx, Entry{Type: TypeConstraint, Workspace: "/a", Lesson: "a"})
	_ = s.Record(ctx, Entry{Type: TypeConstraint, Workspace: "/b", Lesson: "b"})
	entries, _ := s.Query(ctx, "/a", 10)
	if len(entries) != 1 || entries[0].Workspace != "/a" {
		t.Fatalf("expected only /a entry, got %+v", entries)
	}
}

func TestDeleteRemovesEntry(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_ = s.Record(ctx, Entry{Type: TypeToolError, Workspace: "/tmp", Lesson: "x"})
	entries, _ := s.Query(ctx, "/tmp", 10)
	if len(entries) != 1 {
		t.Fatal("entry not recorded")
	}
	if err := s.Delete(ctx, entries[0].ID); err != nil {
		t.Fatal(err)
	}
	entries, _ = s.Query(ctx, "/tmp", 10)
	if len(entries) != 0 {
		t.Fatal("entry not deleted")
	}
}

func TestPruneRemovesOldSingletons(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	old := time.Now().UTC().Add(-100 * 24 * time.Hour).Format(time.RFC3339)
	_, err := s.db.Exec(`INSERT INTO lessons (id, type, workspace, context, lesson, occurrences, first_seen, last_seen) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"old1", TypeConstraint, "/tmp", "{}", "old", 1, old, old)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Record(ctx, Entry{Type: TypeConstraint, Workspace: "/tmp", Lesson: "recent"})
	n, err := s.Prune(ctx, 90)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 pruned, got %d", n)
	}
	entries, _ := s.Query(ctx, "/tmp", 10)
	if len(entries) != 1 || entries[0].Lesson != "recent" {
		t.Fatalf("wrong entries remain: %+v", entries)
	}
}

func TestBriefingIncludesOnlyRepeated(t *testing.T) {
	entries := []Entry{
		{Type: TypeFailedVerification, Occurrences: 1, Lesson: "noise"},
		{Type: TypeToolError, Occurrences: 3, Lesson: "signal-1"},
		{Type: TypeConstraint, Occurrences: 5, Lesson: "signal-2"},
	}
	b := Briefing(entries, 10, 2048)
	if !contains(b, "signal-1") || !contains(b, "signal-2") {
		t.Fatalf("briefing must include repeated entries: %q", b)
	}
	if contains(b, "noise") {
		t.Fatalf("briefing must NOT include singletons: %q", b)
	}
}

func TestBriefingCapsLessons(t *testing.T) {
	entries := make([]Entry, 20)
	for i := range entries {
		entries[i] = Entry{Type: TypeConstraint, Occurrences: 2, Lesson: "less" + string(rune('A'+i))}
	}
	b := Briefing(entries, 5, 2048)
	if !contains(b, "lessA") || !contains(b, "lessE") {
		t.Fatalf("briefing must include first 5: %q", b)
	}
	if contains(b, "lessF") {
		t.Fatalf("briefing must cap at 5 lessons: %q", b)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestFingerprintIs64Hex(t *testing.T) {
	fp := Fingerprint(TypeConstraint, "/tmp", map[string]any{"k": "v"})
	if len(fp) != 64 {
		t.Fatalf("expected 64-hex fingerprint, got len=%d: %s", len(fp), fp)
	}
	for _, c := range fp {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("expected lowercase hex, got %c in %s", c, fp)
		}
	}
}

func TestFingerprintMigration(t *testing.T) {
	ctx := context.Background()
	db := filepath.Join(t.TempDir(), "l.db")
	oldID := "1234567890abcdef"
	oldTime := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)

	// Seed a DB with a 16-hex fingerprint directly.
	s1, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s1.db.Exec(`INSERT INTO lessons (id, type, workspace, context, lesson, occurrences, first_seen, last_seen) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		oldID, TypeConstraint, "/tmp", "{}", "old lesson", 5, oldTime, oldTime)
	if err != nil {
		t.Fatal(err)
	}
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}

	// Re-open triggers migration.
	s2, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	entries, err := s2.Query(ctx, "/tmp", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if len(entries[0].ID) != 64 {
		t.Fatalf("expected migrated 64-hex id, got %s", entries[0].ID)
	}
	if entries[0].Occurrences != 5 {
		t.Fatalf("expected occurrences 5, got %d", entries[0].Occurrences)
	}
	if entries[0].Lesson != "old lesson" {
		t.Fatalf("expected lesson preserved, got %s", entries[0].Lesson)
	}
}

func TestRecordCollisionCreatesVariant(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	base := Entry{Type: TypeConstraint, Workspace: "/tmp", Context: map[string]any{"k": "v"}, Lesson: "original"}
	if err := s.Record(ctx, base); err != nil {
		t.Fatal(err)
	}
	// Same fingerprint but different lesson -> collision path.
	colliding := base
	colliding.Lesson = "different"
	if err := s.Record(ctx, colliding); err != nil {
		t.Fatal(err)
	}
	entries, err := s.Query(ctx, "/tmp", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries after collision, got %d", len(entries))
	}
	if entries[0].Occurrences != 1 || entries[1].Occurrences != 1 {
		t.Fatalf("expected both entries to have 1 occurrence, got %+v", entries)
	}
}

func TestRecordSameFingerprintIncrementsOccurrences(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	e := Entry{Type: TypeConstraint, Workspace: "/tmp", Context: map[string]any{"k": "v"}, Lesson: "same"}
	for i := 0; i < 3; i++ {
		if err := s.Record(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := s.Query(ctx, "/tmp", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Occurrences != 3 {
		t.Fatalf("expected 1 entry with 3 occurrences, got %+v", entries)
	}
}
