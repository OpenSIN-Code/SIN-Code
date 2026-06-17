// SPDX-License-Identifier: MIT
// Purpose: DeepPlanner — turns a user prompt into a PARALLEL DAG of tasks
// with probability scores and expected outputs. Replaces the linear-chain
// approach of the legacy Planner (issue #282).
//
// The legacy Planner builds tasks as a single chain: Task2→Task1, Task3→Task2.
// The DeepPlanner builds a real DAG where independent tasks run in parallel
// and dependent tasks start as soon as their dependencies are green.
//
// Example:
//   "implement JWT auth"
//   → architect (P=1.0, no deps)
//   → coder:jwt-middleware (P=0.95, deps=[architect])
//   → coder:login-endpoint (P=0.90, deps=[architect])  ← PARALLEL to jwt-middleware
//   → tester (P=0.85, deps=[jwt-middleware, login-endpoint])
//   → security (P=0.70, deps=[jwt-middleware])          ← can start before tester
//   → docs (P=0.50, deps=[login-endpoint])
//   → review (P=0.30, deps=[all code tasks])
package orchestrator

// TaskPrediction holds the predicted properties of a task in the DAG.
type TaskPrediction struct {
	Type           TaskType
	Description    string
	ExpectedOutput string
	Probability    float64
	DependsOnTypes []TaskType // deps by type, resolved to IDs by BuildDAGPlan
}

// DeepPlanner turns a user prompt into a parallel DAG plan with
// probability scores. It uses the Router for intent classification
// but builds a DAG instead of a linear chain.
type DeepPlanner struct {
	Router *Router
	Agents []AgentConfig
}

func NewDeepPlanner(agents []AgentConfig) *DeepPlanner {
	return &DeepPlanner{
		Router: NewRouter(),
		Agents: agents,
	}
}

// BuildDAGPlan analyzes the prompt, classifies intents, and produces a
// Plan with a parallel DAG of tasks. Each task has a Probability score
// and ExpectedOutput. Independent tasks have no DependsOn and will run
// concurrently when dispatched.
func (p *DeepPlanner) BuildDAGPlan(prompt string) *Plan {
	intent := p.Router.Classify(prompt)
	subIntents := p.Router.SubIntents(prompt)
	if len(subIntents) == 0 {
		subIntents = []Intent{intent}
	}

	predictions := p.predictTasks(prompt, subIntents)

	// Build tasks from predictions, resolving type-deps to IDs.
	typeToID := map[TaskType]string{}
	tasks := make([]*Task, 0, len(predictions))
	for _, pred := range predictions {
		id := GenerateID("tk")
		if pred.Type != "" {
			typeToID[pred.Type] = id
		}
	}

	for _, pred := range predictions {
		deps := make([]string, 0, len(pred.DependsOnTypes))
		for _, depType := range pred.DependsOnTypes {
			if depID, ok := typeToID[depType]; ok {
				deps = append(deps, depID)
			}
		}
		id := typeToID[pred.Type]
		if id == "" {
			id = GenerateID("tk")
		}
		tasks = append(tasks, &Task{
			ID:             id,
			Type:           pred.Type,
			Description:    pred.Description,
			AgentName:      findAgent(p.Agents, pred.Type),
			DependsOn:      deps,
			Status:         TaskPending,
			Created:        timeNow(),
			Probability:    pred.Probability,
			ExpectedOutput: pred.ExpectedOutput,
		})
	}

	return &Plan{
		ID:        GenerateID("pl"),
		Prompt:    prompt,
		Intent:    intent,
		ToolChain: ToolChainForIntent(intent),
		Tasks:     tasks,
		Created:   timeNow(),
	}
}

