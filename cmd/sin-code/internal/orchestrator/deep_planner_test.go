// SPDX-License-Identifier: MIT
// Purpose: tests for issue #282 — DeepPlanner produces parallel DAGs
// with probability scores instead of linear chains. All tests pass
// under -race (mandate M7).
package orchestrator

import (
	"strings"
	"testing"
)

func TestDeepPlanner_ArchitectFirst(t *testing.T) {
	dp := NewDeepPlanner(DefaultAgents())
	plan := dp.BuildDAGPlan("implement and design user authentication with JWT")

	if len(plan.Tasks) == 0 {
		t.Fatal("expected at least one task")
	}

	// First task should be architect with P=1.0 and no deps.
	first := plan.Tasks[0]
	if first.Type != TaskArchitect {
		t.Errorf("first task type = %s, want architect", first.Type)
	}
	if first.Probability != 1.0 {
		t.Errorf("architect probability = %.2f, want 1.0", first.Probability)
	}
	if len(first.DependsOn) != 0 {
		t.Errorf("architect should have no deps, got %v", first.DependsOn)
	}
}

func TestDeepPlanner_ParallelTasksNotChained(t *testing.T) {
	// "implement and test auth" should produce:
	//   architect → coder (deps=architect)
	//   architect → tester (deps=architect+coder)
	//   architect → review (deps=architect+coder+tester)
	// The key: coder and tester should NOT be chained to each other
	// linearly (tester depends on coder, but security/docs if present
	// should be PARALLEL to coder, not chained after it).
	dp := NewDeepPlanner(DefaultAgents())
	plan := dp.BuildDAGPlan("implement and test user authentication")

	typeIDs := map[TaskType]string{}
	for _, task := range plan.Tasks {
		typeIDs[task.Type] = task.ID
	}

	coder, hasCoder := typeIDs[TaskCode]
	tester, hasTester := typeIDs[TaskTest]
	if !hasCoder || !hasTester {
		t.Fatal("expected coder and tester tasks")
	}

	// Tester should depend on coder (correct DAG dependency).
	testerTask := findTaskByID(plan.Tasks, tester)
	if testerTask == nil {
		t.Fatal("tester task not found")
	}
	if !containsDep(testerTask.DependsOn, coder) {
		t.Error("tester should depend on coder")
	}

	// Coder should NOT depend on tester (not a reverse chain).
	coderTask := findTaskByID(plan.Tasks, coder)
	if coderTask == nil {
		t.Fatal("coder task not found")
	}
	if containsDep(coderTask.DependsOn, tester) {
		t.Error("coder should NOT depend on tester (would be linear chain)")
	}
}

func TestDeepPlanner_SecurityDocsParallelToCode(t *testing.T) {
	// "implement, test, document, and security review auth" should produce:
	//   architect → coder (deps=architect)
	//   architect → security (deps=architect)    ← PARALLEL to coder
	//   architect → docs (deps=architect)         ← PARALLEL to coder
	//   architect → coder → tester
	//   architect → coder → tester → review
	dp := NewDeepPlanner(DefaultAgents())
	plan := dp.BuildDAGPlan("implement, test, document, and security review user auth")

	typeIDs := map[TaskType]string{}
	for _, task := range plan.Tasks {
		typeIDs[task.Type] = task.ID
	}

	archID := typeIDs[TaskArchitect]
	coderID := typeIDs[TaskCode]
	secID, hasSec := typeIDs[TaskSecurity]
	docsID, hasDocs := typeIDs[TaskDocs]

	if !hasSec {
		t.Fatal("expected security task")
	}
	if !hasDocs {
		t.Fatal("expected docs task")
	}

	// Security should depend ONLY on architect, NOT on coder.
	secTask := findTaskByID(plan.Tasks, secID)
	if secTask == nil {
		t.Fatal("security task not found")
	}
	if !containsDep(secTask.DependsOn, archID) {
		t.Error("security should depend on architect")
	}
	if containsDep(secTask.DependsOn, coderID) {
		t.Error("security should NOT depend on coder (should be parallel)")
	}

	// Docs should depend ONLY on architect, NOT on coder.
	docsTask := findTaskByID(plan.Tasks, docsID)
	if docsTask == nil {
		t.Fatal("docs task not found")
	}
	if !containsDep(docsTask.DependsOn, archID) {
		t.Error("docs should depend on architect")
	}
	if containsDep(docsTask.DependsOn, coderID) {
		t.Error("docs should NOT depend on coder (should be parallel)")
	}
}

func TestDeepPlanner_ProbabilityScores(t *testing.T) {
	dp := NewDeepPlanner(DefaultAgents())
	plan := dp.BuildDAGPlan("implement, test, document, and security review auth")

	for _, task := range plan.Tasks {
		switch task.Type {
		case TaskArchitect:
			if task.Probability != 1.0 {
				t.Errorf("architect P = %.2f, want 1.0", task.Probability)
			}
		case TaskCode:
			if task.Probability < 0.9 {
				t.Errorf("coder P = %.2f, want >= 0.9", task.Probability)
			}
		case TaskTest:
			if task.Probability < 0.8 {
				t.Errorf("tester P = %.2f, want >= 0.8", task.Probability)
			}
		case TaskSecurity:
			if task.Probability < 0.6 {
				t.Errorf("security P = %.2f, want >= 0.6", task.Probability)
			}
		case TaskDocs:
			if task.Probability < 0.4 {
				t.Errorf("docs P = %.2f, want >= 0.4", task.Probability)
			}
		case TaskReview:
			if task.Probability > 0.5 {
				t.Errorf("review P = %.2f, want <= 0.5", task.Probability)
			}
		}
	}
}

