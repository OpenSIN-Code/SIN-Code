// SPDX-License-Identifier: MIT
// Purpose: additional coverage tests for orchestrator functions that are
// uncovered in normal (non-"coverage" build tag) test runs.
package orchestrator

import (
	"context"
	"encoding/json"
	"go/ast"
	"testing"
)

// --- planToJSON (orchestrator.go:185) ---

func TestPlanToJSON(t *testing.T) {
	plan := &Plan{ID: "p1", Tasks: []*Task{{ID: "t1", Type: TaskCode}}}
	raw := planToJSON(plan)
	if len(raw) == 0 {
		t.Fatal("expected non-empty RawMessage")
	}
	var p Plan
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.ID != "p1" {
		t.Errorf("ID = %q, want p1", p.ID)
	}
}

func TestPlanToJSONNil(t *testing.T) {
	// json.Marshal(nil) returns "null", not an error, so planToJSON
	// returns the raw null bytes.
	raw := planToJSON(nil)
	if string(raw) != "null" {
		t.Errorf("expected 'null' for nil plan, got %q", raw)
	}
}

// --- recvTypeName (semmerge.go:198) ---

func TestRecvTypeNameIdent(t *testing.T) {
	expr := &ast.Ident{Name: "MyType"}
	if got := recvTypeName(expr); got != "MyType" {
		t.Errorf("recvTypeName(Ident) = %q, want MyType", got)
	}
}

func TestRecvTypeNameStarExpr(t *testing.T) {
	expr := &ast.StarExpr{X: &ast.Ident{Name: "MyType"}}
	if got := recvTypeName(expr); got != "MyType" {
		t.Errorf("recvTypeName(StarExpr) = %q, want MyType", got)
	}
}

func TestRecvTypeNameIndexExpr(t *testing.T) {
	expr := &ast.IndexExpr{X: &ast.Ident{Name: "Generic"}}
	if got := recvTypeName(expr); got != "Generic" {
		t.Errorf("recvTypeName(IndexExpr) = %q, want Generic", got)
	}
}

func TestRecvTypeNameUnknown(t *testing.T) {
	expr := &ast.SelectorExpr{X: &ast.Ident{Name: "pkg"}, Sel: &ast.Ident{Name: "Type"}}
	if got := recvTypeName(expr); got != "?" {
		t.Errorf("recvTypeName(SelectorExpr) = %q, want ?", got)
	}
}

// --- DefaultUserAgentsPath (registry.go:61) ---

func TestDefaultUserAgentsPath(t *testing.T) {
	old := userConfigDir
	userConfigDir = func() (string, error) { return "/tmp/testconfig", nil }
	defer func() { userConfigDir = old }()

	got := DefaultUserAgentsPath()
	if got != "/tmp/testconfig/sin-code/agents" {
		t.Errorf("DefaultUserAgentsPath = %q, want /tmp/testconfig/sin-code/agents", got)
	}
}

func TestDefaultUserAgentsPathError(t *testing.T) {
	old := userConfigDir
	userConfigDir = func() (string, error) { return "", context.Canceled }
	defer func() { userConfigDir = old }()

	got := DefaultUserAgentsPath()
	if got != "" {
		t.Errorf("expected empty on error, got %q", got)
	}
}

// --- StrategyRouter.load (strategy_router.go:86) ---

func TestStrategyRouterLoadFromDB(t *testing.T) {
	db := covSqlOpen(t)
	// Seed the table before creating the router.
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS router_arms (class TEXT NOT NULL, strategy TEXT NOT NULL, alpha REAL NOT NULL DEFAULT 1, beta REAL NOT NULL DEFAULT 1, PRIMARY KEY (class, strategy))`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO router_arms (class, strategy, alpha, beta) VALUES ('bugfix', 'ast-edit', 5.0, 2.0)`)
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewStrategyRouter(db, 42)
	if err != nil {
		t.Fatalf("NewStrategyRouter: %v", err)
	}
	// The loaded arm should influence picks (alpha=5, beta=2 favors ast-edit).
	pick := r.Pick(ClassBugfix, []Strategy{StratASTEdit, StratHashline})
	if pick != StratASTEdit && pick != StratHashline {
		t.Errorf("expected a valid strategy pick, got %q", pick)
	}
}

