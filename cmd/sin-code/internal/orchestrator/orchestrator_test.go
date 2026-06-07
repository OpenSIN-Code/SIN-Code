// SPDX-License-Identifier: MIT
// Purpose: tests for the orchestrator package: model, router, planner, agents,
// registry, scratchpad, dispatcher, aggregator.
package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestGenerateIDUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := GenerateID("tk")
		if seen[id] {
			t.Errorf("duplicate id: %s", id)
		}
		seen[id] = true
	}
}

func TestGenerateIDPrefix(t *testing.T) {
	id := GenerateID("pl")
	if !strings.HasPrefix(id, "pl-") {
		t.Errorf("expected pl- prefix, got %q", id)
	}
}

func TestTaskTypeValid(t *testing.T) {
	types := []TaskType{TaskCode, TaskTest, TaskReview, TaskDocs, TaskSecurity, TaskArchitect, TaskGeneral}
	for _, tt := range types {
		if tt == "" {
			t.Error("empty task type")
		}
	}
}

func TestIntentValues(t *testing.T) {
	intents := []Intent{
		IntentCodebase, IntentTest, IntentReview, IntentDocs,
		IntentSecurity, IntentArchitecture, IntentGeneral,
	}
	for _, i := range intents {
		if i == "" {
			t.Error("empty intent")
		}
	}
}

func TestScratchpadWriteRead(t *testing.T) {
	s := NewScratchpad()
	s.Write("coder", "inputs", "task 1")
	got, ok := s.Read("inputs")
	if !ok {
		t.Fatal("expected to read")
	}
	if got != "task 1" {
		t.Errorf("got %q", got)
	}
}

func TestScratchpadVersioning(t *testing.T) {
	s := NewScratchpad()
	s.Write("coder", "x", "v1")
	s.Write("coder", "x", "v2")
	s.Write("coder", "x", "v3")
	all := s.ReadAll()
	if all["x"].Version != 3 {
		t.Errorf("expected version 3, got %d", all["x"].Version)
	}
	if all["x"].Content != "v3" {
		t.Errorf("got %q", all["x"].Content)
	}
}

func TestScratchpadMerge(t *testing.T) {
	a := NewScratchpad()
	a.Write("coder", "a", "value-a")
	a.Write("coder", "b", "value-b")
	time.Sleep(2 * time.Millisecond)
	b := NewScratchpad()
	b.Write("tester", "b", "value-b-2")
	b.Write("tester", "c", "value-c")
	a.Merge(b)
	all := a.ReadAll()
	if all["a"].Content != "value-a" {
		t.Errorf("a overwritten")
	}
	if all["b"].Content != "value-b-2" {
		t.Errorf("b not updated, got %q", all["b"].Content)
	}
	if all["c"].Content != "value-c" {
		t.Errorf("c missing, got %v", all)
	}
}

func TestRouterClassifyCode(t *testing.T) {
	r := NewRouter()
	intent := r.Classify("Add user authentication with OAuth2")
	if intent != IntentCodebase {
		t.Errorf("expected IntentCodebase, got %s", intent)
	}
}

func TestRouterClassifyTest(t *testing.T) {
	r := NewRouter()
	intent := r.Classify("Write unit tests for the billing module")
	if intent != IntentTest {
		t.Errorf("expected IntentTest, got %s", intent)
	}
}

func TestRouterClassifySecurity(t *testing.T) {
	r := NewRouter()
	intent := r.Classify("Check for XSS and SQL injection vulnerabilities")
	if intent != IntentSecurity {
		t.Errorf("expected IntentSecurity, got %s", intent)
	}
}

func TestRouterClassifyDocs(t *testing.T) {
	r := NewRouter()
	intent := r.Classify("Add documentation to the README")
	if intent != IntentDocs {
		t.Errorf("expected IntentDocs, got %s", intent)
	}
}

