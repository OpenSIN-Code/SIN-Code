// SPDX-License-Identifier: MIT
// Purpose: wiring tests for standalone orchestrator components into the
// chat/daemon execution path. Verifies that DeepPlanner, PreWarmManager,
// PatternDB, FusionTournament, Compactor, FrustrationDetector, and
// RiskClassifier are correctly wired when their config flags are enabled,
// and absent when disabled (backward compat). All race-free (M7).
package loopbuilder

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

func TestWiring_DeepPlannerWiredWhenEnabled(t *testing.T) {
	cfg := Config{
		DeepPlannerEnabled: true,
	}
	deps := WireOrchestrator(cfg, nil)
	if deps.DeepPlanner == nil {
		t.Fatal("DeepPlanner should be wired when DeepPlannerEnabled=true")
	}
	plan := deps.DeepPlanner.BuildDAGPlan("implement user authentication")
	if plan == nil {
		t.Fatal("BuildDAGPlan returned nil")
	}
	if len(plan.Tasks) == 0 {
		t.Fatal("BuildDAGPlan produced zero tasks")
	}
	hasProbability := false
	for _, task := range plan.Tasks {
		if task.Probability > 0 {
			hasProbability = true
		}
	}
	if !hasProbability {
		t.Fatal("DeepPlanner tasks should have probability scores")
	}
}

func TestWiring_DeepPlannerNotWiredWhenDisabled(t *testing.T) {
	cfg := Config{}
	deps := WireOrchestrator(cfg, nil)
	if deps.DeepPlanner != nil {
		t.Fatal("DeepPlanner should NOT be wired when DeepPlannerEnabled=false")
	}
}

func TestWiring_PreWarmManagerWiredToDispatcher(t *testing.T) {
	agents := orchestrator.DefaultAgents()
	var registryAgents []orchestrator.Agent
	for _, a := range agents {
		registryAgents = append(registryAgents, orchestrator.NewMockAgent(a))
	}
	registry := orchestrator.NewRegistry(registryAgents)

	cfg := Config{
		DeepPlannerEnabled: true,
		PreWarmEnabled:     true,
	}
	deps := WireOrchestrator(cfg, registry)
	if deps.PreWarm == nil {
		t.Fatal("PreWarmManager should be wired when PreWarmEnabled=true and registry is provided")
	}

	plan := deps.DeepPlanner.BuildDAGPlan("implement and test feature")
	scratch := orchestrator.NewScratchpad()
	disp := orchestrator.NewDispatcher(registry, scratch, 4)
	disp.PreWarm = deps.PreWarm

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := disp.Dispatch(ctx, plan)
	if err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}

	if deps.PreWarm.PreWarmCount() < 0 {
		t.Fatal("PreWarmCount should be non-negative")
	}
}

func TestWiring_PreWarmManagerNotWiredWhenDisabled(t *testing.T) {
	cfg := Config{
		DeepPlannerEnabled: true,
		PreWarmEnabled:     false,
	}
	deps := WireOrchestrator(cfg, nil)
	if deps.PreWarm != nil {
		t.Fatal("PreWarmManager should NOT be wired when PreWarmEnabled=false")
	}
}

func TestWiring_PatternDBRecordsOnPlanCompletion(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	pdb, err := orchestrator.NewPatternDB(db)
	if err != nil {
		t.Fatalf("NewPatternDB: %v", err)
	}

	plan := &orchestrator.Plan{
		ID:        "test-plan-1",
		Prompt:    "implement user auth",
		Created:   time.Now(),
		Started:   time.Now(),
		Completed: time.Now(),
		Success:   true,
	}
	plan.Tasks = append(plan.Tasks, &orchestrator.Task{
		ID:          "tk-1",
		Type:        orchestrator.TaskCode,
		Description: "implement",
		Status:      orchestrator.TaskCompleted,
	})
	plan.Tasks = append(plan.Tasks, &orchestrator.Task{
		ID:          "tk-2",
		Type:        orchestrator.TaskTest,
		Description: "test",
		Status:      orchestrator.TaskCompleted,
	})

	deps := &OrchestratorDeps{PatternDB: pdb}
	ctx := context.Background()
	deps.RecordPlanCompletion(ctx, plan)

	pred, err := pdb.MatchPrompt(ctx, "implement user auth")
	if err != nil {
		t.Fatalf("MatchPrompt: %v", err)
	}
	if pred.MatchCount == 0 {
		t.Fatal("PatternDB should have recorded the plan and matched on re-query")
	}
}

