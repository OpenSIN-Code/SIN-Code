// SPDX-License-Identifier: MIT
// Purpose: additional coverage tests for autopilot package to reach 100% statement coverage.
package autopilot

import (
	"context"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ── autopilot.go ────────────────────────────────────────────────────────────

func TestAutopilot_NotAGitRepo(t *testing.T) {
	dir := t.TempDir()
	ap := New(Config{
		Workspace: dir,
		Program:   &Program{Objective: "x"},
		Snap:      NewSnapshotter(dir),
		Budget:    NewBudget(60, 1),
		Journal:   mustOpenJournal(t, dir),
	})
	_, _, err := ap.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not a git repo") {
		t.Fatalf("expected not-a-git-repo error, got %v", err)
	}
}

func TestAutopilot_BudgetExhaustedBeforeStart(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	ap := New(Config{
		Workspace: dir,
		Program:   &Program{Objective: "x"},
		Snap:      NewSnapshotter(dir),
		Budget:    NewBudget(-1, 0),
		Journal:   mustOpenJournal(t, dir),
	})
	kept, _, err := ap.Run(context.Background())
	if err != nil || kept != 0 {
		t.Fatalf("expected early exit with 0 kept, got %d / %v", kept, err)
	}
}

func TestAutopilot_BaselineError(t *testing.T) {
	dir := t.TempDir()
	// Unborn repo: git init, but no commit.
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	ap := New(Config{
		Workspace: dir,
		Program:   &Program{Objective: "x"},
		Snap:      NewSnapshotter(dir),
		Budget:    NewBudget(60, 1),
		Journal:   mustOpenJournal(t, dir),
		Proposer:  &Proposer{Program: &Program{Objective: "x"}},
		RunGoal:   func(context.Context, string) (LoopResult, string, error) { return LoopResult{}, "", nil },
	})
	_, _, err := ap.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "baseline") {
		t.Fatalf("expected baseline error, got %v", err)
	}
}

func TestAutopilot_RunGoalError(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	var recorded []string
	prog := &Program{Objective: "x"}
	ap := New(Config{
		Workspace: dir,
		Program:   prog,
		Snap:      NewSnapshotter(dir),
		Budget:    NewBudget(60, 1),
		Journal:   mustOpenJournal(t, dir),
		Proposer:  &Proposer{Program: prog},
		RunGoal:   func(context.Context, string) (LoopResult, string, error) { return LoopResult{}, "", errors.New("boom") },
		Record:    func(_ context.Context, _, lesson string) { recorded = append(recorded, lesson) },
		Out:       os.Stderr,
	})
	kept, _, err := ap.Run(context.Background())
	if err != nil || kept != 0 {
		t.Fatalf("expected 0 kept, got %d / %v", kept, err)
	}
	if len(recorded) == 0 {
		t.Fatal("expected record callback on failure")
	}
}

func TestAutopilot_VerifiedFalse(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	prog := &Program{Objective: "x"}
	ap := New(Config{
		Workspace: dir,
		Program:   prog,
		Snap:      NewSnapshotter(dir),
		Budget:    NewBudget(60, 1),
		Journal:   mustOpenJournal(t, dir),
		Proposer:  &Proposer{Program: prog},
		RunGoal:   func(context.Context, string) (LoopResult, string, error) { return LoopResult{Verified: false}, "", nil },
	})
	kept, _, err := ap.Run(context.Background())
	if err != nil || kept != 0 {
		t.Fatalf("expected 0 kept, got %d / %v", kept, err)
	}
}

func TestAutopilot_MetricNotFound(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	prog := &Program{Objective: "x", MetricName: "m", Direction: Minimize, ExtractRegex: regexp.MustCompile("m=([0-9.]+)")}
	ap := New(Config{
		Workspace: dir, Program: prog,
		Snap: NewSnapshotter(dir), Budget: NewBudget(60, 1),
		Journal:  mustOpenJournal(t, dir),
		Proposer: &Proposer{Program: prog},
		RunGoal: func(context.Context, string) (LoopResult, string, error) {
			return LoopResult{Verified: true}, "no metric here", nil
		},
	})
	kept, _, err := ap.Run(context.Background())
	if err != nil || kept != 1 {
		t.Fatalf("expected 1 kept, got %d / %v", kept, err)
	}
}

