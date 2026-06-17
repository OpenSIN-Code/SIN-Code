// SPDX-License-Identifier: MIT
// Purpose: tests for the todo<->goal bridge (issue #317). Uses real temp
// SQLite goal stores + bbolt todo stores so race-detector coverage is real.
package todo

import (
	"context"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
)

func openGoalStore(t *testing.T) *autonomy.GoalStore {
	t.Helper()
	q, err := autonomy.Open(filepath.Join(t.TempDir(), "g.db"))
	if err != nil {
		t.Fatalf("autonomy.Open: %v", err)
	}
	t.Cleanup(func() { _ = q.Close() })
	return autonomy.NewGoalStore(q)
}

func TestGoalFromTodoMapsFields(t *testing.T) {
	b := NewGoalBridge(openGoalStore(t))
	td := &Todo{Title: "Refactor auth", Description: "Move to JWT", Priority: PriorityP0, Project: "sin-code"}
	g := b.GoalFromTodo(td)
	if g.Prompt != "Refactor auth\n\nMove to JWT" {
		t.Errorf("Prompt = %q", g.Prompt)
	}
	if g.Workspace != "sin-code" {
		t.Errorf("Workspace = %q", g.Workspace)
	}
	if g.Priority != 3 {
		t.Errorf("Priority = %d, want 3", g.Priority)
	}
	if g.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d", g.MaxRetries)
	}
	if g.Status != autonomy.StatusPending {
		t.Errorf("Status = %q", g.Status)
	}
}

func TestGoalFromTodoDefaultsWorkspace(t *testing.T) {
	b := NewGoalBridge(openGoalStore(t))
	g := b.GoalFromTodo(&Todo{Title: "X"})
	if g.Workspace != "." {
		t.Errorf("Workspace = %q, want '.'", g.Workspace)
	}
	if g.Prompt != "X" {
		t.Errorf("Prompt = %q", g.Prompt)
	}
}

func TestTodoToGoalPersistsAndLinks(t *testing.T) {
	gs := openGoalStore(t)
	ts := tempStore(t)
	b := NewGoalBridge(gs)
	b.SetTodoStore(ts)
	td := &Todo{Title: "Fix leak", Priority: PriorityP1, Project: "p"}
	if err := ts.Add(td); err != nil {
		t.Fatal(err)
	}
	g, err := b.TodoToGoal(td)
	if err != nil {
		t.Fatalf("TodoToGoal: %v", err)
	}
	if g.ID == 0 {
		t.Fatal("expected non-zero goal id")
	}
	links := b.Links()
	if links[strconv.FormatInt(g.ID, 10)] != td.ID {
		t.Errorf("link not recorded: %+v", links)
	}
	got, _ := ts.Get(td.ID)
	if got.Status != StatusInProgress {
		t.Errorf("todo status = %q, want in_progress", got.Status)
	}
	if got.ExternalRef != "goal:"+strconv.FormatInt(g.ID, 10) {
		t.Errorf("ExternalRef = %q", got.ExternalRef)
	}
}

func TestTodoToGoalNilTodo(t *testing.T) {
	b := NewGoalBridge(openGoalStore(t))
	if _, err := b.TodoToGoal(nil); err == nil {
		t.Error("expected error for nil todo")
	}
}

func TestBatchConvert(t *testing.T) {
	gs := openGoalStore(t)
	b := NewGoalBridge(gs)
	todos := []*Todo{
		{Title: "A", Priority: PriorityP2},
		{Title: "B", Priority: PriorityP1},
		{Title: "C", Priority: PriorityP0},
	}
	goals, err := b.BatchConvert(todos)
	if err != nil {
		t.Fatalf("BatchConvert: %v", err)
	}
	if len(goals) != 3 {
		t.Fatalf("expected 3 goals, got %d", len(goals))
	}
	for i, g := range goals {
		if g.ID == 0 {
			t.Errorf("goal %d has zero id", i)
		}
	}
}

func TestSyncStatusGoalVerifiedCompletesTodo(t *testing.T) {
	gs := openGoalStore(t)
	ts := tempStore(t)
	b := NewGoalBridge(gs)
	b.SetTodoStore(ts)
	td := &Todo{Title: "Task", Priority: PriorityP2}
	_ = ts.Add(td)
	g, _ := b.TodoToGoal(td)
	gid := strconv.FormatInt(g.ID, 10)
	_ = gs.CompleteGoal(context.Background(), g.ID, "sess")
	if err := b.SyncStatus(gid, td.ID); err != nil {
		t.Fatalf("SyncStatus: %v", err)
	}
	got, _ := ts.Get(td.ID)
	if got.Status != StatusDone {
		t.Errorf("todo status = %q, want done", got.Status)
	}
}

func TestSyncStatusGoalFailedReopensTodo(t *testing.T) {
	gs := openGoalStore(t)
	ts := tempStore(t)
	b := NewGoalBridge(gs)
	b.SetTodoStore(ts)
	td := &Todo{Title: "Task", Priority: PriorityP2}
	_ = ts.Add(td)
	g, _ := b.TodoToGoal(td)
	gid := strconv.FormatInt(g.ID, 10)
	for i := 0; i < 3; i++ {
		leased, _ := gs.LeaseGoal(context.Background(), time.Minute)
		if leased == nil {
			break
		}
		_ = gs.FailGoal(context.Background(), leased.ID, "", "boom")
	}
	if err := b.SyncStatus(gid, td.ID); err != nil {
		t.Fatalf("SyncStatus: %v", err)
	}
	got, _ := ts.Get(td.ID)
	if got.Status != StatusOpen {
		t.Errorf("todo status = %q, want open", got.Status)
	}
}

func TestSyncStatusConcurrent(t *testing.T) {
	gs := openGoalStore(t)
	ts := tempStore(t)
	b := NewGoalBridge(gs)
	b.SetTodoStore(ts)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			td := &Todo{Title: "C", Priority: PriorityP2}
			_ = ts.Add(td)
			g, err := b.TodoToGoal(td)
			if err != nil {
				return
			}
			_ = b.SyncStatus(strconv.FormatInt(g.ID, 10), td.ID)
		}()
	}
	wg.Wait()
}
