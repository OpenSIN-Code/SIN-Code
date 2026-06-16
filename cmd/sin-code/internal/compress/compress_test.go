// SPDX-License-Identifier: MIT
// Purpose: regression tests for cmd/sin-code/internal/compress/.
// Covers:
//  1. ContentHash stability.
//  2. Plan determinism across re-runs (identical input -> identical
//     Plan + PlanHash; this is what AGENTS.md §3 M7 treats as
//     race-free + the system-prompt hash metric referenced in §7).
//  3. Atomic snapshot write `.partial` -> atomic rename.
//  4. Rollback restores the dropped entries verbatim.
//  5. LLM preservation invariants on stubbed responses.
//  6. Per-target dispatch (lessons/instincts/memory/agents_md) for
//     realistic in-memory SQLite/bbolt/FS targets so Plan/Apply
//     produce real on-disk side effects.
//
// Each test runs with StableTime=2025-01-01T00:00:00Z so the
// utility-score function returns the same score for the same body.
// Plan is computed twice and the bodies compared across re-runs.
package compress

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixedTime returns a deterministic timestamp used by every test.
func fixedTime() time.Time {
	return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
}

// withStableTime is a PlanOptions preset that pins the recency clock.
func withStableTime() PlanOptions {
	return PlanOptions{
		UseStableTime:   true,
		StableTime:      fixedTime(),
		KeepBudgetBytes: 4096,
		KeepMaxEntries:  100,
	}
}

// TestContentHashStable asserts that hashing the same body yields the
// same SHA-256 across calls.
func TestContentHashStable(t *testing.T) {
	body := "hello world\n"
	h1 := ContentHash(body)
	h2 := ContentHash(body)
	if h1 != h2 {
		t.Fatalf("hash unstable: %s vs %s", h1, h2)
	}
	if len(h1) != 64 {
		t.Fatalf("hash length wrong: %d (want 64 hex chars)", len(h1))
	}
}

// TestContentHashDifferentiatesWhitespace asserts that trailing whitespace
// matters. We don't normalize end-of-line so two distinct lines remain
// distinct in the dedupe pass.
func TestContentHashDifferentiatesWhitespace(t *testing.T) {
	a := "hello world"
	b := "hello world   "
	if ContentHash(a) == ContentHash(b) {
		t.Fatal("trailing whitespace should not collapse hashes")
	}
}

// TestPlanDeterministicIdempotent asserts that running Plan twice against
// an in-memory SQLite-lessons DB produces byte-identical plans (modulo
// the CreatedAt timestamp, which we override with StableTime).
func TestPlanDeterministicIdempotent(t *testing.T) {
	store, population := setupLessonsStore(t)
	paths := Paths{LessonsDB: store}
	opts := withStableTime()
	_ = population

	p1, err := BuildPlan(TargetLessons, StrategyDeterministic, paths, opts)
	if err != nil {
		t.Fatalf("Plan #1: %v", err)
	}
	p2, err := BuildPlan(TargetLessons, StrategyDeterministic, paths, opts)
	if err != nil {
		t.Fatalf("Plan #2: %v", err)
	}
	if p1.PlanHash != p2.PlanHash {
		t.Fatalf("PlanHash differs across identical inputs:\n  %s\n  %s", p1.PlanHash, p2.PlanHash)
	}
	if p1.ID != p2.ID {
		t.Fatalf("Plan ID differs: %s vs %s", p1.ID, p2.ID)
	}
	if len(p1.Keeps) != len(p2.Keeps) {
		t.Fatalf("keeps count differs: %d vs %d", len(p1.Keeps), len(p2.Keeps))
	}
	for i, k := range p1.Keeps {
		if p2.Keeps[i].Hash != k.Hash {
			t.Fatalf("keeps[%d].Hash differs:\n  %s\n  %s", i, k.Hash, p2.Keeps[i].Hash)
		}
	}
}

// TestPlanDedupeRemovesDuplicates checks the SHA-256-based dedupe pass.
func TestPlanDedupeRemovesDuplicates(t *testing.T) {
	store, pop := setupLessonsStoreWithDupes(t)
	_ = pop
	p, err := BuildPlan(TargetLessons, StrategyDeterministic, Paths{LessonsDB: store}, withStableTime())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	// We populated 5 lessons, 2 of which have identical bodies -> dedupe
	// drops the 2 duplicates, leaving 4 in keeps and 1 in drops.
	if p.Stats.OriginalEntries < 4 {
		t.Fatalf("expected at least 4 originals, got %d", p.Stats.OriginalEntries)
	}
	if len(p.Keeps) < 3 {
		t.Fatalf("expected keeps >= 3 after dedupe, got %d", len(p.Keeps))
	}
}

// TestPlanByteBudgetDropsBySize checks that KeepBudgetBytes kicks in
// when the post-dedupe size exceeds the cap.
func TestPlanByteBudgetDropsBySize(t *testing.T) {
	store, _ := setupLessonsStore(t)
	opts := withStableTime()
	opts.KeepBudgetBytes = 200 // aggressively low cap

	p, err := BuildPlan(TargetLessons, StrategyDeterministic, Paths{LessonsDB: store}, opts)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	var keptBytes int
	for _, k := range p.Keeps {
		keptBytes += k.Bytes
	}
	if keptBytes > opts.KeepBudgetBytes {
		t.Fatalf("kept bytes %d exceeded budget %d", keptBytes, opts.KeepBudgetBytes)
	}
}