func TestAutopilot_MetricRegression(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	prog := &Program{Objective: "x", MetricName: "m", Direction: Minimize, ExtractRegex: regexp.MustCompile("m=([0-9.]+)")}
	_ = os.WriteFile(filepath.Join(dir, "out.txt"), []byte("seed"), 0o644)
	j := mustOpenJournal(t, dir)
	_, _ = j.Record(context.Background(), Experiment{Outcome: OutcomeKept, MetricAfter: 50, MetricFound: true})
	var recorded []string
	ap := New(Config{
		Workspace: dir, Program: prog,
		Snap: NewSnapshotter(dir), Budget: NewBudget(60, 1),
		Journal:  j,
		Proposer: &Proposer{Program: prog},
		RunGoal: func(context.Context, string) (LoopResult, string, error) {
			_ = os.WriteFile(filepath.Join(dir, "out.txt"), []byte("worse"), 0o644)
			return LoopResult{Verified: true}, "m=100", nil
		},
		Record: func(_ context.Context, _, lesson string) { recorded = append(recorded, lesson) },
	})
	kept, _, err := ap.Run(context.Background())
	if err != nil || kept != 0 {
		t.Fatalf("expected 0 kept, got %d / %v", kept, err)
	}
	if len(recorded) == 0 {
		t.Fatal("expected record callback on regression")
	}
}

func TestAutopilot_ConsumeFails(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	prog := &Program{Objective: "x"}
	testBudgetForceConsumeFail = true
	defer func() { testBudgetForceConsumeFail = false }()
	ap := New(Config{
		Workspace: dir, Program: prog,
		Snap: NewSnapshotter(dir), Budget: NewBudget(60, 1),
		Journal:  mustOpenJournal(t, dir),
		Proposer: &Proposer{Program: prog},
	})
	kept, _, err := ap.Run(context.Background())
	if err != nil || kept != 0 {
		t.Fatalf("expected 0 kept, got %d / %v", kept, err)
	}
}

func TestAutopilot_WithLessonsAndInvariants(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	prog := &Program{Objective: "x", MetricName: "m", Direction: Minimize, Invariants: []string{"API"}, ExtractRegex: regexp.MustCompile("m=([0-9.]+)")}
	ap := New(Config{
		Workspace: dir, Program: prog,
		Snap: NewSnapshotter(dir), Budget: NewBudget(60, 1),
		Journal:  mustOpenJournal(t, dir),
		Proposer: &Proposer{Program: prog},
		Lessons:  func(context.Context, string, int) []string { return []string{"do not break API"} },
		RunGoal: func(_ context.Context, goal string) (LoopResult, string, error) {
			if !strings.Contains(goal, "HARD INVARIANTS") {
				t.Fatal("expected invariants briefing in goal")
			}
			return LoopResult{Verified: true}, "m=40", nil
		},
	})
	kept, best, err := ap.Run(context.Background())
	if err != nil || kept != 1 || best != 40 {
		t.Fatalf("expected 1 kept best=40, got %d / %v / %v", kept, best, err)
	}
}

func TestShortCommit(t *testing.T) {
	if got := short("abc"); got != "abc" {
		t.Fatalf("short commit should be unchanged, got %q", got)
	}
}

// ── budget.go ───────────────────────────────────────────────────────────────

func TestBudget_StopReasonExperimentCap(t *testing.T) {
	b := NewBudget(60, 1)
	_ = b.Consume()
	if r := b.StopReason(); r == "" || !strings.Contains(r, "experiment cap") {
		t.Fatalf("expected experiment cap reason, got %q", r)
	}
	if b.CanContinue() {
		t.Fatal("CanContinue should be false")
	}
}

func TestBudget_RemainingNegative(t *testing.T) {
	b := NewBudget(-1, -5)
	d, left := b.Remaining()
	if d != 0 || left != 0 {
		t.Fatalf("expected 0/0, got %v/%d", d, left)
	}
}

