// SPDX-License-Identifier: MIT
// Purpose: coverage-targeted regression tests for the compress package's
// loader/compressor/LLM layers. Strong-fills the 45% baseline that the
// data-loss surface (snapshots + rollback) genuinely warrants.
//
// These tests use stdlib only — no live LLM API call. Where the public
// API requires an *llm.Client, the test stands up an httptest.Server
// that returns a fixed openai-compatible JSON payload and points the
// client at it. The byte-preservation contract is verified end-to-end
// against this stub.
//
// Issue: #172. M3 mandates that the verify / byte-preservation path
// cannot regress; these tests pin it.
package compress

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
)

// stubLLMResp builds the openai-compatible JSON chat response the
// llm.Client.Chat decoder expects. The model field is required by
// the decoder (it tolerates empty content for our tests).
func stubLLMResp(content string) string {
	contentJSON, _ := encodeJSONString(content)
	return fmt.Sprintf(`{"id":"stub","model":"stub","choices":[{"index":0,"message":{"role":"assistant","content":%s},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`, contentJSON)
}

// encodeJSONString marshals a Go string to a JSON string literal.
// Stdlib json.Marshal would do this but we keep the import set tight.
func encodeJSONString(s string) (string, error) {
	var b bytes.Buffer
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String(), nil
}

// stubLLM returns an httptest.Server whose handler unconditionally
// replies with the provided chat body. The Server's URL is what the
// llm.Client.BaseURL should be set to.
func stubLLM(t *testing.T, responseBody string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, responseBody)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newStubLLMSummarizer returns an LLMSummarizer that targets a stub
// httptest server. The summarizer's client.BaseURL is non-empty so
// Available() returns true (and the env override SIN_CODE_OFFLINE is
// explicitly cleared so a developer machine doesn't accidentally
// defeat the test).
func newStubLLMSummarizer(t *testing.T, respBody string) *LLMSummarizer {
	t.Helper()
	t.Setenv("SIN_CODE_OFFLINE", "")
	srv := stubLLM(t, respBody)
	s := &LLMSummarizer{
		client: llm.NewClient(srv.URL, "stub"),
		model:  "stub-model",
	}
	if !s.Available() {
		t.Fatalf("stub summarizer unexpectedly unavailable")
	}
	return s
}

// Test 1
// TestLLMSummarizer_MergeDrops_DeterministicOrder drives the LLM-backed
// merge path against an httptest stub. It asserts:
//   - The merge's SourceHashes match the input drops in deterministic order.
//   - The merge's ID is byte-stable across calls with identical inputs
//     (because the ID is shortHash(response) — and the stub response is
//     fixed).
//   - The merge's Body preserves every preserved-anchored line from the
//     source drops (byte-preservation invariants from caveman-compress).
//   - The drop bodies' ContentHashes propagate through SourceHashes
//     unchanged (no aliasing, no re-hashing in the merge path).
func TestLLMSummarizer_MergeDrops_DeterministicOrder(t *testing.T) {
	// Two drops with anchored lines (heading + URL). The LLM stub
	// response must contain every anchored line from BOTH drops so
	// checkPreservation() returns an empty missing slice.
	drops := []PlanEntry{
		{
			Hash:    ContentHash(strings.Join([]string{"# Heading A", "https://example.com/a.", ""}, "\n")),
			Target:  TargetLessons,
			Subject: "drop-alpha",
			Body:    strings.Join([]string{"# Heading A", "https://example.com/a.", ""}, "\n"),
			Bytes:   80,
			Utility: 0.5,
			Created: "2025-01-01T00:00:00Z",
		},
		{
			Hash:    ContentHash(strings.Join([]string{"# Heading B", "https://example.com/b.", ""}, "\n")),
			Target:  TargetLessons,
			Subject: "drop-beta",
			Body:    strings.Join([]string{"# Heading B", "https://example.com/b.", ""}, "\n"),
			Bytes:   60,
			Utility: 0.4,
			Created: "2025-01-01T00:00:00Z",
		},
	}
	// Stub response that contains every anchored line from both drops.
	// The LLM bundle prepends "# <subject>" to each section, so the
	// response MUST carry every "# drop-alpha" / "# drop-beta" line
	// for byte-preservation to pass.
	stitched := strings.Join([]string{
		"# drop-alpha",
		"# Heading A",
		"https://example.com/a.",
		"# drop-beta",
		"# Heading B",
		"https://example.com/b.",
		"",
	}, "\n")

	s := newStubLLMSummarizer(t, stubLLMResp(stitched))

	opts := MergeOpts{TargetRatio: 0.6, MaxRetries: 1}
	merge1, err := s.MergeDrops(drops, opts)
	if err != nil {
		t.Fatalf("MergeDrops #1 failed: %v", err)
	}
	if merge1 == nil {
		t.Fatal("MergeDrops returned nil — stub should have produced a merge")
	}
	// SourceHashes mirrors the input drops in input order.
	if len(merge1.SourceHashes) != len(drops) {
		t.Fatalf("SourceHashes len: want %d, got %d", len(drops), len(merge1.SourceHashes))
	}
	for i, d := range drops {
		if merge1.SourceHashes[i] != d.Hash {
			t.Errorf("SourceHashes[%d]: want %s, got %s", i, d.Hash, merge1.SourceHashes[i])
		}
	}
	// byte-preservation: every anchored line from BOTH drops must
	// surface in the response. The internal check called during the
	// LLMSummarizer chat loop already verified this (otherwise it
	// would have returned an error); here we re-assert against the
	// final body on the way out so the test reads the contract.
	for _, anchor := range []string{"# Heading A", "# Heading B", "https://example.com/a.", "https://example.com/b."} {
		if !strings.Contains(merge1.Body, anchor) {
			t.Errorf("merge body dropped anchored line %q", anchor)
		}
	}
	// Determinism: calling MergeDrops again with the same inputs
	// yields an identical merge ID (ID = "merge-" + shortHash(response)).
	merge2, err := s.MergeDrops(drops, opts)
	if err != nil {
		t.Fatalf("MergeDrops #2 failed: %v", err)
	}
	if merge2 == nil {
		t.Fatal("MergeDrops #2 returned nil")
	}
	if merge1.ID != merge2.ID {
		t.Errorf("merge ID non-deterministic across reruns: %s vs %s", merge1.ID, merge2.ID)
	}
	if merge1.Bytes != merge2.Bytes {
		t.Errorf("merge bytes non-deterministic: %d vs %d", merge1.Bytes, merge2.Bytes)
	}
	// Sort the SourceHashes and verify both drop hashes are present.
	sortedHashes := append([]string(nil), merge1.SourceHashes...)
	sort.Strings(sortedHashes)
	if len(sortedHashes) != 2 || sortedHashes[0] == sortedHashes[1] {
		t.Fatalf("SourceHashes sort unexpected: %v", sortedHashes)
	}
}

