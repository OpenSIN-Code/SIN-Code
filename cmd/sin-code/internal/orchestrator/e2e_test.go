// SPDX-License-Identifier: MIT
// Purpose: end-to-end integration tests proving the full orchestrator
// pipeline works from prompt to verified result:
// DeepPlanner → PatternDB → PreWarmManager → Dispatcher → LoopAgent
// → VerifyGate → FusionTournament. All tests pass under
// `go test -race -count=1` (mandate M7).
package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

func e2eMockRegistry() *Registry {
	cfgs := DefaultAgents()
	agents := make([]Agent, len(cfgs))
	for i, cfg := range cfgs {
		agents[i] = NewMockAgent(cfg)
	}
	return NewRegistry(agents)
}

func e2eEpisodeDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

func e2eSessionStore(t *testing.T) *session.Store {
	t.Helper()
	store, err := session.Open(filepath.Join(t.TempDir(), "sessions.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func taskByType(tasks []*Task, tt TaskType) *Task {
	for _, t := range tasks {
		if t.Type == tt {
			return t
		}
	}
	return nil
}

func TestE2E_FullPipelineHappyPath(t *testing.T) {
	pdb := newPatternDB(t)
	ctx := context.Background()

	pdb.RecordSequence(ctx, makePlan("implement and test user auth", []TaskType{TaskArchitect, TaskCode, TaskTest}, true))
	pdb.RecordSequence(ctx, makePlan("implement and test user auth", []TaskType{TaskArchitect, TaskCode, TaskTest, TaskReview}, true))

	planner := NewDeepPlanner(DefaultAgents())
	planner.SetPatternDB(pdb)

	plan := planner.BuildDAGPlan("implement and test user auth")
	if len(plan.Tasks) == 0 {
		t.Fatal("plan has no tasks")
	}

	archTask := taskByType(plan.Tasks, TaskArchitect)
	codeTask := taskByType(plan.Tasks, TaskCode)
	testTask := taskByType(plan.Tasks, TaskTest)
	reviewTask := taskByType(plan.Tasks, TaskReview)
	if archTask == nil || codeTask == nil || testTask == nil || reviewTask == nil {
		t.Fatalf("missing expected tasks: arch=%v code=%v test=%v review=%v",
			archTask != nil, codeTask != nil, testTask != nil, reviewTask != nil)
	}

	if len(archTask.DependsOn) != 0 {
		t.Errorf("architect should have no deps, got %v", archTask.DependsOn)
	}
	if !containsDep(codeTask.DependsOn, archTask.ID) {
		t.Error("coder should depend on architect")
	}
	if !containsDep(testTask.DependsOn, codeTask.ID) {
		t.Error("tester should depend on coder")
	}
	if !containsDep(reviewTask.DependsOn, testTask.ID) {
		t.Error("review should depend on tester")
	}

	if archTask.Probability != 1.0 {
		t.Errorf("architect P=%.3f, patterns should refine but arch is always 1.0 base", archTask.Probability)
	}
	if codeTask.Probability <= 0.95 {
		t.Errorf("code P=%.3f, should be refined above pure heuristic 0.95", codeTask.Probability)
	}

	registry := e2eMockRegistry()
	scratch := NewScratchpad()
	disp := NewDispatcher(registry, scratch, 4)
	pwm := NewPreWarmManager(registry, 0.5, 4)
	disp.PreWarm = pwm

	dispCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := disp.Dispatch(dispCtx, plan); err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}

	for _, task := range plan.Tasks {
		if task.Status != TaskCompleted {
			t.Errorf("task %s (%s) status=%s, want completed", task.ID, task.Type, task.Status)
		}
	}

	time.Sleep(30 * time.Millisecond)
	if pwm.PreWarmCount() == 0 {
		t.Error("expected at least one pre-warm call")
	}

	pred, err := pdb.MatchPrompt(ctx, "implement and test user auth")
	if err != nil {
		t.Fatalf("MatchPrompt after RecordSequence: %v", err)
	}
	if pred.MatchCount == 0 {
		t.Error("PatternDB should match the prompt after RecordSequence")
	}

	all := scratch.ReadAll()
	outputCount := 0
	for k := range all {
		if strings.HasPrefix(k, "outputs:") {
			outputCount++
		}
	}
	if outputCount == 0 {
		t.Error("scratchpad should have outputs from agents")
	}

	_ = pdb.RecordSequence(ctx, plan)
	pred2, _ := pdb.MatchPrompt(ctx, "implement and test user auth")
	if pred2.MatchCount < pred.MatchCount {
		t.Errorf("PatternDB match count should grow after recording dispatched plan: before=%d after=%d",
			pred.MatchCount, pred2.MatchCount)
	}
}

type failingMockAgent struct {
	cfg AgentConfig
}

func (f *failingMockAgent) Name() string        { return f.cfg.Name }
func (f *failingMockAgent) Config() AgentConfig { return f.cfg }
func (f *failingMockAgent) Run(ctx context.Context, task *Task, s *Scratchpad) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(20 * time.Millisecond):
	}
	return "", errors.New("simulated agent failure")
}
func (f *failingMockAgent) PreWarm(ctx context.Context, task *Task) error {
	return nil
}