func TestBudget_ConsumeFailure(t *testing.T) {
	testBudgetForceConsumeFail = true
	defer func() { testBudgetForceConsumeFail = false }()
	b := NewBudget(60, 1)
	if b.StopReason() != "" {
		t.Fatal("expected StopReason empty with hook")
	}
	if b.Consume() {
		t.Fatal("expected Consume to fail with hook")
	}
}

// ── journal.go ──────────────────────────────────────────────────────────────

func TestOpenJournal_DBError(t *testing.T) {
	testJournalDBErr = errJournalOpen
	defer func() { testJournalDBErr = nil }()
	if _, err := OpenJournal(filepath.Join(t.TempDir(), "j.db")); err != errJournalOpen {
		t.Fatalf("expected injected db error, got %v", err)
	}
}

func TestOpenJournal_ExecError(t *testing.T) {
	testJournalExecErr = errJournalOpen
	defer func() { testJournalExecErr = nil }()
	if _, err := OpenJournal(filepath.Join(t.TempDir(), "j.db")); err != errJournalOpen {
		t.Fatalf("expected injected exec error, got %v", err)
	}
}

func TestJournal_RecentDefaultLimit(t *testing.T) {
	j := mustOpenJournal(t, t.TempDir())
	ctx := context.Background()
	_, _ = j.Record(ctx, Experiment{Objective: "o", Proposal: "p", Outcome: OutcomeKept})
	recent, err := j.Recent(ctx, 0)
	if err != nil || len(recent) != 1 {
		t.Fatalf("expected 1 recent with default limit, got %d / %v", len(recent), err)
	}
}

func TestJournal_RecentError(t *testing.T) {
	j := mustOpenJournal(t, t.TempDir())
	_ = j.Close()
	if _, err := j.Recent(context.Background(), 10); err == nil {
		t.Fatal("expected recent error after close")
	}
}

func TestJournal_BestKeptError(t *testing.T) {
	j := mustOpenJournal(t, t.TempDir())
	_ = j.Close()
	if got := j.BestKept(context.Background(), Minimize); !math.IsNaN(got) {
		t.Fatalf("expected NaN after close, got %v", got)
	}
}

func TestJournal_BestKeptMaximize(t *testing.T) {
	j := mustOpenJournal(t, t.TempDir())
	ctx := context.Background()
	_, _ = j.Record(ctx, Experiment{Outcome: OutcomeKept, MetricAfter: 10, MetricFound: true})
	_, _ = j.Record(ctx, Experiment{Outcome: OutcomeKept, MetricAfter: 30, MetricFound: true})
	if got := j.BestKept(ctx, Maximize); got != 30 {
		t.Fatalf("expected best 30 for maximize, got %v", got)
	}
}

func TestDefaultJournalPath(t *testing.T) {
	dir := t.TempDir()
	p := DefaultJournalPath(dir)
	if !strings.HasSuffix(p, ".sin-code/autopilot.db") {
		t.Fatalf("unexpected path %q", p)
	}
}

// ── metric.go ───────────────────────────────────────────────────────────────

func TestExtractMetric_ParseError(t *testing.T) {
	re := regexp.MustCompile("m=(\\S+)")
	m := ExtractMetric(re, "m=abc")
	if m.Found {
		t.Fatal("expected parse failure")
	}
	if m.Raw != "abc" {
		t.Fatalf("expected raw abc, got %q", m.Raw)
	}
}

func TestBetterOf_AllBranches(t *testing.T) {
	if BetterOf(Minimize, math.NaN(), 10) != 10 {
		t.Fatal("NaN a should return b")
	}
	if BetterOf(Minimize, 10, math.NaN()) != 10 {
		t.Fatal("NaN b should return a")
	}
	if BetterOf(Maximize, 5, 10) != 10 {
		t.Fatal("maximize should pick larger")
	}
	if BetterOf(Minimize, 5, 10) != 5 {
		t.Fatal("minimize should pick smaller")
	}
}

// ── program.go ──────────────────────────────────────────────────────────────

func TestLoadProgram_ReadError(t *testing.T) {
	if _, err := LoadProgram("/nonexistent/program.md"); err == nil {
		t.Fatal("expected read error")
	}
}