// predictTasks produces a list of TaskPredictions with parallel DAG
// dependencies. The structure is:
//
//   architect (P=1.0, no deps)
//       ├── coder tasks (P=0.9+, deps=[architect])  ← PARALLEL to each other
//       ├── security (P=0.7, deps=[architect])       ← PARALLEL to coder
//       └── docs (P=0.5, deps=[architect])            ← PARALLEL to coder
//   tester (P=0.85, deps=[all coder tasks])
//   review (P=0.30, deps=[all coder tasks + tester])
func (p *DeepPlanner) predictTasks(prompt string, intents []Intent) []TaskPrediction {
	hasArchitect := needsArchitect(intents)
	hasCode := false
	hasTest := false
	hasReview := false
	hasDocs := false
	hasSecurity := false
	for _, i := range intents {
		switch i {
		case IntentCodebase:
			hasCode = true
		case IntentTest:
			hasTest = true
		case IntentReview:
			hasReview = true
		case IntentDocs:
			hasDocs = true
		case IntentSecurity:
			hasSecurity = true
		}
	}

	predictions := make([]TaskPrediction, 0, 8)

	// Level 0: Architect (if needed) — no deps, P=1.0
	var archDeps []TaskType
	if hasArchitect {
		predictions = append(predictions, TaskPrediction{
			Type:           TaskArchitect,
			Description:    "Design the solution: high-level approach, data flow, and component boundaries for: " + prompt,
			ExpectedOutput: "Architecture design document with component boundaries and data flow",
			Probability:    1.0,
			DependsOnTypes: nil,
		})
		archDeps = []TaskType{TaskArchitect}
	}

	// Level 1: Code tasks — depend on architect (if present), PARALLEL to each other
	codeTaskTypes := make([]TaskType, 0)
	if hasCode {
		// If multiple code sub-intents, we'd create multiple coder tasks.
		// For now, one coder task per IntentCodebase.
		predictions = append(predictions, TaskPrediction{
			Type:           TaskCode,
			Description:    "Implement: " + prompt,
			ExpectedOutput: "Working implementation with passing tests",
			Probability:    0.95,
			DependsOnTypes: archDeps,
		})
		codeTaskTypes = append(codeTaskTypes, TaskCode)
	}

	// Level 1 (parallel): Security — depends on architect only, not on code
	if hasSecurity {
		predictions = append(predictions, TaskPrediction{
			Type:           TaskSecurity,
			Description:    "Security review: " + prompt,
			ExpectedOutput: "Security assessment with vulnerability findings and remediation suggestions",
			Probability:    0.70,
			DependsOnTypes: archDeps,
		})
	}

	// Level 1 (parallel): Docs — depends on architect only, not on code
	if hasDocs {
		predictions = append(predictions, TaskPrediction{
			Type:           TaskDocs,
			Description:    "Document: " + prompt,
			ExpectedOutput: "Documentation covering new APIs, types, and usage examples",
			Probability:    0.50,
			DependsOnTypes: archDeps,
		})
	}

	// Level 2: Tester — depends on ALL code tasks, not on security/docs
	if hasTest || hasCode {
		codeDeps := archDeps
		codeDeps = append(codeDeps, codeTaskTypes...)
		predictions = append(predictions, TaskPrediction{
			Type:           TaskTest,
			Description:    "Write tests for: " + prompt,
			ExpectedOutput: "Test suite with unit, integration, and edge-case coverage",
			Probability:    0.85,
			DependsOnTypes: codeDeps,
		})
	}

	// Level 3: Review — depends on code + tests
	if hasReview || hasCode {
		reviewDeps := archDeps
		reviewDeps = append(reviewDeps, codeTaskTypes...)
		if hasTest || hasCode {
			reviewDeps = append(reviewDeps, TaskTest)
		}
		predictions = append(predictions, TaskPrediction{
			Type:           TaskReview,
			Description:    "Review the implementation for correctness, style, and missing tests: " + prompt,
			ExpectedOutput: "Code review with findings, suggestions, and approval/rejection decision",
			Probability:    0.30,
			DependsOnTypes: reviewDeps,
		})
	}

	// If nothing was predicted, produce a single general task.
	if len(predictions) == 0 {
		predictions = append(predictions, TaskPrediction{
			Type:           TaskGeneral,
			Description:    prompt,
			ExpectedOutput: "Response to the user's query",
			Probability:    1.0,
		})
	}

	return predictions
}
