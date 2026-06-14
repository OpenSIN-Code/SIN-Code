// SPDX-License-Identifier: MIT
// Purpose: tests for the autopilot package — program parsing, metric decisions,
// budget caps, journal round-trips, and a full OBSERVE->...->LEARN cycle driven
// by fakes (no real LLM, no real git beyond a temp repo).
package autopilot

import (
	"context"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

func TestLoadProgram(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "program.md")
	content := `# Objective
Reduce parser latency.

## Metric
name: bench_ns
direction: minimize
extract: /bench_ns=([0-9.]+)/

## Budget
minutes: 90
max_experiments: 20

## Invariants (DO NOT MODIFY)
- Public API stays stable
- Tests keep passing
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := LoadProgram(path)
	if err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}
	if p.MetricName != "bench_ns" {
		t.Errorf("MetricName = %q, want bench_ns", p.MetricName)
	}
	if p.Direction != Minimize {
		t.Errorf("Direction = %q, want minimize", p.Direction)
	}
	if p.BudgetMinutes != 90 || p.MaxExperiments != 20 {
		t.Errorf("budget = %d/%d, want 90/20", p.BudgetMinutes, p.MaxExperiments)
	}
	if len(p.Invariants) != 2 {
		t.Errorf("invariants = %d, want 2", len(p.Invariants))
	}
	if p.ExtractRegex == nil || !p.ExtractRegex.MatchString("bench_ns=123.4") {
		t.Error("extract regex did not compile/match")
	}
}

func TestLoadProgramRequiresObjective(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "program.md")
	_ = os.WriteFile(path, []byte("## Metric\nname: x\n"), 0o644)
	if _, err := LoadProgram(path); err == nil {
		t.Fatal("expected error for missing objective")
	}
}

func TestExtractMetric(t *testing.T) {
	re := regexp.MustCompile("bench_ns=([0-9.]+)")
	m := ExtractMetric(re, "running... bench_ns=42.5 done")
	if !m.Found || m.Value != 42.5 {
		t.Fatalf("got %+v, want 42.5", m)
	}
	if got := ExtractMetric(re, "no match here"); got.Found {
		t.Error("expected no match")
	}
	if got := ExtractMetric(nil, "anything"); got.Found {
		t.Error("nil regex must yield not-found")
	}
}

func TestImproved(t *testing.T) {
	if !Improved(Minimize, NoMetric(), 100) {
		t.Error("any value should beat unset best")
	}
	if !Improved(Minimize, 100, 90) {
		t.Error("90 < 100 should improve under minimize")
	}
	if Improved(Minimize, 100, 110) {
		t.Error("110 should not improve under minimize")
	}
	if !Improved(Maximize, 100, 110) {
		t.Error("110 > 100 should improve under maximize")
	}
}

func TestBudgetCaps(t *testing.T) {
	b := NewBudget(60, 3)
	for i := 0; i < 3; i++ {
		if !b.Consume() {
			t.Fatalf("consume %d should succeed", i)
		}
	}
	if b.Consume() {
		t.Error("4th consume should fail (cap=3)")
	}
	if b.StopReason() == "" {
		t.Error("StopReason should be set after cap")
	}
}

func TestJournalRoundTrip(t *testing.T) {
	dir := t.TempDir()
	j, err := OpenJournal(filepath.Join(dir, "j.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	ctx := context.Background()
	_, _ = j.Record(ctx, Experiment{Objective: "o", Proposal: "p1", Outcome: OutcomeKept, MetricAfter: 50, MetricFound: true})
	_, _ = j.Record(ctx, Experiment{Objective: "o", Proposal: "p2", Outcome: OutcomeKept, MetricAfter: 30, MetricFound: true})
	_, _ = j.Record(ctx, Experiment{Objective: "o", Proposal: "p3", Outcome: OutcomeReverted, MetricAfter: 80, MetricFound: true})

	if best := j.BestKept(ctx, Minimize); best != 30 {
		t.Errorf("BestKept = %v, want 30", best)
	}
	kept, _ := j.Count(ctx, OutcomeKept)
	if kept != 2 {
		t.Errorf("kept = %d, want 2", kept)
	}
	recent, _ := j.Recent(ctx, 10)
	if len(recent) != 3 {
		t.Errorf("recent = %d, want 3", len(recent))
	}
}

func TestProposerFallback(t *testing.T) {
	p := &Proposer{Program: &Program{Objective: "speed up parser", Direction: Minimize}}
	goal, err := p.Next(context.Background(), nil, nil)
	if err != nil || goal == "" {
		t.Fatalf("fallback proposal failed: %v / %q", err, goal)
	}
}

func TestAutopilotFullCycle(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	prog := &Program{
		Objective: "lower the metric", Direction: Minimize,
		MetricName: "m", BudgetMinutes: 60, MaxExperiments: 3,
	}
	prog.ExtractRegex = regexp.MustCompile("m=([0-9.]+)")

	j, _ := OpenJournal(filepath.Join(dir, "j.db"))
	defer j.Close()

	// Fake run: improves the first time, regresses the second.
	values := []float64{50, 999}
	call := 0
	run := func(ctx context.Context, goal string) (LoopResult, string, error) {
		v := values[call%len(values)]
		call++
		// write a file so git has something to keep
		_ = os.WriteFile(filepath.Join(dir, "out.txt"), []byte(goal), 0o644)
		return LoopResult{SessionID: "s", Verified: true, Turns: 1}, "m=" + ftoa(v), nil
	}

	ap := New(Config{
		Workspace: dir, Program: prog, Proposer: &Proposer{Program: prog},
		Journal: j, Budget: NewBudget(60, 3), Snap: NewSnapshotter(dir),
		RunGoal: run, Out: os.Stderr,
	})
	kept, best, err := ap.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if kept < 1 {
		t.Errorf("expected at least 1 kept, got %d", kept)
	}
	if math.IsNaN(best) || best != 50 {
		t.Errorf("best = %v, want 50", best)
	}
}

// ── test helpers ─────────────────────────────────────────────────────────────

func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// initGitRepo creates a minimal committed git repo in dir so the snapshotter
// has a baseline to keep/revert against.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "test@test.local")
	run("config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(dir, "seed.txt"), []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "seed")
}