func TestLoadProgram_ScannerError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "program.md")
	// Create a line longer than bufio.Scanner's default 64KB token limit.
	_ = os.WriteFile(path, []byte("# Objective\n"+strings.Repeat("x", 70*1024)+"\n"), 0o644)
	if _, err := LoadProgram(path); err == nil {
		t.Fatal("expected scanner error")
	}
}

func TestLoadProgram_Maximize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "program.md")
	_ = os.WriteFile(path, []byte("# Objective\nO\n## Metric\ndirection: maximize\n"), 0o644)
	p, err := LoadProgram(path)
	if err != nil || p.Direction != Maximize {
		t.Fatalf("expected maximize, got %v / %v", p.Direction, err)
	}
}

func TestLoadProgram_InvalidBudget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "program.md")
	_ = os.WriteFile(path, []byte("# Objective\nO\n## Budget\nminutes: not-a-number\n"), 0o644)
	p, err := LoadProgram(path)
	if err != nil || p.BudgetMinutes != 60 {
		t.Fatalf("expected default budget on invalid line, got %+v / %v", p, err)
	}
}

func TestBulletOf(t *testing.T) {
	if bulletOf("plain text") != "" {
		t.Fatal("expected empty bullet for plain text")
	}
}

func TestLoadProgram_MissingObjective(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "program.md")
	_ = os.WriteFile(path, []byte("## Metric\nname: x\n"), 0o644)
	if _, err := LoadProgram(path); err == nil || !strings.Contains(err.Error(), "no # Objective") {
		t.Fatalf("expected missing objective error, got %v", err)
	}
}

// ── proposer.go ──────────────────────────────────────────────────────────────

func TestProposer_LLMReturnsGoal(t *testing.T) {
	p := &Proposer{Program: &Program{Objective: "speed up"}, Propose: func(context.Context, string) (string, error) { return "  do X  ", nil }}
	goal, err := p.Next(context.Background(), nil, nil)
	if err != nil || goal != "do X" {
		t.Fatalf("expected trimmed goal, got %q / %v", goal, err)
	}
}

func TestProposer_LLMError(t *testing.T) {
	p := &Proposer{Program: &Program{Objective: "speed up"}, Propose: func(context.Context, string) (string, error) { return "", errors.New("llm down") }}
	goal, err := p.Next(context.Background(), nil, nil)
	if err != nil || goal == "" {
		t.Fatalf("expected fallback on LLM error, got %q / %v", goal, err)
	}
}

func TestProposer_EmptyGoalFallback(t *testing.T) {
	p := &Proposer{Program: &Program{Objective: "speed up"}, Propose: func(context.Context, string) (string, error) { return "   ", nil }}
	goal, err := p.Next(context.Background(), nil, nil)
	if err != nil || goal == "" {
		t.Fatalf("expected fallback on empty goal, got %q / %v", goal, err)
	}
}