func TestRouterClassifyGeneral(t *testing.T) {
	r := NewRouter()
	intent := r.Classify("What's the weather today?")
	if intent != IntentGeneral {
		t.Errorf("expected IntentGeneral, got %s", intent)
	}
}

func TestRouterSubIntents(t *testing.T) {
	r := NewRouter()
	subs := r.SubIntents("Implement the feature and write tests and document the API")
	if len(subs) < 2 {
		t.Errorf("expected multiple sub-intents, got %d: %v", len(subs), subs)
	}
}

func TestIntentToType(t *testing.T) {
	cases := map[Intent]TaskType{
		IntentCodebase:     TaskCode,
		IntentTest:         TaskTest,
		IntentReview:       TaskReview,
		IntentDocs:         TaskDocs,
		IntentSecurity:     TaskSecurity,
		IntentArchitecture: TaskArchitect,
		IntentGeneral:      TaskGeneral,
	}
	for i, want := range cases {
		if got := intentToType(i); got != want {
			t.Errorf("intentToType(%s) = %s, want %s", i, got, want)
		}
	}
}

func TestPlannerSimpleBuild(t *testing.T) {
	p := NewPlanner(DefaultAgents())
	plan := p.BuildPlan("Add a hello world function")
	if plan == nil {
		t.Fatal("expected plan")
	}
	if len(plan.Tasks) == 0 {
		t.Error("expected at least 1 task")
	}
}

func TestPlannerMultiIntent(t *testing.T) {
	p := NewPlanner(DefaultAgents())
	plan := p.BuildPlan("Add user authentication and write tests and document the API")
	if len(plan.Tasks) < 3 {
		t.Errorf("expected multiple tasks, got %d", len(plan.Tasks))
	}
}

func TestPlannerTaskDependencies(t *testing.T) {
	p := NewPlanner(DefaultAgents())
	plan := p.BuildPlan("Add auth with tests and review")
	for i, task := range plan.Tasks {
		if i > 0 && len(task.DependsOn) == 0 {
			t.Errorf("task %d should have dependencies", i)
		}
	}
}

func TestPlannerArchitectAddedForMultiIntent(t *testing.T) {
	p := NewPlanner(DefaultAgents())
	plan := p.BuildPlan("Add feature and write tests")
	hasArchitect := false
	for _, t := range plan.Tasks {
		if t.Type == TaskArchitect {
			hasArchitect = true
		}
	}
	if !hasArchitect {
		t.Error("expected architect task for multi-intent")
	}
}

func TestDefaultAgents(t *testing.T) {
	agents := DefaultAgents()
	if len(agents) < 5 {
		t.Errorf("expected at least 5 default agents, got %d", len(agents))
	}
	seen := map[TaskType]bool{}
	for _, a := range agents {
		seen[a.Type] = true
		if a.Model == "" {
			t.Errorf("agent %s has no model", a.Name)
		}
		if a.Name == "" {
			t.Error("agent has no name")
		}
	}
	for _, tt := range []TaskType{TaskCode, TaskTest, TaskReview, TaskDocs, TaskSecurity} {
		if !seen[tt] {
			t.Errorf("missing default agent for type %s", tt)
		}
	}
}

func TestMockAgentRun(t *testing.T) {
	a := NewMockAgent(DefaultAgents()[0])
	task := &Task{ID: "tk-1", Type: TaskCode, Description: "test"}
	scratch := NewScratchpad()
	out, err := a.Run(context.Background(), task, scratch)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "tk-1") {
		t.Errorf("output should mention task id, got %q", out)
	}
	all := scratch.ReadAll()
	if len(all) == 0 {
		t.Error("expected scratchpad writes")
	}
}

func TestMockAgentContextCancel(t *testing.T) {
	a := NewMockAgent(AgentConfig{Name: "test", Model: "m", Type: TaskCode})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	task := &Task{ID: "tk-1", Type: TaskCode}
	_, err := a.Run(ctx, task, NewScratchpad())
	if err == nil {
		t.Error("expected context error")
	}
}