func TestDeepPlanner_ExpectedOutput(t *testing.T) {
	dp := NewDeepPlanner(DefaultAgents())
	plan := dp.BuildDAGPlan("implement auth")

	for _, task := range plan.Tasks {
		if task.ExpectedOutput == "" {
			t.Errorf("task %s (%s): empty ExpectedOutput", task.ID, task.Type)
		}
	}
}

func TestDeepPlanner_NoDepsForArchitect(t *testing.T) {
	dp := NewDeepPlanner(DefaultAgents())
	plan := dp.BuildDAGPlan("implement something complex with tests and docs")

	// Find the architect task.
	for _, task := range plan.Tasks {
		if task.Type == TaskArchitect {
			if len(task.DependsOn) != 0 {
				t.Errorf("architect should have zero deps, got %d", len(task.DependsOn))
			}
			return
		}
	}
	t.Error("no architect task found")
}

func TestDeepPlanner_ReviewDependsOnCodeAndTest(t *testing.T) {
	dp := NewDeepPlanner(DefaultAgents())
	plan := dp.BuildDAGPlan("implement and test auth")

	typeIDs := map[TaskType]string{}
	for _, task := range plan.Tasks {
		typeIDs[task.Type] = task.ID
	}

	reviewID, hasReview := typeIDs[TaskReview]
	coderID, hasCoder := typeIDs[TaskCode]
	testerID, hasTester := typeIDs[TaskTest]

	if !hasReview {
		t.Skip("no review task in this plan")
	}
	if !hasCoder || !hasTester {
		t.Fatal("expected coder and tester tasks")
	}

	reviewTask := findTaskByID(plan.Tasks, reviewID)
	if reviewTask == nil {
		t.Fatal("review task not found")
	}
	if !containsDep(reviewTask.DependsOn, coderID) {
		t.Error("review should depend on coder")
	}
	if !containsDep(reviewTask.DependsOn, testerID) {
		t.Error("review should depend on tester")
	}
}

func TestDeepPlanner_GeneralFallback(t *testing.T) {
	dp := NewDeepPlanner(DefaultAgents())
	plan := dp.BuildDAGPlan("what is the meaning of life")

	if len(plan.Tasks) != 1 {
		t.Fatalf("expected 1 task for general query, got %d", len(plan.Tasks))
	}
	if plan.Tasks[0].Type != TaskGeneral {
		t.Errorf("type = %s, want general", plan.Tasks[0].Type)
	}
	if plan.Tasks[0].Probability != 1.0 {
		t.Errorf("general P = %.2f, want 1.0", plan.Tasks[0].Probability)
	}
}

func TestDeepPlanner_ParallelTasksCount(t *testing.T) {
	// A complex prompt should produce multiple tasks that can run in parallel.
	dp := NewDeepPlanner(DefaultAgents())
	plan := dp.BuildDAGPlan("implement, test, document, and security review the payment module")

	// Count tasks with zero or architect-only deps (these can run in parallel
	// after architect completes).
	archID := ""
	for _, task := range plan.Tasks {
		if task.Type == TaskArchitect {
			archID = task.ID
			break
		}
	}

	parallelCount := 0
	for _, task := range plan.Tasks {
		if task.Type == TaskArchitect {
			continue
		}
		// Tasks that depend ONLY on architect are parallel to each other.
		if len(task.DependsOn) == 1 && task.DependsOn[0] == archID {
			parallelCount++
		}
	}

	// coder, security, docs should all be parallel (depend only on architect).
	if parallelCount < 2 {
		t.Errorf("expected at least 2 parallel tasks after architect, got %d", parallelCount)
	}
}

func TestDeepPlanner_PlanHasToolChain(t *testing.T) {
	dp := NewDeepPlanner(DefaultAgents())
	plan := dp.BuildDAGPlan("implement auth")
	if plan.ToolChain == nil {
		t.Error("expected non-nil ToolChain")
	}
	if len(plan.ToolChain.Required) == 0 {
		t.Error("expected at least one required tool in ToolChain")
	}
}

func TestDeepPlanner_LegacyPlannerStillWorks(t *testing.T) {
	// Ensure the legacy Planner still compiles and works (backwards compat).
	p := NewPlanner(DefaultAgents())
	plan := p.BuildPlan("implement auth")
	if len(plan.Tasks) == 0 {
		t.Fatal("legacy planner produced no tasks")
	}
	// Legacy planner builds linear chains — verify it still does.
	for i := 1; i < len(plan.Tasks); i++ {
		if len(plan.Tasks[i].DependsOn) == 0 {
			t.Errorf("legacy task %d should have deps (linear chain)", i)
		}
	}
}

func TestDeepPlanner_DescriptionContainsPrompt(t *testing.T) {
	dp := NewDeepPlanner(DefaultAgents())
	prompt := "implement user auth with JWT tokens"
	plan := dp.BuildDAGPlan(prompt)
	for _, task := range plan.Tasks {
		if !strings.Contains(task.Description, prompt) {
			t.Errorf("task %s description does not contain original prompt", task.Type)
		}
	}
}

// Helpers

func findTaskByID(tasks []*Task, id string) *Task {
	for _, t := range tasks {
		if t.ID == id {
			return t
		}
	}
	return nil
}

func containsDep(deps []string, id string) bool {
	for _, d := range deps {
		if d == id {
			return true
		}
	}
	return false
}
