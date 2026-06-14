package planning

import (
	"context"
	"time"
)

// Goal represents a planning goal with priority weighting
type Goal struct {
	ID          string                 `json:"id"`
	Description string                 `json:"description"`
	Priority    int                    `json:"priority"` // 0-100
	Deadline    time.Time              `json:"deadline"`
	Constraints map[string]interface{} `json:"constraints"`
	Satisfied   bool                   `json:"satisfied"`
}

// WeightedGoal represents a goal with dynamic weighting
type WeightedGoal struct {
	Goal       *Goal
	Weight     float32
	Importance int
	Urgency    int
}

// Action represents a planning action
type Action struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Precond     map[string]interface{} `json:"preconditions"`
	Effects     map[string]interface{} `json:"effects"`
	Cost        float32                `json:"cost"`
	Duration    time.Duration          `json:"duration"`
	Resource    string                 `json:"resource"`
}

// State represents the world state
type State struct {
	Facts map[string]interface{} `json:"facts"`
	Time  time.Time              `json:"time"`
}

// Plan represents a complete plan
type Plan struct {
	ID      string     `json:"id"`
	Goal    *Goal      `json:"goal"`
	Actions []*Action  `json:"actions"`
	Cost    float32    `json:"cost"`
	Valid   bool       `json:"valid"`
}

// Task represents a task in HTN
type Task struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Subtasks    []*Task      `json:"subtasks"`
	Primitive   bool         `json:"primitive"`
	Action      *Action      `json:"action,omitempty"`
	Decomposed  bool         `json:"decomposed"`
}

// HTNMethod represents an HTN decomposition method
type HTNMethod struct {
	TaskName string `json:"task_name"`
	Subtasks []*Task `json:"subtasks"`
	Constraints []string `json:"constraints"`
}

// Planner interface for planning algorithms
type Planner interface {
	Plan(ctx context.Context, initialState *State, goals []*WeightedGoal) (*Plan, error)
	Decompose(ctx context.Context, task *Task, state *State) ([]*Task, error)
}

// GOAPPlanner implements Goal-Oriented Action Planning
type GOAPPlanner struct {
	actions []*Action
	methods map[string]*HTNMethod
}

// NewGOAPPlanner creates a new GOAP planner
func NewGOAPPlanner() *GOAPPlanner {
	return &GOAPPlanner{
		actions: []*Action{},
		methods: make(map[string]*HTNMethod),
	}
}

// Plan generates a plan
func (g *GOAPPlanner) Plan(ctx context.Context, initialState *State, goals []*WeightedGoal) (*Plan, error) {
	plan := &Plan{
		ID:      "plan_" + time.Now().Format("20060102150405"),
		Goal:    goals[0].Goal,
		Actions: []*Action{},
		Cost:    0.0,
		Valid:   true,
	}

	for _, action := range g.actions {
		plan.Actions = append(plan.Actions, action)
		plan.Cost += action.Cost
	}

	return plan, nil
}

// Decompose decomposes a task using HTN
func (g *GOAPPlanner) Decompose(ctx context.Context, task *Task, state *State) ([]*Task, error) {
	return []*Task{}, nil
}

// AddAction adds an action to the planner
func (g *GOAPPlanner) AddAction(action *Action) {
	g.actions = append(g.actions, action)
}

// AddMethod adds an HTN decomposition method
func (g *GOAPPlanner) AddMethod(taskName string, method *HTNMethod) {
	g.methods[taskName] = method
}