func TestProposer_BuildPrompt_WithAll(t *testing.T) {
	prog := &Program{
		Objective:  "optimize",
		MetricName: "m",
		Direction:  Minimize,
		Invariants: []string{"API"},
	}
	p := &Proposer{Program: prog}
	recent := []Experiment{
		{Outcome: OutcomeKept, Proposal: "p1", MetricFound: true, MetricAfter: 10},
		{Outcome: OutcomeReverted, Proposal: "p2", MetricFound: false},
	}
	lessons := []string{"lesson1"}
	prompt := p.buildPrompt(recent, lessons)
	for _, want := range []string{"# OBJECTIVE", "# METRIC", "HARD INVARIANTS", "# RECENT EXPERIMENTS", "p1", "p2", "# LESSONS", "lesson1"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
}

func TestProposer_FallbackCases(t *testing.T) {
	p := &Proposer{Program: &Program{Objective: "O"}}
	for n := 1; n <= 3; n++ {
		recent := make([]Experiment, n)
		goal, err := p.Next(context.Background(), recent, nil)
		if err != nil || goal == "" {
			t.Fatalf("len=%d: unexpected failure %v / %q", n, err, goal)
		}
	}
}

func TestProposer_RecentAndLessonLimits(t *testing.T) {
	p := &Proposer{Program: &Program{Objective: "O", MetricName: "m", Direction: Minimize}}
	recent := make([]Experiment, 12)
	for i := range recent {
		recent[i] = Experiment{Outcome: OutcomeKept, Proposal: "p", MetricFound: true, MetricAfter: float64(i)}
	}
	lessons := make([]string, 15)
	for i := range lessons {
		lessons[i] = "lesson"
	}
	prompt := p.buildPrompt(recent, lessons)
	if strings.Count(prompt, "- [kept]") > 8 {
		t.Fatal("expected at most 8 recent experiments in prompt")
	}
	if strings.Count(prompt, "- lesson") > 10 {
		t.Fatal("expected at most 10 lessons in prompt")
	}
}

// ── snapshot.go ──────────────────────────────────────────────────────────────

func TestSnapshotter_GitError(t *testing.T) {
	s := NewSnapshotter(t.TempDir())
	if _, err := s.Baseline(context.Background()); err == nil {
		t.Fatal("expected git error in non-git dir")
	}
}

func TestSnapshotter_KeepNoChanges(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	s := NewSnapshotter(dir)
	base, _ := s.Baseline(context.Background())
	commit, err := s.Keep(context.Background(), "no changes")
	if err != nil || commit != base {
		t.Fatalf("expected baseline commit on clean keep, got %q / %v", commit, err)
	}
}

func TestSnapshotter_KeepAddError(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	s := NewSnapshotter(dir)
	testGitHook = func(ctx context.Context, snap *Snapshotter, args []string) (string, error) {
		if len(args) > 0 && args[0] == "add" {
			return "", errors.New("add fail")
		}
		return realGit(t, snap.Workspace, args...), nil
	}
	defer func() { testGitHook = nil }()
	if _, err := s.Keep(context.Background(), "msg"); err == nil || !strings.Contains(err.Error(), "add fail") {
		t.Fatalf("expected add error, got %v", err)
	}
}

func TestSnapshotter_KeepStatusError(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	s := NewSnapshotter(dir)
	testGitHook = func(ctx context.Context, snap *Snapshotter, args []string) (string, error) {
		if len(args) > 0 && args[0] == "status" {
			return "", errors.New("status fail")
		}
		return realGit(t, snap.Workspace, args...), nil
	}
	defer func() { testGitHook = nil }()
	if _, err := s.Keep(context.Background(), "msg"); err == nil || !strings.Contains(err.Error(), "status fail") {
		t.Fatalf("expected status error, got %v", err)
	}
}

func TestSnapshotter_KeepCommitError(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	s := NewSnapshotter(dir)
	_ = os.WriteFile(filepath.Join(dir, "change.txt"), []byte("x"), 0o644)
	testGitHook = func(ctx context.Context, snap *Snapshotter, args []string) (string, error) {
		if sliceContains(args, "commit") {
			return "", errors.New("commit fail")
		}
		return realGit(t, snap.Workspace, args...), nil
	}
	defer func() { testGitHook = nil }()
	if _, err := s.Keep(context.Background(), "msg"); err == nil || !strings.Contains(err.Error(), "commit fail") {
		t.Fatalf("expected commit error, got %v", err)
	}
}

func TestSnapshotter_RevertError(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	s := NewSnapshotter(dir)
	testGitHook = func(ctx context.Context, snap *Snapshotter, args []string) (string, error) {
		if len(args) > 0 && args[0] == "reset" {
			return "", errors.New("reset fail")
		}
		return realGit(t, snap.Workspace, args...), nil
	}
	defer func() { testGitHook = nil }()
	if err := s.Revert(context.Background(), "HEAD"); err == nil || !strings.Contains(err.Error(), "reset fail") {
		t.Fatalf("expected reset error, got %v", err)
	}
}

// ── helpers ─────────────────────────────────────────────────────────────────

func realGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func sliceContains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func mustOpenJournal(t *testing.T, dir string) *Journal {
	t.Helper()
	j, err := OpenJournal(filepath.Join(dir, "autopilot.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	return j
}