func TestStrategyRouterLoadError(t *testing.T) {
	db := covSqlOpen(t)
	r, err := NewStrategyRouter(db, 42)
	if err != nil {
		t.Fatal(err)
	}
	// Drop the table after creation so load() fails on next call.
	if _, err := db.Exec("DROP TABLE router_arms"); err != nil {
		t.Fatal(err)
	}
	if err := r.load(); err == nil {
		t.Fatal("expected error from load() with dropped table")
	}
}

func TestStrategyRouterReport(t *testing.T) {
	db := covSqlOpen(t)
	r, err := NewStrategyRouter(db, 42)
	if err != nil {
		t.Fatal(err)
	}
	err = r.Report(context.Background(), ClassRefactor, StratASTEdit, true)
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	mean, n := r.Posterior(ClassRefactor, StratASTEdit)
	if n != 1 || mean <= 0 {
		t.Errorf("after 1 success, mean=%f n=%d", mean, n)
	}
}

func TestStrategyRouterReportError(t *testing.T) {
	db := covSqlOpen(t)
	r, err := NewStrategyRouter(db, 42)
	if err != nil {
		t.Fatal(err)
	}
	// Drop table to cause error.
	if _, err := db.Exec("DROP TABLE router_arms"); err != nil {
		t.Fatal(err)
	}
	if err := r.Report(context.Background(), ClassRefactor, StratASTEdit, true); err == nil {
		t.Fatal("expected error from Report with dropped table")
	}
}

// --- Kernel tests (kernel.go) ---

func TestKernelNewWithDB(t *testing.T) {
	db := covSqlOpen(t)
	k, err := NewKernel(db, "")
	if err != nil {
		t.Fatalf("NewKernel: %v", err)
	}
	if k == nil {
		t.Fatal("nil kernel")
	}
}

func TestKernelNewNilDB(t *testing.T) {
	k, err := NewKernel(nil, "")
	if err != nil {
		t.Fatalf("NewKernel(nil): %v", err)
	}
	if k.db != nil {
		t.Error("expected nil db")
	}
}

func TestKernelCaptureNilDB(t *testing.T) {
	k, _ := NewKernel(nil, "")
	cp, err := k.Capture(context.Background(), "label", AgentState{}, true)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if cp.Label != "label" || !cp.Green {
		t.Errorf("unexpected checkpoint: %+v", cp)
	}
}

func TestKernelCaptureEmptyWorkdir(t *testing.T) {
	db := covSqlOpen(t)
	k, _ := NewKernel(db, "") // empty workdir
	cp, err := k.Capture(context.Background(), "label", AgentState{TaskID: "t1"}, true)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if cp.Label != "label" {
		t.Errorf("Label = %q", cp.Label)
	}
}

func TestKernelCaptureWithWorkdir(t *testing.T) {
	db := covSqlOpen(t)
	dir := t.TempDir()
	k, _ := NewKernel(db, dir)

	// Mock git commands so we don't need a real repo.
	oldGit := kernelGitCmd
	oldGitOut := kernelGitOutCmd
	kernelGitCmd = func(ctx context.Context, d string, args ...string) ([]byte, error) {
		return []byte("ok"), nil
	}
	kernelGitOutCmd = func(ctx context.Context, d string, args ...string) ([]byte, error) {
		return []byte("abc123\n"), nil
	}
	defer func() {
		kernelGitCmd = oldGit
		kernelGitOutCmd = oldGitOut
	}()

	cp, err := k.Capture(context.Background(), "test", AgentState{TaskID: "t1"}, true)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if cp.TreeSHA != "abc123" {
		t.Errorf("TreeSHA = %q, want abc123", cp.TreeSHA)
	}
}

func TestKernelTimelineWithData(t *testing.T) {
	db := covSqlOpen(t)
	dir := t.TempDir()
	k, _ := NewKernel(db, dir)

	oldGit := kernelGitCmd
	oldGitOut := kernelGitOutCmd
	kernelGitCmd = func(ctx context.Context, d string, args ...string) ([]byte, error) { return nil, nil }
	kernelGitOutCmd = func(ctx context.Context, d string, args ...string) ([]byte, error) { return []byte("sha\n"), nil }
	defer func() {
		kernelGitCmd = oldGit
		kernelGitOutCmd = oldGitOut
	}()

	_, _ = k.Capture(context.Background(), "first", AgentState{TaskID: "t1"}, true)
	_, _ = k.Capture(context.Background(), "second", AgentState{TaskID: "t2"}, false)

	tl, err := k.Timeline(context.Background(), 10)
	if err != nil {
		t.Fatalf("Timeline: %v", err)
	}
	if tl == "" {
		t.Fatal("expected non-empty timeline")
	}
}

