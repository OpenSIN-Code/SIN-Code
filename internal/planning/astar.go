package planning

import (
	"container/heap"
	"context"
	"fmt"
	"math"
)

// AStarNode represents a node in A* search
type AStarNode struct {
	State    *State
	Plan     *Plan
	GCost    float32
	HCost    float32
	FCost    float32
	Parent   *AStarNode
	Index    int
}

// NodeHeap is a min-heap of AStarNode
type NodeHeap []*AStarNode

func (h NodeHeap) Len() int           { return len(h) }
func (h NodeHeap) Less(i, j int) bool { return h[i].FCost < h[j].FCost }
func (h NodeHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].Index = i
	h[j].Index = j
}

func (h *NodeHeap) Push(x interface{}) {
	node := x.(*AStarNode)
	node.Index = len(*h)
	*h = append(*h, node)
}

func (h *NodeHeap) Pop() interface{} {
	old := *h
	n := len(old)
	node := old[n-1]
	*h = old[0 : n-1]
	return node
}

// AStarPlanner implements A* search for planning
type AStarPlanner struct {
	actions []*Action
}

// NewAStarPlanner creates a new A* planner
func NewAStarPlanner(actions []*Action) *AStarPlanner {
	return &AStarPlanner{
		actions: actions,
	}
}

// Search performs A* search for a plan
func (a *AStarPlanner) Search(ctx context.Context, initialState *State, goal *Goal) (*Plan, error) {
	openSet := &NodeHeap{}
	heap.Init(openSet)

	initialNode := &AStarNode{
		State:  initialState,
		Plan:   &Plan{Actions: []*Action{}, Cost: 0},
		GCost:  0,
		HCost:  a.heuristic(initialState, goal),
	}
	initialNode.FCost = initialNode.GCost + initialNode.HCost

	heap.Push(openSet, initialNode)
	closedSet := make(map[string]bool)

	for openSet.Len() > 0 {
		current := heap.Pop(openSet).(*AStarNode)

		if a.isGoal(current.State, goal) {
			return current.Plan, nil
		}

		stateKey := fmt.Sprintf("%v", current.State.Facts)
		if closedSet[stateKey] {
			continue
		}
		closedSet[stateKey] = true

		for _, action := range a.actions {
			if a.canApply(action, current.State) {
				newState := a.applyAction(action, current.State)
				newPlan := &Plan{
					Actions: append(current.Plan.Actions, action),
					Cost:    current.Plan.Cost + action.Cost,
				}

				newNode := &AStarNode{
					State:  newState,
					Plan:   newPlan,
					GCost:  newPlan.Cost,
					HCost:  a.heuristic(newState, goal),
					Parent: current,
				}
				newNode.FCost = newNode.GCost + newNode.HCost

				heap.Push(openSet, newNode)
			}
		}
	}

	return nil, fmt.Errorf("no plan found")
}

func (a *AStarPlanner) heuristic(state *State, goal *Goal) float32 {
	return float32(math.Abs(float64(len(goal.Description)))) * 0.1
}

func (a *AStarPlanner) isGoal(state *State, goal *Goal) bool {
	return goal.Satisfied
}

func (a *AStarPlanner) canApply(action *Action, state *State) bool {
	for key, value := range action.Precond {
		if stateVal, exists := state.Facts[key]; !exists || stateVal != value {
			return false
		}
	}
	return true
}

func (a *AStarPlanner) applyAction(action *Action, state *State) *State {
	newFacts := make(map[string]interface{})
	for k, v := range state.Facts {
		newFacts[k] = v
	}

	for k, v := range action.Effects {
		newFacts[k] = v
	}

	return &State{
		Facts: newFacts,
		Time:  state.Time,
	}
}