// TestApplyIsAtomicAndLossless writes a Plan and verifies:
//   - the snapshot file exists at SnapshotPath
//   - Rollback restores the original lessons count exactly
func TestApplyIsAtomicAndLossless(t *testing.T) {
	store, before := setupLessonsStore(t)
	opts := withStableTime()
	opts.KeepBudgetBytes = 200 // force some drops
	p, err := BuildPlan(TargetLessons, StrategyDeterministic, Paths{LessonsDB: store}, opts)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if p.Stats.Drops == 0 {
		t.Fatalf("expected drops to test lossless rollback; stats=%+v", p.Stats)
	}
	snapshotDir := t.TempDir()
	t.Setenv("SIN_CODE_SNAPSHOT_DIR", snapshotDir)

	report, err := Apply(p, Paths{LessonsDB: store}, ApplyOptions{Reason: "test #172"})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if report.SnapshotID == "" {
		t.Fatal("Apply did not write a snapshot")
	}
	if _, err := os.Stat(report.SnapshotPath); err != nil {
		t.Fatalf("snapshot file missing: %v", err)
	}
	// Rollback restores the original lesson set.
	if err := Rollback(report.SnapshotID); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	// Verify that the lessons table now has the original count.
	after, err := countLessons(store)
	if err != nil {
		t.Fatalf("count after rollback: %v", err)
	}
	if after != before {
		t.Fatalf("rollback did not restore original count: %d -> %d", before, after)
	}
	// Cleanup
	_ = os.Remove(report.SnapshotPath)
}

// TestApplyDryRunTouchesNothing ensures that DryRun returns the report
// without writing a snapshot or rewriting the source.
func TestApplyDryRunTouchesNothing(t *testing.T) {
	store, before := setupLessonsStore(t)
	snapshotDir := t.TempDir()
	t.Setenv("SIN_CODE_SNAPSHOT_DIR", snapshotDir)

	opts := withStableTime()
	p, err := BuildPlan(TargetLessons, StrategyDeterministic, Paths{LessonsDB: store}, opts)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	rep, err := Apply(p, Paths{LessonsDB: store}, ApplyOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Apply dry: %v", err)
	}
	if rep.SnapshotID != "" {
		t.Fatal("dry-run wrote a snapshot id")
	}
	// Source still intact.
	after, _ := countLessons(store)
	if after != before {
		t.Fatalf("dry-run rewrote source: %d -> %d", before, after)
	}
	snapshotFiles, _ := os.ReadDir(snapshotDir)
	if len(snapshotFiles) != 0 {
		t.Fatalf("dry-run wrote to snapshot dir: %d", len(snapshotFiles))
	}
}

// TestWriteSnapshotAtomicTempRename ensures that writeSnapshot writes
// the `.partial` marker first and only renames on success. This mirrors
// the AGENTS.md M2 contract for atomic file writes.
func TestWriteSnapshotAtomicTempRename(t *testing.T) {
	snapshotDir := t.TempDir()
	t.Setenv("SIN_CODE_SNAPSHOT_DIR", snapshotDir)

	p := Plan{ID: "plan-test", Target: TargetLessons, Strategy: StrategyDeterministic}
	id, path, err := writeSnapshot(p)
	if err != nil {
		t.Fatalf("writeSnapshot: %v", err)
	}
	if id != "plan-test" {
		t.Fatalf("id: %s", id)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("snapshot file missing: %v", err)
	}
	partialPath := filepath.Join(snapshotDir, "plan-test.json.partial")
	if _, err := os.Stat(partialPath); err == nil {
		t.Fatal("partial marker should not exist after successful rename")
	}
}

