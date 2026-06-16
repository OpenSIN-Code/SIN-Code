// SPDX-License-Identifier: MIT
// Purpose: race-clean tests for the byte-stable auto-memory markdown
// package (Claude Code MEMORY.md equivalent, sin-code variant: every
// write is hook-triggered, never silent — mandate M3).
package auto_mem

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	home := t.TempDir()
	// Stable key — the OS will resolve to the same hash for the same input.
	key := "test-project-a"
	s, err := Open(home, key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestOpenCreatesDir(t *testing.T) {
	s := openTemp(t)
	if !strings.HasSuffix(s.Path(), "MEMORY.md") {
		t.Fatalf("Path() must end in MEMORY.md: %q", s.Path())
	}
	if _, err := s.Index(); err != nil {
		t.Fatal(err)
	}
}

func TestAppendAndIndex(t *testing.T) {
	s := openTemp(t)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.Append(Entry{
		Heading:   "build-commands",
		Body:      "make build\nmake test",
		SourceTag: "tool-error",
		AddedAt:   now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(Entry{
		Heading:   "debug-tricks",
		Body:      "use `git log --all` to find lost work",
		SourceTag: "verify-fail",
		AddedAt:   now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	idx, err := s.Index()
	if err != nil {
		t.Fatal(err)
	}
	if len(idx) != 2 {
		t.Fatalf("want 2 headings, got %d: %v", len(idx), idx)
	}
	// Sorted lexicographically: "build-commands" < "debug-tricks"
	if idx[0] != "build-commands" || idx[1] != "debug-tricks" {
		t.Fatalf("order: want [build-commands, debug-tricks], got %v", idx)
	}
	body, err := s.ReadTopic("build-commands")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "make build") {
		t.Fatalf("body must contain 'make build': %q", body)
	}
}

func TestAppendNormalisesHeading(t *testing.T) {
	s := openTemp(t)
	now := time.Now().UTC()
	variants := []string{"Build-Commands", "build_commands", "BUILD COMMANDS", "build-commands"}
	for _, h := range variants {
		if err := s.Append(Entry{
			Heading: h,
			Body:    "x",
			AddedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	idx, _ := s.Index()
	if len(idx) != 1 {
		t.Fatalf("normalisation must dedupe; got %v", idx)
	}
	if idx[0] != "build-commands" {
		t.Fatalf("normalised form: want build-commands, got %q", idx[0])
	}
}

func TestAppendReplaces(t *testing.T) {
	s := openTemp(t)
	if err := s.Append(Entry{Heading: "h", Body: "first", AddedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(Entry{Heading: "h", Body: "second", AddedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	idx, _ := s.Index()
	if len(idx) != 1 {
		t.Fatalf("want 1 entry, got %d", len(idx))
	}
	body, _ := s.ReadTopic("h")
	if strings.TrimSpace(string(body)) != "second" {
		t.Fatalf("Append must replace; got %q", body)
	}
}

func TestIndexBytesByteStable(t *testing.T) {
	// Two independent writes with the same logical data must produce
	// byte-identical IndexBytes. This is the user-visible hash for the
	// system-prompt component.
	s1 := openTemp(t)
	s2 := openTemp(t)
	now := time.Date(2026, 6, 17, 1, 0, 0, 0, time.UTC)
	for _, s := range []*Store{s1, s2} {
		if err := s.Append(Entry{Heading: "k1", Body: "b1", AddedAt: now}); err != nil {
			t.Fatal(err)
		}
		if err := s.Append(Entry{Heading: "k2", Body: "b2", AddedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	b1, err := s1.IndexBytes()
	if err != nil {
		t.Fatal(err)
	}
	b2, err := s2.IndexBytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(b1) != string(b2) {
		t.Fatalf("IndexBytes must be byte-stable:\n%s\n!=\n%s", b1, b2)
	}
}

func TestIndexBytesCap(t *testing.T) {
	s := openTemp(t)
	// Build entries whose combined body exceeds the 25 KB cap.
	huge := strings.Repeat("x", 2000)
	for i := 0; i < 30; i++ {
		h := string(rune('a'+(i%26))) + string(rune('A'+(i/26))) + "-" + "topic"
		if err := s.Append(Entry{
			Heading: h,
			Body:    huge,
			AddedAt: time.Now(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	out, err := s.IndexBytes()
	if err != nil {
		t.Fatal(err)
	}
	if len(out) > IndexBytesCap {
		t.Fatalf("IndexBytes over cap: %d > %d", len(out), IndexBytesCap)
	}
}

func TestReadTopicMissing(t *testing.T) {
	s := openTemp(t)
	_, err := s.ReadTopic("nonexistent")
	if err == nil {
		t.Fatal("want error for missing topic")
	}
	if !errors.Is(err, ErrNoSuchTopic) {
		t.Fatalf("want ErrNoSuchTopic, got %T", err)
	}
}

func TestRemove(t *testing.T) {
	s := openTemp(t)
	_ = s.Append(Entry{Heading: "k1", Body: "b1", AddedAt: time.Now()})
	_ = s.Append(Entry{Heading: "k2", Body: "b2", AddedAt: time.Now()})
	if err := s.Remove("k1"); err != nil {
		t.Fatal(err)
	}
	idx, _ := s.Index()
	if len(idx) != 1 || idx[0] != "k2" {
		t.Fatalf("Remove: want [k2], got %v", idx)
	}
	if err := s.Remove("k1"); !errors.Is(err, ErrNoSuchTopic) {
		t.Fatalf("Remove(k1) twice: want ErrNoSuchTopic, got %v", err)
	}
}

func TestRotate(t *testing.T) {
	s := openTemp(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		_ = s.Append(Entry{
			Heading: string(rune('a' + i)),
			Body:    "x",
			AddedAt: base.Add(time.Duration(i) * time.Hour),
		})
	}
	kept, err := s.Rotate(4)
	if err != nil {
		t.Fatal(err)
	}
	if kept != 4 {
		t.Fatalf("Rotate(4) must retain 4; got %d", kept)
	}
	idx, _ := s.Index()
	if len(idx) != 4 {
		t.Fatalf("want 4 entries, got %d", len(idx))
	}
	// Insertion order: a, b, c, ..., j (a oldest, j newest).
	// After Rotate(4): keep the 4 most-recent (g, h, i, j), then
	// re-sort alphabetically for byte-stable on-disk output.
	want := []string{"g", "h", "i", "j"}
	for i := range want {
		if idx[i] != want[i] {
			t.Fatalf("after Rotate(4), want %v, got %v", want, idx)
		}
	}
}

func TestEmptyTopic(t *testing.T) {
	s := openTemp(t)
	if err := s.Append(Entry{Heading: "", Body: "x"}); err == nil {
		t.Fatal("empty heading must error")
	}
}

func TestStoreDirLayout(t *testing.T) {
	home := filepath.Join(t.TempDir(), "sin-home")
	s, err := Open(home, "the-key")
	if err != nil {
		t.Fatal(err)
	}
	// project-hash must be deterministic sha256-prefix12 of "the-key"
	wantHash := projHash("the-key")
	wantDir := filepath.Join(home, "memory", wantHash, "memory", "MEMORY.md")
	if s.Path() != wantDir {
		t.Fatalf("dir layout: want %q, got %q", wantDir, s.Path())
	}
}
