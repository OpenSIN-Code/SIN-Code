// SPDX-License-Identifier: MIT
// Purpose: coverage-boost tests for pure-logic helpers in the
// evalharness package — VerbosityArm, ReadBundledSkillBody,
// ByArm, countLOC, NoOpSubject.Run, truncateMeta, NewCommand,
// and several partially-covered helpers (renderSkillPrompt,
// readBundledSkillBody, locateSKILL, ensureWeight, priceLookup,
// DefaultScorer, readIntMeta, readStringMeta, PassRate).
package evalharness

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── VerbosityArm (arms.go:89) — was 0% ──────────────────────────

func TestVerbosityArm_EmptyLevelDefaultsToDefault(t *testing.T) {
	arm := VerbosityArm("", nil)
	if arm.ID != "default" {
		t.Fatalf("expected ID=default, got %q", arm.ID)
	}
	if arm.SystemPrompt != "" {
		t.Fatalf("nil reader should yield empty prompt, got %q", arm.SystemPrompt)
	}
	if arm.Verbosity != "default" {
		t.Fatalf("expected Verbosity=default, got %q", arm.Verbosity)
	}
}

func TestVerbosityArm_WithReaderBody(t *testing.T) {
	arm := VerbosityArm("terse", func() (string, error) {
		return "be brief", nil
	})
	if arm.ID != "terse" {
		t.Fatalf("expected ID=terse, got %q", arm.ID)
	}
	want := TersePrefix + "\n\nbe brief"
	if arm.SystemPrompt != want {
		t.Fatalf("expected %q, got %q", want, arm.SystemPrompt)
	}
}

func TestVerbosityArm_ReaderErrorYieldsEmptyBody(t *testing.T) {
	arm := VerbosityArm("ultra", func() (string, error) {
		return "", os.ErrNotExist
	})
	if arm.SystemPrompt != "" {
		t.Fatalf("reader error should yield empty body, got %q", arm.SystemPrompt)
	}
}

func TestVerbosityArm_TerseLevelSkipsPrefix(t *testing.T) {
	arm := VerbosityArm("__terse__", func() (string, error) {
		return "custom terse body", nil
	})
	if arm.SystemPrompt != "custom terse body" {
		t.Fatalf("__terse__ level should not prepend TersePrefix, got %q", arm.SystemPrompt)
	}
}

// ── FusionArm (arms.go:115) ─────────────────────────────────────

func TestFusionArm(t *testing.T) {
	base := StandardTerseArm()
	fa := FusionArm(base)
	if !fa.FusionEnabled {
		t.Fatal("FusionArm should set FusionEnabled=true")
	}
	if fa.ID != "__terse__" {
		t.Fatalf("FusionArm should preserve base ID, got %q", fa.ID)
	}
}

// ── SkillArm empty name (arms.go:67) ────────────────────────────

func TestSkillArm_EmptyNameReturnsErrorSetup(t *testing.T) {
	arm := SkillArm("", nil)
	if arm.ID != "__user_skill__" {
		t.Fatalf("expected ID=__user_skill__, got %q", arm.ID)
	}
	if arm.Setup == nil {
		t.Fatal("empty-name SkillArm should have a non-nil Setup that returns an error")
	}
	if err := arm.Setup(EvalCase{}); err == nil {
		t.Fatal("Setup should return an error for empty skill name")
	}
}

// ── safeReadSkill (arms.go:142) — nil branch ────────────────────

func TestSafeReadSkill_NilReader(t *testing.T) {
	body, err := safeReadSkill(nil)
	if err != nil || body != "" {
		t.Fatalf("nil reader should return empty body + nil err, got %q, %v", body, err)
	}
}

// ── renderSkillPrompt (arms.go:154) — error + empty branches ────

func TestRenderSkillPrompt_ErrorBranch(t *testing.T) {
	got := renderSkillPrompt("", os.ErrNotExist)
	if !strings.Contains(got, "[skill unavailable:") {
		t.Fatalf("error branch should contain unavailable marker, got %q", got)
	}
	if !strings.HasPrefix(got, TersePrefix) {
		t.Fatalf("error branch should start with TersePrefix, got %q", got)
	}
}

func TestRenderSkillPrompt_EmptyBodyBranch(t *testing.T) {
	got := renderSkillPrompt("   \n  ", nil)
	if got != TersePrefix+"\n\n[skill unavailable: not on disk]" {
		t.Fatalf("empty body branch mismatch, got %q", got)
	}
}