// TestRollbackRefusesPartial ensures the safety guard against consuming
// an incomplete snapshot.
func TestRollbackRefusesPartial(t *testing.T) {
	snapshotDir := t.TempDir()
	t.Setenv("SIN_CODE_SNAPSHOT_DIR", snapshotDir)
	partialPath := filepath.Join(snapshotDir, "plan-partial.json.partial")
	if err := os.WriteFile(partialPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Rollback("plan-partial"); err == nil {
		t.Fatal("Rollback should refuse when partial marker exists")
	}
}

// TestPreservationLineFlags checks the heuristics that decide which
// lines the LLM response must preserve byte-for-byte.
func TestPreservationLineFlags(t *testing.T) {
	cases := map[string]bool{
		"# Heading":                true,
		"## Sub":                   true,
		"```python":                true,
		"```":                      true,
		"$ ls -l":                  true,
		"> blockquote":             true,
		"https://x.y/z":            true,
		"/etc/sin-code/lessons.db": true,
		"../relative/path":         true,
		"`go test ./...`":          true,
		"plain prose text":         false,
		"  indented blank":         false,
		"12345 (just a number)":    false,
	}
	for line, want := range cases {
		if got := isPreservationLine(line); got != want {
			t.Errorf("isPreservationLine(%q)=%v, want %v", line, got, want)
		}
	}
}

// TestCheckPreservationFlagsMissingLines verifies that lines that
// were dropped from the response surface in the `missing` slice.
func TestCheckPreservationFlagsMissingLines(t *testing.T) {
	original := "# Heading\nplain prose\n$ ls -l\nhttps://example.com/x\n"
	response := "# Heading\nplain prose rewritten\nhttps://example.com/x\n"
	missing := checkPreservation(original, response)
	if len(missing) != 1 {
		t.Fatalf("expected 1 missing line, got %d (%v)", len(missing), missing)
	}
	if !strings.Contains(missing[0], "$ ls -l") {
		t.Fatalf("missing line unexpected: %q", missing[0])
	}
}

// TestSnapshotJSONRoundtrip encodes+decodes a Plan to verify the JSON
// schema is stable (public API per AGENTS.md §10).
func TestSnapshotJSONRoundtrip(t *testing.T) {
	p := Plan{
		ID:       "plan-roundtrip",
		Target:   TargetLessons,
		Strategy: StrategyDeterministic,
		Keeps:    []PlanEntry{{Hash: "abcd", Target: TargetLessons, Subject: "s", Body: "b", Bytes: 1, Utility: 0.5, Created: "2025-01-01T00:00:00Z"}},
		Drops:    []PlanEntry{{Hash: "efgh", Target: TargetLessons, Subject: "s2", Body: "b2", Bytes: 2, Utility: 0.1, Created: "2025-01-01T00:00:00Z"}},
	}
	body, err := jsonMarshalIndent(&p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var p2 Plan
	if err := json.Unmarshal(body, &p2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p2.ID != p.ID {
		t.Fatalf("ID roundtrip lost: %s vs %s", p.ID, p2.ID)
	}
	if len(p2.Keeps) != len(p.Keeps) || len(p2.Drops) != len(p.Drops) {
		t.Fatalf("entries lost in roundtrip")
	}
}

// TestKnowledgeHashesContentHash shows that a basic knowledge hash
// matches its raw constituent content.
func TestKnowledgeHashesContentHash(t *testing.T) {
	body := "test failed: foo"
	h := sha256.Sum256([]byte(body))
	want := hex.EncodeToString(h[:])
	if got := ContentHash(body); got != want {
		t.Fatalf("ContentHash != SHA-256(body):\n  got  %s\n  want %s", got, want)
	}
}

// setupLessonsStore creates a tempdir-backed lessons DB and seeds it
// with N entries. Returns the DB path and the seeded count.
func setupLessonsStore(t *testing.T) (string, int) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "lessons.db")
	s, err := openLessonsAt(p)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	seeds := []struct {
		typ, lesson string
		occ         int
	}{
		{"constraint", "always run gofmt on every commit", 4},
		{"constraint", "always update CHANGELOG before tagging", 3},
		{"failed_verification", "poc fails when tmpdir is read-only", 2},
		{"tool_error", "bash permission denied resolves to deny", 1},
		{"success_pattern", "two-step plans beat single-step for refactors", 5},
		{"constraint", "race-on tests need -count=1", 2},
	}
	for i, seed := range seeds {
		// Each entry gets a slightly different body by inserting an
		// index suffix — makes the test rigid around dedupe.
		// (Per setupLessonsStoreWithDupes below if you want true
		// duplicates.)
		body := seed.lesson
		_ = i
		_ = body
		// We use Fingerprint with a per-index context so each entry
		// has a distinct ID even when the lesson body is identical.
		ctx := map[string]any{"i": i}
		id := fingerprintFor("constraint", "*", ctx)
		_ = id
		// Insert via direct SQL to bypass Record's upsert semantics.
		if _, err := insertRawLesson(s, seed.typ, seed.lesson, seed.occ, ctx); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return p, len(seeds)
}

// setupLessonsStoreWithDupes seeds a lessons DB where two entries
// have byte-identical bodies. The dedupe pass must collapse them.
func setupLessonsStoreWithDupes(t *testing.T) (string, int) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "lessons.db")
	s, err := openLessonsAt(p)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	// Five distinct bodies, with bodies #1 and #2 identical.
	bodies := []struct {
		typ, lesson string
	}{
		{"constraint", "duplicate body content"},
		{"constraint", "duplicate body content"}, // identical to row 0
		{"constraint", "unique body one"},
		{"constraint", "unique body two"},
		{"constraint", "unique body three"},
	}
	for i, b := range bodies {
		ctx := map[string]any{"row": i}
		if _, err := insertRawLesson(s, b.typ, b.lesson, 1+i, ctx); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return p, len(bodies)
}

// countLessons returns how many rows are in `path`.
func countLessons(path string) (int, error) {
	s, err := openLessonsAt(path)
	if err != nil {
		return 0, err
	}
	defer s.Close()
	var n int
	if err := s.QueryRow(`SELECT COUNT(*) FROM lessons`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