func TestRegistryGet(t *testing.T) {
	r := NewRegistryWithDefaults(nil)
	a, ok := r.Get("coder")
	if !ok {
		t.Fatal("expected coder")
	}
	if a.Name() != "coder" {
		t.Errorf("got %s", a.Name())
	}
}

func TestRegistryForType(t *testing.T) {
	r := NewRegistryWithDefaults(nil)
	a, ok := r.ForType(TaskCode)
	if !ok {
		t.Fatal("expected code agent")
	}
	if a.Config().Type != TaskCode {
		t.Errorf("got %s", a.Config().Type)
	}
}

func TestRegistryRegister(t *testing.T) {
	r := NewRegistry(nil)
	custom := AgentConfig{Name: "custom", Type: TaskCode, Model: "m"}
	r.Register(NewMockAgent(custom))
	a, ok := r.Get("custom")
	if !ok {
		t.Fatal("expected custom")
	}
	if a.Config().Model != "m" {
		t.Errorf("got %s", a.Config().Model)
	}
}

func TestRegistryList(t *testing.T) {
	r := NewRegistryWithDefaults(nil)
	list := r.List()
	if len(list) < 5 {
		t.Errorf("expected at least 5, got %d", len(list))
	}
}

func TestMergeConfigOverride(t *testing.T) {
	base := AgentConfig{Name: "coder", Model: "m1", MaxTokens: 1000, Temperature: 0.0, MemoryNS: "coder"}
	override := AgentConfig{Model: "m2", MaxTokens: 2000, MemoryNS: "coder2"}
	merged := mergeConfig(base, override)
	if merged.Model != "m2" {
		t.Errorf("model: got %s", merged.Model)
	}
	if merged.MaxTokens != 2000 {
		t.Errorf("tokens: got %d", merged.MaxTokens)
	}
	if merged.Temperature != 0.0 {
		t.Errorf("temp: got %f", merged.Temperature)
	}
	if merged.MemoryNS != "coder2" {
		t.Errorf("ns: got %s", merged.MemoryNS)
	}
}