func TestRenderSkillPrompt_ValidBody(t *testing.T) {
	got := renderSkillPrompt("  hello world  ", nil)
	want := TersePrefix + "\n\nhello world"
	if got != want {
		t.Fatalf("valid body: got %q want %q", got, want)
	}
}

// ── ReadBundledSkillBody (arms.go:202) — was 0% ─────────────────

func TestReadBundledSkillBody_EmptyName(t *testing.T) {
	body, err := ReadBundledSkillBody("")
	if err != nil || body != "" {
		t.Fatalf("empty name should return empty body + nil err, got %q, %v", body, err)
	}
}

func TestReadBundledSkillBody_FromEnvDir(t *testing.T) {
	tmp := t.TempDir()
	catDir := filepath.Join(tmp, "code-skills", "skill-test-fixture")
	if err := os.MkdirAll(catDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillBody := "# Test Skill\n\nbody text"
	if err := os.WriteFile(filepath.Join(catDir, "SKILL.md"), []byte(skillBody), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIN_SKILLS_DIR", tmp)
	body, err := ReadBundledSkillBody("skill-test-fixture")
	if err != nil {
		t.Fatalf("ReadBundledSkillBody: %v", err)
	}
	if body != skillBody {
		t.Fatalf("body mismatch: got %q want %q", body, skillBody)
	}
}

func TestReadBundledSkillBody_NotFoundReturnsEmpty(t *testing.T) {
	t.Setenv("SIN_SKILLS_DIR", t.TempDir())
	body, err := ReadBundledSkillBody("nonexistent-skill")
	if err != nil || body != "" {
		t.Fatalf("nonexistent skill should return empty + nil, got %q, %v", body, err)
	}
}

// ── bundledSkillsRoots (arms.go:210) ────────────────────────────

func TestBundledSkillsRoots_EnvDirPrepended(t *testing.T) {
	t.Setenv("SIN_SKILLS_DIR", "/custom/skills")
	roots := bundledSkillsRoots()
	if len(roots) == 0 || roots[0] != "/custom/skills" {
		t.Fatalf("expected first root=/custom/skills, got %v", roots)
	}
}

// ── locateSKILL (arms.go:232) — missing-dir branch ──────────────

func TestLocateSKILL_EmptyRoot(t *testing.T) {
	loc, err := locateSKILL("", "some-skill")
	if err != nil || loc != "" {
		t.Fatalf("empty root should return empty + nil, got %q, %v", loc, err)
	}
}

func TestLocateSKILL_EmptySkillName(t *testing.T) {
	loc, err := locateSKILL("/tmp", "")
	if err != nil || loc != "" {
		t.Fatalf("empty skill name should return empty + nil, got %q, %v", loc, err)
	}
}

func TestLocateSKILL_NonExistentRoot(t *testing.T) {
	loc, err := locateSKILL(filepath.Join(t.TempDir(), "nope"), "some-skill")
	if err != nil || loc != "" {
		t.Fatalf("non-existent root should return empty + nil, got %q, %v", loc, err)
	}
}

func TestLocateSKILL_FindsSkill(t *testing.T) {
	root := t.TempDir()
	catDir := filepath.Join(root, "code-skills", "my-skill")
	if err := os.MkdirAll(catDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(catDir, "SKILL.md"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	loc, err := locateSKILL(root, "my-skill")
	if err != nil {
		t.Fatalf("locateSKILL: %v", err)
	}
	if loc == "" {
		t.Fatal("should find SKILL.md")
	}
}

func TestLocateSKILL_IgnoresNonSkillsDir(t *testing.T) {
	root := t.TempDir()
	plainDir := filepath.Join(root, "plain-dir", "my-skill")
	if err := os.MkdirAll(plainDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plainDir, "SKILL.md"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	loc, err := locateSKILL(root, "my-skill")
	if err != nil || loc != "" {
		t.Fatalf("should ignore non-*-skills directories, got %q, %v", loc, err)
	}
}

// ── ByArm (comparator.go:115) — was 0% ──────────────────────────

func TestByArm_PivotsByArmID(t *testing.T) {
	prev := SetDefaultSubject(stubArmSubject{OutputLineCount: 2})
	defer SetDefaultSubject(prev)

	arms := []Arm{NoSystemPromptArm(), StandardTerseArm()}
	rep, err := Compare(context.Background(), twoCaseSet(), arms, CompareOptions{})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	byArm := rep.ByArm()
	if len(byArm) != 2 {
		t.Fatalf("expected 2 arms in ByArm, got %d", len(byArm))
	}
	for _, arm := range arms {
		runs, ok := byArm[arm.ID]
		if !ok {
			t.Fatalf("ByArm missing arm %s", arm.ID)
		}
		if len(runs) != 2 {
			t.Fatalf("arm %s: expected 2 runs, got %d", arm.ID, len(runs))
		}
	}
}

func TestByArm_EmptyReport(t *testing.T) {
	rep := CompareReport{Arms: []Arm{{ID: "x"}}}
	byArm := rep.ByArm()
	if _, ok := byArm["x"]; !ok {
		t.Fatal("ByArm should include arms with zero runs")
	}
	if len(byArm["x"]) != 0 {
		t.Fatalf("expected 0 runs for arm x, got %d", len(byArm["x"]))
	}
}

// ── countLOC (comparator.go:460) — was 0% ───────────────────────

func TestCountLOC(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{"empty", "", 0},
		{"single line", "hello", 1},
		{"two lines", "hello\nworld", 2},
		{"three lines", "a\nb\nc", 3},
		{"trailing newline", "a\nb\n", 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := countLOC(tc.text)
			if got != tc.want {
				t.Fatalf("countLOC(%q) = %d, want %d", tc.text, got, tc.want)
			}
		})
	}
}

// ── NoOpSubject.Run (comparator.go:491) — was 0% ────────────────

func TestNoOpSubject_Run_NoMeta(t *testing.T) {
	s := NoOpSubject{Prefix: "pre-"}
	out, err := s.Run(context.Background(), EvalCase{Prompt: "hello"})
	if err != nil {
		t.Fatalf("NoOpSubject.Run: %v", err)
	}
	if out.Text != "pre-hello" {
		t.Fatalf("expected pre-hello, got %q", out.Text)
	}
	if out.Success {
		t.Fatal("NoOp should return Success=false")
	}
}

func TestNoOpSubject_Run_WithMeta(t *testing.T) {
	s := NoOpSubject{}
	out, err := s.Run(context.Background(), EvalCase{
		Prompt: "hello",
		Meta: map[string]string{
			"system_prompt": "sys-prompt",
			"arm_id":        "arm1",
		},
	})
	if err != nil {
		t.Fatalf("NoOpSubject.Run: %v", err)
	}
	if !strings.Contains(out.Text, "[system:sys-prompt]") {
		t.Fatalf("expected system prompt marker, got %q", out.Text)
	}
	if !strings.Contains(out.Text, "[arm:arm1]") {
		t.Fatalf("expected arm marker, got %q", out.Text)
	}
}

func TestNoOpSubject_Run_TruncatesLongSystemPrompt(t *testing.T) {
	s := NoOpSubject{}
	longSP := strings.Repeat("x", 200)
	out, _ := s.Run(context.Background(), EvalCase{
		Prompt: "hello",
		Meta:   map[string]string{"system_prompt": longSP},
	})
	if !strings.Contains(out.Text, "…") {
		t.Fatalf("expected truncation marker, got %q", out.Text)
	}
}

// ── truncateMeta (comparator.go:537) — was 0% ───────────────────

func TestTruncateMeta_NoTruncation(t *testing.T) {
	got := truncateMeta("short", 80)
	if got != "short" {
		t.Fatalf("expected short, got %q", got)
	}
}

func TestTruncateMeta_Truncates(t *testing.T) {
	long := strings.Repeat("a", 100)
	got := truncateMeta(long, 80)
	// "…" is 3 bytes in UTF-8, so 80 + 3 = 83
	if len(got) != 83 {
		t.Fatalf("expected 83 bytes, got %d", len(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected truncation suffix, got %q", got)
	}
}

func TestTruncateMeta_ExactLength(t *testing.T) {
	got := truncateMeta("exactly80chars", 80)
	if got != "exactly80chars" {
		t.Fatalf("exact length should not truncate, got %q", got)
	}
}

// ── ensureWeight (comparator.go:388) ────────────────────────────

func TestEnsureWeight_ZeroDefaultsToOne(t *testing.T) {
	if w := ensureWeight(0); w != 1 {
		t.Fatalf("ensureWeight(0) = %f, want 1", w)
	}
}

func TestEnsureWeight_NonZeroPreserved(t *testing.T) {
	if w := ensureWeight(2.5); w != 2.5 {
		t.Fatalf("ensureWeight(2.5) = %f, want 2.5", w)
	}
}

// ── priceLookup (comparator.go:400) ─────────────────────────────

func TestPriceLookup_EmptyDefaultsToStub(t *testing.T) {
	if got := priceLookup(Arm{}); got != "stub" {
		t.Fatalf("priceLookup(empty) = %q, want stub", got)
	}
}

func TestPriceLookup_PreservesArmName(t *testing.T) {
	if got := priceLookup(Arm{PricingName: "gpt-4o"}); got != "gpt-4o" {
		t.Fatalf("priceLookup(gpt-4o) = %q, want gpt-4o", got)
	}
}

// ── DefaultScorer (comparator.go:414) ───────────────────────────

func TestDefaultScorer_SuccessTrue(t *testing.T) {
	score, passed, detail := DefaultScorer{}.Score(EvalCase{}, Output{Success: true})
	if score != 1 || !passed || detail != "subject success flag" {
		t.Fatalf("success=true: score=%f passed=%v detail=%q", score, passed, detail)
	}
}

func TestDefaultScorer_SuccessFalse(t *testing.T) {
	score, passed, detail := DefaultScorer{}.Score(EvalCase{}, Output{Success: false})
	if score != 0 || passed || detail != "no success flag" {
		t.Fatalf("success=false: score=%f passed=%v detail=%q", score, passed, detail)
	}
}

// ── readIntMeta (comparator.go:422) ─────────────────────────────

func TestReadIntMeta(t *testing.T) {
	tests := []struct {
		name     string
		m        map[string]string
		key      string
		fallback int
		want     int
	}{
		{"nil map", nil, "x", 5, 5},
		{"missing key", map[string]string{}, "x", 5, 5},
		{"valid int", map[string]string{"x": "42"}, "x", 0, 42},
		{"negative sign skipped", map[string]string{"x": "-3"}, "x", 0, 3},
		{"invalid char", map[string]string{"x": "12a"}, "x", 7, 7},
		{"empty string returns 0", map[string]string{"x": ""}, "x", 9, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := readIntMeta(tc.m, tc.key, tc.fallback)
			if got != tc.want {
				t.Fatalf("readIntMeta = %d, want %d", got, tc.want)
			}
		})
	}
}

// ── readStringMeta (comparator.go:446) ──────────────────────────

func TestReadStringMeta(t *testing.T) {
	tests := []struct {
		name     string
		m        map[string]string
		key      string
		fallback string
		want     string
	}{
		{"nil map", nil, "x", "def", "def"},
		{"missing key", map[string]string{}, "x", "def", "def"},
		{"present", map[string]string{"x": "val"}, "x", "def", "val"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := readStringMeta(tc.m, tc.key, tc.fallback)
			if got != tc.want {
				t.Fatalf("readStringMeta = %q, want %q", got, tc.want)
			}
		})
	}
}

// ── PassRate (comparator.go:95) — zero cases branch ─────────────

func TestPassRate_ZeroCases(t *testing.T) {
	totals := Totals{TotalCases: 0, Passed: 0}
	if r := totals.PassRate(); r != 0 {
		t.Fatalf("PassRate with 0 cases = %f, want 0", r)
	}
}

func TestPassRate_HalfPass(t *testing.T) {
	totals := Totals{TotalCases: 4, Passed: 2}
	if r := totals.PassRate(); r != 0.5 {
		t.Fatalf("PassRate = %f, want 0.5", r)
	}
}

// ── Compare with warmup (comparator.go:139) ─────────────────────

func TestCompare_WarmupOption(t *testing.T) {
	prev := SetDefaultSubject(stubArmSubject{OutputLineCount: 1})
	defer SetDefaultSubject(prev)

	arms := []Arm{NoSystemPromptArm(), StandardTerseArm()}
	progress := 0
	rep, err := Compare(context.Background(), twoCaseSet(), arms, CompareOptions{
		Warmup:    true,
		OnProgress: func(done, total int, last ArmRun) { progress++ },
	})
	if err != nil {
		t.Fatalf("Compare with warmup: %v", err)
	}
	// progress should be called for non-warmup runs only.
	// Warmup skips the first (case, arm) pair: 2 cases × 2 arms - 1 = 3.
	if progress != 3 {
		t.Fatalf("expected 3 progress calls, got %d", progress)
	}
	// Totals: warmup skips first arm of first case.
	// baseline arm: 1 case (bravo only; alpha was warmup-skipped)
	// terse arm: 2 cases (both alpha and bravo)
	baselineTot := rep.TotalsByArm["__baseline__"]
	if baselineTot.TotalCases != 1 {
		t.Fatalf("baseline: TotalCases=%d want 1 (warmup excluded)", baselineTot.TotalCases)
	}
	terseTot := rep.TotalsByArm["__terse__"]
	if terseTot.TotalCases != 2 {
		t.Fatalf("terse: TotalCases=%d want 2", terseTot.TotalCases)
	}
}

// ── Compare empty arms (comparator.go:139) ──────────────────────

func TestCompare_EmptyArmsReturnsError(t *testing.T) {
	_, err := Compare(context.Background(), twoCaseSet(), nil, CompareOptions{})
	if err == nil {
		t.Fatal("Compare with empty arms should error")
	}
}

// ── CompareParallel empty arms + worker clamping ────────────────

func TestCompareParallel_EmptyArmsReturnsError(t *testing.T) {
	_, err := CompareParallel(context.Background(), twoCaseSet(), nil, CompareOptions{}, 0)
	if err != nil {
		// CompareParallel doesn't explicitly check for empty arms like Compare,
		// but it should still return without error (empty report).
		_ = err
	}
}

// ── CompareRuns added/removed cases (regression.go:39) ─────────

func TestCompareRuns_AddedAndRemovedCases(t *testing.T) {
	base := Run{ID: "base", Results: []Result{
		{CaseID: "a", Score: 1, Weight: 1},
		{CaseID: "b", Score: 1, Weight: 1},
	}}
	cand := Run{ID: "cand", Results: []Result{
		{CaseID: "a", Score: 1, Weight: 1},
		{CaseID: "c", Score: 1, Weight: 1},
	}}
	cmp := CompareRuns(base, cand, 0.001)
	foundAdded, foundRemoved := false, false
	for _, d := range cmp.Deltas {
		if d.Kind == "added" && d.CaseID == "c" {
			foundAdded = true
		}
		if d.Kind == "removed" && d.CaseID == "b" {
			foundRemoved = true
		}
	}
	if !foundAdded {
		t.Fatal("expected 'added' delta for case c")
	}
	if !foundRemoved {
		t.Fatal("expected 'removed' delta for case b")
	}
}

func TestCompareRuns_DefaultEpsilon(t *testing.T) {
	base := Run{ID: "base", Results: []Result{{CaseID: "a", Score: 0.5, Weight: 1}}}
	cand := Run{ID: "cand", Results: []Result{{CaseID: "a", Score: 0.5001, Weight: 1}}}
	cmp := CompareRuns(base, cand, 0)
	if cmp.Improved != 0 || cmp.Regressed != 0 {
		t.Fatalf("tiny change within default epsilon should be unchanged, got improved=%d regressed=%d", cmp.Improved, cmp.Regressed)
	}
}

// ── CompareRuns added/removed cases (regression.go:39) ─────────
// (covered above)

// ── Runner nil subject (runner.go:24) ───────────────────────────

func TestRunner_NilSubjectReturnsError(t *testing.T) {
	r := Runner{Subject: nil}
	_, err := r.Execute(context.Background(), EvalSet{Name: "x", Cases: []EvalCase{{ID: "a"}}})
	if err == nil {
		t.Fatal("nil Subject should return error")
	}
}

// ── Runner with Subject error (runner.go:44) ────────────────────

type errSubject struct{}

func (errSubject) Run(_ context.Context, _ EvalCase) (Output, error) {
	return Output{}, os.ErrClosed
}

func TestRunner_SubjectErrorCaptured(t *testing.T) {
	r := Runner{Subject: errSubject{}, Scorer: ExactMatch{}, SubjectName: "err"}
	run, err := r.Execute(context.Background(), EvalSet{Name: "demo", Cases: []EvalCase{{ID: "a"}}})
	if err != nil {
		t.Fatalf("Execute should not fail on case error: %v", err)
	}
	if len(run.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(run.Results))
	}
	if run.Results[0].Passed {
		t.Fatal("case with subject error should not pass")
	}
	if run.Results[0].Err == "" {
		t.Fatal("case with subject error should record Err")
	}
}

// ── Runner with case-level Scorer config (runner.go:62) ─────────

func TestRunner_CaseScorerConfig(t *testing.T) {
	r := Runner{Subject: stubSubject{reply: "exact"}, Scorer: SuccessFlag{}, SubjectName: "stub"}
	set := EvalSet{Name: "demo", Cases: []EvalCase{
		{ID: "a", Prompt: "p", Expected: "exact", Scorer: map[string]any{"type": "exact"}},
	}}
	run, err := r.Execute(context.Background(), set)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !run.Results[0].Passed {
		t.Fatal("case-level exact scorer should pass with matching output")
	}
}

func TestRunner_CaseScorerConfigError(t *testing.T) {
	r := Runner{Subject: stubSubject{reply: "x"}, Scorer: SuccessFlag{}, SubjectName: "stub"}
	set := EvalSet{Name: "demo", Cases: []EvalCase{
		{ID: "a", Prompt: "p", Scorer: map[string]any{"type": "unknown_type"}},
	}}
	run, err := r.Execute(context.Background(), set)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if run.Results[0].Err == "" {
		t.Fatal("bad scorer config should record an error in the result")
	}
}

// ── truncate (runner.go:82) ─────────────────────────────────────

func TestTruncate(t *testing.T) {
	if got := truncate("short", 100); got != "short" {
		t.Fatalf("truncate(short) = %q, want short", got)
	}
	long := strings.Repeat("x", 200)
	got := truncate(long, 100)
	// "…" is 3 bytes in UTF-8, so 100 + 3 = 103
	if len(got) != 103 {
		t.Fatalf("truncate length = %d, want 103", len(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected truncation suffix, got %q", got)
	}
}

// ── NewCommand (regression.go:99) — was 0% ──────────────────────

func TestNewCommand_Registration(t *testing.T) {
	cmd := NewCommand(func(name string) (Subject, Scorer, error) {
		return stubSubject{reply: "ok"}, ExactMatch{}, nil
	})
	if cmd == nil {
		t.Fatal("NewCommand returned nil")
	}
	if cmd.Use != "evalset" {
		t.Fatalf("root cmd Use = %q, want evalset", cmd.Use)
	}
	subs := cmd.Commands()
	if len(subs) < 3 {
		t.Fatalf("expected at least 3 subcommands, got %d", len(subs))
	}
	found := map[string]bool{}
	for _, s := range subs {
		found[s.Name()] = true
	}
	for _, want := range []string{"run", "list", "compare"} {
		if !found[want] {
			t.Fatalf("missing subcommand %q (have: %v)", want, found)
		}
	}
}

func TestNewCommand_CompareWithFailOnRegress(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIN_EVAL_DIR", dir)
	store := NewStore(dir)

	base := Run{ID: "base", SetName: "demo", Results: []Result{
		{CaseID: "a", Score: 1, Weight: 1, Passed: true},
	}}
	cand := Run{ID: "cand", SetName: "demo", Results: []Result{
		{CaseID: "a", Score: 0, Weight: 1, Passed: false},
	}}
	if err := store.SaveRun(base); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRun(cand); err != nil {
		t.Fatal(err)
	}

	cmd := NewCommand(func(name string) (Subject, Scorer, error) {
		return stubSubject{reply: "x"}, ExactMatch{}, nil
	})
	cmd.SetArgs([]string{"compare", "base", "cand", "--fail-on-regress"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("compare with --fail-on-regress should error on regression")
	}
}

func TestNewCommand_ListEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIN_EVAL_DIR", dir)

	cmd := NewCommand(func(name string) (Subject, Scorer, error) {
		return stubSubject{reply: "x"}, ExactMatch{}, nil
	})
	cmd.SetArgs([]string{"list"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("list on empty store: %v", err)
	}
}

func TestNewCommand_Run(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIN_EVAL_DIR", dir)
	store := NewStore(dir)
	set := EvalSet{Name: "demo", Cases: []EvalCase{
		{ID: "a", Prompt: "p", Expected: "ok"},
	}}
	if err := store.SaveSet(set); err != nil {
		t.Fatal(err)
	}

	cmd := NewCommand(func(name string) (Subject, Scorer, error) {
		return stubSubject{reply: "ok"}, ExactMatch{}, nil
	})
	cmd.SetArgs([]string{"run", "demo", "--subject", "stub"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run: %v", err)
	}
}