func TestKernelTimelineEmptyCov(t *testing.T) {
	db := covSqlOpen(t)
	k, _ := NewKernel(db, "")
	tl, err := k.Timeline(context.Background(), 10)
	if err != nil {
		t.Fatalf("Timeline: %v", err)
	}
	if tl == "" {
		t.Fatal("expected non-empty timeline header even with no data")
	}
}

func TestKernelLastGreen(t *testing.T) {
	db := covSqlOpen(t)
	dir := t.TempDir()
	k, _ := NewKernel(db, dir)

	oldGit := kernelGitCmd
	oldGitOut := kernelGitOutCmd
	kernelGitCmd = func(ctx context.Context, d string, args ...string) ([]byte, error) { return nil, nil }
	kernelGitOutCmd = func(ctx context.Context, d string, args ...string) ([]byte, error) { return []byte("sha\n"), nil }
	defer func() {
		kernelGitCmd = oldGit
		kernelGitOutCmd = oldGitOut
	}()

	_, _ = k.Capture(context.Background(), "green1", AgentState{}, true)
	_, _ = k.Capture(context.Background(), "red1", AgentState{}, false)

	id, label, err := k.LastGreen(context.Background())
	if err != nil {
		t.Fatalf("LastGreen: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero id")
	}
	if label != "green1" {
		t.Errorf("label = %q, want green1", label)
	}
}

func TestKernelLastGreenNone(t *testing.T) {
	db := covSqlOpen(t)
	k, _ := NewKernel(db, "")
	_, _, err := k.LastGreen(context.Background())
	if err == nil {
		t.Fatal("expected error for no green checkpoint")
	}
}

func TestKernelRewindNilDB(t *testing.T) {
	k, _ := NewKernel(nil, "")
	_, err := k.Rewind(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error for nil DB rewind")
	}
}

func TestKernelRewindNotFound(t *testing.T) {
	db := covSqlOpen(t)
	k, _ := NewKernel(db, "")
	_, err := k.Rewind(context.Background(), 999)
	if err == nil {
		t.Fatal("expected error for non-existent checkpoint")
	}
}

func TestKernelTimelineNilDB(t *testing.T) {
	k, _ := NewKernel(nil, "")
	tl, err := k.Timeline(context.Background(), 10)
	if err != nil || tl != "" {
		t.Fatalf("nil DB Timeline should return empty, got %q err=%v", tl, err)
	}
}

func TestKernelLastGreenNilDB(t *testing.T) {
	k, _ := NewKernel(nil, "")
	_, _, err := k.LastGreen(context.Background())
	if err == nil {
		t.Fatal("expected error for nil DB LastGreen")
	}
}

func TestHashScratchpad(t *testing.T) {
	h := HashScratchpad([]byte("test"))
	if len(h) != 16 { // 8 bytes = 16 hex chars
		t.Errorf("expected 16 hex chars, got %d: %q", len(h), h)
	}
	h2 := HashScratchpad([]byte("test"))
	if h != h2 {
		t.Error("hash should be deterministic")
	}
	h3 := HashScratchpad([]byte("different"))
	if h == h3 {
		t.Error("different inputs should produce different hashes")
	}
}

// --- SpeculativeRunner.Run ---

func TestSpeculativeRunnerRunNoAgents(t *testing.T) {
	s := NewSpeculativeRunner("", nil)
	_, err := s.Run(context.Background(), &Task{ID: "t1"}, nil, NewScratchpad())
	if err == nil {
		t.Fatal("expected error for no agents")
	}
}

func TestSpeculativeRunnerRunWithMockAgents(t *testing.T) {
	s := NewSpeculativeRunner("", nil)
	s.WorkdirBase = t.TempDir()
	cfg := AgentConfig{Name: "mock", Type: TaskCode, Model: "test"}
	agents := []Agent{NewMockAgent(cfg)}
	res, err := s.Run(context.Background(), &Task{ID: "t1", Description: "test"}, agents, NewScratchpad())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
	if len(res.Candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(res.Candidates))
	}
}

