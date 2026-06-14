package planning

import (
	"context"
	"fmt"
)

// HTNPlanner implements Hierarchical Task Network planning
type HTNPlanner struct {
	methods map[string][]*HTNMethod
}

// NewHTNPlanner creates a new HTN planner
func NewHTNPlanner() *HTNPlanner {
	return &HTNPlanner{
		methods: make(map[string][]*HTNMethod),
	}
}

// RegisterMethod registers a decomposition method
func (h *HTNPlanner) RegisterMethod(taskName string, method *HTNMethod) {
	if _, exists := h.methods[taskName]; !exists {
		h.methods[taskName] = []*HTNMethod{}
	}
	h.methods[taskName] = append(h.methods[taskName], method)
}

// Decompose decomposes a task using registered methods
func (h *HTNPlanner) Decompose(ctx context.Context, task *Task, state *State) ([]*Task, error) {
	if task.Primitive {
		return []*Task{task}, nil
	}

	methods, exists := h.methods[task.Name]
	if !exists {
		return nil, fmt.Errorf("no methods for task %s", task.Name)
	}

	for _, method := range methods {
		if h.checkConstraints(method.Constraints, state) {
			return method.Subtasks, nil
		}
	}

	return nil, fmt.Errorf("no applicable method for task %s", task.Name)
}

func (h *HTNPlanner) checkConstraints(constraints []string, state *State) bool {
	if len(constraints) == 0 {
		return true
	}
	return true
}

// DecomposeComplex decomposes complex tasks recursively
func (h *HTNPlanner) DecomposeComplex(ctx context.Context, tasks []*Task, state *State) ([]*Task, error) {
	result := []*Task{}

	for _, task := range tasks {
		if task.Primitive {
			result = append(result, task)
		} else {
			subtasks, err := h.Decompose(ctx, task, state)
			if err != nil {
				return nil, err
			}

			deeperTasks, err := h.DecomposeComplex(ctx, subtasks, state)
			if err != nil {
				return nil, err
			}

			result = append(result, deeperTasks...)
		}
	}

	return result, nil
}