func TestDispatcherSimple(t *testing.T) {
	r := NewRegistryWithDefaults(nil)
	scratch := NewScratchpad()
	d := NewDispatcher(r, scratch, 2)
	tasks := []*Task{
		{ID: "t1", Type: TaskCode, Description: "a", AgentName: "coder", Status: TaskPending, Created: timeNow()},
	}
	plan := &Plan{ID: "p1", Tasks: tasks}
	if err := d.Dispatch(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if tasks[0].Status != TaskCompleted {
		t.Errorf("got %s", tasks[0].Status)
	}
	if tasks[0].Result == "" {
		t.Error("expected result")
	}
}

func TestDispatcherParallel(t *testing.T) {
	r := NewRegistryWithDefaults(nil)
	scratch := NewScratchpad()
	d := NewDispatcher(r, scratch, 4)
	tasks := []*Task{}
	for i := 0; i < 4; i++ {
		tasks = append(tasks, &Task{
			ID: GenerateID("tk"), Type: TaskCode, Description: "x",
			AgentName: "coder", Status: TaskPending, Created: timeNow(),
		})
	}
	plan := &Plan{ID: "p1", Tasks: tasks}
	if err := d.Dispatch(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	for _, task := range tasks {
		if task.Status != TaskCompleted {
			t.Errorf("task %s: %s", task.ID, task.Status)
		}
	}
}

func TestDispatcherDependencyWait(t *testing.T) {
	r := NewRegistryWithDefaults(nil)
	scratch := NewScratchpad()
	d := NewDispatcher(r, scratch, 4)
	t1 := &Task{ID: "t1", Type: TaskCode, AgentName: "coder", Status: TaskPending, Created: timeNow()}
	t2 := &Task{ID: "t2", Type: TaskTest, AgentName: "tester", Status: TaskPending, DependsOn: []string{"t1"}, Created: timeNow()}
	plan := &Plan{ID: "p1", Tasks: []*Task{t1, t2}}
	if err := d.Dispatch(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if t2.Status != TaskCompleted {
		t.Errorf("t2 should be completed, got %s", t2.Status)
	}
}

func TestDispatcherMissingAgent(t *testing.T) {
	r := NewRegistry(nil)
	scratch := NewScratchpad()
	d := NewDispatcher(r, scratch, 1)
	task := &Task{ID: "t1", Type: TaskCode, AgentName: "ghost", Status: TaskPending, Created: timeNow()}
	plan := &Plan{ID: "p1", Tasks: []*Task{task}}
	err := d.Dispatch(context.Background(), plan)
	if err == nil {
		t.Error("expected error for missing agent")
	}
}

func TestDispatcherContextCancel(t *testing.T) {
	r := NewRegistryWithDefaults(nil)
	scratch := NewScratchpad()
	d := NewDispatcher(r, scratch, 1)
	task := &Task{ID: "t1", Type: TaskCode, AgentName: "coder", Status: TaskPending, Created: timeNow()}
	plan := &Plan{ID: "p1", Tasks: []*Task{task}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	_ = d.Dispatch(ctx, plan)
}

func TestAggregator(t *testing.T) {
	scratch := NewScratchpad()
	agg := NewAggregator(scratch)
	plan := &Plan{
		ID:     "p1",
		Intent: IntentCodebase,
		Tasks: []*Task{
			{ID: "t1", Type: TaskCode, Status: TaskCompleted, Result: "out1"},
			{ID: "t2", Type: TaskTest, Status: TaskFailed, Error: "boom"},
		},
	}
	res := agg.Aggregate(plan)
	if res.TotalTasks != 2 {
		t.Errorf("total: %d", res.TotalTasks)
	}
	if res.OKTasks != 1 {
		t.Errorf("ok: %d", res.OKTasks)
	}
	if res.FailedTasks != 1 {
		t.Errorf("failed: %d", res.FailedTasks)
	}
	if !strings.Contains(res.Summary, "Plan p1") {
		t.Error("summary should mention plan id")
	}
}

func TestAggregatorSummary(t *testing.T) {
	agg := NewAggregator(NewScratchpad())
	plan := &Plan{
		ID: "p1", Intent: IntentGeneral,
		Tasks: []*Task{
			{ID: "t1", Type: TaskGeneral, Status: TaskCompleted, AgentName: "coder"},
		},
	}
	res := agg.Aggregate(plan)
	if !strings.Contains(res.Summary, "[general]") {
		t.Error("summary should show task type")
	}
	if !strings.Contains(res.Summary, "coder") {
		t.Error("summary should show agent name")
	}
}

func TestAllDepsDone(t *testing.T) {
	completed := map[string]bool{"a": true, "b": true}
	t1 := &Task{ID: "x", DependsOn: []string{"a", "b"}}
	if !allDepsDone(t1, completed) {
		t.Error("should be done")
	}
	t2 := &Task{ID: "x", DependsOn: []string{"a", "c"}}
	if allDepsDone(t2, completed) {
		t.Error("should not be done, c missing")
	}
	t3 := &Task{ID: "x"}
	if !allDepsDone(t3, completed) {
		t.Error("no deps should be done")
	}
}

func TestEstimateTokens(t *testing.T) {
	if n := estimateTokens(""); n != 0 {
		t.Errorf("empty: %d", n)
	}
	if n := estimateTokens(strings.Repeat("a", 400)); n != 100 {
		t.Errorf("400 chars: got %d, want 100", n)
	}
}

func TestEstimateCost(t *testing.T) {
	cases := map[string]float64{
		"claude-opus-5.1":   15.0,
		"claude-sonnet-4.7": 3.0,
		"claude-haiku-4.5":  0.25,
		"gpt-4o":            1.0,
	}
	for model, want := range cases {
		cost := estimateCost(1_000_000, model)
		if cost < want*0.9 || cost > want*1.1 {
			t.Errorf("model %s: got %f, want ~%f", model, cost, want)
		}
	}
}

func TestContainsAny(t *testing.T) {
	if !containsAny("claude-opus-5.1", "opus") {
		t.Error("should match")
	}
	if containsAny("claude-haiku-4.5", "opus", "sonnet") {
		t.Error("should not match")
	}
	if containsAny("test", "") {
		t.Error("empty substring should NOT match")
	}
	if containsAny("short", "longersubstring") {
		t.Error("substring longer than s should not match")
	}
}

func TestNewOrchestrator(t *testing.T) {
	o := New()
	if o.Registry == nil || o.Planner == nil || o.Dispatcher == nil {
		t.Fatal("nil components")
	}
}

func TestNewWithAgentsEmpty(t *testing.T) {
	o := NewWithAgents(nil)
	if o.Registry == nil {
		t.Fatal("nil registry")
	}
}

func TestOrchestratorPlan(t *testing.T) {
	o := New()
	plan := o.Plan("Add OAuth2 authentication")
	if plan == nil || len(plan.Tasks) == 0 {
		t.Fatal("expected plan")
	}
}

func TestOrchestratorRunSimple(t *testing.T) {
	o := New()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := o.Run(ctx, "Add a simple hello function")
	if err != nil {
		t.Fatal(err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
	if !res.Plan.Success {
		t.Error("expected success")
	}
}

func TestOrchestratorRunMultiIntent(t *testing.T) {
	o := New()
	ctx := context.Background()
	res, err := o.Run(ctx, "Add OAuth2 auth and write tests and document the API and review")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Plan.Tasks) < 3 {
		t.Errorf("expected multi-task plan, got %d", len(res.Plan.Tasks))
	}
}

func TestOrchestratorRunWithTimeout(t *testing.T) {
	o := New()
	ctx := context.Background()
	_, err := o.Run(ctx, "test", WithTimeout(1*time.Millisecond))
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestOrchestratorRunWithMaxParallel(t *testing.T) {
	o := New()
	ctx := context.Background()
	res, err := o.Run(ctx, "test", WithMaxParallel(2))
	if err != nil {
		t.Fatal(err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
}

func TestLoadUserAgentsMissing(t *testing.T) {
	dir := t.TempDir()
	agents, err := LoadUserAgents(filepath_join(dir, "missing"))
	if err != nil {
		t.Fatal(err)
	}
	if agents != nil {
		t.Errorf("expected nil, got %v", agents)
	}
}

func TestLoadUserAgentsValid(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath_join(dir, "myagent")
	must_mkdir(t, agentDir)
	must_write(t, filepath_join(agentDir, "agent.toml"), `
name = "myagent"
description = "Custom agent"
type = "code"
model = "anthropic/claude-sonnet-4.7"
max_tokens = 8000
`)
	agents, err := LoadUserAgents(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1, got %d", len(agents))
	}
	if agents[0].Name != "myagent" {
		t.Errorf("got %s", agents[0].Name)
	}
}

func TestLoadUserAgentsInvalid(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath_join(dir, "bad")
	must_mkdir(t, agentDir)
	must_write(t, filepath_join(agentDir, "agent.toml"), "this is not toml = =")
	_, err := LoadUserAgents(dir)
	if err == nil {
		t.Error("expected error")
	}
}

func TestLoadUserAgentsIgnoresFiles(t *testing.T) {
	dir := t.TempDir()
	must_write(t, filepath_join(dir, "not-a-dir.txt"), "ignored")
	agents, err := LoadUserAgents(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 0 {
		t.Errorf("expected 0, got %d", len(agents))
	}
}

func TestUserAgentOverridesDefault(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath_join(dir, "coder")
	must_mkdir(t, agentDir)
	must_write(t, filepath_join(agentDir, "agent.toml"), `
name = "coder"
model = "custom/model-v2"
max_tokens = 9999
`)
	extras, err := LoadUserAgents(dir)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRegistryWithDefaults(extras)
	coder, ok := r.Get("coder")
	if !ok {
		t.Fatal("expected coder")
	}
	if coder.Config().Model != "custom/model-v2" {
		t.Errorf("expected override, got %s", coder.Config().Model)
	}
	if coder.Config().MaxTokens != 9999 {
		t.Errorf("expected 9999, got %d", coder.Config().MaxTokens)
	}
}

func TestOrchestratorString(t *testing.T) {
	o := New()
	s := o.String()
	if !strings.Contains(s, "Orchestrator") {
		t.Error("expected 'Orchestrator' in string")
	}
}

func TestAggregatorMultipleTasks(t *testing.T) {
	agg := NewAggregator(NewScratchpad())
	plan := &Plan{ID: "p", Tasks: []*Task{
		{ID: "t1", Type: TaskCode, Status: TaskCompleted, Result: "r1"},
		{ID: "t2", Type: TaskTest, Status: TaskCompleted, Result: "r2"},
		{ID: "t3", Type: TaskReview, Status: TaskCompleted, Result: "r3"},
	}}
	res := agg.Aggregate(plan)
	if res.OKTasks != 3 {
		t.Errorf("ok: %d", res.OKTasks)
	}
	if res.FailedTasks != 0 {
		t.Errorf("failed: %d", res.FailedTasks)
	}
}

func TestDispatcherEmptyPlan(t *testing.T) {
	r := NewRegistryWithDefaults(nil)
	d := NewDispatcher(r, NewScratchpad(), 1)
	plan := &Plan{ID: "p1"}
	if err := d.Dispatch(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
}

func TestDispatcherCostAccumulation(t *testing.T) {
	r := NewRegistryWithDefaults(nil)
	scratch := NewScratchpad()
	d := NewDispatcher(r, scratch, 2)
	tasks := []*Task{
		{ID: "t1", Type: TaskCode, AgentName: "coder", Status: TaskPending, Created: timeNow()},
		{ID: "t2", Type: TaskTest, AgentName: "tester", Status: TaskPending, Created: timeNow()},
	}
	plan := &Plan{ID: "p1", Tasks: tasks}
	_ = d.Dispatch(context.Background(), plan)
	if plan.TotalCost == 0 {
		t.Error("expected non-zero total cost")
	}
	if plan.TokensUsed == 0 {
		t.Error("expected non-zero tokens used")
	}
}

func TestTaskStatusValues(t *testing.T) {
	statuses := []TaskStatus{
		TaskPending, TaskRunning, TaskCompleted, TaskFailed, TaskCancelled, TaskBlocked,
	}
	for _, s := range statuses {
		if s == "" {
			t.Error("empty status")
		}
	}
}

func TestPlannerReviewerAddedForCodeOrTest(t *testing.T) {
	p := NewPlanner(DefaultAgents())
	plan := p.BuildPlan("Implement user login")
	hasReview := false
	for _, t := range plan.Tasks {
		if t.Type == TaskReview {
			hasReview = true
		}
	}
	if !hasReview {
		t.Error("expected review task")
	}
}

func TestPlannerNoReviewerForDocsOnly(t *testing.T) {
	p := NewPlanner(DefaultAgents())
	plan := p.BuildPlan("Just document the README")
	hasReview := false
	for _, t := range plan.Tasks {
		if t.Type == TaskReview {
			hasReview = true
		}
	}
	if hasReview {
		t.Error("docs-only should not have review")
	}
}

func TestOrchestratorStringer(t *testing.T) {
	o := New()
	if o.String() == "" {
		t.Error("expected non-empty string")
	}
}

func TestScratchpadReadMissing(t *testing.T) {
	s := NewScratchpad()
	if _, ok := s.Read("missing"); ok {
		t.Error("expected not found")
	}
}
