// SPDX-License-Identifier: MIT
// Purpose: coverage-boost tests for the compress package. Targets 0%
// and low-coverage functions identified by `go tool cover -func`.
package compress

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
)

// ---------------------------------------------------------------------------
// isFromTarget (0% → 100%)
// ---------------------------------------------------------------------------

func TestIsFromTarget_MatchAndNoMatch(t *testing.T) {
	before := []rawEntry{
		{Body: "hello world"},
		{Body: "another entry"},
	}
	h := ContentHash("hello world")
	if !isFromTarget(TargetLessons, h, before) {
		t.Error("isFromTarget should find matching hash")
	}
	h2 := ContentHash("nonexistent")
	if isFromTarget(TargetLessons, h2, before) {
		t.Error("isFromTarget should not find nonexistent hash")
	}
}

func TestIsFromTarget_EmptyBefore(t *testing.T) {
	h := ContentHash("anything")
	if isFromTarget(TargetLessons, h, []rawEntry{}) {
		t.Error("isFromTarget should return false for empty before list")
	}
}

// ---------------------------------------------------------------------------
// writeSummaries (0% → 100%) — no-op writer
// ---------------------------------------------------------------------------

func TestWriteSummaries_IsNoOp(t *testing.T) {
	if err := writeSummaries([]rawEntry{{Body: "test"}}, Paths{}); err != nil {
		t.Fatalf("writeSummaries returned error: %v", err)
	}
	if err := writeSummaries(nil, Paths{}); err != nil {
		t.Fatalf("writeSummaries(nil) returned error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// writeTarget dispatch (28.6% → higher)
// ---------------------------------------------------------------------------

func TestWriteTarget_UnknownTarget(t *testing.T) {
	err := writeTarget(Target("bogus"), nil, Paths{})
	if err == nil {
		t.Fatal("writeTarget should error on unknown target")
	}
	if !strings.Contains(err.Error(), "unknown target") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWriteTarget_SummariesDispatch(t *testing.T) {
	if err := writeTarget(TargetSummaries, []rawEntry{}, Paths{}); err != nil {
		t.Fatalf("writeTarget(TargetSummaries): %v", err)
	}
}

// ---------------------------------------------------------------------------
// writeAgentsMD (15.4% → higher) — with actual content
// ---------------------------------------------------------------------------

func TestWriteAgentsMD_WritesContent(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "AGENTS.md")
	kept := []rawEntry{{Body: "# New AGENTS.md\n\nRules here.\n"}}
	if err := writeAgentsMD(kept, Paths{AgentsMD: p}); err != nil {
		t.Fatalf("writeAgentsMD: %v", err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "# New AGENTS.md\n\nRules here.\n" {
		t.Errorf("unexpected content: %q", data)
	}
}

func TestWriteAgentsMD_DiscoveryFallback(t *testing.T) {
	dir := t.TempDir()
	agentsFile := filepath.Join(dir, "AGENTS.md")
	if err := os.WriteFile(agentsFile, []byte("OLD"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	kept := []rawEntry{{Body: "REPLACED"}}
	if err := writeAgentsMD(kept, Paths{}); err != nil {
		t.Fatalf("writeAgentsMD discovery: %v", err)
	}
	data, _ := os.ReadFile(agentsFile)
	if string(data) != "REPLACED" {
		t.Errorf("expected REPLACED, got %q", data)
	}
}

// ---------------------------------------------------------------------------
// SnapshotDir (33.3% → higher)
// ---------------------------------------------------------------------------

func TestSnapshotDir_DefaultHome(t *testing.T) {
	t.Setenv("SIN_CODE_SNAPSHOT_DIR", "")
	t.Setenv("SIN_CODE_HOME", "")
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".local", "share", "sin-code", "compress-snapshots")
	if got := SnapshotDir(); got != want {
		t.Errorf("SnapshotDir default: want %s, got %s", want, got)
	}
}

func TestSnapshotDir_SIN_CODE_HOME(t *testing.T) {
	t.Setenv("SIN_CODE_SNAPSHOT_DIR", "")
	t.Setenv("SIN_CODE_HOME", "/custom/home")
	want := "/custom/home/compress-snapshots"
	if got := SnapshotDir(); got != want {
		t.Errorf("SnapshotDir SIN_CODE_HOME: want %s, got %s", want, got)
	}
}

// ---------------------------------------------------------------------------
// mergesBySourceHash (60% → 100%)
// ---------------------------------------------------------------------------

func TestMergesBySourceHash(t *testing.T) {
	merges := []PlanMerge{
		{ID: "m1", SourceHashes: []string{"h1", "h2"}},
		{ID: "m2", SourceHashes: []string{"h3"}},
	}
	out := mergesBySourceHash(TargetLessons, merges)
	if !out["h1"] || !out["h2"] || !out["h3"] {
		t.Errorf("mergesBySourceHash missing hashes: %v", out)
	}
	if len(out) != 3 {
		t.Errorf("expected 3 entries, got %d", len(out))
	}
	// Empty merges
	out2 := mergesBySourceHash(TargetLessons, nil)
	if len(out2) != 0 {
		t.Errorf("expected empty map for nil merges, got %d", len(out2))
	}
}

// ---------------------------------------------------------------------------
// isMergedSource (40% → 100%)
// ---------------------------------------------------------------------------

func TestIsMergedSource(t *testing.T) {
	p := Plan{Merges: []PlanMerge{
		{SourceHashes: []string{"abc", "def"}},
	}}
	if !isMergedSource(TargetLessons, "abc", p) {
		t.Error("isMergedSource should find abc")
	}
	if isMergedSource(TargetLessons, "xyz", p) {
		t.Error("isMergedSource should not find xyz")
	}
	// Empty merges
	p2 := Plan{}
	if isMergedSource(TargetLessons, "abc", p2) {
		t.Error("isMergedSource should return false for empty merges")
	}
}

// ---------------------------------------------------------------------------
// parseTimeOrNow (66.7% → 100%)
// ---------------------------------------------------------------------------

func TestParseTimeOrNow(t *testing.T) {
	// Empty string → now
	got := parseTimeOrNow("")
	if got.IsZero() {
		t.Error("parseTimeOrNow('') should return non-zero time")
	}
	// Valid RFC3339
	got2 := parseTimeOrNow("2025-01-01T00:00:00Z")
	want := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if !got2.Equal(want) {
		t.Errorf("parseTimeOrNow(valid): want %v, got %v", want, got2)
	}
	// Invalid string → now
	got3 := parseTimeOrNow("not-a-date")
	if got3.IsZero() {
		t.Error("parseTimeOrNow('not-a-date') should return non-zero fallback")
	}
}

// ---------------------------------------------------------------------------
// now (66.7% → 100%)
// ---------------------------------------------------------------------------

func TestPlanOptionsNow_StableTime(t *testing.T) {
	st := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	opts := PlanOptions{UseStableTime: true, StableTime: st}
	got := opts.now()
	if !got.Equal(st.UTC()) {
		t.Errorf("now with stable time: want %v, got %v", st.UTC(), got)
	}
}

func TestPlanOptionsNow_Default(t *testing.T) {
	opts := PlanOptions{}
	got := opts.now()
	if got.IsZero() {
		t.Error("now default should return non-zero time")
	}
}

// ---------------------------------------------------------------------------
// itoa (85.7% → 100%)
// ---------------------------------------------------------------------------

func TestItoa(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{9, "9"},
		{10, "10"},
		{123, "123"},
		{9999, "9999"},
	}
	for _, c := range cases {
		if got := itoa(c.in); got != c.want {
			t.Errorf("itoa(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// guessLessonType (83.3% → 100%)
// ---------------------------------------------------------------------------

func TestGuessLessonType_AllPrefixes(t *testing.T) {
	cases := []struct {
		subject string
		want    string
	}{
		{"[failed_verification] something", "failed_verification"},
		{"[success_pattern] good", "success_pattern"},
		{"[constraint] rule", "constraint"},
		{"[tool_error] boom", "tool_error"},
		{"unknown prefix", "constraint"}, // fallback
	}
	for _, c := range cases {
		got := guessLessonType(c.subject)
		if string(got) != c.want {
			t.Errorf("guessLessonType(%q) = %s, want %s", c.subject, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// NewLLMSummarizer (0% → higher)
// ---------------------------------------------------------------------------

func TestNewLLMSummarizer_NilClientEnvFallback(t *testing.T) {
	t.Setenv("SIN_CODE_OFFLINE", "")
	s, err := NewLLMSummarizer(nil)
	if err != nil {
		t.Fatalf("NewLLMSummarizer(nil): %v", err)
	}
	if s == nil {
		t.Fatal("summarizer should not be nil")
	}
	// In a test env without provider config, Available() should be false
	// (no BaseURL resolved). This covers the env-fallback path.
	_ = s.Available()
}

func TestNewLLMSummarizer_ClientWithEmptyBaseURL(t *testing.T) {
	// A non-nil client with empty BaseURL should error.
	c := &llm.Client{BaseURL: ""}
	_, err := NewLLMSummarizer(c)
	if err == nil {
		t.Fatal("expected error for client with empty BaseURL")
	}
	if !strings.Contains(err.Error(), "BaseURL") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// resolvedModel (0% → 100%)
// ---------------------------------------------------------------------------

func TestResolvedModel_EnvOverride(t *testing.T) {
	t.Setenv("SIN_LLM_MODEL", "my-custom-model")
	if got := resolvedModel(); got != "my-custom-model" {
		t.Errorf("resolvedModel with env: want %q, got %q", "my-custom-model", got)
	}
}

func TestResolvedModel_Default(t *testing.T) {
	t.Setenv("SIN_LLM_MODEL", "")
	if got := resolvedModel(); got != "compress-summary" {
		t.Errorf("resolvedModel default: want %q, got %q", "compress-summary", got)
	}
}

// ---------------------------------------------------------------------------
// Available (57.1% → 100%)
// ---------------------------------------------------------------------------

func TestLLMSummarizerAvailable_NilSummarizer(t *testing.T) {
	var s *LLMSummarizer
	if s.Available() {
		t.Error("nil summarizer should not be available")
	}
}

func TestLLMSummarizerAvailable_NilClient(t *testing.T) {
	s := &LLMSummarizer{}
	if s.Available() {
		t.Error("summarizer with nil client should not be available")
	}
}

func TestLLMSummarizerAvailable_EmptyBaseURL(t *testing.T) {
	s := &LLMSummarizer{client: &llm.Client{BaseURL: ""}}
	if s.Available() {
		t.Error("summarizer with empty BaseURL should not be available")
	}
}

func TestLLMSummarizerAvailable_OfflineEnv(t *testing.T) {
	t.Setenv("SIN_CODE_OFFLINE", "1")
	s := &LLMSummarizer{client: &llm.Client{BaseURL: "http://stub"}}
	if s.Available() {
		t.Error("summarizer should not be available when SIN_CODE_OFFLINE is set")
	}
}

// ---------------------------------------------------------------------------
// expandTargets / targetNames / Target.IsValid / Strategy.IsValid
// ---------------------------------------------------------------------------

func TestExpandTargets_EmptyTarget(t *testing.T) {
	_, err := expandTargets("")
	if err == nil {
		t.Fatal("expandTargets('') should error")
	}
}

func TestExpandTargets_InvalidTarget(t *testing.T) {
	_, err := expandTargets(Target("bogus"))
	if err == nil {
		t.Fatal("expandTargets('bogus') should error")
	}
}

func TestExpandTargets_All(t *testing.T) {
	out, err := expandTargets(TargetAll)
	if err != nil {
		t.Fatalf("expandTargets(all): %v", err)
	}
	if len(out) != len(AllTargets) {
		t.Errorf("expected %d targets, got %d", len(AllTargets), len(out))
	}
}

func TestTargetIsValid(t *testing.T) {
	for _, tgt := range AllTargets {
		if !tgt.IsValid() {
			t.Errorf("target %q should be valid", tgt)
		}
	}
	if !TargetAll.IsValid() {
		t.Error("TargetAll should be valid")
	}
	if Target("bogus").IsValid() {
		t.Error("bogus target should be invalid")
	}
}

func TestStrategyIsValid(t *testing.T) {
	for _, s := range []Strategy{StrategyDeterministic, StrategyLLM, StrategyHybrid} {
		if !s.IsValid() {
			t.Errorf("strategy %q should be valid", s)
		}
	}
	if Strategy("bogus").IsValid() {
		t.Error("bogus strategy should be invalid")
	}
}

func TestTargetNames(t *testing.T) {
	names := targetNames()
	if len(names) != len(AllTargets)+1 {
		t.Errorf("targetNames length: want %d, got %d", len(AllTargets)+1, len(names))
	}
	// Last should be "all"
	if names[len(names)-1] != string(TargetAll) {
		t.Errorf("last target name should be 'all', got %q", names[len(names)-1])
	}
}

// ---------------------------------------------------------------------------
// BuildPlan error paths (72.1% → higher)
// ---------------------------------------------------------------------------

func TestBuildPlan_InvalidStrategy(t *testing.T) {
	_, err := BuildPlan(TargetLessons, Strategy("bogus"), Paths{}, withStableTime())
	if err == nil {
		t.Fatal("BuildPlan should error on invalid strategy")
	}
	if !strings.Contains(err.Error(), "unknown strategy") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBuildPlan_InvalidTarget(t *testing.T) {
	_, err := BuildPlan(Target("bogus"), StrategyDeterministic, Paths{}, withStableTime())
	if err == nil {
		t.Fatal("BuildPlan should error on invalid target")
	}
}

// ---------------------------------------------------------------------------
// utilityScore (78.6% → higher)
// ---------------------------------------------------------------------------

func TestUtilityScore_FutureTime(t *testing.T) {
	e := PlanEntry{Created: "2099-01-01T00:00:00Z", Bytes: 100}
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	score := utilityScore(e, now)
	// Future time → age=0 → recency=1.0
	if score < 1.0 {
		t.Errorf("future entry should have recency=1.0, got score=%f", score)
	}
}

func TestUtilityScore_InvalidCreated(t *testing.T) {
	e := PlanEntry{Created: "not-a-date", Bytes: 100}
	score := utilityScore(e, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	// Invalid date → no recency, only size bonus
	if score > 0.31 {
		t.Errorf("invalid date should have only size bonus, got %f", score)
	}
}

func TestUtilityScore_OldEntry(t *testing.T) {
	e := PlanEntry{Created: "2000-01-01T00:00:00Z", Bytes: 100}
	score := utilityScore(e, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	// 25 years old → recency=0, only size bonus
	if score > 0.31 {
		t.Errorf("very old entry should have ~0 recency, got %f", score)
	}
}

func TestUtilityScore_LargeBody(t *testing.T) {
	e := PlanEntry{Created: "2025-01-01T00:00:00Z", Bytes: 100000}
	score := utilityScore(e, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	// Large body → sizeBonus=0, so score = recency only
	if score > 1.01 {
		t.Errorf("large body should have ~0 size bonus, got %f", score)
	}
}

// ---------------------------------------------------------------------------
// deterministic (60% → higher)
// ---------------------------------------------------------------------------

func TestDeterministic_KeepRecentDays(t *testing.T) {
	st := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	set := normalizedSet{
		target: TargetLessons,
		entries: []PlanEntry{
			{Hash: "a", Target: TargetLessons, Body: "old", Bytes: 3, Created: "2020-01-01T00:00:00Z"},
			{Hash: "b", Target: TargetLessons, Body: "new", Bytes: 3, Created: "2025-05-01T00:00:00Z"},
		},
	}
	opts := PlanOptions{UseStableTime: true, StableTime: st, KeepRecentDays: 365}
	keeps, drops, warns := deterministic(set, opts)
	if len(drops) == 0 {
		t.Fatal("expected age-based drops")
	}
	if len(warns) == 0 {
		t.Fatal("expected age warning")
	}
	_ = keeps
}

func TestDeterministic_KeepMaxEntries(t *testing.T) {
	set := normalizedSet{
		target: TargetLessons,
		entries: []PlanEntry{
			{Hash: "a", Target: TargetLessons, Body: "a", Bytes: 1, Created: "2025-01-01T00:00:00Z"},
			{Hash: "b", Target: TargetLessons, Body: "b", Bytes: 1, Created: "2025-01-01T00:00:00Z"},
			{Hash: "c", Target: TargetLessons, Body: "c", Bytes: 1, Created: "2025-01-01T00:00:00Z"},
		},
	}
	opts := PlanOptions{UseStableTime: true, StableTime: fixedTime(), KeepMaxEntries: 1}
	keeps, drops, _ := deterministic(set, opts)
	if len(keeps) != 1 {
		t.Fatalf("expected 1 keep, got %d", len(keeps))
	}
	if len(drops) != 2 {
		t.Fatalf("expected 2 drops, got %d", len(drops))
	}
}

func TestDeterministic_AgeFilterWarning(t *testing.T) {
	st := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	set := normalizedSet{
		target: TargetLessons,
		entries: []PlanEntry{
			{Hash: "a", Target: TargetLessons, Body: "old", Bytes: 3, Created: "2020-01-01T00:00:00Z"},
		},
	}
	opts := PlanOptions{UseStableTime: true, StableTime: st, KeepRecentDays: 365}
	_, drops, warns := deterministic(set, opts)
	if len(drops) == 0 {
		t.Fatal("expected age-based drops")
	}
	found := false
	for _, w := range warns {
		if strings.Contains(w, "older than") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected age warning, got %v", warns)
	}
}

func TestDeterministic_EmptySet(t *testing.T) {
	set := normalizedSet{target: TargetLessons}
	opts := PlanOptions{UseStableTime: true, StableTime: fixedTime()}
	keeps, drops, warns := deterministic(set, opts)
	if len(keeps) != 0 || len(drops) != 0 || len(warns) != 0 {
		t.Errorf("empty set should produce no keeps/drops/warnings")
	}
}

// ---------------------------------------------------------------------------
// applyMemoryAtomic (0% → higher) — requires a bbolt path
// ---------------------------------------------------------------------------

func TestApplyMemoryAtomic_BasicRoundTrip(t *testing.T) {
	memPath := filepath.Join(t.TempDir(), "memory.db")
	kept := []rawEntry{
		{Body: strings.Repeat("a", 200), Created: "2025-01-01T00:00:00Z"},
	}
	if err := applyMemoryAtomic(kept, Paths{Memory: memPath}); err != nil {
		t.Fatalf("applyMemoryAtomic: %v", err)
	}
}

// ---------------------------------------------------------------------------
// oneLine / stableAnyMap / idFor / shortHash
// ---------------------------------------------------------------------------

func TestOneLine(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"hello world", 80, "hello world"},
		{"\n\nfirst non-empty", 80, "first non-empty"},
		{strings.Repeat("x", 100), 10, "xxxxxxxxx…"},
		{"", 80, ""},
	}
	for _, c := range cases {
		got := oneLine(c.in, c.n)
		if got != c.want {
			t.Errorf("oneLine(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}

func TestStableAnyMap(t *testing.T) {
	if got := stableAnyMap(nil); got == nil {
		t.Error("stableAnyMap(nil) should return non-nil map")
	}
	m := map[string]any{"k": "v"}
	if got := stableAnyMap(m); got["k"] != "v" {
		t.Error("stableAnyMap should preserve values")
	}
}

func TestShortHash(t *testing.T) {
	h := shortHash("test body")
	if len(h) != 16 {
		t.Errorf("shortHash length: want 16, got %d", len(h))
	}
}

func TestIdFor(t *testing.T) {
	id := idFor(TargetLessons, "somehash")
	if !strings.HasPrefix(id, "plan-") {
		t.Errorf("idFor should start with 'plan-', got %q", id)
	}
	if len(id) != len("plan-")+16 {
		t.Errorf("idFor length: want %d, got %d", len("plan-")+16, len(id))
	}
}

// ---------------------------------------------------------------------------
// load error path
// ---------------------------------------------------------------------------

func TestLoad_UnknownTarget(t *testing.T) {
	_, _, err := load(Target("bogus"), Paths{})
	if err == nil {
		t.Fatal("load should error on unknown target")
	}
}