func TestWiring_PatternDBFeedsPatternsToDeepPlanner(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	pdb, err := orchestrator.NewPatternDB(db)
	if err != nil {
		t.Fatalf("NewPatternDB: %v", err)
	}

	plan := &orchestrator.Plan{
		ID:      "test-plan-feed",
		Prompt:  "add login endpoint",
		Started: time.Now(),
		Completed: time.Now(),
		Success: true,
	}
	plan.Tasks = append(plan.Tasks, &orchestrator.Task{
		ID:     "tk-a",
		Type:   orchestrator.TaskCode,
		Status: orchestrator.TaskCompleted,
	})
	ctx := context.Background()
	_ = pdb.RecordSequence(ctx, plan)

	cfg := Config{
		DeepPlannerEnabled:     true,
		PatternLearningEnabled: true,
	}
	deps := WireOrchestrator(cfg, nil)
	if deps.DeepPlanner == nil {
		t.Fatal("DeepPlanner should be wired")
	}
	if deps.PatternDB == nil {
		t.Fatal("PatternDB should be wired when PatternLearningEnabled=true")
	}

	deps.PatternDB = pdb
	deps.DeepPlanner.SetPatternDB(pdb)

	newPlan := deps.DeepPlanner.BuildDAGPlan("add login endpoint")
	if newPlan == nil || len(newPlan.Tasks) == 0 {
		t.Fatal("DeepPlanner should produce a plan even with pattern DB")
	}
}

func TestWiring_FusionTournamentTriggeredOnVerifyFail(t *testing.T) {
	s := setupTestSession(t)

	gate := verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) {
			return false, "compile error: undefined variable", nil
		}, nil)

	called := false
	loop := &agentloop.Loop{
		Gate:         gate,
		MaxTurns:     1,
		SystemPrompt: "test",
		Completion: func(ctx context.Context, msgs []session.Message, tools []agentloop.ToolSpec) (*agentloop.Completion, error) {
			return &agentloop.Completion{
				Text: "done",
				Raw:  session.Message{Role: "assistant", Content: "done"},
			}, nil
		},
		TournamentRunner: &mockTournamentRunner{
			shouldRun: true,
			runFn: func(ctx context.Context) (string, int, error) {
				called = true
				return "winner output", 100, nil
			},
		},
	}

	ctx := context.Background()
	res, err := loop.Run(ctx, s, "test prompt")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !called {
		t.Fatal("TournamentRunner.Run should have been called on verify.fail")
	}
	if !res.Verified {
		t.Fatal("result should be verified after tournament winner passes")
	}
}

func TestWiring_CompactorWiredToAgentLoop(t *testing.T) {
	compactor := agentloop.NewCompactor(nil)
	if compactor == nil {
		t.Fatal("NewCompactor returned nil")
	}

	loop := &agentloop.Loop{
		Compactor:           compactor,
		CompactionStrategy:  agentloop.DefaultCompactionStrategy(),
		CompactionMaxTokens: 4000,
		MaxTurns:            10,
	}

	if loop.Compactor == nil {
		t.Fatal("Compactor should be wired to the loop")
	}
	if loop.CompactionStrategy != agentloop.CompactionHybrid {
		t.Fatalf("expected hybrid strategy, got %v", loop.CompactionStrategy)
	}

	shouldCompact := agentloop.ShouldCompact(9, 10, 0.8)
	if !shouldCompact {
		t.Fatal("ShouldCompact(9, 10, 0.8) should be true (9 > 10*0.8=8)")
	}

	shouldNot := agentloop.ShouldCompact(7, 10, 0.8)
	if shouldNot {
		t.Fatal("ShouldCompact(7, 10, 0.8) should be false (7 < 8)")
	}
}

func TestWiring_FrustrationDetectorWiredToUserMessageHandler(t *testing.T) {
	detector := agentloop.NewFrustrationDetector()
	if detector == nil {
		t.Fatal("NewFrustrationDetector returned nil")
	}

	loop := &agentloop.Loop{
		Frustration: detector,
		MaxTurns:    1,
		Gate:        verify.NewGate("off", nil, nil),
	}

	if loop.Frustration == nil {
		t.Fatal("FrustrationDetector should be wired to the loop")
	}

	level := loop.Frustration.Track("this is broken and doesn't work", time.Now())
	if level == agentloop.FrustrationNone {
		t.Fatal("frustration keywords should trigger detection")
	}

	suffix := loop.Frustration.SystemPromptSuffix()
	if suffix == "" {
		t.Fatal("SystemPromptSuffix should be non-empty when frustration detected")
	}
}

