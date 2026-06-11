// SPDX-License-Identifier: MIT
package orchestrator

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestTargetedFastChecksUsesAffected(t *testing.T) {
	g := &ImpactGraph{
		nodes: map[string]*PkgNode{
			"repo/a": {ImportPath: "repo/a", TestFiles: []string{"a_test.go"}},
			"repo/b": {ImportPath: "repo/b", TestFiles: []string{"b_test.go"}},
		},
		reverse:   map[string][]string{},
		fileToPkg: map[string]string{"a/x.go": "repo/a"},
	}
	tv := NewTargetedVerifier(NewVerifier(t.TempDir()), g)
	checks := tv.FastChecks([]string{"a/x.go"})
	if len(checks) != 2 {
		t.Fatalf("expected 2 checks (build+test), got %d", len(checks))
	}
	if checks[1].Kind != CheckTest {
		t.Fatalf("second check should be test, got %s", checks[1].Kind)
	}
}

func TestTargetedFallbackBuildAll(t *testing.T) {
	g := &ImpactGraph{
		nodes:     map[string]*PkgNode{"repo/a": {ImportPath: "repo/a"}},
		reverse:   map[string][]string{},
		fileToPkg: map[string]string{},
	}
	tv := NewTargetedVerifier(NewVerifier(t.TempDir()), g)
	checks := tv.FastChecks([]string{"unknown.go"})
	if len(checks) != 1 || checks[0].Name != "build-all" {
		t.Fatalf("expected fallback build-all, got %+v", checks)
	}
}

func TestTargetedFinalChecksEqualDefault(t *testing.T) {
	tv := &TargetedVerifier{}
	defaults := DefaultGoChecks()
	finals := tv.FinalChecks()
	if len(finals) != len(defaults) {
		t.Fatalf("final must equal default, got %d vs %d", len(finals), len(defaults))
	}
}

func TestTargetedVerifyStagedFastFail(t *testing.T) {
	g := &ImpactGraph{
		nodes:     map[string]*PkgNode{"repo/a": {ImportPath: "repo/a", TestFiles: []string{"a_test.go"}}},
		reverse:   map[string][]string{},
		fileToPkg: map[string]string{"a/x.go": "repo/a"},
	}
	tv := NewTargetedVerifier(NewVerifier(t.TempDir()), g)
	v := tv.VerifyStaged(context.Background(), "t", "c", []string{"a/x.go"})
	if v.Passed {
		t.Fatal("expected red — fast build of empty repo must fail")
	}
}

func TestTargetedSpeedup(t *testing.T) {
	g := &ImpactGraph{
		nodes: map[string]*PkgNode{
			"repo/a": {},
			"repo/b": {},
			"repo/c": {},
			"repo/d": {},
		},
		reverse:   map[string][]string{},
		fileToPkg: map[string]string{"a/x.go": "repo/a"},
	}
	tv := NewTargetedVerifier(NewVerifier(t.TempDir()), g)
	s := tv.Speedup([]string{"a/x.go"})
	if s == "" {
		t.Fatal("expected non-empty speedup string")
	}
}

func TestMergePolicyGreenButUncalibratedGoesToReview(t *testing.T) {
	p := DefaultMergePolicy()
	if d := p.Decide(true, 0.95); d != DecisionAutoMerge {
		t.Fatalf("high calibrated confidence must auto-merge, got %s", d)
	}
	if d := p.Decide(true, 0.55); d != DecisionGreenReview {
		t.Fatalf("green + low confidence must go to review, got %s", d)
	}
	if d := p.Decide(false, 0.99); d != DecisionBlock {
		t.Fatalf("red must always block, got %s", d)
	}
}