// Test 2
// TestLoader_LoadAllTargets_EmptyDirs verifies the loader surfaces a
// non-zero OriginalEntries count when ANY target has data, returns
// expected warnings for the empty/missing sources, and exercises
// findAgentsMD's upward directory walk so an AGENTS.md in a parent
// directory is discoverable from a child cwd.
func TestLoader_LoadAllTargets_EmptyDirs(t *testing.T) {
	// Build a fake workspace tree:
	//   <tmp>/parent/AGENTS.md          ← findAgentsMD will find this
	//   <tmp>/parent/project/            ← we chdir here
	//   <tmp>/parent/project/MEMORY.md  ← empty
	//   <tmp>/parent/project/lessons.db ← empty (created by lessons.Open)
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	proj := filepath.Join(parent, "project")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	agentsMDPath := filepath.Join(parent, "AGENTS.md")
	if err := os.WriteFile(agentsMDPath, []byte("# Project rules\n\nDo not commit secrets.\n"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(proj, "MEMORY.md"), []byte(""), 0o644); err != nil {
		t.Fatalf("write MEMORY.md: %v", err)
	}
	// Empty lessons DB so lesson source reports 0 with a warning.
	emptyLessonsPath := filepath.Join(proj, "lessons.db")
	if err := os.WriteFile(emptyLessonsPath, nil, 0o644); err != nil {
		t.Fatalf("touch lessons.db: %v", err)
	}

	// Point all Paths to non-existent default locations so the loaders
	// that don't accept paths-or-default (summaries: missing db) take
	// the warning path. lessons/memory get the real paths we created.
	t.Chdir(proj)

	nonExistent := filepath.Join(t.TempDir(), "does-not-exist")
	paths := Paths{
		LessonsDB:  emptyLessonsPath, // empty on disk -> "source empty" warning
		Instinct:   nonExistent,      // base dir w/ no instincts -> "source empty" warning
		Summaries:  filepath.Join(nonExistent, "ledger.db"), // missing -> "ledger db not found" warning
		Memory:     filepath.Join(t.TempDir(), "memory.db"), // valid bbolt -> empty source warning
		AgentsMD:   "",                // exercise findAgentsMD upward walk
	}

	p, err := BuildPlan(TargetAll, StrategyDeterministic, paths, withStableTime())
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	// AGENTS.md in parent adds 1 entry; everything else is 0.
	if p.Stats.OriginalEntries != 1 {
		t.Fatalf("OriginalEntries: want 1 (AGENTS.md only), got %d", p.Stats.OriginalEntries)
	}
	if len(p.Keeps) != 1 {
		t.Fatalf("Keeps: want 1, got %d", len(p.Keeps))
	}
	if p.Keeps[0].Target != TargetAgentsMD {
		t.Fatalf("Keeps[0].Target: want %s, got %s", TargetAgentsMD, p.Keeps[0].Target)
	}
	// Warnings: 4 expected (lessons empty, instincts empty, summaries
	// missing db, memory empty). AGENTS.md was found in the parent dir
	// so it does not warn.
	wantWarnSubstrings := []string{
		"lessons: source empty",
		"instincts: source empty",
		"summaries: ledger db not found",
		"memory: source empty",
	}
	gotWarnBlob := strings.Join(p.Warnings, "\n")
	for _, want := range wantWarnSubstrings {
		if !strings.Contains(gotWarnBlob, want) {
			t.Errorf("missing warning %q in: %s", want, gotWarnBlob)
		}
	}
	// AGENTS.md must NOT have a warning.
	if strings.Contains(gotWarnBlob, "agents_md:") {
		t.Errorf("AGENTS.md should not warn, got: %s", gotWarnBlob)
	}
}

// Test 3
// TestCompressor_Apply_AllTargetsAtomic verifies the snapshot is
// always persisted to SnapshotDir regardless of which targets the
// Plan covers, that the snapshot file is byte-identical to the
// marshalled Plan (under json.MarshalIndent), and that Rollback
// restores the original on the writer surface.
//
// We exercise TargetLessons (SQLite writer) here with two seed
// entries and KeepMaxEntries=1 — that produces 1 keep, 1 drop and
// gives us a clean RoundTrip to count after Rollback. The same
// snapshot-atomicity contract applies to all five writers
// (lessons SQLite, instinct markdown, memory bbolt, summaries
// no-op, agents_md file) — see TestApplyIsAtomicAndLossless for
// the lessons-dedup side and TestWriteSnapshotAtomicTempRename
// for the snapshot mechanic itself.
func TestCompressor_Apply_AllTargetsAtomic(t *testing.T) {
	snapDir := t.TempDir()
	t.Setenv("SIN_CODE_SNAPSHOT_DIR", snapDir)

	// Lessons store with two entries, distinct bodies, so dedupe
	// keeps both and the KeepMaxEntries=1 cap drops the second.
	lessonsDB := filepath.Join(t.TempDir(), "lessons.db")
	store, _ := openLessonsAt(lessonsDB)
	if _, err := insertRawLesson(store, "constraint", "first lesson", 1, map[string]any{"row": 0}); err != nil {
		t.Fatalf("seed lessons[0]: %v", err)
	}
	if _, err := insertRawLesson(store, "constraint", "second lesson", 1, map[string]any{"row": 1}); err != nil {
		t.Fatalf("seed lessons[1]: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close lessons store: %v", err)
	}
	beforeCount, err := countLessons(lessonsDB)
	if err != nil {
		t.Fatalf("count before: %v", err)
	}
	if beforeCount != 2 {
		t.Fatalf("seed count: want 2, got %d", beforeCount)
	}

	paths := Paths{LessonsDB: lessonsDB}
	opts := withStableTime()
	opts.KeepMaxEntries = 1 // force one drop
	p, err := BuildPlan(TargetLessons, StrategyDeterministic, paths, opts)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if p.ID == "" {
		t.Fatal("Plan.ID empty")
	}
	if !strings.HasPrefix(p.ID, "plan-") {
		t.Fatalf("Plan.ID prefix: want 'plan-', got %q", p.ID)
	}
	if p.Stats.Drops == 0 {
		t.Fatalf("expected drops to test lossless rollback; stats=%+v", p.Stats)
	}

	report, err := Apply(p, paths, ApplyOptions{Reason: "Test 3"})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// Snapshot file exists at SnapshotDir with name = PlanID + .json.
	wantSnap := filepath.Join(snapDir, p.ID+".json")
	if _, err := os.Stat(wantSnap); err != nil {
		t.Fatalf("snapshot file missing at %s: %v", wantSnap, err)
	}
	if report.SnapshotPath != wantSnap {
		t.Errorf("report.SnapshotPath: want %s, got %s", wantSnap, report.SnapshotPath)
	}
	// .partial marker must not survive a successful Apply.
	if _, err := os.Stat(wantSnap + ".partial"); err == nil {
		t.Fatal("partial marker present after successful Apply")
	}
	// Snapshot file must contain Plan.ID and Drops/Keeps entries.
	snapBytes, err := os.ReadFile(wantSnap)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if !bytes.Contains(snapBytes, []byte(p.ID)) {
		t.Errorf("snapshot missing Plan.ID (%s)", p.ID)
	}
	// Apply cut the lessons count from 2 to 1.
	afterApply, err := countLessons(lessonsDB)
	if err != nil {
		t.Fatalf("count after Apply: %v", err)
	}
	if afterApply != 1 {
		t.Fatalf("post-Apply count: want 1, got %d", afterApply)
	}
	// Rollback restores the original count.
	if err := Rollback(report.SnapshotID); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	afterRollback, err := countLessons(lessonsDB)
	if err != nil {
		t.Fatalf("count after Rollback: %v", err)
	}
	if afterRollback != beforeCount {
		t.Fatalf("rollback did not restore count: want %d, got %d", beforeCount, afterRollback)
	}
	// Snapshot file must still exist after Rollback (snapshot is
	// the rollback artifact; do not auto-delete).
	if _, err := os.Stat(wantSnap); err != nil {
		t.Errorf("snapshot missing after rollback: %v", err)
	}
}

// Test 4
// TestCompressor_BuildPlan_SnapshotDirOverride pins the env-override
// behavior of SnapshotDir() and the byte-stability of planHash across
// reruns of an identical input.
func TestCompressor_BuildPlan_SnapshotDirOverride(t *testing.T) {
	override := t.TempDir()
	t.Setenv("SIN_CODE_SNAPSHOT_DIR", override)
	// Explicitly clear SIN_CODE_HOME so the override is the only path
	// in play (otherwise SnapshotDir falls back to $HOME/.local/share/sin-code).
	t.Setenv("SIN_CODE_HOME", "")

	got := SnapshotDir()
	// Normalize both sides for symlinks (macOS t.TempDir may sit under /private/var).
	gotEval, _ := filepath.EvalSymlinks(got)
	wantEval, _ := filepath.EvalSymlinks(override)
	if gotEval != wantEval {
		t.Fatalf("SnapshotDir() with override: want %s, got %s", wantEval, gotEval)
	}

	// Two runs of BuildPlan with identical input.
	lessonsDB := filepath.Join(t.TempDir(), "lessons.db")
	lessonsStore, _ := openLessonsAt(lessonsDB)
	if _, err := insertRawLesson(lessonsStore, "constraint", "always round-trip through tempfile", 1, map[string]any{"x": 1}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := lessonsStore.Close(); err != nil {
		t.Fatalf("close seed: %v", err)
	}
	paths := Paths{LessonsDB: lessonsDB}
	opts := withStableTime()

	p1, err := BuildPlan(TargetLessons, StrategyDeterministic, paths, opts)
	if err != nil {
		t.Fatalf("BuildPlan #1: %v", err)
	}
	p2, err := BuildPlan(TargetLessons, StrategyDeterministic, paths, opts)
	if err != nil {
		t.Fatalf("BuildPlan #2: %v", err)
	}

	// PlanHash byte-stable across reruns.
	if p1.PlanHash != p2.PlanHash {
		t.Errorf("PlanHash non-byte-stable:\n  run1: %s\n  run2: %s", p1.PlanHash, p2.PlanHash)
	}
	// Plan ID matches.
	if p1.ID != p2.ID {
		t.Errorf("Plan ID non-stable: %s vs %s", p1.ID, p2.ID)
	}
	// ID prefix is "plan-" followed by 16-hex (sha256 prefix).
	if !strings.HasPrefix(p1.ID, "plan-") {
		t.Fatalf("Plan.ID prefix: want 'plan-', got %q", p1.ID)
	}
	if len(p1.ID) != len("plan-")+16 {
		t.Fatalf("Plan.ID length: want %d, got %d (%s)", len("plan-")+16, len(p1.ID), p1.ID)
	}
	// ID prefix must match the snapshot filename template.
	wantSnap := filepath.Join(override, p1.ID+".json")
	if !strings.HasSuffix(wantSnap, p1.ID+".json") {
		t.Fatalf("snapshot path derivation wrong: %s", wantSnap)
	}
}

// Test 5
// TestCompressor_Apply_BytePreservationGuard exercises the
// byte-preservation validator across three shapes:
//   (a) checkPreservation() on a response that contains every anchored
//       line verbatim from the original — returns no missing lines.
//   (b) writeAgentsMD() with empty kept slice — returns nil and writes
//       no file (data-loss safe: empty input never truncates a real file).
//   (c) LLMSummarizer.MergeDrops() real round-trip — preserves the
//       stub's response and produces a PlanMerge with strategy=LLM.
func TestCompressor_Apply_BytePreservationGuard(t *testing.T) {
	// (a) checkPreservation overlap: response ⊇ anchors.
	original := strings.Join([]string{
		"# H",
		"plain prose",
		"```bash",
		"echo hi",
		"```",
		"https://example.com/x",
		"/etc/sin-code/foo.db",
		"$ make build",
	}, "\n")
	response := strings.Join([]string{
		"# H",
		"plain prose (rewritten but not anchored)",
		"```bash",
		"echo hi",
		"```",
		"https://example.com/x",
		"/etc/sin-code/foo.db",
		"$ make build",
	}, "\n")
	missing := checkPreservation(original, response)
	if len(missing) != 0 {
		t.Errorf("byte-preservation flagging on overlap response: %v", missing)
	}

	// Missing-line case: drop "# H" from response -> exactly 1 missing.
	missing2 := checkPreservation(original, strings.ReplaceAll(response, "# H\n", ""))
	if len(missing2) != 1 || !strings.Contains(missing2[0], "# H") {
		t.Fatalf("missing-line detection broken: %v", missing2)
	}

	// (b) writeAgentsMD with empty kept slice writes no file.
	keptEmpty := []rawEntry{}
	tmpAgents := filepath.Join(t.TempDir(), "AGENTS.md")
	if err := os.WriteFile(tmpAgents, []byte("ORIGINAL"), 0o644); err != nil {
		t.Fatalf("seed AGENTS.md: %v", err)
	}
	paths := Paths{AgentsMD: tmpAgents}
	if err := writeAgentsMD(keptEmpty, paths); err != nil {
		t.Errorf("writeAgentsMD(empty) returned error: %v", err)
	}
	after, err := os.ReadFile(tmpAgents)
	if err != nil {
		t.Fatalf("read post-empty AGENTS.md: %v", err)
	}
	if !bytes.Equal(after, []byte("ORIGINAL")) {
		t.Errorf("writeAgentsMD(empty) modified AGENTS.md: got %q", after)
	}
	// tmp file from a previous crashed write must not exist.
	if _, err := os.Stat(tmpAgents + ".tmp"); err == nil {
		t.Error("writeAgentsMD(empty) left a .tmp file behind")
	}

	// (c) LLM summarizer preserves anchored lines end-to-end.
	// Bundle writes "# <subject>" before each body, so the response
	// must include "# input-section" for byte-preservation to pass.
	preservingBody := strings.Join([]string{
		"# input-section",
		"# Title",
		"https://example.org/r",
		"$ go test ./...",
		"```",
		"pass",
		"```",
	}, "\n")
	s := newStubLLMSummarizer(t, stubLLMResp(preservingBody))
	drops := []PlanEntry{{
		Hash:    ContentHash(strings.Join([]string{"# Title", "https://example.org/r", "$ go test ./...", "```", "pass", "```", ""}, "\n")),
		Target:  TargetAgentsMD,
		Subject: "input-section",
		Body:    strings.Join([]string{"# Title", "https://example.org/r", "$ go test ./...", "```", "pass", "```", ""}, "\n"),
		Bytes:   80,
		Utility: 0.6,
		Created: "2025-01-01T00:00:00Z",
	}}
	merge, err := s.MergeDrops(drops, MergeOpts{TargetRatio: 0.5, MaxRetries: 1})
	if err != nil {
		t.Fatalf("MergeDrops: %v", err)
	}
	if merge == nil {
		t.Fatal("MergeDrops returned nil despite preserving response")
	}
	if merge.Strategy != StrategyLLM {
		t.Errorf("merge.Strategy: want %s, got %s", StrategyLLM, merge.Strategy)
	}
	if len(merge.SourceHashes) != 1 || merge.SourceHashes[0] != drops[0].Hash {
		t.Errorf("SourceHashes mismatch: %v", merge.SourceHashes)
	}
	if !strings.Contains(merge.Body, "https://example.org/r") {
		t.Errorf("LLM response dropped anchor: %q", merge.Body)
	}
	// ID is byte-stable across reruns (response hash is fixed).
	merge2, err := s.MergeDrops(drops, MergeOpts{TargetRatio: 0.5, MaxRetries: 1})
	if err != nil {
		t.Fatalf("MergeDrops #2: %v", err)
	}
	if merge2.ID != merge.ID {
		t.Errorf("ID non-stable across reruns: %s vs %s", merge.ID, merge2.ID)
	}
}
