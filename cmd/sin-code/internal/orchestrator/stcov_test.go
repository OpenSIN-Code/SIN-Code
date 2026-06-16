//go:build coverage

// SPDX-License-Identifier: MIT
// Purpose: targeted coverage tests for remaining uncovered statements in the
// orchestrator package. Uses the package-level test hooks added to files that
// call external subprocesses.
package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
)

// sqlOpen returns a fresh in-memory SQLite database registered with the sqlite
// driver imported above. The pool is restricted to one connection so that the
// in-memory database is shared by all queries in the same test.
func sqlOpen(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

// restoreHooks resets package-level test hooks after a test that mutates them.
func restoreHooks(m map[*any]any) func() {
	for k, v := range m {
		*k = v
	}
	return func() {}
}

// ── model.go ─────────────────────────────────────────────────────

func TestScratchpadMergeNil(t *testing.T) {
	s := NewScratchpad()
	s.Write("a", "x", "v")
	s.Merge(nil) // should be a no-op
	if all := s.ReadAll(); len(all) != 1 {
		t.Fatalf("Merge(nil) must not alter scratchpad, got %v", all)
	}
}

// ── contract.go ─────────────────────────────────────────────────

func TestViolationStringNoLine(t *testing.T) {
	v := Violation{Kind: "k", Path: "p", Detail: "d"}
	if s := v.String(); strings.Contains(s, ":0") || !strings.Contains(s, "p") {
		t.Fatalf("unexpected violation string: %q", s)
	}
}

func TestCompileContractAllowsWorkflowForCITask(t *testing.T) {
	task := &Task{Title: "Update CI workflow", Description: "add pipeline step"}
	c := CompileContract(task)
	if v := c.CheckEdit(filepath.Join(".github", "workflows", "ci.yml"), []string{"x"}); len(v) != 0 {
		t.Fatalf("CI task should allow workflow edits, got %v", v)
	}
}

func TestContractStringWithoutLine(t *testing.T) {
	v := Violation{Kind: "k", Path: "p", Detail: "d"}
	if s := v.String(); strings.Contains(s, ":0") {
		t.Fatalf("Violation.String without line should not contain :0: %q", s)
	}
}

// ── contextc.go ──────────────────────────────────────────────────

func TestNewContextCompilerDefaultsBudget(t *testing.T) {
	cc := NewContextCompiler(0)
	if cc.Budget != 12000 {
		t.Fatalf("expected default budget 12000, got %d", cc.Budget)
	}
}

// ── dag.go ────────────────────────────────────────────────────────

func TestLeaseGlobsDefault(t *testing.T) {
	n := &PlanNode{Task: &Task{ID: "x"}}
	if got := leaseGlobs(n); len(got) != 1 || got[0] != "**" {
		t.Fatalf("expected default [**], got %v", got)
	}
}

func TestLeaseGlobsUsesPathGlobs(t *testing.T) {
	n := &PlanNode{Task: &Task{ID: "x"}, PathGlobs: []string{"pkg/**"}}
	if got := leaseGlobs(n); len(got) != 1 || got[0] != "pkg/**" {
		t.Fatalf("expected [pkg/**], got %v", got)
	}
}

// ── planner.go ───────────────────────────────────────────────────

func TestFindAgentReturnsDefault(t *testing.T) {
	if got := findAgent([]AgentConfig{}, TaskCode); got != "default" {
		t.Fatalf("expected 'default', got %q", got)
	}
}

func TestPlannerBuildPlanWithUnknownIntent(t *testing.T) {
	p := NewPlanner(DefaultAgents())
	plan := p.BuildPlan("Zephyr quantum flux")
	if plan == nil || len(plan.Tasks) == 0 {
		t.Fatal("expected plan even for unknown intent")
	}
}

// ── semmerge.go ──────────────────────────────────────────────────

func TestRecvTypeNamePointer(t *testing.T) {
	src := []byte(`package x
func (p *T) M() {}
`)
	res, err := extractDecls(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res["method:T.M"]; !ok {
		t.Fatalf("expected method:T.M, got %v", res)
	}
}

func TestRecvTypeNameIndex(t *testing.T) {
	src := []byte(`package x
func (p T[K]) M() {}
`)
	res, err := extractDecls(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res["method:T.M"]; !ok {
		t.Fatalf("expected method:T.M, got %v", res)
	}
}

func TestRecvTypeNameUnknown(t *testing.T) {
	if got := recvTypeName(nil); got != "?" {
		t.Fatalf("expected ?, got %q", got)
	}
}

func TestPackageClauseDefault(t *testing.T) {
	if got := packageClause([]byte(`// no package`)); got != "package main\n\n" {
		t.Fatalf("expected package main, got %q", got)
	}
}

func TestSortKVsInsertion(t *testing.T) {
	kvs := []struct {
		key string
		pos int
	}{
		{"c", 3},
		{"a", 1},
		{"b", 2},
	}
	sortKVs(kvs)
	for i := 1; i < len(kvs); i++ {
		if kvs[i].pos < kvs[i-1].pos {
			t.Fatalf("not sorted: %+v", kvs)
		}
	}
}

// ── targeted.go ─────────────────────────────────────────────────

func TestTargetedVerifyStagedPass(t *testing.T) {
	old := verifierRunCheck
	verifierRunCheck = func(ctx context.Context, c Check, workdir string) CheckResult {
		return CheckResult{Check: c, Passed: true}
	}
	defer func() { verifierRunCheck = old }()

	g := &ImpactGraph{nodes: map[string]*PkgNode{"repo/a": {ImportPath: "repo/a"}}, fileToPkg: map[string]string{"a.go": "repo/a"}}
	tv := NewTargetedVerifier(NewVerifier(t.TempDir()), g)
	v := tv.VerifyStaged(context.Background(), "t", "c", []string{"a.go"})
	if !v.Passed {
		t.Fatal("expected staged verify to pass")
	}
}

func TestTargetedSpeedupNoPrediction(t *testing.T) {
	g := &ImpactGraph{nodes: map[string]*PkgNode{"repo/a": {}}, fileToPkg: map[string]string{}}
	tv := NewTargetedVerifier(NewVerifier(t.TempDir()), g)
	if s := tv.Speedup([]string{"unknown.go"}); !strings.Contains(s, "no prediction") {
		t.Fatalf("expected no-prediction message, got %q", s)
	}
}

func TestTargetedSpeedupFullReduction(t *testing.T) {
	g := &ImpactGraph{
		nodes: map[string]*PkgNode{
			"repo/a": {},
			"repo/b": {},
			"repo/c": {},
			"repo/d": {},
		},
		fileToPkg: map[string]string{"a.go": "repo/a"},
	}
	tv := NewTargetedVerifier(NewVerifier(t.TempDir()), g)
	if s := tv.Speedup([]string{"a.go"}); !strings.Contains(s, "reduction") {
		t.Fatalf("expected reduction message, got %q", s)
	}
}

// ── verifier.go ─────────────────────────────────────────────────

func TestBestVerdictTieBreakerByTime(t *testing.T) {
	now := time.Now()
	v1 := &Verdict{Candidate: "a", Passed: true, Score: 0.8, CreatedAt: now.Add(1 * time.Second)}
	v2 := &Verdict{Candidate: "b", Passed: true, Score: 0.8, CreatedAt: now}
	best := BestVerdict([]*Verdict{v1, v2})
	if best.Candidate != "b" {
		t.Fatalf("expected older candidate to win tie, got %q", best.Candidate)
	}
}

func TestVerifierUnknownCheckWeight(t *testing.T) {
	old := verifierRunCheck
	verifierRunCheck = func(ctx context.Context, c Check, workdir string) CheckResult {
		return CheckResult{Check: c, Passed: true}
	}
	defer func() { verifierRunCheck = old }()

	vf := NewVerifier(t.TempDir())
	v := vf.Verify(context.Background(), "t", "c", []Check{{Kind: "unknown-kind", Name: "x", Cmd: []string{"true"}}})
	if !v.Passed {
		t.Fatal("expected pass")
	}
}

// ── registry.go ───────────────────────────────────────────────────

func TestDefaultUserAgentsPath(t *testing.T) {
	path := DefaultUserAgentsPath()
	if path == "" {
		t.Fatal("expected non-empty path")
	}
	if !strings.Contains(path, "sin-code") {
		t.Fatalf("expected sin-code in path, got %q", path)
	}
}

func TestLoadUserAgentsReadDirErrorStcov(t *testing.T) {
	// Use a file path as the base dir so ReadDir returns an error.
	f := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadUserAgents(f)
	if err == nil {
		t.Fatal("expected error when baseDir is a file")
	}
}

func TestMergeConfigKeepsBase(t *testing.T) {
	base := AgentConfig{Name: "a", Model: "m1", Description: "d1"}
	override := AgentConfig{Name: "b", Description: "", Model: "m2"}
	merged := mergeConfig(base, override)
	if merged.Name != "a" {
		t.Fatalf("expected name preserved, got %q", merged.Name)
	}
	if merged.Description != "d1" {
		t.Fatalf("expected description preserved, got %q", merged.Description)
	}
	if merged.Model != "m2" {
		t.Fatalf("expected model overridden, got %q", merged.Model)
	}
}

func TestMergeConfigOverrideAll(t *testing.T) {
	base := AgentConfig{}
	override := AgentConfig{
		Description:   "d",
		Model:         "m",
		MaxTokens:     1,
		Temperature:   0.5,
		SystemFile:    "s",
		MaxContext:    2,
		ToolsAllow:    []string{"a"},
		ToolsDeny:     []string{"b"},
		MemoryNS:      "n",
		RetentionDays: 3,
	}
	merged := mergeConfig(base, override)
	if merged.Description != "d" || merged.Model != "m" || merged.MaxTokens != 1 ||
		merged.Temperature != 0.5 || merged.SystemFile != "s" || merged.MaxContext != 2 ||
		len(merged.ToolsAllow) != 1 || len(merged.ToolsDeny) != 1 || merged.MemoryNS != "n" ||
		merged.RetentionDays != 3 {
		t.Fatalf("unexpected merge result: %+v", merged)
	}
}

// ── confidence.go ─────────────────────────────────────────────────

func TestCalibratorRecord(t *testing.T) {
	db := sqlOpen(t)
	c, err := NewCalibrator(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Record(context.Background(), ConfidenceClaim{AgentName: "a", TaskClass: ClassBugfix, Declared: 0.8, Passed: true}); err != nil {
		t.Fatal(err)
	}
}

func TestCalibratorBrierScore(t *testing.T) {
	db := sqlOpen(t)
	c, err := NewCalibrator(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_ = c.Record(ctx, ConfidenceClaim{AgentName: "a", Declared: 0.8, Passed: true})
	_ = c.Record(ctx, ConfidenceClaim{AgentName: "a", Declared: 0.2, Passed: false})
	score, n, err := c.BrierScore(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("expected n=2, got %d", n)
	}
	if score == 0 {
		t.Fatal("expected non-zero brier score")
	}
}

func TestCalibratorCalibrateWithData(t *testing.T) {
	db := sqlOpen(t)
	c, err := NewCalibrator(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for i := 0; i < 15; i++ {
		_ = c.Record(ctx, ConfidenceClaim{AgentName: "a", Declared: 0.8, Passed: true})
	}
	cal, err := c.Calibrate(ctx, "a", 0.8)
	if err != nil {
		t.Fatal(err)
	}
	if cal < 0 || cal > 1 {
		t.Fatalf("calibrated out of range: %f", cal)
	}
}

// ── episodic.go ───────────────────────────────────────────────────

func TestEpisodeStoreRecord(t *testing.T) {
	db := sqlOpen(t)
	s, err := NewEpisodeStore(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Record(ctx, &Episode{Intent: "x", TaskTitle: "t", PlanJSON: []byte("{}"), Score: 0.5, Passed: true, Rounds: 1}); err != nil {
		t.Fatal(err)
	}
}

func TestEpisodeStoreSimilar(t *testing.T) {
	db := sqlOpen(t)
	s, err := NewEpisodeStore(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_ = s.Record(ctx, &Episode{Intent: "fix nil pointer", TaskTitle: "nil pointer bug", PlanJSON: []byte("{}"), Score: 0.9, Passed: true, Rounds: 1})
	eps, err := s.Similar(ctx, "nil pointer bug", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) == 0 {
		t.Fatal("expected similar episodes")
	}
}

func TestEpisodeStoreSimilarKZero(t *testing.T) {
	db := sqlOpen(t)
	s, err := NewEpisodeStore(db)
	if err != nil {
		t.Fatal(err)
	}
	eps, err := s.Similar(context.Background(), "x", 0)
	if err != nil || eps != nil {
		t.Fatalf("expected nil for k<=0, got %v err=%v", eps, err)
	}
}

// ── impact.go ─────────────────────────────────────────────────────

func TestBuildImpactGraphEmptyRoot(t *testing.T) {
	g, err := BuildImpactGraph(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if g == nil || len(g.nodes) != 0 {
		t.Fatal("expected empty graph")
	}
}

func TestBuildImpactGraphWithMockGoList(t *testing.T) {
	old := impactGoList
	impactGoList = func(ctx context.Context, dir string) ([]byte, error) {
		return []byte(`
{"ImportPath": "repo/a", "Dir": "/tmp/repo/a", "GoFiles": ["a.go"], "TestGoFiles": ["a_test.go"], "XTestGoFiles": [], "Imports": ["repo/b"]}
{"ImportPath": "repo/b", "Dir": "/tmp/repo/b", "GoFiles": ["b.go"], "TestGoFiles": [], "XTestGoFiles": [], "Imports": []}
`), nil
	}
	defer func() { impactGoList = old }()

	g, err := BuildImpactGraph(context.Background(), "/tmp/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(g.nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(g.nodes))
	}
	if len(g.reverse["repo/b"]) != 1 {
		t.Fatalf("expected reverse edge b->a, got %v", g.reverse)
	}
	imp := g.Predict([]string{"a/a.go"})
	if len(imp.ChangedPkgs) != 1 || imp.ChangedPkgs[0] != "repo/a" {
		t.Fatalf("expected changed repo/a, got %v", imp.ChangedPkgs)
	}
	if len(imp.AffectedPkgs) != 1 || imp.AffectedPkgs[0] != "repo/a" {
		t.Fatalf("expected affected repo/a, got %v", imp.AffectedPkgs)
	}
	if len(imp.AffectedTestPkgs) != 1 || imp.AffectedTestPkgs[0] != "repo/a" {
		t.Fatalf("expected affected test repo/a, got %v", imp.AffectedTestPkgs)
	}
}

func TestBuildImpactGraphGoListError(t *testing.T) {
	old := impactGoList
	impactGoList = func(ctx context.Context, dir string) ([]byte, error) {
		return nil, errors.New("boom")
	}
	defer func() { impactGoList = old }()

	_, err := BuildImpactGraph(context.Background(), "/tmp/repo")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildImpactGraphDecodeError(t *testing.T) {
	old := impactGoList
	impactGoList = func(ctx context.Context, dir string) ([]byte, error) {
		return []byte("not-json"), nil
	}
	defer func() { impactGoList = old }()

	_, err := BuildImpactGraph(context.Background(), "/tmp/repo")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ── blame.go ──────────────────────────────────────────────────────

func TestBlameResultDiagnosisNoCulprit(t *testing.T) {
	br := &BlameResult{Check: Check{Name: "c"}}
	if s := br.Diagnosis(); !strings.Contains(s, "pre-existing") {
		t.Fatalf("expected pre-existing diagnosis, got %q", s)
	}
}

func TestBlameResultDiagnosisWithCulprit(t *testing.T) {
	br := &BlameResult{
		Check:      Check{Name: "c"},
		Culprit:    &EditRecord{Seq: 2, SHA: "abc123", Path: "p", Summary: "s"},
		PriorGreen: 1,
	}
	if s := br.Diagnosis(); !strings.Contains(s, "CULPRIT") {
		t.Fatalf("expected culprit diagnosis, got %q", s)
	}
}

func TestBlameEmptyEdits(t *testing.T) {
	bl := &Blamer{Verifier: NewVerifier(t.TempDir())}
	_, err := bl.Blame(context.Background(), &EditLog{Edits: []EditRecord{}}, Check{})
	if err == nil {
		t.Fatal("expected error for empty edits")
	}
}

func TestBlameBisect(t *testing.T) {
	old := verifierRunCheck
	verifierRunCheck = func(ctx context.Context, c Check, workdir string) CheckResult {
		return CheckResult{Check: c, Passed: true}
	}
	defer func() { verifierRunCheck = old }()

	bl := &Blamer{Verifier: NewVerifier(t.TempDir())}
	log := &EditLog{
		TaskID:  "t",
		Workdir: "", // empty means checkAt returns true
		Base:    "base",
		Edits: []EditRecord{
			{Seq: 1, SHA: "sha1"},
			{Seq: 2, SHA: "sha2"},
			{Seq: 3, SHA: "sha3"},
		},
	}
	res, err := bl.Blame(context.Background(), log, Check{Name: "c"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Culprit == nil || res.Culprit.Seq != 3 {
		t.Fatalf("expected culprit edit 3, got %+v", res.Culprit)
	}
	if res.PriorGreen != 2 {
		t.Fatalf("expected priorGreen 2, got %d", res.PriorGreen)
	}
}

func TestBlameBaseAlreadyFailing(t *testing.T) {
	oldGit := blameGitCmd
	oldVerify := verifierRunCheck
	blameGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) { return nil, nil }
	verifierRunCheck = func(ctx context.Context, c Check, workdir string) CheckResult {
		return CheckResult{Check: c, Passed: false}
	}
	defer func() {
		blameGitCmd = oldGit
		verifierRunCheck = oldVerify
	}()

	bl := &Blamer{Verifier: NewVerifier(t.TempDir())}
	log := &EditLog{TaskID: "t", Workdir: t.TempDir(), Base: "base", Edits: []EditRecord{{Seq: 1, SHA: "sha1"}}}
	res, err := bl.Blame(context.Background(), log, Check{Kind: CheckBuild, Name: "c"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Culprit != nil {
		t.Fatal("expected no culprit when base already failing")
	}
}

func TestBlameGitHookError(t *testing.T) {
	oldGit := blameGitCmd
	oldVerify := verifierRunCheck
	blameGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return nil, errors.New("git error")
	}
	verifierRunCheck = func(ctx context.Context, c Check, workdir string) CheckResult {
		return CheckResult{Check: c, Passed: true}
	}
	defer func() {
		blameGitCmd = oldGit
		verifierRunCheck = oldVerify
	}()

	bl := &Blamer{Verifier: NewVerifier(t.TempDir())}
	log := &EditLog{TaskID: "t", Workdir: t.TempDir(), Base: "base", Edits: []EditRecord{{Seq: 1, SHA: "sha1"}}}
	_, err := bl.Blame(context.Background(), log, Check{Name: "c"})
	if err == nil {
		t.Fatal("expected git error")
	}
}

// ── kernel.go ─────────────────────────────────────────────────────

func TestNewKernelNoDB(t *testing.T) {
	dir := t.TempDir()
	k, err := NewKernel(nil, dir)
	if err != nil {
		t.Fatal(err)
	}
	if k.db != nil || k.Workdir != dir {
		t.Fatal("unexpected kernel fields")
	}
}

func TestKernelCaptureWithDB(t *testing.T) {
	db := sqlOpen(t)
	k, err := NewKernel(db, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	oldGit := kernelGitCmd
	oldGitOut := kernelGitOutCmd
	kernelGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) { return nil, nil }
	kernelGitOutCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if args[0] == "write-tree" {
			return []byte("TREE"), nil
		}
		return nil, nil
	}
	defer func() {
		kernelGitCmd = oldGit
		kernelGitOutCmd = oldGitOut
	}()

	cp, err := k.Capture(context.Background(), "start", AgentState{TaskID: "t"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if cp.Label != "start" || !cp.Green || cp.TreeSHA != "TREE" {
		t.Fatalf("unexpected checkpoint: %+v", cp)
	}
}

func TestKernelCaptureWriteTreeError(t *testing.T) {
	db := sqlOpen(t)
	k, err := NewKernel(db, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	old := kernelGitCmd
	kernelGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return nil, errors.New("add error")
	}
	defer func() { kernelGitCmd = old }()

	_, err = k.Capture(context.Background(), "x", AgentState{}, true)
	if err == nil {
		t.Fatal("expected write-tree error")
	}
}

func TestKernelTimelineAndLastGreen(t *testing.T) {
	db := sqlOpen(t)
	k, _ := NewKernel(db, "")
	_, err := k.Timeline(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = k.LastGreen(context.Background())
	if err == nil {
		t.Fatal("expected no-green error")
	}
}

func TestKernelRewind(t *testing.T) {
	db := sqlOpen(t)
	dir := t.TempDir()
	k, _ := NewKernel(db, dir)
	oldGit := kernelGitCmd
	oldGitOut := kernelGitOutCmd
	kernelGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) { return nil, nil }
	kernelGitOutCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if args[0] == "write-tree" {
			return []byte("TREE"), nil
		}
		return nil, nil
	}
	defer func() {
		kernelGitCmd = oldGit
		kernelGitOutCmd = oldGitOut
	}()

	cp, err := k.Capture(context.Background(), "green", AgentState{TaskID: "t", EpisodeCursor: 7}, true)
	if err != nil {
		t.Fatal(err)
	}
	if cp.ID == 0 {
		t.Fatalf("expected non-zero checkpoint id, got %+v", cp)
	}
	state, err := k.Rewind(context.Background(), cp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.EpisodeCursor != 7 {
		t.Fatalf("expected cursor 7, got %d", state.EpisodeCursor)
	}
}

func TestKernelRewindNoDB(t *testing.T) {
	k, _ := NewKernel(nil, "")
	_, err := k.Rewind(context.Background(), 1)
	if err == nil {
		t.Fatal("expected no-DB error")
	}
}

func TestKernelLastGreenNoDB(t *testing.T) {
	k, _ := NewKernel(nil, "")
	_, _, err := k.LastGreen(context.Background())
	if err == nil {
		t.Fatal("expected no-DB error")
	}
}

func TestHashScratchpad(t *testing.T) {
	if HashScratchpad([]byte("abc")) == "" {
		t.Fatal("expected non-empty hash")
	}
}

// scriptAgent is a deterministic Agent implementation for tests.
type stcovAgent struct {
	name string
	out  string
	err  error
}

func (a stcovAgent) Name() string        { return a.name }
func (a stcovAgent) Config() AgentConfig { return AgentConfig{} }
func (a stcovAgent) Run(ctx context.Context, task *Task, scratch *Scratchpad) (string, error) {
	return a.out, a.err
}

type stcovAdversary struct{ attacks []Attack }

func (s stcovAdversary) ProposeAttacks(ctx context.Context, diff, impactBrief string, maxAttacks int) ([]Attack, error) {
	return s.attacks, nil
}

// ── adversary.go ─────────────────────────────────────────────────

func TestAdversaryNewAndReviewCleared(t *testing.T) {
	adv := NewAdversary(stcovAdversary{}, t.TempDir())
	if adv.MaxAttacks != 6 || adv.ProbeTimeout != 90*time.Second {
		t.Fatal("unexpected default adversary fields")
	}
	res, err := adv.Review(context.Background(), "", "")
	if err != nil || !res.Cleared || len(res.Attacks) != 0 {
		t.Fatalf("expected cleared empty review, got %+v err=%v", res, err)
	}
}

func TestAdversaryReviewLandedProbe(t *testing.T) {
	old := adversaryExecCommand
	adversaryExecCommand = func(ctx context.Context, workdir, rel string, timeout time.Duration) ([]byte, error) {
		return []byte("fail"), errors.New("probe failed")
	}
	defer func() { adversaryExecCommand = old }()

	workdir := t.TempDir()
	// Create a real package directory so probePackageDir finds it.
	pkgDir := filepath.Join(workdir, "pkg")
	if err := os.MkdirAll(pkgDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "pkg.go"), []byte("package pkg\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	adv := NewAdversary(stcovAdversary{attacks: []Attack{{Kind: AttackBoundary, Hypothesis: "h", ProbeSource: "package pkg\nfunc TestAdversary(t *testing.T) {}\n"}}}, workdir)
	res, err := adv.Review(context.Background(), "", "")
	if err != nil || res.Cleared || res.Landed != 1 {
		t.Fatalf("expected one landed attack, got %+v err=%v", res, err)
	}
}

func TestAdversaryExecuteProbeEmptyWorkdir(t *testing.T) {
	adv := &Adversary{Workdir: ""}
	_, _, err := adv.executeProbe(context.Background(), &Attack{}, 0)
	if err == nil || !strings.Contains(err.Error(), "empty workdir") {
		t.Fatalf("expected empty workdir error, got %v", err)
	}
}

func TestProbePackageDirNotFound(t *testing.T) {
	_, err := probePackageDir("package missing\n", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestProbePackageDirNoPackage(t *testing.T) {
	_, err := probePackageDir("// no package\n", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "no package clause") {
		t.Fatalf("expected no package clause error, got %v", err)
	}
}

func TestRelOrDot(t *testing.T) {
	if got := relOrDot("/a", "/a"); got != "." {
		t.Fatalf("expected ., got %q", got)
	}
	if got := relOrDot("/a/b", "/a/c"); got != "../c" {
		t.Fatalf("expected ../c, got %q", got)
	}
}

// ── mutation.go ─────────────────────────────────────────────────

func TestMutationProbeRunKilled(t *testing.T) {
	mp := NewMutationProbe("", []string{"true"})
	mp.MaxMutations = 10
	old := mutationExecCommand
	mutationExecCommand = func(ctx context.Context, workdir string, args ...string) error { return errors.New("fail") }
	defer func() { mutationExecCommand = old }()

	res, err := mp.Run(context.Background(), []ChangedLine{
		{File: "x.go", Line: 1, Text: "if a == b {"},
		{File: "x.go", Line: 2, Text: "if c < d {"},
		{File: "x.go", Line: 3, Text: "if e && f {"},
		{File: "x.go", Line: 4, Text: "return true"},
	})
	if err != nil || res == nil || res.Survived != 0 || res.Killed == 0 {
		t.Fatalf("expected mutations killed, got %+v err=%v", res, err)
	}
}

func TestMutationProbeRunSurvived(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte("if a == b {\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mp := NewMutationProbe(dir, []string{"true"})
	mp.MaxMutations = 10
	old := mutationExecCommand
	mutationExecCommand = func(ctx context.Context, workdir string, args ...string) error { return nil }
	defer func() { mutationExecCommand = old }()

	res, err := mp.Run(context.Background(), []ChangedLine{{File: "x.go", Line: 1, Text: "if a == b {"}})
	if err != nil || res == nil || res.Survived == 0 {
		t.Fatalf("expected mutation survived, got %+v err=%v", res, err)
	}
}

func TestMutationProbeRunEmpty(t *testing.T) {
	mp := NewMutationProbe("", []string{"true"})
	res, err := mp.Run(context.Background(), []ChangedLine{})
	if err != nil || res == nil || res.ObservabilityScore != 1.0 {
		t.Fatalf("expected empty result with score 1, got %+v err=%v", res, err)
	}
}

func TestMutationApplyAndTestOutOfRange(t *testing.T) {
	mp := &MutationProbe{Workdir: t.TempDir()}
	dir := filepath.Join(mp.Workdir, "pkg")
	os.MkdirAll(dir, 0o750)
	os.WriteFile(filepath.Join(dir, "x.go"), []byte("line1\n"), 0o600)
	_, err := mp.applyAndTest(context.Background(), ChangedLine{File: "pkg/x.go", Line: 5, Text: "line1"}, "mut")
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("expected out of range error, got %v", err)
	}
}

func TestMutationApplyAndTestTextMismatch(t *testing.T) {
	mp := &MutationProbe{Workdir: t.TempDir()}
	os.WriteFile(filepath.Join(mp.Workdir, "x.go"), []byte("line1\n"), 0o600)
	killed, err := mp.applyAndTest(context.Background(), ChangedLine{File: "x.go", Line: 1, Text: "different"}, "mut")
	if err != nil || !killed {
		t.Fatalf("expected killed=true for mismatch, got killed=%v err=%v", killed, err)
	}
}

func TestMutationApplyAndTestNoCmd(t *testing.T) {
	mp := &MutationProbe{Workdir: t.TempDir()}
	os.WriteFile(filepath.Join(mp.Workdir, "x.go"), []byte("line1\n"), 0o600)
	killed, err := mp.applyAndTest(context.Background(), ChangedLine{File: "x.go", Line: 1, Text: "line1"}, "mut")
	if err != nil || killed {
		t.Fatalf("expected killed=false with no test cmd, got killed=%v err=%v", killed, err)
	}
}

// ── speculative.go ────────────────────────────────────────────────

func TestSpeculativeRunSingleWinner(t *testing.T) {
	old := specWorktreeAdd
	specWorktreeAdd = func(ctx context.Context, repoRoot, dir string) ([]byte, error) { return nil, nil }
	defer func() { specWorktreeAdd = old }()

	s := NewSpeculativeRunner("", []Check{{Kind: CheckBuild, Name: "c", Cmd: []string{"true"}}})
	s.MaxParallel = 1

	oldVerify := verifierRunCheck
	verifierRunCheck = func(ctx context.Context, c Check, workdir string) CheckResult {
		return CheckResult{Check: c, Passed: true}
	}
	defer func() { verifierRunCheck = oldVerify }()

	res, err := s.Run(context.Background(), &Task{ID: "t"}, []Agent{stcovAgent{name: "a", out: "ok"}}, NewScratchpad())
	if err != nil || res == nil || res.Winner == nil || !res.Winner.Verdict.Passed {
		t.Fatalf("expected winner, got %+v err=%v", res, err)
	}
}

func TestSpeculativeAddWorktreeNoRepoRoot(t *testing.T) {
	s := NewSpeculativeRunner("", nil)
	dir, err := s.addWorktree(context.Background(), "id")
	if err != nil || dir == "" {
		t.Fatalf("expected dir, got %q err=%v", dir, err)
	}
}

func TestSpeculativeAddWorktreeError(t *testing.T) {
	old := specWorktreeAdd
	specWorktreeAdd = func(ctx context.Context, repoRoot, dir string) ([]byte, error) {
		return []byte("boom"), errors.New("fail")
	}
	defer func() { specWorktreeAdd = old }()

	s := NewSpeculativeRunner(t.TempDir(), nil)
	_, err := s.addWorktree(context.Background(), "id")
	if err == nil || !strings.Contains(err.Error(), "worktree add") {
		t.Fatalf("expected worktree add error, got %v", err)
	}
}

func TestSpeculativeMergeWinnerNoWinner(t *testing.T) {
	s := NewSpeculativeRunner(t.TempDir(), nil)
	_, err := s.MergeWinner(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "no winner") {
		t.Fatalf("expected no winner error, got %v", err)
	}
}

func TestSpeculativeMergeWinnerNoRepoRoot(t *testing.T) {
	s := NewSpeculativeRunner("", nil)
	_, err := s.MergeWinner(context.Background(), &Candidate{Worktree: "x"})
	if err == nil || !strings.Contains(err.Error(), "RepoRoot unset") {
		t.Fatalf("expected RepoRoot unset error, got %v", err)
	}
}

func TestSpeculativeMergeWinnerEmptyDiff(t *testing.T) {
	old := specDiff
	specDiff = func(ctx context.Context, worktree string) ([]byte, error) { return nil, nil }
	defer func() { specDiff = old }()

	s := NewSpeculativeRunner(t.TempDir(), nil)
	diff, err := s.MergeWinner(context.Background(), &Candidate{Worktree: t.TempDir()})
	if err != nil || diff != "" {
		t.Fatalf("expected empty diff, got %q err=%v", diff, err)
	}
}

func TestSpeculativeMergeWinnerApplyError(t *testing.T) {
	oldDiff := specDiff
	oldApply := specApply
	specDiff = func(ctx context.Context, worktree string) ([]byte, error) { return []byte("diff"), nil }
	specApply = func(ctx context.Context, repoRoot string, diff []byte) ([]byte, error) {
		return []byte("fail"), errors.New("apply error")
	}
	defer func() { specDiff = oldDiff; specApply = oldApply }()

	s := NewSpeculativeRunner(t.TempDir(), nil)
	_, err := s.MergeWinner(context.Background(), &Candidate{Worktree: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "apply winner diff") {
		t.Fatalf("expected apply error, got %v", err)
	}
}

func TestSpeculativeRemoveWorktreeEmpty(t *testing.T) {
	s := NewSpeculativeRunner(t.TempDir(), nil)
	// Should not panic.
	s.removeWorktree("")
}

// ── governor.go ───────────────────────────────────────────────────

func TestGovernorRunRungMultiAgentPass(t *testing.T) {
	old := verifierRunCheck
	verifierRunCheck = func(ctx context.Context, c Check, workdir string) CheckResult {
		return CheckResult{Check: c, Passed: true}
	}
	defer func() { verifierRunCheck = old }()

	oldSpec := specWorktreeAdd
	specWorktreeAdd = func(ctx context.Context, repoRoot, dir string) ([]byte, error) { return nil, nil }
	defer func() { specWorktreeAdd = oldSpec }()

	g := &Governor{
		Ladder:   []Rung{{Name: "best-of-3", Agents: 2, RepairRounds: 1, Timeout: 5 * time.Second}},
		Verifier: NewVerifier(t.TempDir()),
		Checks:   []Check{{Kind: CheckBuild, Name: "c", Cmd: []string{"true"}}},
		Factory: func(rung Rung) []Agent {
			return []Agent{stcovAgent{name: "a1", out: "ok"}, stcovAgent{name: "a2", out: "ok"}}
		},
	}
	res, err := g.Execute(context.Background(), &Task{ID: "t"}, NewScratchpad())
	if err != nil || !res.Passed || res.FinalRung != "best-of-3" {
		t.Fatalf("expected multi-agent pass, got %+v err=%v", res, err)
	}
}

func TestGovernorRunRungFactoryEmpty(t *testing.T) {
	g := &Governor{
		Ladder:  []Rung{{Name: "x", Agents: 1, RepairRounds: 1, Timeout: time.Second}},
		Factory: func(rung Rung) []Agent { return nil },
	}
	res, err := g.Execute(context.Background(), &Task{ID: "t"}, NewScratchpad())
	if err == nil || !strings.Contains(err.Error(), "factory returned no agents") {
		t.Fatalf("expected factory empty error, got %+v err=%v", res, err)
	}
}

func TestEscalationReasonNil(t *testing.T) {
	if s := escalationReason(nil); !strings.Contains(s, "no verdict") {
		t.Fatalf("expected no verdict reason, got %q", s)
	}
}

// ── miner.go ──────────────────────────────────────────────────────

func TestMinerWithDB(t *testing.T) {
	db := sqlOpen(t)
	m, err := NewMiner(db)
	if err != nil {
		t.Fatal(err)
	}
	if m.db == nil || m.MinSupport != 5 {
		t.Fatal("unexpected miner defaults")
	}
}

func TestMinerRecordAndMine(t *testing.T) {
	db := sqlOpen(t)
	m, _ := NewMiner(db)
	m.MinSupport = 2
	m.MinSuccessRate = 0.5
	m.MinLength = 1
	m.MaxLength = 3

	ctx := context.Background()
	seq := []ToolCall{{Tool: "read", ArgShape: "{}"}, {Tool: "edit", ArgShape: "{}"}}
	_ = m.RecordSequence(ctx, SeqEpisode{EpisodeID: 1, Class: ClassBugfix, Sequence: seq, Passed: true})
	_ = m.RecordSequence(ctx, SeqEpisode{EpisodeID: 2, Class: ClassBugfix, Sequence: seq, Passed: true})
	_ = m.RecordSequence(ctx, SeqEpisode{EpisodeID: 3, Class: ClassBugfix, Sequence: seq, Passed: false})

	fresh, err := m.Mine(ctx, ClassBugfix)
	if err != nil {
		t.Fatal(err)
	}
	if len(fresh) == 0 {
		t.Fatal("expected fresh templates")
	}
	if err := m.ReportLive(ctx, fresh[0].ID, true); err != nil {
		t.Fatal(err)
	}
	s, err := m.SuggestionsFor(ctx, ClassBugfix)
	if err != nil || s == "" {
		t.Fatalf("expected suggestions, got %q err=%v", s, err)
	}
}

func TestMinerMineNotEnoughSupport(t *testing.T) {
	db := sqlOpen(t)
	m, _ := NewMiner(db)
	m.MinSupport = 100
	fresh, err := m.Mine(context.Background(), ClassBugfix)
	if err != nil || len(fresh) != 0 {
		t.Fatalf("expected no templates, got %v err=%v", fresh, err)
	}
}

func TestMinerNilDBSafe(t *testing.T) {
	m, _ := NewMiner(nil)
	ctx := context.Background()
	if err := m.RecordSequence(ctx, SeqEpisode{}); err != nil {
		t.Fatal(err)
	}
	if fresh, err := m.Mine(ctx, ClassBugfix); err != nil || len(fresh) != 0 {
		t.Fatalf("expected nil mine, got %v err=%v", fresh, err)
	}
	if err := m.ReportLive(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	if s, err := m.SuggestionsFor(ctx, ClassBugfix); err != nil || s != "" {
		t.Fatalf("expected empty suggestions, got %q err=%v", s, err)
	}
}

// ── strategy_router.go ────────────────────────────────────────────

func TestStrategyRouterLoadFromDB(t *testing.T) {
	db := sqlOpen(t)
	ctx := context.Background()
	// Create schema first.
	if _, err := NewStrategyRouter(db, 1); err != nil {
		t.Fatal(err)
	}
	_, err := db.ExecContext(ctx, `INSERT INTO router_arms (class, strategy, alpha, beta) VALUES (?, ?, ?, ?)`,
		string(ClassBugfix), string(StratASTEdit), 5.0, 2.0)
	if err != nil {
		t.Fatal(err)
	}

	r, err := NewStrategyRouter(db, 1)
	if err != nil {
		t.Fatal(err)
	}
	mean, n := r.Posterior(ClassBugfix, StratASTEdit)
	if n != 5 || mean <= 0 {
		t.Fatalf("expected loaded posterior, got mean=%f n=%d", mean, n)
	}
}

func TestStrategyRouterReportNil(t *testing.T) {
	var r *StrategyRouter
	if err := r.Report(context.Background(), ClassBugfix, StratASTEdit, true); err != nil {
		t.Fatal(err)
	}
}

func TestStrategyRouterPickEmpty(t *testing.T) {
	r, _ := NewStrategyRouter(nil, 1)
	if pick := r.Pick(ClassBugfix, nil); pick != "" {
		t.Fatalf("expected empty pick, got %q", pick)
	}
}

func TestStrategyRouterPickNoDB(t *testing.T) {
	r, _ := NewStrategyRouter(nil, 1)
	pick := r.Pick(ClassBugfix, []Strategy{StratASTEdit, StratHashline})
	if pick != StratASTEdit && pick != StratHashline {
		t.Fatalf("expected valid pick, got %q", pick)
	}
}

// ── cartographer.go ───────────────────────────────────────────────

func TestCartographerCallName(t *testing.T) {
	src := []byte(`package x
func A() {}
func B() { A() }
func C() { x.A() }
`)
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if n := callName(call, "x"); n != "" {
				names = append(names, n)
			}
		}
		return true
	})
	if len(names) != 2 || names[0] != "x.A" || names[1] != "x.A" {
		t.Fatalf("expected two x.A calls, got %v", names)
	}
}

func TestLastSegment(t *testing.T) {
	if got := lastSegment("a/b/c"); got != "c" {
		t.Fatalf("expected c, got %q", got)
	}
	if got := lastSegment("c"); got != "c" {
		t.Fatalf("expected c, got %q", got)
	}
}

func TestAffectedByPath(t *testing.T) {
	affected := map[string]bool{"repo/a": true}
	if !affectedByPath(affected, "repo/a/b.go") {
		t.Fatal("expected affected by path")
	}
	if affectedByPath(map[string]bool{}, "x.go") {
		t.Fatal("expected not affected")
	}
}

// ── kernel.go ─────────────────────────────────────────────────────

func TestKernelTimelineNonEmpty(t *testing.T) {
	db := sqlOpen(t)
	dir := t.TempDir()
	k, _ := NewKernel(db, dir)
	oldGit := kernelGitCmd
	oldGitOut := kernelGitOutCmd
	kernelGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) { return nil, nil }
	kernelGitOutCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if args[0] == "write-tree" {
			return []byte("TREE"), nil
		}
		return nil, nil
	}
	defer func() { kernelGitCmd = oldGit; kernelGitOutCmd = oldGitOut }()

	if _, err := k.Capture(context.Background(), "green", AgentState{TaskID: "t"}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Capture(context.Background(), "red", AgentState{TaskID: "t"}, false); err != nil {
		t.Fatal(err)
	}
	s, err := k.Timeline(context.Background(), 0)
	if err != nil || !strings.Contains(s, "GREEN") {
		t.Fatalf("expected timeline with GREEN, got %q err=%v", s, err)
	}
}

func TestKernelTimelineNegativeLimit(t *testing.T) {
	db := sqlOpen(t)
	k, _ := NewKernel(db, "")
	_, err := k.Timeline(context.Background(), -5)
	if err != nil {
		t.Fatal(err)
	}
}

// ── leases.go ──────────────────────────────────────────────────────

func TestLeaseAcquireConflict(t *testing.T) {
	lt := NewLeaseTable()
	_, _, err := lt.Acquire("a", "t1", "intent", []string{"pkg/**"})
	if err != nil {
		t.Fatal(err)
	}
	_, conflicts, err := lt.Acquire("b", "t2", "intent", []string{"pkg/foo.go"})
	if err != nil || len(conflicts) == 0 {
		t.Fatalf("expected conflict, got conflicts=%v err=%v", conflicts, err)
	}
}

func TestLeaseBoardEmpty(t *testing.T) {
	lt := NewLeaseTable()
	if s := lt.Board(); s != "" {
		t.Fatalf("expected empty board, got %q", s)
	}
}

func TestLeaseGlobsOverlapPrefix(t *testing.T) {
	if !globsOverlap("pkg/*", "pkg/a/b") {
		t.Fatal("expected prefix overlap")
	}
}

// ── contextc.go ─────────────────────────────────────────────────────

func TestGatherStandardAll(t *testing.T) {
	items := GatherStandard(
		CompileContract(&Task{ID: "t", Title: "x", Description: "y"}),
		"diagnosis",
		&Impact{AffectedPkgs: []string{"pkg"}},
		[]*Episode{{Intent: "i", TaskTitle: "t", Score: 0.8, Passed: true}},
		"suggestions",
		map[string]string{"a.go": "code"},
		map[string]float64{"a.go": 0.9},
	)
	if len(items) < 6 {
		t.Fatalf("expected all item kinds, got %d", len(items))
	}
}

// ── contract.go ───────────────────────────────────────────────────

func TestContractCheckDiffStats(t *testing.T) {
	c := CompileContract(&Task{ID: "t"})
	c.MaxFilesChanged = 1
	c.MaxLinesChanged = 10
	v := c.CheckDiffStats(2, 5)
	if len(v) != 1 || !strings.Contains(v[0].Detail, "files") {
		t.Fatalf("expected files violation, got %v", v)
	}
	v = c.CheckDiffStats(1, 20)
	if len(v) != 1 || !strings.Contains(v[0].Detail, "lines") {
		t.Fatalf("expected lines violation, got %v", v)
	}
}

// ── dispatcher.go ──────────────────────────────────────────────────

func TestNewDispatcherDefaults(t *testing.T) {
	d := NewDispatcher(NewRegistry(nil), NewScratchpad(), 0)
	if d.maxPar != 4 {
		t.Fatalf("expected default 4, got %d", d.maxPar)
	}
}

// ── episodic.go ───────────────────────────────────────────────────

func TestEpisodeStoreNilDB(t *testing.T) {
	s, err := NewEpisodeStore(nil)
	if err != nil || s.hasSchema {
		t.Fatalf("expected nil-DB store without schema, got %+v err=%v", s, err)
	}
}

func TestEpisodeStoreSimilarEmptyQuery(t *testing.T) {
	db := sqlOpen(t)
	s, _ := NewEpisodeStore(db)
	eps, err := s.Similar(context.Background(), "", 5)
	if err != nil || eps != nil {
		t.Fatalf("expected nil for empty query, got %v err=%v", eps, err)
	}
}

// ── registry.go ──────────────────────────────────────────────────

func TestUseLLMEnvKey(t *testing.T) {
	t.Setenv("SIN_LLM_API_KEY", "secret")
	if !UseLLM() {
		t.Fatal("expected UseLLM true with SIN_LLM_API_KEY")
	}
}

func TestNewRegistryWithDefaultsOverride(t *testing.T) {
	extra := []AgentConfig{{Name: "coder", Model: "override"}}
	r := NewRegistryWithDefaults(extra)
	cfg, ok := r.config["coder"]
	if !ok || cfg.Model != "override" {
		t.Fatalf("expected model override, got %+v ok=%v", cfg, ok)
	}
}

func TestNewRegistryWithDefaultsNewAgent(t *testing.T) {
	extra := []AgentConfig{{Name: "custom", Type: TaskDocs, Model: "m"}}
	r := NewRegistryWithDefaults(extra)
	if _, ok := r.config["custom"]; !ok {
		t.Fatal("expected custom agent registered")
	}
}

// ── confidence.go ──────────────────────────────────────────────────

func TestCalibratorRecordError(t *testing.T) {
	c, _ := NewCalibrator(nil)
	if err := c.Record(context.Background(), ConfidenceClaim{}); err != nil {
		t.Fatal(err)
	}
}

func TestBrierScoreNoData(t *testing.T) {
	db := sqlOpen(t)
	c, _ := NewCalibrator(db)
	_, n, err := c.BrierScore(context.Background(), "x")
	if err != nil || n != 0 {
		t.Fatalf("expected n=0, got n=%d err=%v", n, err)
	}
}

func TestCalibrateNoData(t *testing.T) {
	db := sqlOpen(t)
	c, _ := NewCalibrator(db)
	cal, err := c.Calibrate(context.Background(), "x", 0.8)
	if err != nil || cal != 0.65 {
		t.Fatalf("expected 0.65 for no data, got %f err=%v", cal, err)
	}
}

// ── semmerge.go ───────────────────────────────────────────────────

func TestDeclKeyGenDecl(t *testing.T) {
	src := []byte(`package x
import "fmt"
const C = 1
var X, Y = 1, 2
type T struct{}
func A() {}
`)
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	for _, d := range f.Decls {
		if k, ok := declKey(d); ok {
			keys[k] = true
		}
	}
	if !keys["import:\"fmt\""] || !keys["const:C"] || !keys["var:X"] || !keys["type:T"] || !keys["func:A"] {
		t.Fatalf("missing expected keys: %v", keys)
	}
}

// ── nim_agent.go ──────────────────────────────────────────────────

func TestNewLLMAgentWithClientError(t *testing.T) {
	// Use an invalid provider so ProviderFromConfig fails and client is nil.
	a := NewLLMAgentWithClient(AgentConfig{Provider: "invalid-provider"}, nil)
	if a.client != nil {
		t.Fatal("expected nil client for invalid provider")
	}
}

func TestNIMAgentPrimeContextError(t *testing.T) {
	old := memoryOpenHook
	memoryOpenHook = func(path string) (memoryStore, error) { return nil, errors.New("no memory") }
	defer func() { memoryOpenHook = old }()

	a := &LLMAgent{cfg: AgentConfig{Name: "n"}}
	_, err := a.primeContext(&Task{Description: "d"})
	if err == nil || !strings.Contains(err.Error(), "no memory") {
		t.Fatalf("expected memory error, got %v", err)
	}
}

func TestGovernorRunRungMultiAgentRepairFail(t *testing.T) {
	old := verifierRunCheck
	verifierRunCheck = func(ctx context.Context, c Check, workdir string) CheckResult {
		return CheckResult{Check: c, Passed: false}
	}
	defer func() { verifierRunCheck = old }()

	oldSpec := specWorktreeAdd
	specWorktreeAdd = func(ctx context.Context, repoRoot, dir string) ([]byte, error) { return nil, nil }
	defer func() { specWorktreeAdd = oldSpec }()

	g := &Governor{
		Ladder:   []Rung{{Name: "best-of-3", Agents: 2, RepairRounds: 1, Timeout: 5 * time.Second}},
		Verifier: NewVerifier(t.TempDir()),
		Checks:   []Check{{Kind: CheckBuild, Name: "c", Cmd: []string{"false"}}},
		Factory: func(rung Rung) []Agent {
			return []Agent{stcovAgent{name: "a1", err: errors.New("fail")}, stcovAgent{name: "a2", err: errors.New("fail")}}
		},
	}
	res, err := g.Execute(context.Background(), &Task{ID: "t"}, NewScratchpad())
	if err != nil || res.Passed || res.FinalRung != "best-of-3" {
		t.Fatalf("expected multi-agent fail, got %+v err=%v", res, err)
	}
}

func TestGovernorRunRungMergeError(t *testing.T) {
	old := verifierRunCheck
	verifierRunCheck = func(ctx context.Context, c Check, workdir string) CheckResult {
		return CheckResult{Check: c, Passed: true}
	}
	defer func() { verifierRunCheck = old }()

	oldSpec := specWorktreeAdd
	specWorktreeAdd = func(ctx context.Context, repoRoot, dir string) ([]byte, error) { return nil, nil }
	defer func() { specWorktreeAdd = oldSpec }()

	oldDiff := specDiff
	specDiff = func(ctx context.Context, worktree string) ([]byte, error) { return nil, errors.New("diff fail") }
	defer func() { specDiff = oldDiff }()

	g := &Governor{
		Ladder:   []Rung{{Name: "best-of-3", Agents: 2, RepairRounds: 0, Timeout: 5 * time.Second}},
		Verifier: NewVerifier(t.TempDir()),
		Checks:   []Check{{Kind: CheckBuild, Name: "c", Cmd: []string{"true"}}},
		RepoRoot: t.TempDir(),
		Factory: func(rung Rung) []Agent {
			return []Agent{stcovAgent{name: "a1", out: "ok"}, stcovAgent{name: "a2", out: "ok"}}
		},
	}
	res, err := g.Execute(context.Background(), &Task{ID: "t"}, NewScratchpad())
	if err == nil || !strings.Contains(err.Error(), "merge winner") {
		t.Fatalf("expected merge error, got %+v err=%v", res, err)
	}
}

// ── macros.go ─────────────────────────────────────────────────────

func TestSinChangePass(t *testing.T) {
	old := verifierRunCheck
	verifierRunCheck = func(ctx context.Context, c Check, workdir string) CheckResult {
		return CheckResult{Check: c, Passed: true}
	}
	defer func() { verifierRunCheck = old }()

	dir := t.TempDir()
	tv := NewTargetedVerifier(NewVerifier(dir), &ImpactGraph{nodes: map[string]*PkgNode{"repo/a": {}}})
	m := &Macros{
		Workdir:  dir,
		Contract: CompileContract(&Task{ID: "t"}),
		Targeted: tv,
		Policy:   DefaultMergePolicy(),
		Agent:    "agent",
	}
	res, err := m.SinChange(context.Background(), EditRequest{
		TaskID: "t",
		Edits:  map[string][]byte{"a.go": []byte("package main\n")},
	})
	if err != nil || !res.Applied || res.Decision != DecisionGreenReview {
		t.Fatalf("expected applied green review, got %+v err=%v", res, err)
	}
}

func TestSinChangeContractViolation(t *testing.T) {
	dir := t.TempDir()
	c := CompileContract(&Task{ID: "t"})
	c.AllowedGlobs = []string{"only_this.go"}
	m := &Macros{Workdir: dir, Contract: c}
	res, err := m.SinChange(context.Background(), EditRequest{
		TaskID: "t",
		Edits:  map[string][]byte{"other.go": []byte("x")},
	})
	if err != nil || res.Applied || res.Decision != DecisionBlock {
		t.Fatalf("expected block, got %+v err=%v", res, err)
	}
}

func TestSinChangeWriteError(t *testing.T) {
	dir := t.TempDir()
	m := &Macros{Workdir: dir, Contract: CompileContract(&Task{ID: "t"})}
	res, err := m.SinChange(context.Background(), EditRequest{
		TaskID: "t",
		Edits:  map[string][]byte{"": []byte("x")}, // empty rel path causes write error
	})
	if err != nil || res.Applied || !strings.Contains(res.ResumeContext, "apply failed") {
		t.Fatalf("expected write error, got %+v err=%v", res, err)
	}
}

func TestSinChangeTargetedNil(t *testing.T) {
	dir := t.TempDir()
	m := &Macros{Workdir: dir, Contract: CompileContract(&Task{ID: "t"})}
	_, err := m.SinChange(context.Background(), EditRequest{
		TaskID: "t",
		Edits:  map[string][]byte{"a.go": []byte("x")},
	})
	if err == nil || !strings.Contains(err.Error(), "TargetedVerifier not wired") {
		t.Fatalf("expected targeted nil error, got %v", err)
	}
}

func TestSinRefactor(t *testing.T) {
	old := verifierRunCheck
	verifierRunCheck = func(ctx context.Context, c Check, workdir string) CheckResult {
		return CheckResult{Check: c, Passed: true}
	}
	defer func() { verifierRunCheck = old }()

	dir := t.TempDir()
	tv := NewTargetedVerifier(NewVerifier(dir), &ImpactGraph{nodes: map[string]*PkgNode{"repo/a": {}}})
	m := &Macros{
		Workdir:  dir,
		Contract: CompileContract(&Task{ID: "t"}),
		Targeted: tv,
		Policy:   DefaultMergePolicy(),
		Agent:    "agent",
		Probe:    func(testCmd []string) *MutationProbe { return NewMutationProbe(dir, testCmd) },
	}
	res, err := m.SinRefactor(context.Background(), EditRequest{
		TaskID: "t",
		Edits:  map[string][]byte{"a.go": []byte("package main\n")},
	})
	if err != nil || !res.Applied {
		t.Fatalf("expected refactor applied, got %+v err=%v", res, err)
	}
}

// ── cartographer.go ───────────────────────────────────────────────

func TestCartographerIndexAllWalkError(t *testing.T) {
	c := NewCartographer("/nonexistent/path/that/should/not/exist")
	if err := c.IndexAll(context.Background()); err == nil || !strings.Contains(err.Error(), "cartographer walk") {
		t.Fatalf("expected cartographer walk error, got %v", err)
	}
}

func TestCartographerIndexAllContextCancel(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewCartographer(dir)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.IndexAll(ctx); err == nil || !strings.Contains(err.Error(), "context") {
		t.Fatalf("expected context error, got %v", err)
	}
}

func TestCartographerIndexFileParseError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("not valid go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewCartographer(dir)
	if err := c.IndexAll(context.Background()); err != nil {
		t.Fatalf("expected IndexAll to skip parse errors, got %v", err)
	}
}

func TestCartographerSliceForKGreater(t *testing.T) {
	c := NewCartographer(t.TempDir())
	c.symbols["x"] = &Symbol{Key: "x", File: "x.go", Rank: 1.0}
	items := c.SliceFor(nil, 5)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
}

func TestSignatureLineOutOfRange(t *testing.T) {
	if got := signatureLine("a\n", 5); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

// ── kernel.go ─────────────────────────────────────────────────────

func TestKernelCaptureNoDBWithWorkdir(t *testing.T) {
	dir := t.TempDir()
	k, _ := NewKernel(nil, dir)
	oldGit := kernelGitCmd
	oldGitOut := kernelGitOutCmd
	kernelGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) { return nil, nil }
	kernelGitOutCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) { return []byte("TREE"), nil }
	defer func() { kernelGitCmd = oldGit; kernelGitOutCmd = oldGitOut }()

	cp, err := k.Capture(context.Background(), "x", AgentState{}, true)
	if err != nil || cp.TreeSHA != "TREE" || cp.ID != 0 {
		t.Fatalf("expected capture without DB, got %+v err=%v", cp, err)
	}
}

func TestKernelRewindWorkdirEmpty(t *testing.T) {
	db := sqlOpen(t)
	k, _ := NewKernel(db, "")

	stateJSON := `{"task_id":"t"}`
	_, err := db.Exec(`INSERT INTO checkpoints (label, tree_sha, state_json, green, created_at) VALUES (?, ?, ?, ?, ?)`,
		"green", "", stateJSON, 1, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	err = db.QueryRow("SELECT id FROM checkpoints ORDER BY id DESC LIMIT 1").Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	state, err := k.Rewind(context.Background(), id)
	if err != nil || state.TaskID != "t" {
		t.Fatalf("expected rewind without workdir, got %+v err=%v", state, err)
	}
}

func TestKernelRewindReadTreeError(t *testing.T) {
	db := sqlOpen(t)
	dir := t.TempDir()
	k, _ := NewKernel(db, dir)

	oldGit := kernelGitCmd
	oldGitOut := kernelGitOutCmd
	kernelGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) { return nil, nil }
	kernelGitOutCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) { return []byte("TREE"), nil }
	defer func() { kernelGitCmd = oldGit; kernelGitOutCmd = oldGitOut }()

	_, err := k.Capture(context.Background(), "green", AgentState{TaskID: "t"}, true)
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	err = db.QueryRow("SELECT id FROM checkpoints ORDER BY id DESC LIMIT 1").Scan(&id)
	if err != nil {
		t.Fatal(err)
	}

	// Now make read-tree fail during Rewind.
	kernelGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "read-tree" {
			return []byte("err"), errors.New("read-tree fail")
		}
		return nil, nil
	}

	_, err = k.Rewind(context.Background(), id)
	if err == nil || !strings.Contains(err.Error(), "read-tree") {
		t.Fatalf("expected read-tree error, got %v", err)
	}
}

func TestKernelLastGreenFound(t *testing.T) {
	db := sqlOpen(t)
	dir := t.TempDir()
	k, _ := NewKernel(db, dir)

	oldGit := kernelGitCmd
	oldGitOut := kernelGitOutCmd
	kernelGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) { return nil, nil }
	kernelGitOutCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) { return []byte("TREE"), nil }
	defer func() { kernelGitCmd = oldGit; kernelGitOutCmd = oldGitOut }()

	_, err := k.Capture(context.Background(), "green", AgentState{TaskID: "t"}, true)
	if err != nil {
		t.Fatal(err)
	}
	id, label, err := k.LastGreen(context.Background())
	if err != nil || id == 0 || label == "" {
		t.Fatalf("expected green checkpoint, got id=%d label=%q err=%v", id, label, err)
	}
}

func TestKernelWriteTreeRestoreHead(t *testing.T) {
	dir := t.TempDir()
	k, _ := NewKernel(nil, dir)
	oldGit := kernelGitCmd
	oldGitOut := kernelGitOutCmd
	kernelGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) { return nil, nil }
	kernelGitOutCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) { return []byte("HEAD"), nil }
	defer func() { kernelGitCmd = oldGit; kernelGitOutCmd = oldGitOut }()

	_, err := k.writeTree(context.Background())
	if err != nil {
		t.Fatalf("expected writeTree success, got %v", err)
	}
}

// ── semmerge.go ───────────────────────────────────────────────────

func TestSemanticMergeGoConflict(t *testing.T) {
	base := []byte("package x\nfunc A() { return 1 }\n")
	a := []byte("package x\nfunc A() { return 2 }\n")
	b := []byte("package x\nfunc A() { return 3 }\n")
	res, err := SemanticMergeGo(base, a, b)
	if err != nil || len(res.Conflicts) == 0 {
		t.Fatalf("expected conflict, got %+v err=%v", res, err)
	}
}

func TestExtractDeclsWithDoc(t *testing.T) {
	src := []byte(`package x
// Foo is a thing.
func Foo() {}
`)
	decls, err := extractDecls(src)
	if err != nil || len(decls) != 1 {
		t.Fatalf("expected one decl, got %v err=%v", decls, err)
	}
}

func TestDeclKeyGenDeclGroup(t *testing.T) {
	src := []byte(`package x
const (
	A = 1
	B = 2
)
`)
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range f.Decls {
		if k, ok := declKey(d); !ok || !strings.Contains(k, "group:") {
			t.Fatalf("expected group key, got %q", k)
		}
	}
}

// ── nim_agent.go ──────────────────────────────────────────────────

func TestNIMAgentPrimeContextReturned(t *testing.T) {
	old := memoryOpenHook
	memoryOpenHook = func(path string) (memoryStore, error) {
		return &memoryStoreStub{prime: "prior memory"}, nil
	}
	defer func() { memoryOpenHook = old }()

	a := &LLMAgent{cfg: AgentConfig{Name: "n"}}
	ctx, err := a.primeContext(&Task{Description: "d"})
	if err != nil || ctx != "prior memory" {
		t.Fatalf("expected prior memory, got %q err=%v", ctx, err)
	}
}

func TestNIMAgentPrimeContextPrimeError(t *testing.T) {
	old := memoryOpenHook
	memoryOpenHook = func(path string) (memoryStore, error) {
		return &memoryStoreStub{primeErr: errors.New("prime fail")}, nil
	}
	defer func() { memoryOpenHook = old }()

	a := &LLMAgent{cfg: AgentConfig{Name: "n"}}
	_, err := a.primeContext(&Task{Description: "d"})
	if err == nil || !strings.Contains(err.Error(), "prime fail") {
		t.Fatalf("expected prime error, got %v", err)
	}
}

func TestInferProviderFromEnvAll(t *testing.T) {
	t.Setenv("SIN_NIM_API_KEY", "x")
	if got := inferProviderFromEnv(); got != "nim" {
		t.Fatalf("expected nim, got %q", got)
	}
	t.Setenv("SIN_NIM_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "x")
	if got := inferProviderFromEnv(); got != "openai" {
		t.Fatalf("expected openai, got %q", got)
	}
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "x")
	if got := inferProviderFromEnv(); got != "anthropic" {
		t.Fatalf("expected anthropic, got %q", got)
	}
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GROQ_API_KEY", "x")
	if got := inferProviderFromEnv(); got != "groq" {
		t.Fatalf("expected groq, got %q", got)
	}
}

func TestLoadSystemPromptFromEnvDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIN_AGENTS_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "sys.md"), []byte("prompt"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &LLMAgent{cfg: AgentConfig{Name: "n", SystemFile: "sys.md"}}
	p, err := a.loadSystemPrompt()
	if err != nil || p != "prompt" {
		t.Fatalf("expected prompt from env dir, got %q err=%v", p, err)
	}
}

type memoryStoreStub struct {
	prime    string
	primeErr error
}

func (m *memoryStoreStub) Prime(query, project string, topK int) (string, error) {
	return m.prime, m.primeErr
}
func (m *memoryStoreStub) Close() error { return nil }

// ── speculative.go ─────────────────────────────────────────────────

func TestSpeculativeRunCandidateWorktreeError(t *testing.T) {
	old := specWorktreeAdd
	specWorktreeAdd = func(ctx context.Context, repoRoot, dir string) ([]byte, error) { return nil, errors.New("fail") }
	defer func() { specWorktreeAdd = old }()

	s := NewSpeculativeRunner(t.TempDir(), nil)
	c := s.runCandidate(context.Background(), &Task{ID: "t"}, stcovAgent{name: "a"}, 0, NewScratchpad())
	if c == nil || c.Err == nil || c.Verdict == nil || c.Verdict.Passed {
		t.Fatalf("expected failed candidate, got %+v", c)
	}
}

func TestSpeculativeAddWorktreeMkdirError(t *testing.T) {
	s := NewSpeculativeRunner("", nil)
	s.WorkdirBase = "/proc/invalid-base"
	_, err := s.addWorktree(context.Background(), "id")
	if err == nil {
		t.Fatal("expected mkdir error")
	}
}

func TestSpeculativeMergeWinnerKeepLosers(t *testing.T) {
	oldDiff := specDiff
	oldApply := specApply
	specDiff = func(ctx context.Context, worktree string) ([]byte, error) { return []byte("diff"), nil }
	specApply = func(ctx context.Context, repoRoot string, diff []byte) ([]byte, error) { return nil, nil }
	defer func() { specDiff = oldDiff; specApply = oldApply }()

	s := NewSpeculativeRunner(t.TempDir(), nil)
	s.KeepLosers = true
	_, err := s.MergeWinner(context.Background(), &Candidate{Worktree: t.TempDir()})
	if err != nil {
		t.Fatalf("expected merge success, got %v", err)
	}
}

// ── txn.go ─────────────────────────────────────────────────────────

func TestTxnDeleteFileSnapshotError(t *testing.T) {
	txn := BeginTxn(t.TempDir())
	err := txn.DeleteFile("nonexistent")
	if err == nil || !strings.Contains(err.Error(), "snapshot-for-delete") {
		t.Fatalf("expected snapshot error, got %v", err)
	}
}

func TestTxnCommitAfterRollback(t *testing.T) {
	txn := BeginTxn(t.TempDir())
	if err := txn.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := txn.Commit(); err == nil || !strings.Contains(err.Error(), "already rolled back") {
		t.Fatalf("expected commit after rollback error, got %v", err)
	}
}

func TestTxnRollbackRestoreError(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("orig"), 0o600); err != nil {
		t.Fatal(err)
	}

	txn := BeginTxn(dir)
	if err := txn.WriteFile("a/b", []byte("x")); err != nil {
		t.Fatal(err)
	}
	// Remove parent dir so restoring the original snapshot fails.
	if err := os.RemoveAll(filepath.Join(dir, "a")); err != nil {
		t.Fatal(err)
	}
	err := txn.Rollback()
	if err == nil || !strings.Contains(err.Error(), "txn rollback") {
		t.Fatalf("expected rollback error, got %v", err)
	}
}

// ── additional targeted coverage tests ───────────────────────────────

// zeroSource is a deterministic rand.Source64 that always returns 0, used to
// hit the x+y==0 guard in sampleBeta.
type zeroSource struct{}

func (zeroSource) Int63() int64   { return 0 }
func (zeroSource) Seed(int64)     {}
func (zeroSource) Uint64() uint64 { return 0 }

// ── confidence.go ───────────────────────────────────────────────────

func TestNewCalibratorSchemaError(t *testing.T) {
	db := sqlOpen(t)
	db.Close()
	if _, err := NewCalibrator(db); err == nil || !strings.Contains(err.Error(), "calibrator schema") {
		t.Fatalf("expected calibrator schema error, got %v", err)
	}
}

func TestCalibratorTenRecords(t *testing.T) {
	db := sqlOpen(t)
	c, err := NewCalibrator(db)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if err := c.Record(context.Background(), ConfidenceClaim{AgentName: "a", Declared: 0.8, Passed: i%2 == 0}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := c.Calibrate(context.Background(), "a", 0.8)
	if err != nil || got <= 0 || got >= 1 {
		t.Fatalf("expected calibrated value in (0,1), got %v err=%v", got, err)
	}
}

func TestBrierScoreEmpty(t *testing.T) {
	db := sqlOpen(t)
	c, err := NewCalibrator(db)
	if err != nil {
		t.Fatal(err)
	}
	score, n, err := c.BrierScore(context.Background(), "a")
	if err != nil || score != 0 || n != 0 {
		t.Fatalf("expected empty brier score, got %v/%d err=%v", score, n, err)
	}
}

func TestMergePolicyDecideBranches(t *testing.T) {
	p := MergePolicy{AutoMergeThreshold: 0.85, ReviewThreshold: 0.6}
	if got := p.Decide(false, 0.9); got != DecisionBlock {
		t.Fatalf("expected block, got %q", got)
	}
	if got := p.Decide(true, 0.9); got != DecisionAutoMerge {
		t.Fatalf("expected auto-merge, got %q", got)
	}
	if got := p.Decide(true, 0.7); got != DecisionGreenReview {
		t.Fatalf("expected green-review, got %q", got)
	}
	if got := p.Decide(true, 0.5); got != DecisionGreenReview {
		t.Fatalf("expected green-review below threshold, got %q", got)
	}
}

// ── strategy_router.go ──────────────────────────────────────────────

func TestStrategyRouterReportDBError(t *testing.T) {
	db := sqlOpen(t)
	r, err := NewStrategyRouter(db, 1)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := r.Report(context.Background(), ClassUnknown, StratASTEdit, true); err == nil {
		t.Fatal("expected db write error")
	}
}

func TestSampleBetaZeroSum(t *testing.T) {
	// With an always-zero rng and tiny alpha/beta, both gamma draws become 0,
	// exercising the x+y==0 guard in sampleBeta.
	rng := rand.New(zeroSource{})
	got := sampleBeta(rng, 0.0001, 0.0001)
	if got != 0.5 {
		t.Fatalf("expected 0.5 from zero-sum guard, got %v", got)
	}
}

func TestStrategyRouterLoadRowsError(t *testing.T) {
	db := sqlOpen(t)
	// Create the table first, then corrupt the column type so Scan fails.
	if _, err := NewStrategyRouter(db, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO router_arms (class, strategy, alpha, beta) VALUES (?, ?, ?, ?)`,
		"x", "y", 1, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ALTER TABLE router_arms RENAME COLUMN alpha TO old_alpha`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ALTER TABLE router_arms ADD COLUMN alpha TEXT`); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStrategyRouter(db, 1); err == nil {
		t.Fatal("expected load rows error")
	}
}

// ── kernel.go ───────────────────────────────────────────────────────

func TestKernelRewindEmptyTreeSHA(t *testing.T) {
	db := sqlOpen(t)
	k, _ := NewKernel(db, t.TempDir())
	_, err := db.Exec(`INSERT INTO checkpoints (label, tree_sha, state_json, green, created_at) VALUES (?, ?, ?, ?, ?)`,
		"empty", "", `{"task_id":"t"}`, 1, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := db.QueryRow("SELECT id FROM checkpoints ORDER BY id DESC LIMIT 1").Scan(&id); err != nil {
		t.Fatal(err)
	}
	state, err := k.Rewind(context.Background(), id)
	if err != nil || state.TaskID != "t" {
		t.Fatalf("expected rewind with empty tree, got %+v err=%v", state, err)
	}
}

func TestKernelWriteTreeAddError(t *testing.T) {
	k, _ := NewKernel(nil, t.TempDir())
	old := kernelGitCmd
	kernelGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "add" {
			return []byte("err"), errors.New("add fail")
		}
		return nil, nil
	}
	defer func() { kernelGitCmd = old }()

	_, err := k.writeTree(context.Background())
	if err == nil || !strings.Contains(err.Error(), "add") {
		t.Fatalf("expected add error, got %v", err)
	}
}

func TestKernelWriteTreeWriteTreeError(t *testing.T) {
	k, _ := NewKernel(nil, t.TempDir())
	oldOut := kernelGitOutCmd
	oldGit := kernelGitCmd
	kernelGitOutCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "write-tree" {
			return nil, errors.New("write-tree fail")
		}
		return []byte("HEAD"), nil
	}
	kernelGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) { return nil, nil }
	defer func() { kernelGitOutCmd = oldOut; kernelGitCmd = oldGit }()

	_, err := k.writeTree(context.Background())
	if err == nil || !strings.Contains(err.Error(), "write-tree") {
		t.Fatalf("expected write-tree error, got %v", err)
	}
}

// ── macros.go ───────────────────────────────────────────────────────

func TestSinRefactorProbeLowObservability(t *testing.T) {
	old := verifierRunCheck
	verifierRunCheck = func(ctx context.Context, c Check, workdir string) CheckResult {
		return CheckResult{Check: c, Passed: true}
	}
	defer func() { verifierRunCheck = old }()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\n\nfunc A() { return true }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a_test.go"), []byte("package main\nimport \"testing\"\nfunc TestA(t *testing.T) {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module probe\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ig := &ImpactGraph{
		nodes:     map[string]*PkgNode{"repo/a": {TestFiles: []string{"a_test.go"}}},
		fileToPkg: map[string]string{"a.go": "repo/a", "a_test.go": "repo/a"},
	}
	tv := NewTargetedVerifier(NewVerifier(dir), ig)
	m := &Macros{
		Workdir:  dir,
		Contract: CompileContract(&Task{ID: "t"}),
		Targeted: tv,
		Policy:   DefaultMergePolicy(),
		Agent:    "agent",
		Probe: func(testCmd []string) *MutationProbe {
			mp := NewMutationProbe(dir, testCmd)
			mp.MaxMutations = 1
			return mp
		},
	}

	res, err := m.SinRefactor(context.Background(), EditRequest{
		TaskID: "t", Edits: map[string][]byte{"a.go": []byte("package main\n\nfunc A() { return false }\n")},
	})
	if err != nil || !res.Applied || res.Decision != DecisionGreenReview {
		t.Fatalf("expected refactor applied with green review, got %+v err=%v", res, err)
	}
}

// ── miner.go ────────────────────────────────────────────────────────

func TestNewMinerSchemaError(t *testing.T) {
	db := sqlOpen(t)
	db.Close()
	if _, err := NewMiner(db); err == nil || !strings.Contains(err.Error(), "miner schema") {
		t.Fatalf("expected miner schema error, got %v", err)
	}
}

func TestMinerRecordSequenceNilDB(t *testing.T) {
	var m *Miner
	if err := m.RecordSequence(context.Background(), SeqEpisode{}); err != nil {
		t.Fatalf("expected nil db record to be no-op, got %v", err)
	}
}

func TestMinerSuggestionsForEmpty(t *testing.T) {
	db := sqlOpen(t)
	m, err := NewMiner(db)
	if err != nil {
		t.Fatal(err)
	}
	got, err := m.SuggestionsFor(context.Background(), ClassUnknown)
	if err != nil || got != "" {
		t.Fatalf("expected empty suggestions, got %q err=%v", got, err)
	}
}

// ── nim_agent.go ────────────────────────────────────────────────────

func TestNewLLMAgentWithClientBadProvider(t *testing.T) {
	a := NewLLMAgentWithClient(AgentConfig{Provider: "bad-provider"}, nil)
	if a.client != nil {
		t.Fatal("expected nil client for bad provider")
	}
}

func TestLLMAgentRunNoClient(t *testing.T) {
	a := &LLMAgent{cfg: AgentConfig{Name: "n"}}
	if _, err := a.Run(context.Background(), &Task{ID: "t"}, NewScratchpad()); err == nil {
		t.Fatal("expected error for no client")
	}
}

func TestLLMAgentLoadSystemPromptDefault(t *testing.T) {
	a := &LLMAgent{cfg: AgentConfig{Name: "n"}}
	p, err := a.loadSystemPrompt()
	if err != nil || !strings.Contains(p, "You are n") {
		t.Fatalf("expected default prompt, got %q err=%v", p, err)
	}
}

// ── mutation.go ──────────────────────────────────────────────────────

func TestProbeDiagnosisWithSurvivors(t *testing.T) {
	p := &ProbeResult{
		Mutations: []Mutation{{File: "a.go", Line: 1, Before: "true", After: "false", Rule: "true-to-false", Killed: false}},
		Killed:    0, Survived: 1,
	}
	if d := p.Diagnosis(); !strings.Contains(d, "SURVIVED") {
		t.Fatalf("expected survivor diagnosis, got %q", d)
	}
}

func TestMutationProbeRunEmptyLines(t *testing.T) {
	mp := NewMutationProbe(t.TempDir(), []string{"go", "test"})
	res, err := mp.Run(context.Background(), nil)
	if err != nil || res.ObservabilityScore != 1.0 {
		t.Fatalf("expected empty run to score 1.0, got %+v err=%v", res, err)
	}
}

func TestMutationProbeApplyAndTestOutOfRange(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mp := &MutationProbe{Workdir: dir, TestCmd: []string{"go", "test"}}
	_, err := mp.applyAndTest(context.Background(), ChangedLine{File: "a.go", Line: 100, Text: "x"}, "y")
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("expected out of range error, got %v", err)
	}
}

// ── governor.go ───────────────────────────────────────────────────────

func TestGovernorRunRungNoAgents(t *testing.T) {
	g := &Governor{
		Ladder:  DefaultLadder(),
		Factory: func(rung Rung) []Agent { return nil },
	}
	_, _, err := g.runRung(context.Background(), DefaultLadder()[0], &Task{ID: "t"}, NewScratchpad())
	if err == nil || !strings.Contains(err.Error(), "no agents") {
		t.Fatalf("expected no agents error, got %v", err)
	}
}

// ── episodic.go ───────────────────────────────────────────────────────

func TestNewEpisodeStoreSchemaError(t *testing.T) {
	db := sqlOpen(t)
	db.Close()
	if _, err := NewEpisodeStore(db); err == nil || !strings.Contains(err.Error(), "episodes schema") {
		t.Fatalf("expected episodes schema error, got %v", err)
	}
}

func TestEpisodeStoreSimilarNonPositiveK(t *testing.T) {
	db := sqlOpen(t)
	s, err := NewEpisodeStore(db)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.Similar(context.Background(), "task", 0)
	if err != nil || out != nil {
		t.Fatalf("expected nil for k<=0, got %+v err=%v", out, err)
	}
}

// ── leases.go ─────────────────────────────────────────────────────────

func TestLeaseAcquireEmptyGlobs(t *testing.T) {
	lt := NewLeaseTable()
	if _, _, err := lt.Acquire("a", "t", "i", nil); err == nil || !strings.Contains(err.Error(), "empty glob set") {
		t.Fatalf("expected empty glob error, got %v", err)
	}
}

func TestLeaseBoardNonEmpty(t *testing.T) {
	lt := NewLeaseTable()
	l, _, err := lt.Acquire("a", "t", "intent", []string{"*.go"})
	if err != nil {
		t.Fatal(err)
	}
	if got := lt.Board(); !strings.Contains(got, "a") || !strings.Contains(got, "*.go") {
		t.Fatalf("expected board to contain agent and globs, got %q", got)
	}
	_ = l
}

// ── cartographer.go ───────────────────────────────────────────────────

func TestCartographerSliceForAffectedPkg(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\nfunc A() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewCartographer(dir)
	if err := c.IndexAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	items := c.SliceFor(&Impact{AffectedPkgs: []string{"a"}}, 10)
	if len(items) == 0 {
		t.Fatal("expected items for affected package")
	}
}

func TestCartographerComputeRankWithEdges(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\nfunc A() {}\nfunc B() { A() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewCartographer(dir)
	if err := c.IndexAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sym := c.symbols["a.A"]; sym.Rank <= 0 {
		t.Fatalf("expected positive rank for A, got %v", sym.Rank)
	}
}

func TestCartographerIndexFileNoRoot(t *testing.T) {
	c := NewCartographer("")
	if err := c.indexFile("/totally/unrelated/path.go"); err == nil || !strings.Contains(err.Error(), "no repo root") {
		t.Fatalf("expected no repo root error, got %v", err)
	}
}

// ── additional txn.go branches ────────────────────────────────────

func TestTxnWriteFileAlreadyFinished(t *testing.T) {
	txn := BeginTxn(t.TempDir())
	if err := txn.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := txn.WriteFile("x", []byte("y")); err == nil || !strings.Contains(err.Error(), "already finished") {
		t.Fatalf("expected already finished error, got %v", err)
	}
}

func TestTxnWriteFileMkdirError(t *testing.T) {
	dir := t.TempDir()
	txn := BeginTxn(dir)
	// First write primes the snapshot for x.go/sub while x.go is a directory.
	if err := txn.WriteFile("x.go/sub", []byte("y")); err != nil {
		t.Fatal(err)
	}
	// Replace x.go directory with a regular file so MkdirAll(x.go) fails on the next write.
	if err := os.RemoveAll(filepath.Join(dir, "x.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := txn.WriteFile("x.go/sub", []byte("z")); err == nil || !strings.Contains(err.Error(), "txn mkdir") {
		t.Fatalf("expected mkdir error, got %v", err)
	}
}

func TestTxnDeleteFileAlreadySeen(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "a")
	if err := os.WriteFile(abs, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	txn := BeginTxn(dir)
	if err := txn.DeleteFile("a"); err != nil {
		t.Fatal(err)
	}
	if err := txn.DeleteFile("a"); err == nil || !os.IsNotExist(err) {
		t.Fatalf("expected file not exist on second delete, got %v", err)
	}
}

func TestTxnRollbackAlreadyFinished(t *testing.T) {
	txn := BeginTxn(t.TempDir())
	if err := txn.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := txn.Rollback(); err != nil {
		t.Fatalf("expected rollback after commit to be no-op, got %v", err)
	}
}

// ── semmerge.go branches ────────────────────────────────────────────

func TestExtractDeclsParseError(t *testing.T) {
	if _, err := extractDecls([]byte("not go at all")); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestDeclKeyGenDeclNoSpecs(t *testing.T) {
	src := []byte(`package x
var ()`)
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range f.Decls {
		if k, ok := declKey(d); !ok || !strings.Contains(k, "group:var:0") {
			t.Fatalf("expected empty group key, got %q", k)
		}
	}
}

func TestDeclKeyGenDeclValueNoNames(t *testing.T) {
	src := []byte(`package x
var _ = 1`)
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range f.Decls {
		if k, ok := declKey(d); !ok || k != "var:_" {
			t.Fatalf("expected var:_ key, got %q", k)
		}
	}
}

func TestSemanticMergeGoParseError(t *testing.T) {
	base := []byte("package x\nfunc A() {}\n")
	a := []byte("not valid go")
	if _, err := SemanticMergeGo(base, a, base); err == nil || !strings.Contains(err.Error(), "semmerge parse A") {
		t.Fatalf("expected parse A error, got %v", err)
	}
}

// ── registry.go branches ────────────────────────────────────────────

func TestLoadUserAgentsDecodeError(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "bad")
	if err := os.MkdirAll(agentDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "agent.toml"), []byte("not valid toml"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadUserAgents(dir); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestLoadUserAgentsNameFallback(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "fallback")
	if err := os.MkdirAll(agentDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "agent.toml"), []byte("model = \"x\""), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadUserAgents(dir)
	if err != nil || len(cfg) != 1 || cfg[0].Name != "fallback" {
		t.Fatalf("expected name fallback, got %+v err=%v", cfg, err)
	}
}

// ── planner.go branches ─────────────────────────────────────────────

func TestNeedsArchitectMultipleIntents(t *testing.T) {
	p := NewPlanner(nil)
	plan := p.BuildPlan("write code and tests")
	hasArchitect := false
	for _, task := range plan.Tasks {
		if task.Type == TaskArchitect {
			hasArchitect = true
		}
	}
	if !hasArchitect {
		t.Fatal("expected architect task for multiple intents")
	}
}

func TestPromptForTaskAllIntents(t *testing.T) {
	cases := []struct {
		intent Intent
		want   string
	}{
		{IntentCodebase, "Implement: "},
		{IntentTest, "Write tests for: "},
		{IntentReview, "Review: "},
		{IntentDocs, "Document: "},
		{IntentSecurity, "Security review: "},
		{IntentArchitecture, "Architect: "},
		{IntentGeneral, "prompt"},
	}
	for _, c := range cases {
		got := promptForTask(c.intent, "prompt")
		if !strings.HasPrefix(got, c.want) {
			t.Fatalf("intent %v: expected prefix %q, got %q", c.intent, c.want, got)
		}
	}
}

// ── strategy_router.go branches ───────────────────────────────────

func TestStrategyRouterNewSchemaError(t *testing.T) {
	db := sqlOpen(t)
	db.Close()
	if _, err := NewStrategyRouter(db, 1); err == nil || !strings.Contains(err.Error(), "router schema") {
		t.Fatalf("expected router schema error, got %v", err)
	}
}

func TestStrategyRouterReportNoDB(t *testing.T) {
	r, err := NewStrategyRouter(nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Report(context.Background(), ClassUnknown, StratASTEdit, true); err != nil {
		t.Fatalf("expected no-db report to be no-op, got %v", err)
	}
}

func TestStrategyRouterSampleGammaShapeLessThanOne(t *testing.T) {
	// Should not panic for shape < 1.
	rng := rand.New(rand.NewSource(1))
	_ = sampleGamma(rng, 0.5)
}

func TestStrategyRouterPickEmptyCandidates(t *testing.T) {
	r, err := NewStrategyRouter(nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Pick(ClassUnknown, nil); got != "" {
		t.Fatalf("expected empty strategy for empty candidates, got %q", got)
	}
}

// ── kernel.go branches ──────────────────────────────────────────────

func TestNewKernelSchemaError(t *testing.T) {
	db := sqlOpen(t)
	db.Close()
	if _, err := NewKernel(db, t.TempDir()); err == nil || !strings.Contains(err.Error(), "kernel schema") {
		t.Fatalf("expected kernel schema error, got %v", err)
	}
}

func TestKernelWriteTreeRestoreHeadIndex(t *testing.T) {
	k, _ := NewKernel(nil, t.TempDir())
	oldOut := kernelGitOutCmd
	oldGit := kernelGitCmd
	call := 0
	kernelGitOutCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		call++
		if call == 1 {
			return []byte("HEAD"), nil
		}
		return []byte("TREE"), nil
	}
	kernelGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) { return nil, nil }
	defer func() { kernelGitOutCmd = oldOut; kernelGitCmd = oldGit }()

	sha, err := k.writeTree(context.Background())
	if err != nil || sha != "TREE" {
		t.Fatalf("expected write-tree restore head, got %q err=%v", sha, err)
	}
}

func TestKernelRewindCheckoutIndexError(t *testing.T) {
	db := sqlOpen(t)
	dir := t.TempDir()
	k, _ := NewKernel(db, dir)

	oldGit := kernelGitCmd
	oldGitOut := kernelGitOutCmd
	kernelGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) { return nil, nil }
	kernelGitOutCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) { return []byte("TREE"), nil }
	defer func() { kernelGitCmd = oldGit; kernelGitOutCmd = oldGitOut }()

	_, err := k.Capture(context.Background(), "green", AgentState{TaskID: "t"}, true)
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	err = db.QueryRow("SELECT id FROM checkpoints ORDER BY id DESC LIMIT 1").Scan(&id)
	if err != nil {
		t.Fatal(err)
	}

	kernelGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "checkout-index" {
			return []byte("err"), errors.New("checkout-index fail")
		}
		return nil, nil
	}

	_, err = k.Rewind(context.Background(), id)
	if err == nil || !strings.Contains(err.Error(), "checkout-index") {
		t.Fatalf("expected checkout-index error, got %v", err)
	}
}

// ── macros.go SinRefactor error paths ───────────────────────────────

func TestSinRefactorSinChangeFailed(t *testing.T) {
	old := verifierRunCheck
	verifierRunCheck = func(ctx context.Context, c Check, workdir string) CheckResult {
		return CheckResult{Check: c, Passed: false}
	}
	defer func() { verifierRunCheck = old }()

	dir := t.TempDir()
	tv := NewTargetedVerifier(NewVerifier(dir), &ImpactGraph{nodes: map[string]*PkgNode{"repo/a": {}}})
	m := &Macros{
		Workdir:  dir,
		Contract: CompileContract(&Task{ID: "t"}),
		Targeted: tv,
		Policy:   DefaultMergePolicy(),
		Agent:    "agent",
		Probe:    func(testCmd []string) *MutationProbe { return NewMutationProbe(dir, testCmd) },
	}

	res, err := m.SinRefactor(context.Background(), EditRequest{
		TaskID: "t",
		Edits:  map[string][]byte{"a.go": []byte("package main\n")},
	})
	if err != nil || res.Applied || !strings.Contains(res.ResumeContext, "blast radius") {
		t.Fatalf("expected failed refactor with risk context, got applied=%v err=%v ctx=%q", res.Applied, err, res.ResumeContext)
	}
}

// ── further targeted coverage tests ─────────────────────────────────

// ── txn.go ───────────────────────────────────────────────────────────

func TestTxnWriteFileSnapshotReadError(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "a")
	if err := os.WriteFile(abs, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Remove read permission so the second snapshot read fails.
	if err := os.Chmod(abs, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(abs, 0o600)

	txn := BeginTxn(dir)
	if err := txn.WriteFile("a", []byte("y")); err == nil || !strings.Contains(err.Error(), "txn snapshot") {
		t.Fatalf("expected snapshot error, got %v", err)
	}
}

func TestTxnDeleteFileAlreadyFinished(t *testing.T) {
	txn := BeginTxn(t.TempDir())
	if err := txn.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := txn.DeleteFile("x"); err == nil || !strings.Contains(err.Error(), "already finished") {
		t.Fatalf("expected already finished error, got %v", err)
	}
}

// ── semmerge.go ──────────────────────────────────────────────────────

func TestSemanticMergeGoBaseParseError(t *testing.T) {
	base := []byte("not valid go")
	a := []byte("package x\nfunc A() {}\n")
	if _, err := SemanticMergeGo(base, a, a); err == nil || !strings.Contains(err.Error(), "semmerge parse base") {
		t.Fatalf("expected parse base error, got %v", err)
	}
}

func TestSemanticMergeGoBParseError(t *testing.T) {
	base := []byte("package x\nfunc A() {}\n")
	a := []byte("package x\nfunc A() {}\n")
	b := []byte("not valid go")
	if _, err := SemanticMergeGo(base, a, b); err == nil || !strings.Contains(err.Error(), "semmerge parse B") {
		t.Fatalf("expected parse B error, got %v", err)
	}
}

func TestExtractDeclsGenDeclDoc(t *testing.T) {
	src := []byte(`package x
// C is a const.
const C = 1
`)
	decls, err := extractDecls(src)
	if err != nil || len(decls) != 1 || !strings.Contains(decls["const:C"].Src, "C is a const") {
		t.Fatalf("expected doc in decl source, got %+v err=%v", decls, err)
	}
}

func TestDeclKeyImportSpec(t *testing.T) {
	src := []byte(`package x
import "fmt"`)
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range f.Decls {
		if k, ok := declKey(d); !ok || !strings.Contains(k, "import:") {
			t.Fatalf("expected import key, got %q", k)
		}
	}
}

// ── governor.go ───────────────────────────────────────────────────────

// ── nim_agent.go ──────────────────────────────────────────────────────

func TestLLMAgentLoadSystemPromptEnvCandidate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIN_AGENTS_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "sys.md"), []byte("env"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &LLMAgent{cfg: AgentConfig{Name: "n", SystemFile: "sys.md"}}
	p, err := a.loadSystemPrompt()
	if err != nil || p != "env" {
		t.Fatalf("expected env prompt, got %q err=%v", p, err)
	}
}

// ── miner.go ──────────────────────────────────────────────────────────

func TestMinerRecordSequenceDBError(t *testing.T) {
	db := sqlOpen(t)
	m, err := NewMiner(db)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := m.RecordSequence(context.Background(), SeqEpisode{EpisodeID: 1, Sequence: []ToolCall{{Tool: "x"}}}); err == nil {
		t.Fatal("expected db error")
	}
}

func TestMinerMineQualifies(t *testing.T) {
	db := sqlOpen(t)
	m, err := NewMiner(db)
	if err != nil {
		t.Fatal(err)
	}
	seq := []ToolCall{{Tool: "a"}, {Tool: "b"}, {Tool: "c"}, {Tool: "d"}, {Tool: "e"}}
	for i := 0; i < 5; i++ {
		if err := m.RecordSequence(context.Background(), SeqEpisode{EpisodeID: int64(i), Class: ClassUnknown, Sequence: seq, Passed: true}); err != nil {
			t.Fatal(err)
		}
	}
	fresh, err := m.Mine(context.Background(), ClassUnknown)
	if err != nil || len(fresh) == 0 {
		t.Fatalf("expected mined templates, got %+v err=%v", fresh, err)
	}
}

// ── mutation.go ───────────────────────────────────────────────────────

func TestProbeDiagnosisNoSurvivors(t *testing.T) {
	p := &ProbeResult{Killed: 1, Mutations: []Mutation{{Killed: true}}}
	if d := p.Diagnosis(); !strings.Contains(d, "1/1") {
		t.Fatalf("expected all-killed diagnosis, got %q", d)
	}
}

func TestMutationProbeApplyAndTestLineMismatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\n// x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mp := &MutationProbe{Workdir: dir, TestCmd: []string{"go", "test"}}
	killed, err := mp.applyAndTest(context.Background(), ChangedLine{File: "a.go", Line: 2, Text: "// different"}, "// mutated")
	if err != nil || !killed {
		t.Fatalf("expected killed due to line mismatch, got killed=%v err=%v", killed, err)
	}
}

// ── confidence.go ────────────────────────────────────────────────────

func TestCalibrateLocalNZero(t *testing.T) {
	db := sqlOpen(t)
	c, err := NewCalibrator(db)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		// All declared values are far from 0.99 so local bin is empty.
		declared := float64(i) / 20.0 // 0.0 .. 0.45
		if err := c.Record(context.Background(), ConfidenceClaim{AgentName: "a", Declared: declared, Passed: i%2 == 0}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := c.Calibrate(context.Background(), "a", 0.99)
	if err != nil || got <= 0 || got >= 1 {
		t.Fatalf("expected global calibration in (0,1), got %v err=%v", got, err)
	}
}

func TestBrierScoreRowsError(t *testing.T) {
	db := sqlOpen(t)
	c, err := NewCalibrator(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE confidence_claims`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.BrierScore(context.Background(), "a"); err == nil {
		t.Fatal("expected rows error")
	}
}

// ── kernel.go ──────────────────────────────────────────────────────────

func TestKernelTimelineRowsError(t *testing.T) {
	db := sqlOpen(t)
	k, _ := NewKernel(db, "")
	if _, err := db.Exec(`DROP TABLE checkpoints`); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Timeline(context.Background(), 10); err == nil {
		t.Fatal("expected rows error")
	}
}

// ── leases.go ─────────────────────────────────────────────────────────

func TestLeaseAcquireSameAgentNoConflict(t *testing.T) {
	lt := NewLeaseTable()
	_, _, err := lt.Acquire("a", "t1", "intent", []string{"pkg/**"})
	if err != nil {
		t.Fatal(err)
	}
	l2, conflicts, err := lt.Acquire("a", "t2", "intent", []string{"pkg/**"})
	if err != nil || len(conflicts) != 0 || l2 == nil {
		t.Fatalf("expected same-agent acquire to succeed, got l2=%v conflicts=%v err=%v", l2, conflicts, err)
	}
}

func TestLeaseBoardMultipleSorted(t *testing.T) {
	lt := NewLeaseTable()
	_, _, _ = lt.Acquire("b", "t2", "intent", []string{"b/**"})
	_, _, _ = lt.Acquire("a", "t1", "intent", []string{"a/**"})
	if s := lt.Board(); !strings.Contains(s, "a") || !strings.Contains(s, "b") {
		t.Fatalf("expected board with both agents, got %q", s)
	}
}

func TestGlobsOverlapMatch(t *testing.T) {
	if !globsOverlap("pkg/*.go", "pkg/a.go") {
		t.Fatal("expected pkg/*.go to match pkg/a.go")
	}
	if !globsOverlap("pkg/a.go", "pkg/*.go") {
		t.Fatal("expected reverse match")
	}
}

// ── registry.go ─────────────────────────────────────────────────────────

func TestLoadUserAgentsNoAgentToml(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "empty-agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfgs, err := LoadUserAgents(dir)
	if err != nil || len(cfgs) != 0 {
		t.Fatalf("expected no configs, got %v err=%v", cfgs, err)
	}
}

func TestLoadUserAgentsInvalidToml(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "bad")
	if err := os.Mkdir(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "agent.toml"), []byte("[[[not toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadUserAgents(dir)
	if err == nil {
		t.Fatal("expected toml decode error")
	}
}

// ── adversary.go ─────────────────────────────────────────────────────

type mockAdversaryAgent struct{ err error }

func (m *mockAdversaryAgent) ProposeAttacks(ctx context.Context, diff, impactBrief string, maxAttacks int) ([]Attack, error) {
	return nil, m.err
}

func TestAdversaryCounterexampleBriefSkipsUnlanded(t *testing.T) {
	r := &AdversaryResult{
		Attacks: []Attack{
			{Kind: AttackBoundary, Hypothesis: "h1", Landed: true, Output: "boom"},
			{Kind: AttackContract, Hypothesis: "h2", Landed: false, Output: "ok"},
		},
		Landed:  1,
		Cleared: false,
	}
	s := r.CounterexampleBrief()
	if !strings.Contains(s, "h1") || strings.Contains(s, "h2") {
		t.Fatalf("expected only landed attack, got %s", s)
	}
}

func TestAdversaryReviewProposeError(t *testing.T) {
	agent := &mockAdversaryAgent{err: errors.New("no attacks")}
	adv := NewAdversary(agent, t.TempDir())
	_, err := adv.Review(context.Background(), "diff", "impact")
	if err == nil || !strings.Contains(err.Error(), "no attacks") {
		t.Fatalf("expected propose error, got %v", err)
	}
}

func TestAdversaryExecuteProbeEmptyWorkdirWithProbe(t *testing.T) {
	adv := &Adversary{Workdir: ""}
	_, _, err := adv.executeProbe(context.Background(), &Attack{ProbeSource: "package main\n"}, 0)
	if err == nil || !strings.Contains(err.Error(), "empty workdir") {
		t.Fatalf("expected empty workdir error, got %v", err)
	}
}

func TestAdversaryExecuteProbeWriteFileError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o755)
	adv := &Adversary{Workdir: dir, ProbeTimeout: time.Second}
	_, _, err := adv.executeProbe(context.Background(), &Attack{ProbeSource: "package main\nfunc TestX(t *testing.T){}\n"}, 0)
	if err == nil {
		t.Fatal("expected write error")
	}
}

func TestAdversaryExecuteProbeCleanup(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := adversaryExecCommand
	adversaryExecCommand = func(ctx context.Context, workdir, rel string, timeout time.Duration) ([]byte, error) {
		return []byte("ok"), nil
	}
	defer func() { adversaryExecCommand = old }()
	adv := &Adversary{Workdir: dir, ProbeTimeout: time.Second}
	landed, _, err := adv.executeProbe(context.Background(), &Attack{ProbeSource: "package main\nfunc TestX(t *testing.T){}\n"}, 0)
	if err != nil || landed {
		t.Fatalf("expected not landed, got landed=%v err=%v", landed, err)
	}
}

// ── blame.go ─────────────────────────────────────────────────────────

func TestBlameCheckoutError(t *testing.T) {
	old := blameGitCmd
	blameGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return []byte("fail"), errors.New("git error")
	}
	defer func() { blameGitCmd = old }()
	bl := &Blamer{Verifier: NewVerifier(t.TempDir())}
	log := &EditLog{TaskID: "t", Workdir: t.TempDir(), Base: "abc123", Edits: []EditRecord{{Seq: 1, SHA: "def456"}}}
	if _, err := bl.Blame(context.Background(), log, Check{Name: "c"}); err == nil {
		t.Fatal("expected blame error")
	}
}

// ── cartographer.go ──────────────────────────────────────────────────

func TestCartographerIndexAllWalkErrorUnreadable(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sub, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(sub, 0o755)
	c := NewCartographer(dir)
	if err := c.IndexAll(context.Background()); err == nil {
		t.Fatal("expected walk error")
	}
}

func TestCartographerIndexFileParseErrorDirect(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewCartographer(dir)
	if err := c.indexFile(filepath.Join(dir, "a.go")); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestCallNameNonIdentSelector(t *testing.T) {
	src := []byte(`package x
func f() {
	(pkg).Method()
	_ = func() {}()
}
`)
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "a.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	ast.Inspect(f, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			got = append(got, callName(c, "x"))
		}
		return true
	})
	if len(got) != 2 || got[0] != "" || got[1] != "" {
		t.Fatalf("expected empty call names, got %v", got)
	}
}

// ── confidence.go ────────────────────────────────────────────────────

func TestCalibrateQueryError(t *testing.T) {
	db := sqlOpen(t)
	c, _ := NewCalibrator(db)
	if _, err := db.Exec(`DROP TABLE confidence_claims`); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Calibrate(context.Background(), "a", 0.5); err == nil {
		t.Fatal("expected query error")
	}
}

func TestCalibrateLocalBinWeighted(t *testing.T) {
	db := sqlOpen(t)
	c, _ := NewCalibrator(db)
	for i := 0; i < 12; i++ {
		if err := c.Record(context.Background(), ConfidenceClaim{AgentName: "a", Declared: 0.5, Passed: i%2 == 0}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := c.Calibrate(context.Background(), "a", 0.5)
	if err != nil || got < 0 || got > 1 {
		t.Fatalf("expected calibration in [0,1], got %v err=%v", got, err)
	}
}

func TestBrierScoreScanError(t *testing.T) {
	db := sqlOpen(t)
	c, _ := NewCalibrator(db)
	if _, err := db.Exec(`INSERT INTO confidence_claims(agent,class,declared,passed) VALUES(?,?,?,?)`, "a", "x", "not-a-number", 1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.BrierScore(context.Background(), "a"); err == nil {
		t.Fatal("expected scan error")
	}
}

// ── contract.go ─────────────────────────────────────────────────────

func TestContractFrozenPrefixViolation(t *testing.T) {
	c := &Contract{FrozenGlobs: []string{"dir/*"}}
	v := c.CheckEdit("dir/sub/file.txt", []string{"x"})
	if len(v) != 1 || v[0].Kind != "frozen-path" {
		t.Fatalf("expected frozen-path violation, got %v", v)
	}
}

// ── critic.go ────────────────────────────────────────────────────────

func TestCriticMaxAttemptsClamp(t *testing.T) {
	vf := NewVerifier(t.TempDir())
	cr := NewCritic(vf, []Check{})
	cr.Policy.MaxAttempts = 0
	agent := NewMockAgent(AgentConfig{Name: "a", Type: TaskCode})
	res, _ := cr.Drive(context.Background(), agent, &Task{ID: "t", Description: "d"}, NewScratchpad())
	if len(res.Attempts) != 1 {
		t.Fatalf("expected 1 attempt, got %d", len(res.Attempts))
	}
}

type errAgent struct{}

func (e *errAgent) Name() string        { return "err" }
func (e *errAgent) Config() AgentConfig { return AgentConfig{} }
func (e *errAgent) Run(ctx context.Context, task *Task, scratch *Scratchpad) (string, error) {
	return "", errors.New("boom")
}

func TestCriticSetsTitleWhenEmpty(t *testing.T) {
	vf := NewVerifier(t.TempDir())
	cr := NewCritic(vf, []Check{})
	cr.Policy.MaxAttempts = 2
	res, _ := cr.Drive(context.Background(), &errAgent{}, &Task{ID: "t", Title: "", Description: "d"}, NewScratchpad())
	if len(res.Attempts) < 2 {
		t.Fatalf("expected attempts, got %d", len(res.Attempts))
	}
}

// ── dag.go ─────────────────────────────────────────────────────────────

func TestDagExecuteContextCancel(t *testing.T) {
	lt := NewLeaseTable()
	if _, _, err := lt.Acquire("b", "pre", "", []string{"**"}); err != nil {
		t.Fatal(err)
	}
	d := NewDagExecutor(lt, "a")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancel()
	node := &PlanNode{Task: &Task{ID: "x"}}
	res, err := d.Execute(ctx, []*PlanNode{node}, func(ctx context.Context, n *PlanNode) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "context") {
		t.Fatalf("expected ctx err, got %v", err)
	}
	if res == nil {
		t.Fatal("expected res")
	}
}

// ── dispatcher.go ──────────────────────────────────────────────────────

func TestDispatchNoAgentError(t *testing.T) {
	r := NewRegistry([]Agent{})
	d := NewDispatcher(r, NewScratchpad(), 1)
	plan := &Plan{Tasks: []*Task{{ID: "x", Type: TaskCode, AgentName: "unknown", Status: TaskPending}}}
	if err := d.Dispatch(context.Background(), plan); err == nil {
		t.Fatal("expected dispatch error")
	}
}

// ── episodic.go ───────────────────────────────────────────────────────

func TestEpisodeSimilarQueryError(t *testing.T) {
	db := sqlOpen(t)
	s, _ := NewEpisodeStore(db)
	if _, err := db.Exec(`DROP TABLE episodes_fts`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Similar(context.Background(), "test title", 3); err == nil {
		t.Fatal("expected query error")
	}
}

func TestEpisodeSimilarScanError(t *testing.T) {
	db := sqlOpen(t)
	s, _ := NewEpisodeStore(db)
	if _, err := db.Exec(`INSERT INTO episodes(intent,task_title,plan_json,score,passed,rounds,created_at) VALUES(?,?,?,?,?,?,?)`,
		"i", "title", "{}", 0.5, "not-int", 1, time.Now().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Similar(context.Background(), "title", 3); err == nil {
		t.Fatal("expected scan error")
	}
}

// ── governor.go ───────────────────────────────────────────────────────

func TestGovernorRunRungClampsAgents(t *testing.T) {
	g := &Governor{
		Ladder:   DefaultLadder(),
		Verifier: NewVerifier(t.TempDir()),
		Checks:   []Check{},
		RepoRoot: "",
		Factory: func(rung Rung) []Agent {
			return []Agent{NewMockAgent(AgentConfig{Name: "a", Type: TaskCode})}
		},
	}
	rung := Rung{Name: "best", Agents: 5, RepairRounds: 1, Timeout: 5 * time.Second}
	v, rounds, err := g.runRung(context.Background(), rung, &Task{ID: "t", Description: "d"}, NewScratchpad())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if v == nil || !v.Passed {
		t.Fatal("expected passing verdict")
	}
	if rounds < 1 {
		t.Fatalf("expected rounds, got %d", rounds)
	}
}

// ── impact.go ─────────────────────────────────────────────────────────

func TestBuildImpactGraphRelErr(t *testing.T) {
	old := impactGoList
	impactGoList = func(ctx context.Context, dir string) ([]byte, error) {
		p := struct {
			ImportPath string   `json:"ImportPath"`
			Dir        string   `json:"Dir"`
			GoFiles    []string `json:"GoFiles"`
			Imports    []string `json:"Imports"`
		}{ImportPath: "example.com/a", Dir: "elsewhere/a", GoFiles: []string{"a.go"}, Imports: []string{}}
		var buf strings.Builder
		enc := json.NewEncoder(&buf)
		_ = enc.Encode(p)
		return []byte(buf.String()), nil
	}
	defer func() { impactGoList = old }()
	g, err := BuildImpactGraph(context.Background(), "/repo/root")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if _, ok := g.fileToPkg["elsewhere/a/a.go"]; !ok {
		t.Fatalf("expected fallback file key, got %v", g.fileToPkg)
	}
}

func TestImpactPredictBackslashPath(t *testing.T) {
	g := &ImpactGraph{
		nodes:     map[string]*PkgNode{"p": {ImportPath: "p", GoFiles: []string{"a.go"}}},
		fileToPkg: map[string]string{"dir\\a.go": "p"},
	}
	imp := g.Predict([]string{"dir\\a.go"})
	if len(imp.ChangedPkgs) != 1 {
		t.Fatalf("expected changed pkg, got %v", imp)
	}
}

// ── kernel.go ─────────────────────────────────────────────────────────

func TestKernelCaptureExecError(t *testing.T) {
	db := sqlOpen(t)
	k, _ := NewKernel(db, t.TempDir())
	if _, err := db.Exec(`DROP TABLE checkpoints`); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Capture(context.Background(), "x", AgentState{}, true); err == nil {
		t.Fatal("expected exec error")
	}
}

func TestKernelRewindUnmarshalError(t *testing.T) {
	db := sqlOpen(t)
	k, _ := NewKernel(db, "")
	if _, err := db.Exec(`INSERT INTO checkpoints(label,tree_sha,state_json,green,created_at) VALUES(?,?,?,?,?)`,
		"x", "tree", "not-json", 1, time.Now().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Rewind(context.Background(), 1); err == nil || !strings.Contains(err.Error(), "state decode") {
		t.Fatalf("expected unmarshal error, got %v", err)
	}
}

func TestKernelWriteTreeError(t *testing.T) {
	oldCmd := kernelGitCmd
	oldOut := kernelGitOutCmd
	kernelGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) { return nil, nil }
	kernelGitOutCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return nil, errors.New("write-tree failed")
	}
	defer func() {
		kernelGitCmd = oldCmd
		kernelGitOutCmd = oldOut
	}()
	dir := t.TempDir()
	k, _ := NewKernel(sqlOpen(t), dir)
	if _, err := k.Capture(context.Background(), "x", AgentState{}, true); err == nil || !strings.Contains(err.Error(), "write-tree") {
		t.Fatalf("expected write-tree error, got %v", err)
	}
}

// ── macros.go ─────────────────────────────────────────────────────────

func withPassedChecks(t *testing.T) func() {
	old := verifierRunCheck
	verifierRunCheck = func(ctx context.Context, c Check, workdir string) CheckResult {
		return CheckResult{Check: c, Passed: true}
	}
	return func() { verifierRunCheck = old }
}

func TestSinChangeWriteFileError(t *testing.T) {
	defer withPassedChecks(t)()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Macros{
		Workdir:  dir,
		Contract: &Contract{},
		Targeted: NewTargetedVerifier(NewVerifier(dir), &ImpactGraph{nodes: map[string]*PkgNode{}, fileToPkg: map[string]string{}}),
	}
	res, err := m.SinChange(context.Background(), EditRequest{TaskID: "t", Edits: map[string][]byte{"bad/file.txt": []byte("x")}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Decision != "" || !strings.Contains(res.ResumeContext, "apply failed") {
		t.Fatalf("expected empty decision with apply failed context, got decision=%v context=%v", res.Decision, res.ResumeContext)
	}
}

func TestSinChangeUsesCalibration(t *testing.T) {
	defer withPassedChecks(t)()
	db := sqlOpen(t)
	cal, _ := NewCalibrator(db)
	for i := 0; i < 10; i++ {
		if err := cal.Record(context.Background(), ConfidenceClaim{AgentName: "a", Declared: 0.9, Passed: true}); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	m := &Macros{
		Workdir:  dir,
		Contract: &Contract{},
		Targeted: NewTargetedVerifier(NewVerifier(dir), &ImpactGraph{nodes: map[string]*PkgNode{}, fileToPkg: map[string]string{}}),
		Calib:    cal,
		Policy:   DefaultMergePolicy(),
		Agent:    "a",
	}
	res, err := m.SinChange(context.Background(), EditRequest{TaskID: "t", Edits: map[string][]byte{"a.go": []byte("package main")}, DeclaredConfidence: 0.9})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Applied {
		t.Fatal("expected applied")
	}
}

func TestSinRefactorImpNil(t *testing.T) {
	defer withPassedChecks(t)()
	dir := t.TempDir()
	m := &Macros{
		Workdir:  dir,
		Contract: &Contract{},
		Targeted: NewTargetedVerifier(NewVerifier(dir), nil),
		Policy:   DefaultMergePolicy(),
	}
	res, err := m.SinRefactor(context.Background(), EditRequest{TaskID: "t", Edits: map[string][]byte{"a.go": []byte("package main")}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Applied {
		t.Fatal("expected applied")
	}
}

// ── miner.go ───────────────────────────────────────────────────────────

func TestMinerMineLoadSequencesError(t *testing.T) {
	db := sqlOpen(t)
	m, _ := NewMiner(db)
	if _, err := db.Exec(`DROP TABLE episode_sequences`); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Mine(context.Background(), "c"); err == nil {
		t.Fatal("expected load error")
	}
}

func TestMinerMineDuplicateKeyContinue(t *testing.T) {
	db := sqlOpen(t)
	m, _ := NewMiner(db)
	m.MinLength = 1
	m.MinSupport = 1
	m.MinSuccessRate = 0
	seq := []ToolCall{{Tool: "a", ArgShape: "x"}}
	if err := m.RecordSequence(context.Background(), SeqEpisode{EpisodeID: 1, Class: "c", Sequence: seq, Passed: true}); err != nil {
		t.Fatal(err)
	}
	if err := m.RecordSequence(context.Background(), SeqEpisode{EpisodeID: 2, Class: "c", Sequence: seq, Passed: true}); err != nil {
		t.Fatal(err)
	}
	templates, err := m.Mine(context.Background(), "c")
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) == 0 {
		t.Fatal("expected templates")
	}
}

func TestMinerMineInsertError(t *testing.T) {
	db := sqlOpen(t)
	m, _ := NewMiner(db)
	m.MinLength = 1
	m.MinSupport = 1
	m.MinSuccessRate = 0
	if err := m.RecordSequence(context.Background(), SeqEpisode{EpisodeID: 1, Class: "c", Sequence: []ToolCall{{Tool: "a", ArgShape: "x"}}, Passed: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE chain_templates`); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Mine(context.Background(), "c"); err == nil {
		t.Fatal("expected insert error")
	}
}

func TestMinerSuggestionsScanError(t *testing.T) {
	db := sqlOpen(t)
	m, _ := NewMiner(db)
	if _, err := db.Exec(`INSERT INTO chain_templates(class,seq_key,sequence_json,support,success_rate,status) VALUES(?,?,?,?,?,?)`,
		"c", "k", "[]", "not-int", 0.5, "shadow"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SuggestionsFor(context.Background(), "c"); err == nil {
		t.Fatal("expected scan error")
	}
}

func TestMinerLoadSequencesScanError(t *testing.T) {
	db := sqlOpen(t)
	m, _ := NewMiner(db)
	if _, err := db.Exec(`INSERT INTO episode_sequences(episode_id,class,sequence_json,passed) VALUES(?,?,?,?)`,
		1, "c", "[]", "not-int"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.loadSequences(context.Background(), "c"); err == nil {
		t.Fatal("expected scan error")
	}
}

// ── mutation.go ───────────────────────────────────────────────────────

func TestMutationRunApplyAndTestError(t *testing.T) {
	mp := NewMutationProbe(t.TempDir(), []string{"go", "test"})
	_, err := mp.Run(context.Background(), []ChangedLine{{File: "missing.go", Line: 1, Text: "x == y"}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMutationRunUsesLaterMutator(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("a && b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mp := NewMutationProbe(dir, []string{})
	res, err := mp.Run(context.Background(), []ChangedLine{{File: "a.go", Line: 1, Text: "a && b"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Mutations) != 1 || res.Mutations[0].Rule != "and-to-or" {
		t.Fatalf("expected and-to-or mutation, got %v", res.Mutations)
	}
}

func TestMutationApplyAndTestWriteError(t *testing.T) {
	old := mutationWriteFile
	mutationWriteFile = func(path string, data []byte, perm os.FileMode) error {
		return errors.New("write error")
	}
	defer func() { mutationWriteFile = old }()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\n// a && b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mp := NewMutationProbe(dir, []string{})
	_, err := mp.Run(context.Background(), []ChangedLine{{File: "a.go", Line: 2, Text: "// a && b"}})
	if err == nil || !strings.Contains(err.Error(), "write error") {
		t.Fatalf("expected write error, got %v", err)
	}
}

// ── nim_agent.go ─────────────────────────────────────────────────────

func TestNewLLMAgentWithClientDefaultsToNim(t *testing.T) {
	t.Setenv("SIN_NIM_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GROQ_API_KEY", "")
	a := NewLLMAgentWithClient(AgentConfig{Name: "x", Type: TaskCode}, nil)
	if a.Name() != "x" {
		t.Fatalf("expected name x, got %s", a.Name())
	}
}

type fakeMemoryStore struct{ prime string }

func (f *fakeMemoryStore) Prime(query, project string, topK int) (string, error) { return f.prime, nil }
func (f *fakeMemoryStore) Close() error                                          { return nil }

func TestNIMAgentRunAppendsPrimeContext(t *testing.T) {
	var captured llm.ChatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()
	client := llm.NewClient(srv.URL, "key")
	a := NewLLMAgentWithClient(AgentConfig{Name: "p", Type: TaskCode, Model: "qwen"}, client)
	old := memoryOpenHook
	memoryOpenHook = func(path string) (memoryStore, error) {
		return &fakeMemoryStore{prime: "PRIME TEXT"}, nil
	}
	defer func() { memoryOpenHook = old }()
	_, err := a.Run(context.Background(), &Task{ID: "t", Description: "hello"}, NewScratchpad())
	if err != nil {
		t.Fatal(err)
	}
	if len(captured.Messages) < 2 || !strings.Contains(captured.Messages[1].Content, "PRIME TEXT") {
		t.Fatalf("expected prime context in prompt, got %s", captured.Messages[1].Content)
	}
}

// ── planner.go ─────────────────────────────────────────────────────────

func TestPlannerAddsArchitectForDesignIntent(t *testing.T) {
	p := NewPlanner(DefaultAgents())
	plan := p.BuildPlan("design the data model")
	hasArchitect := false
	for _, task := range plan.Tasks {
		if task.Type == TaskArchitect {
			hasArchitect = true
		}
	}
	if !hasArchitect {
		t.Fatalf("expected architect task, got %v", plan.Tasks)
	}
}

// ── registry.go ────────────────────────────────────────────────────────

func TestDefaultUserAgentsPathError(t *testing.T) {
	old := userConfigDir
	userConfigDir = func() (string, error) { return "", errors.New("no config dir") }
	defer func() { userConfigDir = old }()
	if got := DefaultUserAgentsPath(); got != "" {
		t.Fatalf("expected empty path, got %q", got)
	}
}

func TestLoadUserAgentsReadDirErrorFilePath(t *testing.T) {
	f := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadUserAgents(f); err == nil {
		t.Fatal("expected read dir error")
	}
}

// ── semmerge.go ────────────────────────────────────────────────────────

func TestSemanticMergeAOnlyAndBOnly(t *testing.T) {
	base := []byte("package main\nfunc Base() {}\n")
	a := []byte("package main\nfunc Base() {}\nfunc AOnly() {}\n")
	b := []byte("package main\nfunc Base() {}\nfunc BOnly() {}\n")
	res, err := SemanticMergeGo(base, a, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %v", res.Conflicts)
	}
	if !strings.Contains(string(res.Merged), "AOnly") || !strings.Contains(string(res.Merged), "BOnly") {
		t.Fatalf("expected both new functions, got %s", res.Merged)
	}
}

// ── speculative.go ─────────────────────────────────────────────────────

func TestSpeculativeAddWorktreeMkdirErrorFileBase(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &SpeculativeRunner{WorkdirBase: filepath.Join(dir, "base", "sub")}
	_, err := s.addWorktree(context.Background(), "id")
	if err == nil {
		t.Fatal("expected mkdir error")
	}
}

// ── strategy_router.go ─────────────────────────────────────────────────

func TestStrategyRouterLoadQueryError(t *testing.T) {
	db := sqlOpen(t)
	r, _ := NewStrategyRouter(db, 1)
	if _, err := db.Exec(`DROP TABLE router_arms`); err != nil {
		t.Fatal(err)
	}
	if err := r.load(); err == nil {
		t.Fatal("expected load error")
	}
}

func TestStrategyRouterReportFailureIncrementsBeta(t *testing.T) {
	db := sqlOpen(t)
	r, _ := NewStrategyRouter(db, 1)
	if err := r.Report(context.Background(), ClassBugfix, StratASTEdit, false); err != nil {
		t.Fatal(err)
	}
	mean, _ := r.Posterior(ClassBugfix, StratASTEdit)
	if mean >= 0.5 {
		t.Fatalf("expected beta to lower mean, got %v", mean)
	}
}

// ── txn.go ─────────────────────────────────────────────────────────────

func TestTxnWriteFileError(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("orig"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o444); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(target, 0o644)
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	txn := BeginTxn(dir)
	if err := txn.WriteFile("link", []byte("mutated")); err == nil {
		t.Fatal("expected write error")
	}
}

func TestTxnRollbackIsNotExist(t *testing.T) {
	dir := t.TempDir()
	txn := BeginTxn(dir)
	if err := txn.WriteFile("new.txt", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "new.txt")); err != nil {
		t.Fatal(err)
	}
	if err := txn.Rollback(); err != nil {
		t.Fatalf("unexpected rollback error: %v", err)
	}
}

// ── stcov2 final coverage push ───────────────────────────────────────

// ── adversary.go ─────────────────────────────────────────────────────

func TestAdversaryReviewProbePackageError(t *testing.T) {
	adv := NewAdversary(stcovAdversary{attacks: []Attack{{
		Kind: AttackBoundary, Hypothesis: "h", ProbeSource: "no package clause",
	}}}, t.TempDir())
	res, err := adv.Review(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Attacks) != 1 || !strings.Contains(res.Attacks[0].Output, "probe error") {
		t.Fatalf("expected probe error attack, got cleared=%v attacks=%+v", res.Cleared, res.Attacks)
	}
}

// ── blame.go ─────────────────────────────────────────────────────────

func TestBlameBisectCheckAtFalse(t *testing.T) {
	bl := &Blamer{Verifier: NewVerifier("")}
	log := &EditLog{
		TaskID: "t", Workdir: t.TempDir(),
		Edits: []EditRecord{
			{Seq: 1, SHA: "0000000000000000000000000000000000000001", Path: "a.go", Summary: "a"},
			{Seq: 2, SHA: "0000000000000000000000000000000000000002", Path: "a.go", Summary: "b"},
			{Seq: 3, SHA: "0000000000000000000000000000000000000003", Path: "a.go", Summary: "c"},
		},
	}
	old := blameGitCmd
	blameGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) { return nil, nil }
	defer func() { blameGitCmd = old }()
	res, err := bl.Blame(context.Background(), log, Check{Kind: CheckBuild, Name: "c"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Culprit == nil || res.Culprit.Seq != 1 {
		t.Fatalf("expected culprit seq 1, got %+v", res.Culprit)
	}
}

func TestBlameBisectCheckAtError(t *testing.T) {
	bl := &Blamer{Verifier: NewVerifier("")}
	log := &EditLog{
		TaskID: "t", Workdir: t.TempDir(),
		Edits: []EditRecord{
			{Seq: 1, SHA: "0000000000000000000000000000000000000001", Path: "a.go", Summary: "a"},
			{Seq: 2, SHA: "0000000000000000000000000000000000000002", Path: "a.go", Summary: "b"},
		},
	}
	old := blameGitCmd
	blameGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return nil, errors.New("git fail")
	}
	defer func() { blameGitCmd = old }()
	_, err := bl.Blame(context.Background(), log, Check{Kind: CheckBuild, Name: "c"})
	if err == nil || !strings.Contains(err.Error(), "blame checkout") {
		t.Fatalf("expected checkout error, got %v", err)
	}
}

// ── cartographer.go ──────────────────────────────────────────────────

func TestCartographerIndexAllSkipDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "hidden.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "visible.go"), []byte("package main\nfunc V() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewCartographer(dir)
	if err := c.IndexAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.SymbolCount() != 1 {
		t.Fatalf("expected 1 symbol, got %d", c.SymbolCount())
	}
}

func TestCartographerIndexAllCancelled(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewCartographer(dir)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := c.IndexAll(ctx)
	if err != context.Canceled {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestCartographerIndexFileParseErrorStcov2(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.go")
	if err := os.WriteFile(path, []byte("package main\nfunc {"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewCartographer(dir)
	if err := c.indexFile(path); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestCartographerIndexFileReadError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0o644)
	c := NewCartographer(dir)
	if err := c.indexFile(path); err == nil {
		t.Fatal("expected read error")
	}
}

func TestCartographerIndexFileRelFallback(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	path := filepath.Join(other, "a.go")
	if err := os.WriteFile(path, []byte("package main\nfunc F() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewCartographer(dir)
	if err := c.indexFile(path); err != nil {
		t.Fatal(err)
	}
}

// ── confidence.go ────────────────────────────────────────────────────

func TestCalibratorScanError(t *testing.T) {
	db := sqlOpen(t)
	c, _ := NewCalibrator(db)
	if _, err := db.Exec(`INSERT INTO confidence_claims (agent, class, declared, passed) VALUES (?, ?, ?, ?)`, "a", "x", "not-a-number", 1); err != nil {
		t.Fatal(err)
	}
	_, err := c.Calibrate(context.Background(), "a", 0.5)
	if err == nil {
		t.Fatal("expected scan error")
	}
}

func TestCalibratorLocalNZero(t *testing.T) {
	db := sqlOpen(t)
	c, _ := NewCalibrator(db)
	for i := 0; i < 10; i++ {
		_ = c.Record(context.Background(), ConfidenceClaim{AgentName: "a", Declared: 0.1, Passed: true})
	}
	v, err := c.Calibrate(context.Background(), "a", 0.9)
	if err != nil || v == 0.1 {
		t.Fatalf("expected global fallback, got %v err=%v", v, err)
	}
}

func TestBrierScoreNoClaims(t *testing.T) {
	db := sqlOpen(t)
	c, _ := NewCalibrator(db)
	score, n, err := c.BrierScore(context.Background(), "a")
	if err != nil || n != 0 || score != 0 {
		t.Fatalf("expected empty brier score, got %v %d %v", score, n, err)
	}
}

// ── dispatcher.go ────────────────────────────────────────────────────

func TestDispatchContextCancelled(t *testing.T) {
	r := NewRegistry([]Agent{stcovAgent{name: "a", out: "ok"}})
	scratch := NewScratchpad()
	d := NewDispatcher(r, scratch, 1)
	plan := &Plan{Tasks: []*Task{{ID: "t1", Type: TaskCode, AgentName: "a", Status: TaskPending, DependsOn: []string{"never"}}}}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	err := d.Dispatch(ctx, plan)
	if err != context.Canceled {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

// ── governor.go ──────────────────────────────────────────────────────

func TestGovernorRunRungWinnerNotPassed(t *testing.T) {
	oldAdd := specWorktreeAdd
	oldVerify := verifierRunCheck
	specWorktreeAdd = func(ctx context.Context, repoRoot, dir string) ([]byte, error) { return nil, nil }
	verifierRunCheck = func(ctx context.Context, c Check, workdir string) CheckResult {
		return CheckResult{Check: c, Passed: false}
	}
	defer func() { specWorktreeAdd = oldAdd; verifierRunCheck = oldVerify }()

	g := &Governor{
		Ladder:   []Rung{{Name: "x", Agents: 2, RepairRounds: 1, Timeout: time.Second}},
		Verifier: NewVerifier(t.TempDir()),
		Checks:   []Check{{Kind: CheckBuild, Name: "c"}},
		RepoRoot: t.TempDir(),
		Factory: func(rung Rung) []Agent {
			return []Agent{stcovAgent{name: "a", out: "ok"}}
		},
	}
	v, _, err := g.runRung(context.Background(), g.Ladder[0], &Task{ID: "t"}, NewScratchpad())
	if err != nil || v == nil || v.Passed {
		t.Fatalf("expected non-passed verdict, got %v %v", v, err)
	}
}

func TestGovernorRunRungWinnerMergeError(t *testing.T) {
	oldAdd := specWorktreeAdd
	oldVerify := verifierRunCheck
	oldDiff := specDiff
	specWorktreeAdd = func(ctx context.Context, repoRoot, dir string) ([]byte, error) { return nil, nil }
	verifierRunCheck = func(ctx context.Context, c Check, workdir string) CheckResult {
		return CheckResult{Check: c, Passed: true}
	}
	specDiff = func(ctx context.Context, worktree string) ([]byte, error) {
		return []byte("diff"), errors.New("diff error")
	}
	defer func() { specWorktreeAdd = oldAdd; verifierRunCheck = oldVerify; specDiff = oldDiff }()

	g := &Governor{
		Ladder:   []Rung{{Name: "x", Agents: 2, RepairRounds: 1, Timeout: time.Second}},
		Verifier: NewVerifier(t.TempDir()),
		Checks:   []Check{{Kind: CheckBuild, Name: "c"}},
		RepoRoot: t.TempDir(),
		Factory: func(rung Rung) []Agent {
			return []Agent{stcovAgent{name: "a", out: "ok"}}
		},
	}
	_, _, err := g.runRung(context.Background(), g.Ladder[0], &Task{ID: "t"}, NewScratchpad())
	if err == nil || !strings.Contains(err.Error(), "merge winner") {
		t.Fatalf("expected merge winner error, got %v", err)
	}
}

func TestGovernorRunRungRepairMergeError(t *testing.T) {
	oldAdd := specWorktreeAdd
	oldVerify := verifierRunCheck
	oldDiff := specDiff
	oldApply := specApply
	calls := 0
	specWorktreeAdd = func(ctx context.Context, repoRoot, dir string) ([]byte, error) { return nil, nil }
	verifierRunCheck = func(ctx context.Context, c Check, workdir string) CheckResult {
		calls++
		if calls == 1 {
			return CheckResult{Check: c, Passed: false}
		}
		return CheckResult{Check: c, Passed: true}
	}
	specDiff = func(ctx context.Context, worktree string) ([]byte, error) { return []byte("diff"), nil }
	specApply = func(ctx context.Context, repoRoot string, diff []byte) ([]byte, error) {
		return []byte("fail"), errors.New("apply error")
	}
	defer func() {
		specWorktreeAdd = oldAdd
		verifierRunCheck = oldVerify
		specDiff = oldDiff
		specApply = oldApply
	}()

	g := &Governor{
		Ladder:   []Rung{{Name: "x", Agents: 2, RepairRounds: 2, Timeout: time.Second}},
		Verifier: NewVerifier(t.TempDir()),
		Checks:   []Check{{Kind: CheckBuild, Name: "c"}},
		RepoRoot: t.TempDir(),
		Factory: func(rung Rung) []Agent {
			return []Agent{stcovAgent{name: "a", out: "ok"}}
		},
	}
	_, _, err := g.runRung(context.Background(), g.Ladder[0], &Task{ID: "t"}, NewScratchpad())
	if err == nil || !strings.Contains(err.Error(), "merge repaired winner") {
		t.Fatalf("expected merge repaired winner error, got %v", err)
	}
}

// ── kernel.go remaining branches ───────────────────────────────────────

func TestKernelCaptureExecWithHookedWriteTree(t *testing.T) {
	db := sqlOpen(t)
	k, _ := NewKernel(db, t.TempDir())
	oldGit := kernelGitCmd
	oldGitOut := kernelGitOutCmd
	kernelGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) { return nil, nil }
	kernelGitOutCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return []byte("TREE"), nil
	}
	defer func() { kernelGitCmd = oldGit; kernelGitOutCmd = oldGitOut }()
	if _, err := db.Exec(`DROP TABLE checkpoints`); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Capture(context.Background(), "x", AgentState{}, true); err == nil {
		t.Fatal("expected exec error")
	}
}

func TestKernelRewindMissingCheckpoint(t *testing.T) {
	db := sqlOpen(t)
	k, _ := NewKernel(db, "")
	if _, err := k.Rewind(context.Background(), 999); err == nil {
		t.Fatal("expected checkpoint not found error")
	}
}

func TestKernelTimelineDirectGreenAndRed(t *testing.T) {
	db := sqlOpen(t)
	k, _ := NewKernel(db, "")
	now := timeNow().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO checkpoints(label, tree_sha, state_json, green, created_at) VALUES (?, ?, ?, ?, ?), (?, ?, ?, ?, ?)`,
		"green", "", "{}", 1, now,
		"red", "", "{}", 0, now); err != nil {
		t.Fatal(err)
	}
	s, err := k.Timeline(context.Background(), 10)
	if err != nil || !strings.Contains(s, "GREEN") || !strings.Contains(s, "red") {
		t.Fatalf("expected GREEN and red markers, got %q err=%v", s, err)
	}
}

// ── macros.go remaining branches ─────────────────────────────────────────

func TestMacrosSinRefactorProbeLowObservability(t *testing.T) {
	oldVerify := verifierRunCheck
	verifierRunCheck = func(ctx context.Context, c Check, workdir string) CheckResult {
		return CheckResult{Check: c, Passed: true}
	}
	defer func() { verifierRunCheck = oldVerify }()

	oldProbeHook := probeRunHook
	defer func() { probeRunHook = oldProbeHook }()
	probeRunHook = func(res *ProbeResult) {
		res.ObservabilityScore = 0.4
		res.Mutations = []Mutation{{Killed: false, Rule: "x"}}
		res.Survived = 1
	}

	dir := t.TempDir()
	ig := &ImpactGraph{
		nodes:     map[string]*PkgNode{"repo/a": {TestFiles: []string{"a_test.go"}}},
		fileToPkg: map[string]string{"a.go": "repo/a", "a_test.go": "repo/a"},
	}
	tv := NewTargetedVerifier(NewVerifier(dir), ig)
	m := &Macros{
		Workdir:  dir,
		Targeted: tv,
		Policy:   DefaultMergePolicy(),
		Agent:    "agent",
		Probe:    func(testCmd []string) *MutationProbe { return NewMutationProbe("", testCmd) },
	}
	res, err := m.SinRefactor(context.Background(), EditRequest{
		TaskID: "t", DeclaredConfidence: 0.9, Edits: map[string][]byte{"a.go": []byte("x")},
	})
	if err != nil || !res.Applied || res.Decision != DecisionGreenReview || res.ResumeContext == "" {
		t.Fatalf("expected low observability override, got %+v err=%v", res, err)
	}
}

// ── miner.go remaining branches ──────────────────────────────────────────

func TestMinerMineDuplicateKeyWithinEpisode(t *testing.T) {
	db := sqlOpen(t)
	m, _ := NewMiner(db)
	m.MinSupport = 1
	m.MinLength = 1
	m.MaxLength = 2
	m.MinSuccessRate = 0
	if err := m.RecordSequence(context.Background(), SeqEpisode{
		EpisodeID: 1,
		Class:     "c",
		Sequence:  []ToolCall{{Tool: "a"}, {Tool: "a"}},
		Passed:    true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Mine(context.Background(), "c"); err != nil {
		t.Fatal(err)
	}
}

func TestMinerSuggestionsForUnknownClass(t *testing.T) {
	db := sqlOpen(t)
	m, _ := NewMiner(db)
	got, err := m.SuggestionsFor(context.Background(), TaskClass("unknown"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("expected empty suggestions, got %q", got)
	}
}

func TestMinerSuggestionsForNonEmpty(t *testing.T) {
	db := sqlOpen(t)
	m, _ := NewMiner(db)
	seq, _ := json.Marshal([]ToolCall{{Tool: "a"}})
	if _, err := db.Exec(`INSERT INTO chain_templates(class, seq_key, sequence_json, support, success_rate) VALUES (?, ?, ?, ?, ?)`,
		"code", "key", string(seq), 1, 1.0); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SuggestionsFor(context.Background(), "code"); err != nil {
		t.Fatal(err)
	}
}

// ── mutation.go remaining branches ───────────────────────────────────────

func TestMutationRunMaxMutations(t *testing.T) {
	old := mutators
	defer func() { mutators = old }()
	mutators = []struct {
		rule string
		re   *regexp.Regexp
		sub  string
	}{
		{"true-to-false", regexp.MustCompile(`\btrue\b`), "false"},
	}
	mp := NewMutationProbe("", nil)
	mp.MaxMutations = 1
	lines := []ChangedLine{
		{File: "a.go", Line: 1, Text: "true"},
		{File: "a.go", Line: 2, Text: "true"},
	}
	res, err := mp.Run(context.Background(), lines)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Mutations) != 1 {
		t.Fatalf("expected 1 mutation, got %d", len(res.Mutations))
	}
}

func TestMutationRunNoOpMutator(t *testing.T) {
	old := mutators
	defer func() { mutators = old }()
	mutators = []struct {
		rule string
		re   *regexp.Regexp
		sub  string
	}{
		{"no-op", regexp.MustCompile(`x`), "x"},
	}
	mp := NewMutationProbe("", nil)
	res, err := mp.Run(context.Background(), []ChangedLine{{File: "a.go", Line: 1, Text: "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Mutations) != 0 {
		t.Fatalf("expected 0 mutations, got %d", len(res.Mutations))
	}
}

// ── nim_agent.go remaining branches ────────────────────────────────────

func TestLLMAgentRunLoadSystemPromptError(t *testing.T) {
	old := loadSystemPromptHook
	defer func() { loadSystemPromptHook = old }()
	loadSystemPromptHook = func(a *LLMAgent) (string, error) { return "", errors.New("prompt fail") }

	a := &LLMAgent{cfg: AgentConfig{Name: "n", Model: "qwen"}, client: &llm.Client{}}
	if _, err := a.Run(context.Background(), &Task{ID: "t"}, NewScratchpad()); err == nil || !strings.Contains(err.Error(), "prompt") {
		t.Fatalf("expected prompt error, got %v", err)
	}
}

func TestLLMAgentRunPriorOutputs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()
	client := llm.NewClient(srv.URL, "key")
	a := NewLLMAgentWithClient(AgentConfig{Name: "n", Model: "qwen"}, client)
	scratch := NewScratchpad()
	scratch.Write(a.Name(), "outputs:prev", "previous output")
	if _, err := a.Run(context.Background(), &Task{ID: "t", Description: "do it"}, scratch); err != nil {
		t.Fatal(err)
	}
}

func TestLLMAgentLoadSystemPromptHookError(t *testing.T) {
	old := loadSystemPromptHook
	defer func() { loadSystemPromptHook = old }()
	loadSystemPromptHook = func(a *LLMAgent) (string, error) { return "", errors.New("hook fail") }

	a := &LLMAgent{cfg: AgentConfig{Name: "n"}}
	if _, err := a.loadSystemPrompt(); err == nil || !strings.Contains(err.Error(), "hook") {
		t.Fatalf("expected hook error, got %v", err)
	}
}

// ── registry.go remaining branches ───────────────────────────────────────

func TestRegistryLoadUserAgentsDecodeError(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "bad")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "agent.toml"), []byte("not valid toml = ["), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadUserAgents(dir); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestRegistryLoadUserAgentsMissingToml(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "empty")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	agents, err := LoadUserAgents(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 0 {
		t.Fatalf("expected no agents, got %d", len(agents))
	}
}

func TestRegistryLoadUserAgentsDefaultPathMissing(t *testing.T) {
	old := userConfigDir
	userConfigDir = func() (string, error) { return filepath.Join(t.TempDir(), "missing"), nil }
	defer func() { userConfigDir = old }()
	agents, err := LoadUserAgents("")
	if err != nil {
		t.Fatal(err)
	}
	if agents != nil {
		t.Fatalf("expected nil agents, got %v", agents)
	}
}

// ── semmerge.go remaining branches ──────────────────────────────────────

func TestSemanticMergeGoConflictAllSides(t *testing.T) {
	base := []byte("package x\nfunc A() { return 1 }\n")
	a := []byte("package x\nfunc A() { return 2 }\n")
	b := []byte("package x\nfunc A() { return 3 }\n")
	res, err := SemanticMergeGo(base, a, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Conflicts) == 0 {
		t.Fatal("expected conflicts")
	}
	brief := res.ConflictBrief()
	if !strings.Contains(brief, "version A") || !strings.Contains(brief, "version B") {
		t.Fatalf("expected conflict brief to include both versions, got %q", brief)
	}
}

func TestSemanticMergeGoFormatError(t *testing.T) {
	old := formatSourceHook
	defer func() { formatSourceHook = old }()
	formatSourceHook = func(src []byte) ([]byte, error) { return nil, errors.New("fmt fail") }

	base := []byte("package x\nfunc A() {}\n")
	a := []byte("package x\nfunc A() {}\nfunc B() {}\n")
	b := []byte("package x\nfunc A() {}\nfunc C() {}\n")
	if _, err := SemanticMergeGo(base, a, b); err == nil || !strings.Contains(err.Error(), "fmt") {
		t.Fatalf("expected format error, got %v", err)
	}
}

// ── speculative.go remaining branches ────────────────────────────────────

func TestSpeculativeAddWorktreeMkdirParentError(t *testing.T) {
	dir := t.TempDir()
	fileBase := filepath.Join(dir, "base")
	if err := os.WriteFile(fileBase, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewSpeculativeRunner("", nil)
	s.WorkdirBase = fileBase
	_, err := s.addWorktree(context.Background(), "id")
	if err == nil {
		t.Fatal("expected mkdir error")
	}
}

func TestSpeculativeMergeWinnerRemovesWorktree(t *testing.T) {
	oldDiff := specDiff
	oldApply := specApply
	specDiff = func(ctx context.Context, worktree string) ([]byte, error) { return []byte("diff"), nil }
	specApply = func(ctx context.Context, repoRoot string, diff []byte) ([]byte, error) { return nil, nil }
	defer func() { specDiff = oldDiff; specApply = oldApply }()

	dir := t.TempDir()
	s := NewSpeculativeRunner(dir, nil)
	s.KeepLosers = false
	wt := filepath.Join(dir, "wt")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	merged, err := s.MergeWinner(context.Background(), &Candidate{Worktree: wt})
	if err != nil {
		t.Fatal(err)
	}
	if merged != "diff" {
		t.Fatalf("expected diff, got %q", merged)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatal("expected worktree to be removed")
	}
}

// ── strategy_router.go remaining branches ────────────────────────────────

func TestKernelTimelineScanError(t *testing.T) {
	db := sqlOpen(t)
	k, _ := NewKernel(db, "")
	now := timeNow().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO checkpoints(label, tree_sha, state_json, green, created_at) VALUES (?, ?, ?, ?, ?)`,
		"x", "", "{}", "notanint", now); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Timeline(context.Background(), 10); err == nil {
		t.Fatal("expected scan error")
	}
}

func TestSpeculativeAddWorktreeMkdirDirError(t *testing.T) {
	dir := t.TempDir()
	s := NewSpeculativeRunner("", nil)
	s.WorkdirBase = dir
	if err := os.WriteFile(filepath.Join(dir, "id"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := s.addWorktree(context.Background(), "id")
	if err == nil {
		t.Fatal("expected mkdir error")
	}
}

func TestStrategyRouterSampleGammaContinue(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 5000; i++ {
		_ = sampleGamma(rng, 0.5)
	}
}

// ── remaining uncovered statement coverage ───────────────────────────────

func TestAdversaryExecCommandBody(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := adversaryExecCommand(ctx, t.TempDir(), ".", time.Second); err == nil {
		t.Fatal("expected error from cancelled adversary exec")
	}
}

func TestBlameGitCmdBody(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := blameGitCmd(ctx, t.TempDir(), "status"); err == nil {
		t.Fatal("expected error from cancelled blame git")
	}
}

func TestImpactGoListBody(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := impactGoList(ctx, t.TempDir()); err == nil {
		t.Fatal("expected error from cancelled impact go list")
	}
}

func TestMutationExecCommandBody(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := mutationExecCommand(ctx, t.TempDir(), "true"); err == nil {
		t.Fatal("expected error from cancelled mutation exec")
	}
}

func TestSpecWorktreeAddBody(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := specWorktreeAdd(ctx, t.TempDir(), "wt"); err == nil {
		t.Fatal("expected error from cancelled spec worktree add")
	}
}

func TestSpecDiffBody(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := specDiff(ctx, t.TempDir()); err == nil {
		t.Fatal("expected error from cancelled spec diff")
	}
}

func TestSpecApplyBody(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := specApply(ctx, t.TempDir(), []byte("diff")); err == nil {
		t.Fatal("expected error from cancelled spec apply")
	}
}

func TestDispatcherAllOKFalse(t *testing.T) {
	errAgent := stcovAgent{name: "err", err: errors.New("boom")}
	reg := NewRegistry([]Agent{errAgent})
	d := NewDispatcher(reg, NewScratchpad(), 1)
	plan := &Plan{ID: "p", Tasks: []*Task{
		{ID: "t", Type: TaskCode, AgentName: "err", Status: TaskPending, Description: "d"},
	}}
	if err := d.Dispatch(context.Background(), plan); err != nil {
		t.Fatalf("unexpected dispatch error: %v", err)
	}
	if plan.Success {
		t.Fatal("expected plan.Success to be false for failed task")
	}
}

func TestCartographerIndexFileRelativeFallback(t *testing.T) {
	dir := "stcov_carto_tmp_dir"
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "stcov.go")
	src := []byte("package x\n\ntype X int\n\nfunc (x X) M() {}\n\nfunc F() {}\n")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewCartographer("/repo/root")
	if err := c.indexFile(path); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.symbols["x.F"]; !ok {
		t.Fatalf("expected func x.F, got %v", c.symbols)
	}
	if _, ok := c.symbols["x.X.M"]; !ok {
		t.Fatalf("expected method x.X.M, got %v", c.symbols)
	}
}

func TestCalibrateRowsError(t *testing.T) {
	db := sqlOpen(t)
	c, err := NewCalibrator(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for i := 0; i < 50000; i++ {
		if err := c.Record(ctx, ConfidenceClaim{AgentName: "a", Declared: 0.8, Passed: true}); err != nil {
			t.Fatal(err)
		}
	}
	ctx2, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(5 * time.Millisecond); cancel() }()
	_, err = c.Calibrate(ctx2, "a", 0.8)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled rows error, got %v", err)
	}
}

func TestBrierScoreNilDB(t *testing.T) {
	c, _ := NewCalibrator(nil)
	score, n, err := c.BrierScore(context.Background(), "a")
	if err != nil || n != 0 || score != 0 {
		t.Fatalf("expected zero return with nil db, got score=%v n=%v err=%v", score, n, err)
	}
}

func TestMinerSuggestionsForQueryError(t *testing.T) {
	db := sqlOpen(t)
	m, _ := NewMiner(db)
	if _, err := db.Exec(`DROP TABLE chain_templates`); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SuggestionsFor(context.Background(), "code"); err == nil {
		t.Fatal("expected query error after dropping table")
	}
}

func TestMutationApplyAndTestRestoreError(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sub, "a.go")
	if err := os.WriteFile(path, []byte("true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mp := NewMutationProbe(dir, []string{"rm", "-rf", sub})
	_, err := mp.Run(context.Background(), []ChangedLine{{File: "pkg/a.go", Line: 1, Text: "true"}})
	if err == nil || !strings.Contains(err.Error(), "probe restore") {
		t.Fatalf("expected probe restore error, got %v", err)
	}
}

func TestDeclKeyDefault(t *testing.T) {
	if k, ok := declKey(&ast.BadDecl{}); ok || k != "" {
		t.Fatalf("expected default declKey to return false, got %q %v", k, ok)
	}
}

// ── confidence.go ─────────────────────────────────────────────────────────

func TestCalibrateRowsErrHook(t *testing.T) {
	db := sqlOpen(t)
	c, err := NewCalibrator(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for i := 0; i < 15; i++ {
		_ = c.Record(ctx, ConfidenceClaim{AgentName: "a", Declared: 0.8, Passed: true})
	}
	old := rowsErrHook
	rowsErrHook = func(rows *sql.Rows) error { return errors.New("rows err") }
	defer func() { rowsErrHook = old }()

	_, err = c.Calibrate(ctx, "a", 0.8)
	if err == nil || !strings.Contains(err.Error(), "rows err") {
		t.Fatalf("expected rows err, got %v", err)
	}
}

// ── impact.go ─────────────────────────────────────────────────────────────

func TestPredictFallbackToRawPath(t *testing.T) {
	old := filepathToSlash
	filepathToSlash = func(s string) string { return "to-slash:" + s }
	defer func() { filepathToSlash = old }()

	g := &ImpactGraph{
		nodes:     map[string]*PkgNode{"pkg/a": {ImportPath: "pkg/a", GoFiles: []string{"a.go"}}},
		reverse:   map[string][]string{},
		fileToPkg: map[string]string{"a.go": "pkg/a"},
	}
	imp := g.Predict([]string{"a.go"})
	if len(imp.ChangedPkgs) != 1 || imp.ChangedPkgs[0] != "pkg/a" {
		t.Fatalf("expected fallback lookup to find pkg/a, got %v", imp.ChangedPkgs)
	}
}

func TestSemanticMergeGoOneSideDelete(t *testing.T) {
	base := []byte(`package x
func Foo() int { return 1 }
func Bar() int { return 1 }
`)
	a := []byte(`package x
func Foo() int { return 1 }
`)
	b := []byte(`package x
func Foo() int { return 1 }
func Bar() int { return 1 }
`)
	res, err := SemanticMergeGo(base, a, b)
	if err != nil {
		t.Fatalf("SemanticMergeGo: %v", err)
	}
	if len(res.Conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %+v", res)
	}
	if !strings.Contains(string(res.Merged), "Bar") {
		t.Fatalf("Bar should be preserved from B")
	}
}

func TestGovernorRunRungSingleAgentCriticError(t *testing.T) {
	old := criticDriveHook
	criticDriveHook = func(c *Critic, ctx context.Context, ag Agent, task *Task, scratch *Scratchpad) (*CriticResult, error) {
		return nil, errors.New("critic fail")
	}
	defer func() { criticDriveHook = old }()

	g := &Governor{
		Ladder:   []Rung{{Name: "single", Agents: 1, RepairRounds: 1, Timeout: time.Second}},
		Verifier: NewVerifier(t.TempDir()),
		Checks:   []Check{{Kind: CheckBuild, Name: "c", Cmd: []string{"true"}}},
		Factory:  func(rung Rung) []Agent { return []Agent{stcovAgent{name: "a", out: "ok"}} },
	}
	res, err := g.Execute(context.Background(), &Task{ID: "t"}, NewScratchpad())
	if err == nil || !strings.Contains(err.Error(), "critic fail") {
		t.Fatalf("expected critic error, got %+v err=%v", res, err)
	}
}

func TestGovernorRunRungSpecRunError(t *testing.T) {
	old := specRunHook
	specRunHook = func(s *SpeculativeRunner, ctx context.Context, task *Task, agents []Agent, scratch *Scratchpad) (*SpecResult, error) {
		return nil, errors.New("spec run fail")
	}
	defer func() { specRunHook = old }()

	g := &Governor{
		Ladder:   []Rung{{Name: "multi", Agents: 2, RepairRounds: 1, Timeout: time.Second}},
		Verifier: NewVerifier(t.TempDir()),
		Checks:   []Check{{Kind: CheckBuild, Name: "c", Cmd: []string{"true"}}},
		Factory: func(rung Rung) []Agent {
			return []Agent{stcovAgent{name: "a1", out: "ok"}, stcovAgent{name: "a2", out: "ok"}}
		},
	}
	res, err := g.Execute(context.Background(), &Task{ID: "t"}, NewScratchpad())
	if err == nil || !strings.Contains(err.Error(), "spec run fail") {
		t.Fatalf("expected spec run error, got %+v err=%v", res, err)
	}
}

func TestGovernorRunRungNoWinner(t *testing.T) {
	old := specRunHook
	specRunHook = func(s *SpeculativeRunner, ctx context.Context, task *Task, agents []Agent, scratch *Scratchpad) (*SpecResult, error) {
		return &SpecResult{Candidates: []*Candidate{}}, nil
	}
	defer func() { specRunHook = old }()

	g := &Governor{
		Ladder:   []Rung{{Name: "multi", Agents: 2, RepairRounds: 1, Timeout: time.Second}},
		Verifier: NewVerifier(t.TempDir()),
		Checks:   []Check{{Kind: CheckBuild, Name: "c", Cmd: []string{"true"}}},
		Factory: func(rung Rung) []Agent {
			return []Agent{stcovAgent{name: "a1", out: "ok"}, stcovAgent{name: "a2", out: "ok"}}
		},
	}
	res, err := g.Execute(context.Background(), &Task{ID: "t"}, NewScratchpad())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.TotalRounds != 0 || res.Passed {
		t.Fatalf("expected no winner pass, got %+v", res)
	}
}

func TestGovernorRunRungRepairCriticError(t *testing.T) {
	old := specRunHook
	specRunHook = func(s *SpeculativeRunner, ctx context.Context, task *Task, agents []Agent, scratch *Scratchpad) (*SpecResult, error) {
		return &SpecResult{
			Winner: &Candidate{Agent: stcovAgent{name: "a", out: "ok"}, Verdict: &Verdict{Passed: false}},
		}, nil
	}
	defer func() { specRunHook = old }()

	oldCritic := criticDriveHook
	criticDriveHook = func(c *Critic, ctx context.Context, ag Agent, task *Task, scratch *Scratchpad) (*CriticResult, error) {
		return nil, errors.New("repair critic fail")
	}
	defer func() { criticDriveHook = oldCritic }()

	g := &Governor{
		Ladder:   []Rung{{Name: "multi", Agents: 2, RepairRounds: 1, Timeout: time.Second}},
		Verifier: NewVerifier(t.TempDir()),
		Checks:   []Check{{Kind: CheckBuild, Name: "c", Cmd: []string{"true"}}},
		Factory:  func(rung Rung) []Agent { return []Agent{stcovAgent{name: "a", out: "ok"}} },
	}
	res, err := g.Execute(context.Background(), &Task{ID: "t"}, NewScratchpad())
	if err == nil || !strings.Contains(err.Error(), "repair critic fail") {
		t.Fatalf("expected repair critic error, got %+v err=%v", res, err)
	}
}

func TestKernelWriteTreeEmptyWorkdir(t *testing.T) {
	k, _ := NewKernel(nil, "")
	tree, err := k.writeTree(context.Background())
	if err != nil || tree != "" {
		t.Fatalf("expected empty tree with no workdir, got %q err=%v", tree, err)
	}
}

func TestSinChangeCalibrationError(t *testing.T) {
	defer withPassedChecks(t)()
	db := sqlOpen(t)
	cal, _ := NewCalibrator(db)
	db.Close()
	dir := t.TempDir()
	m := &Macros{
		Workdir:  dir,
		Contract: &Contract{},
		Targeted: NewTargetedVerifier(NewVerifier(dir), &ImpactGraph{nodes: map[string]*PkgNode{}, fileToPkg: map[string]string{}}),
		Calib:    cal,
		Policy:   DefaultMergePolicy(),
		Agent:    "a",
	}
	res, err := m.SinChange(context.Background(), EditRequest{TaskID: "t", Edits: map[string][]byte{"a.go": []byte("package main")}, DeclaredConfidence: 0.9})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Applied {
		t.Fatal("expected applied even when calibration fails")
	}
}

func TestSinChangeCommitError(t *testing.T) {
	defer withPassedChecks(t)()
	old := txnCommitHook
	txnCommitHook = func(txn *FsTxn) error { return errors.New("commit fail") }
	defer func() { txnCommitHook = old }()

	dir := t.TempDir()
	m := &Macros{
		Workdir:  dir,
		Contract: &Contract{},
		Targeted: NewTargetedVerifier(NewVerifier(dir), &ImpactGraph{nodes: map[string]*PkgNode{}, fileToPkg: map[string]string{}}),
		Policy:   DefaultMergePolicy(),
	}
	res, err := m.SinChange(context.Background(), EditRequest{TaskID: "t", Edits: map[string][]byte{"a.go": []byte("package main")}})
	if err == nil || !strings.Contains(err.Error(), "commit fail") {
		t.Fatalf("expected commit error, got %+v err=%v", res, err)
	}
}