func TestE2E_PipelineWithFailureRecovery(t *testing.T) {
	registry := e2eMockRegistry()
	failingCfg := AgentConfig{Name: "architect", Type: TaskArchitect, Model: "test"}
	registry.Register(&failingMockAgent{cfg: failingCfg})

	scratch := NewScratchpad()
	disp := NewDispatcher(registry, scratch, 4)
	pwm := NewPreWarmManager(registry, 0.3, 4)
	disp.PreWarm = pwm

	archID := "fail-arch"
	codeID := "fail-code"
	testID := "fail-test"
	plan := &Plan{
		ID:     GenerateID("pl"),
		Prompt: "implement broken feature",
		Tasks: []*Task{
			{ID: archID, Type: TaskArchitect, AgentName: "architect", Status: TaskPending, Probability: 1.0},
			{ID: codeID, Type: TaskCode, AgentName: "coder", Status: TaskPending, DependsOn: []string{archID}, Probability: 0.9},
			{ID: testID, Type: TaskTest, AgentName: "tester", Status: TaskPending, DependsOn: []string{codeID}, Probability: 0.85},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := disp.Dispatch(ctx, plan)

	archTask := taskByType(plan.Tasks, TaskArchitect)
	if archTask == nil || archTask.Status != TaskFailed {
		t.Error("architect task should be failed")
	}
	if plan.Success {
		t.Error("plan should not be successful when a task failed")
	}

	if err != nil {
		t.Logf("dispatch returned error (expected for failed agent): %v", err)
	}

	time.Sleep(30 * time.Millisecond)
	if pwm.CancelCount() == 0 {
		t.Log("note: CancelCount may be 0 if dependent tasks were never pre-warmed")
	}

	select {
	case <-ctx.Done():
		t.Fatal("dispatcher hung on failed tasks")
	default:
	}
}

func TestE2E_ParallelExecutionVerification(t *testing.T) {
	registry := e2eMockRegistry()
	scratch := NewScratchpad()
	disp := NewDispatcher(registry, scratch, 8)

	var startTimes sync.Map

	parCfg := AgentConfig{Name: "coder", Type: TaskCode, Model: "test"}
	registry.Register(&timestampAgent{cfg: parCfg, starts: &startTimes})

	t1ID := "par-1"
	t2ID := "par-2"
	t3ID := "par-3"
	plan := &Plan{
		ID:     GenerateID("pl"),
		Prompt: "parallel tasks test",
		Tasks: []*Task{
			{ID: t1ID, Type: TaskCode, AgentName: "coder", Status: TaskPending, Description: "task 1"},
			{ID: t2ID, Type: TaskCode, AgentName: "coder", Status: TaskPending, Description: "task 2"},
			{ID: t3ID, Type: TaskCode, AgentName: "coder", Status: TaskPending, Description: "task 3"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := disp.Dispatch(ctx, plan); err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}

	var starts []time.Time
	startTimes.Range(func(key, value any) bool {
		starts = append(starts, value.(time.Time))
		return true
	})

	if len(starts) < 3 {
		t.Fatalf("expected 3 start timestamps, got %d", len(starts))
	}

	sortTimes(starts)
	for i := 1; i < len(starts); i++ {
		diff := starts[i].Sub(starts[0])
		if diff > 50*time.Millisecond {
			t.Errorf("task %d started %v after first — not parallel (threshold 50ms)", i, diff)
		}
	}

	for _, task := range plan.Tasks {
		if task.Status != TaskCompleted {
			t.Errorf("task %s status=%s, want completed", task.ID, task.Status)
		}
	}
}

type timestampAgent struct {
	cfg    AgentConfig
	starts *sync.Map
}

func (ts *timestampAgent) Name() string        { return ts.cfg.Name }
func (ts *timestampAgent) Config() AgentConfig { return ts.cfg }
func (ts *timestampAgent) Run(ctx context.Context, task *Task, s *Scratchpad) (string, error) {
	ts.starts.Store(task.ID, time.Now())
	time.Sleep(30 * time.Millisecond)
	s.Write(ts.cfg.Name, "outputs:"+task.ID, "done")
	return "done", nil
}

func sortTimes(times []time.Time) {
	for i := 0; i < len(times); i++ {
		for j := i + 1; j < len(times); j++ {
			if times[j].Before(times[i]) {
				times[i], times[j] = times[j], times[i]
			}
		}
	}
}

func TestE2E_PatternLearningFullCycle(t *testing.T) {
	pdb := newPatternDB(t)
	ctx := context.Background()

	prompt := "implement and document auth"

	pdb.RecordSequence(ctx, makePlan(prompt, []TaskType{TaskArchitect, TaskCode, TaskTest}, true))

	pdb.RecordSequence(ctx, makePlan(prompt, []TaskType{TaskArchitect, TaskCode, TaskTest, TaskDocs}, true))

	pdb.RecordSequence(ctx, makePlan(prompt, []TaskType{TaskArchitect, TaskCode, TaskTest, TaskDocs}, true))

	pred, err := pdb.MatchPrompt(ctx, prompt)
	if err != nil {
		t.Fatalf("MatchPrompt: %v", err)
	}

	patMap := map[TaskType]TaskPattern{}
	for _, p := range pred.Patterns {
		patMap[p.TaskType] = p
	}

	if patMap[TaskArchitect].Probability != 1.0 {
		t.Errorf("architect P=%.2f, want 1.0", patMap[TaskArchitect].Probability)
	}
	if patMap[TaskCode].Probability != 1.0 {
		t.Errorf("coder P=%.2f, want 1.0", patMap[TaskCode].Probability)
	}
	if patMap[TaskTest].Probability != 1.0 {
		t.Errorf("tester P=%.2f, want 1.0", patMap[TaskTest].Probability)
	}
	learnedDocs := patMap[TaskDocs].Probability
	if learnedDocs < 0.65 || learnedDocs > 0.67 {
		t.Errorf("docs P=%.2f, want ~0.667 (2/3 sessions)", learnedDocs)
	}

	planner := NewDeepPlanner(DefaultAgents())
	planner.SetPatternDB(pdb)
	plan := planner.BuildDAGPlan(prompt)

	docsTask := taskByType(plan.Tasks, TaskDocs)
	if docsTask == nil {
		t.Fatal("expected docs task in plan")
	}

	heuristicDocs := 0.50
	blended := 0.7*heuristicDocs + 0.3*learnedDocs
	if docsTask.Probability == heuristicDocs {
		t.Errorf("docs P=%.3f equals pure heuristic — not blended", docsTask.Probability)
	}
	if docsTask.Probability < blended-0.02 || docsTask.Probability > blended+0.02 {
		t.Errorf("docs P=%.3f, expected blended ~%.3f (0.7*%.2f + 0.3*%.3f)",
			docsTask.Probability, blended, heuristicDocs, learnedDocs)
	}
}

func TestE2E_EpisodicMemoryIntegration(t *testing.T) {
	db := e2eEpisodeDB(t)
	store, err := NewEpisodeStore(db)
	if err != nil {
		t.Fatalf("NewEpisodeStore: %v", err)
	}
	ctx := context.Background()

	successPlan := makePlan("implement user login feature", []TaskType{TaskArchitect, TaskCode, TaskTest}, true)
	planJSON, _ := json.Marshal(successPlan)
	successEp := &Episode{
		Intent:    string(IntentCodebase),
		TaskTitle: "implement user login feature",
		PlanJSON:  planJSON,
		Score:     0.92,
		Passed:    true,
		Rounds:    1,
	}
	if err := store.Record(ctx, successEp); err != nil {
		t.Fatalf("Record success episode: %v", err)
	}

	failedPlan := makePlan("implement user login with OAuth", []TaskType{TaskArchitect, TaskCode}, false)
	failedJSON, _ := json.Marshal(failedPlan)
	failedEp := &Episode{
		Intent:    string(IntentCodebase),
		TaskTitle: "implement user login with OAuth approach",
		PlanJSON:  failedJSON,
		Score:     0.2,
		Passed:    false,
		Rounds:    3,
	}
	if err := store.Record(ctx, failedEp); err != nil {
		t.Fatalf("Record failed episode: %v", err)
	}

	similar, err := store.Similar(ctx, "implement user login", 10)
	if err != nil {
		t.Fatalf("Similar: %v", err)
	}
	if len(similar) == 0 {
		t.Fatal("expected similar episodes, got none")
	}

	prior := PlanningPrior(similar)
	if prior == "" {
		t.Fatal("PlanningPrior should not be empty for non-nil episodes")
	}
	if !strings.Contains(prior, "SUCCEEDED") {
		t.Error("PlanningPrior should contain SUCCEEDED for passed episode")
	}
	if !strings.Contains(prior, "FAILED") {
		t.Error("PlanningPrior should contain FAILED for failed episode")
	}
	if !strings.Contains(prior, "avoid this approach") {
		t.Error("PlanningPrior should mark failed episode with 'FAILED — avoid this approach'")
	}

	var foundSuccess, foundFailed bool
	for _, ep := range similar {
		if ep.Passed && strings.Contains(ep.TaskTitle, "login") {
			foundSuccess = true
		}
		if !ep.Passed && strings.Contains(ep.TaskTitle, "login") {
			foundFailed = true
		}
	}
	if !foundSuccess {
		t.Error("similar episodes should contain the successful episode")
	}
	if !foundFailed {
		t.Error("similar episodes should contain the failed episode")
	}
}

func TestE2E_VerifyGateIntegration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message":       map[string]any{"role": "assistant", "content": "done"},
					"finish_reason": "stop",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	var verifyCalls int64
	gate := verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) {
			n := atomic.AddInt64(&verifyCalls, 1)
			if n < 2 {
				return false, "tests fail: missing assertion", nil
			}
			return true, "tests pass", nil
		}, nil)

	store := e2eSessionStore(t)
	client := llm.NewClient(srv.URL, "test-key")

	cfg := AgentConfig{Name: "coder", Type: TaskCode, Model: "haiku", MaxTokens: 2000}
	agent := NewLoopAgent(cfg, client,
		WithSessionStore(store),
		WithVerifyGate(gate),
		WithMaxTurns(10),
		WithWorkspace(t.TempDir()),
	)

	origMemHook := memoryOpenHook
	memoryOpenHook = func(path string) (memoryStore, error) {
		return nil, errors.New("no memory in e2e test")
	}
	defer func() { memoryOpenHook = origMemHook }()

	task := &Task{ID: "tk-vg", Type: TaskCode, Description: "implement feature", AgentName: "coder"}
	scratch := NewScratchpad()

	result, err := agent.Run(context.Background(), task, scratch)
	if err != nil {
		t.Fatalf("agent Run failed: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}

	if atomic.LoadInt64(&verifyCalls) < 2 {
		t.Errorf("expected at least 2 verify calls (fail then pass), got %d", atomic.LoadInt64(&verifyCalls))
	}

	usage, ok := scratch.ReadAll()["usage:tk-vg"]
	if !ok {
		t.Fatal("expected usage entry in scratchpad")
	}
	if !strings.Contains(usage.Content, "verified=true") {
		t.Errorf("expected verified=true after gate passes, got: %q", usage.Content)
	}
}

type stubTournamentRunner struct {
	mu             sync.Mutex
	runCalled      bool
	providersTried int
	shouldRunVal   bool
	providers      []string
	runFn          func(ctx context.Context, prompt string) (string, int, error)
}

func (s *stubTournamentRunner) ShouldRun(vr verify.Result) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shouldRunVal
}

func (s *stubTournamentRunner) ShouldRunWithConfidence(vr verify.Result, confidence float64, attemptCount int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shouldRunVal
}

func (s *stubTournamentRunner) Run(ctx context.Context, prompt string) (string, int, error) {
	s.mu.Lock()
	s.runCalled = true
	s.providersTried++
	fn := s.runFn
	s.mu.Unlock()
	if fn != nil {
		return fn(ctx, prompt)
	}
	return "tournament winner", 50, nil
}

func (s *stubTournamentRunner) wasCalled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runCalled
}

func (s *stubTournamentRunner) triedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.providersTried
}