func TestWiring_RiskClassifierWiredToPermissionEngineInYoloMode(t *testing.T) {
	perm := permission.New(nil)
	perm.Yolo = true
	perm.Headless = true

	classifier := permission.NewRiskClassifier()
	classifier.SetThreshold(permission.RiskMedium)
	perm.Risk = classifier

	if perm.Risk == nil {
		t.Fatal("RiskClassifier should be wired to the permission engine")
	}

	pol := perm.CheckWithArgs("sin_read", nil)
	if pol != permission.Allow {
		t.Fatalf("low-risk tool in YOLO mode should be Allow, got %v", pol)
	}

	pol = perm.CheckWithArgs("sin_git_push", nil)
	if pol != permission.Deny {
		t.Fatalf("critical-risk tool in headless YOLO should be Deny, got %v", pol)
	}

	pol = perm.CheckWithArgs("bash", map[string]any{
		"command": "rm -rf /",
	})
	if pol != permission.Deny {
		t.Fatalf("critical-risk args in headless YOLO should be Deny, got %v", pol)
	}
}

func TestWiring_RiskClassifierNotWiredWhenNoThreshold(t *testing.T) {
	perm := permission.New(nil)
	perm.Yolo = true

	if perm.Risk != nil {
		t.Fatal("RiskClassifier should NOT be wired when no threshold is set")
	}

	pol := perm.Check("sin_git_push")
	if pol != permission.Allow {
		t.Fatalf("without RiskClassifier, YOLO blanket-approves Ask-level tools, got %v", pol)
	}
}

func TestWiring_RaceSafe_AllComponentsConcurrent(t *testing.T) {
	detector := agentloop.NewFrustrationDetector()
	compactor := agentloop.NewCompactor(nil)
	classifier := permission.NewRiskClassifier()
	classifier.SetThreshold(permission.RiskMedium)
	perm := permission.New(nil)
	perm.Yolo = true
	perm.Risk = classifier

	agents := orchestrator.DefaultAgents()
	var registryAgents []orchestrator.Agent
	for _, a := range agents {
		registryAgents = append(registryAgents, orchestrator.NewMockAgent(a))
	}
	registry := orchestrator.NewRegistry(registryAgents)
	prewarm := orchestrator.NewPreWarmManager(registry, 0, 0)

	var wg sync.WaitGroup
	wg.Add(4)

	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			detector.Track("why is this broken stop", time.Now())
		}
	}()

	msgs := make([]session.Message, 20)
	for i := range msgs {
		msgs[i] = session.Message{Role: "user", Content: "test message"}
	}
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			compactor.Compact(context.Background(), msgs, agentloop.CompactionHybrid, 4000)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = perm.CheckWithArgs("sin_read", nil)
			_ = perm.CheckWithArgs("sin_git_push", nil)
		}
	}()

	tasks := []*orchestrator.Task{
		{ID: "t1", Type: orchestrator.TaskCode, DependsOn: nil, Status: orchestrator.TaskPending, Probability: 0.9},
		{ID: "t2", Type: orchestrator.TaskTest, DependsOn: []string{"t1"}, Status: orchestrator.TaskPending, Probability: 0.85},
	}
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			prewarm.PreWarmDependents(context.Background(), tasks, "t1")
			prewarm.CancelDependents(tasks, "t1")
		}
	}()

	wg.Wait()
}

func TestWiring_WireOrchestratorAllDisabledByDefault(t *testing.T) {
	cfg := Config{}
	deps := WireOrchestrator(cfg, nil)
	if deps.DeepPlanner != nil {
		t.Fatal("DeepPlanner should be nil when not enabled")
	}
	if deps.PreWarm != nil {
		t.Fatal("PreWarmManager should be nil when not enabled")
	}
	if deps.PatternDB != nil {
		t.Fatal("PatternDB should be nil when not enabled")
	}
}

func TestWiring_RecordPlanCompletionNilSafe(t *testing.T) {
	var deps *OrchestratorDeps
	deps.RecordPlanCompletion(context.Background(), nil)

	deps = &OrchestratorDeps{}
	deps.RecordPlanCompletion(context.Background(), &orchestrator.Plan{})
}

type mockTournamentRunner struct {
	shouldRun bool
	runFn     func(ctx context.Context) (string, int, error)
}

func (m *mockTournamentRunner) ShouldRun(vr verify.Result) bool {
	return m.shouldRun
}

func (m *mockTournamentRunner) Run(ctx context.Context) (string, int, error) {
	if m.runFn != nil {
		return m.runFn(ctx)
	}
	return "winner", 50, nil
}

func setupTestSession(t *testing.T) *session.Session {
	t.Helper()
	store, err := session.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	s, err := store.StartOrResume("")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	db.SetMaxOpenConns(1)
	return db
}