func TestSpeculativeRunnerAddWorktreeNoRepo(t *testing.T) {
	s := NewSpeculativeRunner("", nil)
	s.WorkdirBase = t.TempDir()
	dir, err := s.addWorktree(context.Background(), "test-id")
	if err != nil {
		t.Fatalf("addWorktree: %v", err)
	}
	if dir == "" {
		t.Fatal("expected non-empty dir")
	}
}

func TestSpeculativeRunnerMergeWinnerNil(t *testing.T) {
	s := NewSpeculativeRunner("", nil)
	_, err := s.MergeWinner(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil winner")
	}
}

func TestSpeculativeRunnerMergeWinnerEmptyWorktree(t *testing.T) {
	s := NewSpeculativeRunner("", nil)
	c := &Candidate{ID: "c1", Worktree: ""}
	_, err := s.MergeWinner(context.Background(), c)
	if err == nil {
		t.Fatal("expected error for empty worktree")
	}
}

func TestSpeculativeRunnerMergeWinnerNoRepoRoot(t *testing.T) {
	s := NewSpeculativeRunner("", nil) // empty RepoRoot
	c := &Candidate{ID: "c1", Worktree: "/tmp/something"}
	_, err := s.MergeWinner(context.Background(), c)
	if err == nil {
		t.Fatal("expected error for empty RepoRoot")
	}
}

// --- Orchestrator.String ---

func TestOrchestratorStringCov(t *testing.T) {
	o := New()
	s := o.String()
	if s == "" {
		t.Fatal("expected non-empty String()")
	}
}

// --- ClassifyTask ---

func TestClassifyTaskCov(t *testing.T) {
	cases := []struct {
		title, desc string
		want        TaskClass
	}{
		{"Rename variable", "", ClassRename},
		{"Refactor module", "", ClassRefactor},
		{"Fix bug", "", ClassBugfix},
		{"Create new feature", "", ClassGreenfield},
		{"Update config yaml", "", ClassConfig},
		{"Something else", "", ClassUnknown},
	}
	for _, c := range cases {
		task := &Task{Title: c.title, Description: c.desc}
		got := ClassifyTask(task)
		if got != c.want {
			t.Errorf("ClassifyTask(%q) = %q, want %q", c.title, got, c.want)
		}
	}
}

// --- Calibrate with enough data (>10 samples) ---

func TestCalibrateWithEnoughData(t *testing.T) {
	db := covSqlOpen(t)
	c, _ := NewCalibrator(db)
	ctx := context.Background()
	for i := 0; i < 15; i++ {
		_ = c.Record(ctx, ConfidenceClaim{AgentName: "a", Declared: 0.8, Passed: true})
	}
	cal, err := c.Calibrate(ctx, "a", 0.8)
	if err != nil {
		t.Fatalf("Calibrate: %v", err)
	}
	if cal < 0 || cal > 1 {
		t.Fatalf("calibrated out of range: %f", cal)
	}
}

func TestCalibrateWithMixedData(t *testing.T) {
	db := covSqlOpen(t)
	c, _ := NewCalibrator(db)
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		_ = c.Record(ctx, ConfidenceClaim{AgentName: "a", Declared: 0.9, Passed: i%3 != 0})
	}
	cal, err := c.Calibrate(ctx, "a", 0.9)
	if err != nil {
		t.Fatalf("Calibrate: %v", err)
	}
	if cal < 0 || cal > 1 {
		t.Fatalf("calibrated out of range: %f", cal)
	}
}

func TestCalibrateRowsError(t *testing.T) {
	db := covSqlOpen(t)
	c, _ := NewCalibrator(db)
	// Record some data so the query runs, then drop the table.
	ctx := context.Background()
	_ = c.Record(ctx, ConfidenceClaim{AgentName: "a", Declared: 0.8, Passed: true})
	if _, err := db.Exec("DROP TABLE confidence_claims"); err != nil {
		t.Fatal(err)
	}
	_, err := c.Calibrate(ctx, "a", 0.8)
	if err == nil {
		t.Fatal("expected error from dropped table")
	}
}

func TestCalibrateScanError(t *testing.T) {
	db := covSqlOpen(t)
	c, _ := NewCalibrator(db)
	ctx := context.Background()
	_ = c.Record(ctx, ConfidenceClaim{AgentName: "a", Declared: 0.8, Passed: true})
	// Corrupt the schema by dropping and recreating with wrong types.
	if _, err := db.Exec("DROP TABLE confidence_claims"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE confidence_claims (declared TEXT, passed TEXT)"); err != nil {
		t.Fatal(err)
	}
	_, err := c.Calibrate(ctx, "a", 0.8)
	if err == nil {
		t.Fatal("expected scan error")
	}
}