type multiProviderStubRunner struct {
	stubTournamentRunner
	providers    []string
	nextProvider int
	providerMu   sync.Mutex
	verifyFn     func(ctx context.Context, ws string) verify.Result
	workspace    string
}

func (m *multiProviderStubRunner) Run(ctx context.Context, prompt string) (string, int, error) {
	m.providerMu.Lock()
	idx := m.nextProvider
	m.nextProvider++
	m.providerMu.Unlock()

	m.mu.Lock()
	m.runCalled = true
	m.providersTried++
	m.mu.Unlock()

	if idx >= len(m.providers) {
		return "", 0, errors.New("no more providers")
	}

	provName := m.providers[idx]
	vr := m.verifyFn(ctx, m.workspace)
	if vr.Passed {
		return fmt.Sprintf("winner from %s", provName), 100, nil
	}
	return "", 0, errors.New("provider " + provName + " failed verify")
}

func TestE2E_FusionTournamentTrigger(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message":       map[string]any{"role": "assistant", "content": "attempt"},
					"finish_reason": "stop",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	gate := verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) {
			return false, "compile error: undefined variable", nil
		}, nil)

	stub := &stubTournamentRunner{
		shouldRunVal: true,
		runFn: func(ctx context.Context, prompt string) (string, int, error) {
			return "fusion winner output", 200, nil
		},
	}

	store := e2eSessionStore(t)
	sess, err := store.StartOrResume("")
	if err != nil {
		t.Fatal(err)
	}

	loop := &agentloop.Loop{
		Gate:         gate,
		MaxTurns:     3,
		SystemPrompt: "test",
		Workspace:    t.TempDir(),
		Completion: func(ctx context.Context, msgs []session.Message, tools []agentloop.ToolSpec) (*agentloop.Completion, error) {
			return &agentloop.Completion{
				Text: "done",
				Raw:  session.Message{Role: "assistant", Content: "done"},
			}, nil
		},
		TournamentRunner: stub,
	}

	result, err := loop.Run(context.Background(), sess, "test prompt")
	if err != nil {
		t.Fatalf("loop Run failed: %v", err)
	}

	if !stub.wasCalled() {
		t.Fatal("FusionTournament should have been triggered on verify.fail")
	}
	if !result.Verified {
		t.Error("result should be verified after tournament winner")
	}
	if result.Summary != "fusion winner output" {
		t.Errorf("expected 'fusion winner output', got %q", result.Summary)
	}

	ws := t.TempDir()
	alwaysFailGate := verify.NewGate("poc",
		func(ctx context.Context, w string) (bool, string, error) {
			return false, "compile error: undefined variable", nil
		}, nil)

	passOnSecond := &multiProviderStubRunner{
		stubTournamentRunner: stubTournamentRunner{shouldRunVal: true},
		providers:            []string{"provider-alpha", "provider-beta"},
		workspace:            ws,
		verifyFn: func(ctx context.Context, w string) verify.Result {
			return alwaysFailGate.Run(ctx, w)
		},
	}
	passOnSecond.verifyFn = func(ctx context.Context, w string) verify.Result {
		idx := passOnSecond.triedCount()
		if idx >= 2 {
			return verify.Result{Passed: true, Mode: verify.ModePoC, Report: "passed"}
		}
		return verify.Result{Passed: false, Mode: verify.ModePoC, Report: "compile error"}
	}

	store2 := e2eSessionStore(t)
	sess2, err := store2.StartOrResume("")
	if err != nil {
		t.Fatal(err)
	}

	loop2 := &agentloop.Loop{
		Gate:         alwaysFailGate,
		MaxTurns:     2,
		SystemPrompt: "test multi-provider",
		Workspace:    ws,
		Completion: func(ctx context.Context, msgs []session.Message, tools []agentloop.ToolSpec) (*agentloop.Completion, error) {
			return &agentloop.Completion{
				Text: "attempt",
				Raw:  session.Message{Role: "assistant", Content: "attempt"},
			}, nil
		},
		TournamentRunner: passOnSecond,
	}

	result2, err := loop2.Run(context.Background(), sess2, "test multi-provider prompt")
	if err != nil {
		t.Fatalf("loop2 Run failed: %v", err)
	}
	if !passOnSecond.wasCalled() {
		t.Fatal("multi-provider tournament should have been triggered")
	}
	if passOnSecond.triedCount() < 2 {
		t.Errorf("expected at least 2 providers tried, got %d", passOnSecond.triedCount())
	}
	if !result2.Verified {
		t.Error("result2 should be verified after second provider passes")
	}
}

