// SPDX-License-Identifier: MIT
// Purpose: tests for DeepPlanner + PatternDB integration (issue #288).
// Verifies that learned patterns refine probability scores in the DAG.
package orchestrator

import (
	"context"
	"testing"
)

func TestDeepPlanner_WithPatterns_RefinesProbability(t *testing.T) {
	agents := DefaultAgents()
	pdb := newPatternDB(t)
	ctx := context.Background()

	// Record 3 sessions: all have architect+coder, 2 have docs, 1 has security.
	pdb.RecordSequence(ctx, makePlan("implement auth module", []TaskType{TaskArchitect, TaskCode, TaskTest, TaskDocs}, true))
	pdb.RecordSequence(ctx, makePlan("implement auth module", []TaskType{TaskArchitect, TaskCode, TaskTest, TaskDocs}, true))
	pdb.RecordSequence(ctx, makePlan("implement auth module", []TaskType{TaskArchitect, TaskCode, TaskTest, TaskSecurity}, false))

	planner := NewDeepPlanner(agents)
	planner.SetPatternDB(pdb)

	plan := planner.BuildDAGPlan("implement auth module")

	// Find the code task and check its probability was refined.
	// Heuristic P(code) = 0.95, learned P(code) = 3/3 = 1.0
	// Blended = 0.7*0.95 + 0.3*1.0 = 0.665 + 0.30 = 0.965
	var codeTask *Task
	for _, t := range plan.Tasks {
		if t.Type == TaskCode {
			codeTask = t
			break
		}
	}
	if codeTask == nil {
		t.Fatal("code task not found in plan")
	}
	// Should be between 0.95 (heuristic) and 1.0 (learned).
	if codeTask.Probability <= 0.95 {
		t.Errorf("code probability = %.3f, should be blended above 0.95", codeTask.Probability)
	}
}

func TestDeepPlanner_WithoutPatterns_Unchanged(t *testing.T) {
	agents := DefaultAgents()
	planner := NewDeepPlanner(agents)

	plan := planner.BuildDAGPlan("implement auth module")

	// Without patterns, probabilities should be the heuristic defaults.
	for _, task := range plan.Tasks {
		switch task.Type {
		case TaskArchitect:
			if task.Probability != 1.0 {
				t.Errorf("architect P=%.2f, want 1.0 (no patterns)", task.Probability)
			}
		case TaskCode:
			if task.Probability != 0.95 {
				t.Errorf("coder P=%.2f, want 0.95 (no patterns)", task.Probability)
			}
		}
	}
}

func TestDeepPlanner_WithPatterns_NoMatch_UsesHeuristic(t *testing.T) {
	agents := DefaultAgents()
	pdb := newPatternDB(t)

	planner := NewDeepPlanner(agents)
	planner.SetPatternDB(pdb)

	// No patterns recorded for this prompt — should use heuristic defaults.
	plan := planner.BuildDAGPlan("completely unknown task")

	for _, task := range plan.Tasks {
		switch task.Type {
		case TaskArchitect:
			if task.Probability != 1.0 {
				t.Errorf("architect P=%.2f, want 1.0 (no match)", task.Probability)
			}
		}
	}
}
