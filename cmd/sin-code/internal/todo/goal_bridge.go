// SPDX-License-Identifier: MIT
// Purpose: GoalBridge converts human-facing todos into autonomous goals and
// keeps their completion status in sync bidirectionally (issue #317). The
// bridge holds a reference to an autonomy.GoalStore and, optionally, a todo
// Store for status reconciliation. All shared state is mutex-guarded (M7).
package todo

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
)

// GoalBridge converts todos to autonomy goals and syncs completion status.
type GoalBridge struct {
	mu        sync.RWMutex
	goalStore *autonomy.GoalStore
	todoStore *Store
	links     map[string]string
}

// NewGoalBridge creates a bridge backed by the given goal store.
func NewGoalBridge(goalStore *autonomy.GoalStore) *GoalBridge {
	return &GoalBridge{
		goalStore: goalStore,
		links:     make(map[string]string),
	}
}

// SetTodoStore links a todo Store so SyncStatus can reconcile todo state.
// Without this, SyncStatus only reads goal status and records the link.
func (b *GoalBridge) SetTodoStore(s *Store) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.todoStore = s
}

// GoalFromTodo maps todo fields onto an autonomy Goal without side effects.
// title -> Prompt (description appended when present), priority -> int,
// project -> workspace.
func (b *GoalBridge) GoalFromTodo(todo *Todo) autonomy.Goal {
	prompt := todo.Title
	if todo.Description != "" {
		prompt = todo.Title + "\n\n" + todo.Description
	}
	ws := todo.Project
	if ws == "" {
		ws = "."
	}
	return autonomy.Goal{
		Prompt:     prompt,
		Workspace:  ws,
		Priority:   priorityToInt(todo.Priority),
		MaxRetries: 3,
		Status:     autonomy.StatusPending,
	}
}

// TodoToGoal converts a todo into an autonomous goal, persists it, records
// the link, and (when a todo store is linked) marks the todo in_progress with
// an ExternalRef of "goal:<id>".
func (b *GoalBridge) TodoToGoal(todo *Todo) (*autonomy.Goal, error) {
	if todo == nil {
		return nil, fmt.Errorf("todo: nil todo")
	}
	if b.goalStore == nil {
		return nil, fmt.Errorf("todo: goal store not set")
	}
	g := b.GoalFromTodo(todo)
	id, err := b.goalStore.AddGoal(context.Background(), &g)
	if err != nil {
		return nil, fmt.Errorf("todo: add goal: %w", err)
	}
	g.ID = id
	b.mu.Lock()
	b.links[strconv.FormatInt(id, 10)] = todo.ID
	ts := b.todoStore
	b.mu.Unlock()
	if ts != nil {
		t, err := ts.Get(todo.ID)
		if err == nil && t != nil {
			t.ExternalRef = fmt.Sprintf("goal:%d", id)
			t.Status = StatusInProgress
			_ = ts.Update(t)
		}
	}
	return &g, nil
}

// BatchConvert converts multiple todos in order, stopping on the first error.
func (b *GoalBridge) BatchConvert(todos []*Todo) ([]*autonomy.Goal, error) {
	goals := make([]*autonomy.Goal, 0, len(todos))
	for _, td := range todos {
		g, err := b.TodoToGoal(td)
		if err != nil {
			return goals, err
		}
		goals = append(goals, g)
	}
	return goals, nil
}

// SyncStatus reconciles completion status between a linked goal and todo.
//   - goal verified + todo open  -> todo marked done
//   - goal failed/exhausted       -> todo reopened to open
//   - todo done + goal not verified -> goal marked verified
//
// When no todo store is linked only the link is recorded.
func (b *GoalBridge) SyncStatus(goalID string, todoID string) error {
	if b.goalStore == nil {
		return fmt.Errorf("todo: goal store not set")
	}
	gid, err := strconv.ParseInt(goalID, 10, 64)
	if err != nil {
		return fmt.Errorf("todo: invalid goal id %q: %w", goalID, err)
	}
	g, err := b.goalStore.GetGoal(context.Background(), gid)
	if err != nil {
		return fmt.Errorf("todo: get goal: %w", err)
	}
	if g == nil {
		return fmt.Errorf("todo: goal %s not found", goalID)
	}
	b.mu.Lock()
	b.links[goalID] = todoID
	ts := b.todoStore
	b.mu.Unlock()
	if ts == nil {
		return nil
	}
	t, err := ts.Get(todoID)
	if err != nil {
		return fmt.Errorf("todo: get todo %s: %w", todoID, err)
	}
	switch {
	case g.Status == autonomy.StatusVerified && !t.IsClosed():
		t.Status = StatusDone
		return ts.Update(t)
	case (g.Status == autonomy.StatusFailed || g.Status == autonomy.StatusExhausted) && t.Status == StatusInProgress:
		t.Status = StatusOpen
		t.Notes = "goal " + goalID + " " + string(g.Status)
		return ts.Update(t)
	case t.IsClosed() && g.Status != autonomy.StatusVerified:
		return b.goalStore.CompleteGoal(context.Background(), gid, "")
	default:
		return nil
	}
}

// Links returns a copy of the goalID->todoID link map.
func (b *GoalBridge) Links() map[string]string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make(map[string]string, len(b.links))
	for k, v := range b.links {
		out[k] = v
	}
	return out
}

// priorityToInt maps a todo Priority (P0 highest urgency) to an autonomy
// priority int (higher = leased first).
func priorityToInt(p Priority) int {
	switch p {
	case PriorityP0:
		return 3
	case PriorityP1:
		return 2
	case PriorityP2:
		return 1
	case PriorityP3:
		return 0
	default:
		return 1
	}
}

// HasGoalLink reports whether a todo has been linked to a goal, checking the
// ExternalRef prefix "goal:".
func HasGoalLink(t *Todo) bool {
	if t == nil {
		return false
	}
	return strings.HasPrefix(t.ExternalRef, "goal:")
}