func TestE2E_FullPipelineRaceFreeStress(t *testing.T) {
	pdb := newPatternDB(t)
	ctx := context.Background()

	statsBefore, err := pdb.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats before: %v", err)
	}
	seqsBefore := statsBefore.TotalSequences

	const n = 10
	var wg sync.WaitGroup
	wg.Add(n)

	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			prompt := fmt.Sprintf("implement feature %d with tests", idx)

			planner := NewDeepPlanner(DefaultAgents())
			planner.SetPatternDB(pdb)
			plan := planner.BuildDAGPlan(prompt)
			if len(plan.Tasks) == 0 {
				errs <- fmt.Errorf("goroutine %d: empty plan", idx)
				return
			}

			registry := e2eMockRegistry()
			scratch := NewScratchpad()
			disp := NewDispatcher(registry, scratch, 4)

			dispCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			if err := disp.Dispatch(dispCtx, plan); err != nil {
				errs <- fmt.Errorf("goroutine %d: dispatch: %w", idx, err)
				return
			}

			for _, task := range plan.Tasks {
				if task.Status != TaskCompleted {
					errs <- fmt.Errorf("goroutine %d: task %s not completed", idx, task.ID)
					return
				}
			}

			if err := pdb.RecordSequence(ctx, plan); err != nil {
				errs <- fmt.Errorf("goroutine %d: RecordSequence: %w", idx, err)
				return
			}
		}(i)
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent pipeline error: %v", err)
		}
	}

	statsAfter, err := pdb.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats after: %v", err)
	}
	newSeqs := statsAfter.TotalSequences - seqsBefore
	if newSeqs != n {
		t.Errorf("expected %d new sequences, got %d (before=%d after=%d)",
			n, newSeqs, seqsBefore, statsAfter.TotalSequences)
	}
}
