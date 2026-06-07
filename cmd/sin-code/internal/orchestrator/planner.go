// SPDX-License-Identifier: MIT
// Purpose: planner — turns a user prompt + classified intent into a Plan
// with ordered sub-tasks, each bound to a specialized agent.
package orchestrator

type Planner struct {
	Router *Router
	Agents []AgentConfig
}

func NewPlanner(agents []AgentConfig) *Planner {
	return &Planner{
		Router: NewRouter(),
		Agents: agents,
	}
}

func (p *Planner) BuildPlan(prompt string) *Plan {
	intent := p.Router.Classify(prompt)
	subIntents := p.Router.SubIntents(prompt)
	if len(subIntents) == 0 {
		subIntents = []Intent{intent}
	}

	tasks := make([]*Task, 0, len(subIntents)+2)
	prev := ""

	if needsArchitect(subIntents) {
		t := &Task{
			ID:          GenerateID("tk"),
			Type:        TaskArchitect,
			Description: "Design the solution: high-level approach, data flow, and component boundaries",
			AgentName:   findAgent(p.Agents, TaskArchitect),
			DependsOn:   nil,
			Status:      TaskPending,
			Created:     timeNow(),
		}
		tasks = append(tasks, t)
		prev = t.ID
	}

	for _, intent := range subIntents {
		tt := intentToType(intent)
		t := &Task{
			ID:          GenerateID("tk"),
			Type:        tt,
			Description: promptForTask(intent, prompt),
			AgentName:   findAgent(p.Agents, tt),
			DependsOn:   depOn(prev),
			Status:      TaskPending,
			Created:     timeNow(),
		}
		tasks = append(tasks, t)
		prev = t.ID
	}

	if needsReview(subIntents) {
		t := &Task{
			ID:          GenerateID("tk"),
			Type:        TaskReview,
			Description: "Review the work above for correctness, style, and missing tests",
			AgentName:   findAgent(p.Agents, TaskReview),
			DependsOn:   depOn(prev),
			Status:      TaskPending,
			Created:     timeNow(),
		}
		tasks = append(tasks, t)
	}

	return &Plan{
		ID:      GenerateID("pl"),
		Prompt:  prompt,
		Intent:  intent,
		Tasks:   tasks,
		Created: timeNow(),
	}
}

func needsArchitect(intents []Intent) bool {
	if len(intents) > 1 {
		return true
	}
	for _, i := range intents {
		if i == IntentArchitecture {
			return true
		}
	}
	return false
}

func needsReview(intents []Intent) bool {
	for _, i := range intents {
		if i == IntentCodebase || i == IntentTest {
			return true
		}
	}
	return false
}

func intentToType(intent Intent) TaskType {
	switch intent {
	case IntentCodebase:
		return TaskCode
	case IntentTest:
		return TaskTest
	case IntentReview:
		return TaskReview
	case IntentDocs:
		return TaskDocs
	case IntentSecurity:
		return TaskSecurity
	case IntentArchitecture:
		return TaskArchitect
	default:
		return TaskGeneral
	}
}

func findAgent(agents []AgentConfig, tt TaskType) string {
	for _, a := range agents {
		if a.Type == tt {
			return a.Name
		}
	}
	return "default"
}

func depOn(prev string) []string {
	if prev == "" {
		return nil
	}
	return []string{prev}
}

func promptForTask(intent Intent, originalPrompt string) string {
	switch intent {
	case IntentCodebase:
		return "Implement: " + originalPrompt
	case IntentTest:
		return "Write tests for: " + originalPrompt
	case IntentReview:
		return "Review: " + originalPrompt
	case IntentDocs:
		return "Document: " + originalPrompt
	case IntentSecurity:
		return "Security review: " + originalPrompt
	case IntentArchitecture:
		return "Architect: " + originalPrompt
	default:
		return originalPrompt
	}
}