// --- Macros SinChange ---

func TestSinChangeNoTargeted(t *testing.T) {
	m := &Macros{Workdir: t.TempDir(), Policy: DefaultMergePolicy()}
	_, err := m.SinChange(context.Background(), EditRequest{
		TaskID: "t1",
		Edits:  map[string][]byte{"a.go": []byte("package main\n")},
	})
	if err == nil {
		t.Fatal("expected error for nil Targeted")
	}
}

func TestSinChangeContractViolation(t *testing.T) {
	dir := t.TempDir()
	contract := &Contract{
		AllowedGlobs: []string{"safe/**"},
	}
	m := &Macros{
		Workdir:  dir,
		Contract: contract,
		Policy:   DefaultMergePolicy(),
	}
	res, err := m.SinChange(context.Background(), EditRequest{
		TaskID: "t1",
		Edits:  map[string][]byte{"unsafe/file.go": []byte("package main\n")},
	})
	if err != nil {
		t.Fatalf("SinChange: %v", err)
	}
	if res.Decision != DecisionBlock {
		t.Errorf("expected block for contract violation, got %s", res.Decision)
	}
	if res.Applied {
		t.Error("should not be applied on contract violation")
	}
}

func TestSinChangeEmptyWorkdir(t *testing.T) {
	m := &Macros{Workdir: "", Policy: DefaultMergePolicy()}
	res, err := m.SinChange(context.Background(), EditRequest{
		TaskID: "t1",
		Edits:  map[string][]byte{"a.go": []byte("package main\n")},
	})
	// With empty workdir, BeginTxn("") creates a txn in cwd.
	// WriteFile may succeed, but Targeted is nil so it errors.
	if err == nil {
		t.Fatal("expected error for nil Targeted")
	}
	_ = res
}

// --- agentloop loop_agent WithHookEngine ---

func TestWithHookEngine(t *testing.T) {
	// WithHookEngine returns a LoopAgentOption. We can't easily construct
	// a real hooks.Engine without importing the package, but we can verify
	// the option function doesn't panic when called with nil.
	opt := WithHookEngine(nil)
	if opt == nil {
		t.Fatal("expected non-nil option")
	}
	// Apply it to a LoopAgent — just verify it doesn't panic.
	la := &LoopAgent{}
	opt(la)
	if la.hooks != nil {
		t.Error("expected nil hooks")
	}
}

func TestWithMaxTurns(t *testing.T) {
	opt := WithMaxTurns(42)
	la := &LoopAgent{}
	opt(la)
	if la.maxTurns != 42 {
		t.Errorf("maxTurns = %d, want 42", la.maxTurns)
	}
}

func TestWithWorkspace(t *testing.T) {
	opt := WithWorkspace("/some/path")
	la := &LoopAgent{}
	opt(la)
	if la.workspace != "/some/path" {
		t.Errorf("workspace = %q, want /some/path", la.workspace)
	}
}

// --- packageClause ---

func TestPackageClause(t *testing.T) {
	src := []byte("package main\n\nfunc foo() {}\n")
	got := packageClause(src)
	if got != "package main\n\n" {
		t.Errorf("packageClause = %q, want %q", got, "package main\n\n")
	}
}

func TestPackageClauseNoPackage(t *testing.T) {
	src := []byte("// just a comment\n")
	got := packageClause(src)
	// packageClause defaults to "package main\n\n" when no clause found.
	if got != "package main\n\n" {
		t.Errorf("expected default 'package main\\n\\n', got %q", got)
	}
}

// --- sortKVs ---

func TestSortKVs(t *testing.T) {
	sorted := []struct {
		key string
		pos int
	}{{"b", 2}, {"a", 1}, {"c", 3}}
	sortKVs(sorted)
	if sorted[0].pos != 1 || sorted[1].pos != 2 || sorted[2].pos != 3 {
		t.Errorf("expected sorted by pos [1,2,3], got %v", sorted)
	}
}

func TestSortKVsSingleElement(t *testing.T) {
	sorted := []struct {
		key string
		pos int
	}{{"only", 5}}
	sortKVs(sorted)
	if sorted[0].pos != 5 {
		t.Error("single element should remain unchanged")
	}
}