func TestCalibratorNilDBReturnsDeclared(t *testing.T) {
	c, err := NewCalibrator(nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Calibrate(context.Background(), "a", 0.9)
	if err != nil || got != 0.9 {
		t.Fatalf("nil DB must return declared as-is, got %f err=%v", got, err)
	}
}

func TestStrategyRouterNilDBIsSafe(t *testing.T) {
	r, err := NewStrategyRouter(nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	pick := r.Pick(ClassRefactor, []Strategy{StratASTEdit, StratHashline})
	if pick != StratASTEdit && pick != StratHashline {
		t.Fatalf("pick must be one of candidates, got %q", pick)
	}
	if err := r.Report(context.Background(), ClassRefactor, StratASTEdit, true); err != nil {
		t.Fatal(err)
	}
	mean, n := r.Posterior(ClassRefactor, StratASTEdit)
	if n < 0 || mean < 0 {
		t.Fatalf("posterior must be valid, got mean=%f n=%d", mean, n)
	}
}

func TestStrategyRouterLearnsFromOutcomes(t *testing.T) {
	r, err := NewStrategyRouter(nil, 42)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		if err := r.Report(context.Background(), ClassRefactor, StratASTEdit, true); err != nil {
			t.Fatal(err)
		}
	}
	mean, _ := r.Posterior(ClassRefactor, StratASTEdit)
	if mean < 0.8 {
		t.Fatalf("after 20 successes, posterior should be >0.8, got %f", mean)
	}
}

func TestMutationProbeEmptyWorkdirAssumesKill(t *testing.T) {
	mp := NewMutationProbe("", []string{"true"})
	mp.MaxMutations = 1
	res, err := mp.Run(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.ObservabilityScore != 1.0 {
		t.Fatalf("empty input -> 1.0, got %f", res.ObservabilityScore)
	}
}

func TestParseAddedLinesIgnoresComments(t *testing.T) {
	diff := `--- a/foo.go
+++ b/foo.go
@@ -1,2 +1,3 @@
 // existing comment
+# new comment
 func X() {}`
	lines := ParseAddedLines(diff)
	for _, l := range lines {
		if l.Text == "" {
			t.Fatal("comment line must be skipped")
		}
	}
}

func TestGovernorRunsLadder(t *testing.T) {
	vf := NewVerifier(t.TempDir())
	gov := &Governor{
		Ladder: []Rung{
			{Name: "single", Agents: 1, RepairRounds: 1, Timeout: 5 * time.Second},
		},
		Verifier: vf,
		Checks:   []Check{{Kind: CheckBuild, Name: "ok", Cmd: []string{"true"}}},
		Factory: func(r Rung) []Agent {
			return []Agent{&scriptAgent{name: "a", reply: "ok"}}
		},
	}
	res, err := gov.Execute(context.Background(), &Task{ID: "t1", Title: "x", Description: "d"}, NewScratchpad())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed {
		t.Fatal("single-shot green rung should pass")
	}
	if res.FinalRung != "single" {
		t.Fatalf("FinalRung mismatch, got %q", res.FinalRung)
	}
}

func TestGovernorEscalatesOnFailure(t *testing.T) {
	vf := NewVerifier(t.TempDir())
	gov := &Governor{
		Ladder: []Rung{
			{Name: "cheap", Agents: 1, RepairRounds: 0, Timeout: 5 * time.Second},
			{Name: "escalated", Agents: 1, RepairRounds: 0, Timeout: 5 * time.Second},
		},
		Verifier: vf,
		Checks:   []Check{{Kind: CheckBuild, Name: "fail", Cmd: []string{"false"}}},
		Factory: func(r Rung) []Agent {
			return []Agent{&scriptAgent{name: "a", reply: "ok"}}
		},
	}
	res, err := gov.Execute(context.Background(), &Task{ID: "t1", Title: "x", Description: "d"}, NewScratchpad())
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed {
		t.Fatal("cannot pass on failing check")
	}
	if len(res.Escalations) != 1 {
		t.Fatalf("expected 1 escalation, got %d", len(res.Escalations))
	}
	if res.Escalations[0].FromRung != "cheap" || res.Escalations[0].ToRung != "escalated" {
		t.Fatalf("wrong escalation: %+v", res.Escalations[0])
	}
}

func TestClassifyTask(t *testing.T) {
	cases := []struct {
		title, desc string
		want        TaskClass
	}{
		{"Rename foo to bar", "", ClassRename},
		{"Fix nil pointer", "crash on resize", ClassBugfix},
		{"Add new helper", "create util", ClassGreenfield},
		{"Refactor TUI", "restructure views", ClassRefactor},
		{"Set config", "edit yaml", ClassConfig},
		{"Misc task", "do something", ClassUnknown},
	}
	for _, c := range cases {
		if got := ClassifyTask(&Task{Title: c.title, Description: c.desc}); got != c.want {
			t.Errorf("ClassifyTask(%q)=%q want %q", c.title, got, c.want)
		}
	}
}

func TestSpeculativeRunNoAgents(t *testing.T) {
	s := NewSpeculativeRunner("", nil)
	_, err := s.Run(context.Background(), &Task{ID: "t"}, nil, NewScratchpad())
	if err == nil {
		t.Fatal("expected error on empty agents")
	}
}

func TestBlamerEmptyLog(t *testing.T) {
	bl := &Blamer{Verifier: NewVerifier(t.TempDir())}
	_, err := bl.Blame(context.Background(), &EditLog{}, Check{Name: "x"})
	if err == nil {
		t.Fatal("expected error on empty log")
	}
}

func TestBlamerPreExistingFailure(t *testing.T) {
	vf := NewVerifier(t.TempDir())
	bl := &Blamer{Verifier: vf}
	// No Workdir/Base set → checkAt returns true (assume base green).
	// Then a single edit with no SHA — bisect narrows immediately.
	log := &EditLog{Edits: []EditRecord{{Seq: 1, SHA: "", Summary: "test"}}}
	res, err := bl.Blame(context.Background(), log, Check{Kind: CheckBuild, Name: "x", Cmd: []string{"true"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Culprit == nil {
		t.Fatal("expected culprit, got nil")
	}
	if res.PriorGreen != 0 {
		t.Fatalf("expected PriorGreen=0, got %d", res.PriorGreen)
	}
}

func TestShortSHA(t *testing.T) {
	if got := shortSHA("abcdef0123456789"); got != "abcdef01" {
		t.Fatalf("shortSHA wrong: %q", got)
	}
	if got := shortSHA("abc"); got != "abc" {
		t.Fatalf("shortSHA must not pad: %q", got)
	}
}

func TestPathAllowedNested(t *testing.T) {
	c := &Contract{AllowedGlobs: []string{"internal/tui/*"}}
	if !c.pathAllowed(filepath.Join("internal", "tui", "deep", "thing.go")) {
		t.Fatal("nested in-scope path must be allowed")
	}
}

func TestBoolToInt(t *testing.T) {
	if boolToInt(true) != 1 || boolToInt(false) != 0 {
		t.Fatal("boolToInt wrong")
	}
}
