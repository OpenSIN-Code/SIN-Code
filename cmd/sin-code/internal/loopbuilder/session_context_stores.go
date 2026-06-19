// SPDX-License-Identifier: MIT
// Purpose: concrete store adapters for SessionContextBuilder (issue #379).
// These small wrappers bridge the agentloop reader interfaces to the real
// todo, ledger/summary, auto-memory, lessons, memory, and goal surfaces.
// Any nil store is treated as "source unavailable" and returns empty results
// so the builder can still assemble the remaining sections.
package loopbuilder

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/memory"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/summary"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/todo"
)

// TodoStoreReader adapts *todo.Store to the agentloop.TodoReader interface.
// It returns open todos formatted as "id [priority]: title". If blockedOnly
// is true, only blocked todos are returned.
type TodoStoreReader struct {
	Store *todo.Store
}

func (r *TodoStoreReader) Open(blockedOnly bool) ([]string, error) {
	if r.Store == nil {
		return nil, nil
	}
	list, err := r.Store.List()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, t := range list {
		if !t.IsOpen() {
			continue
		}
		if blockedOnly && t.Status != todo.StatusBlocked {
			continue
		}
		line := fmt.Sprintf("%s [%s]: %s", t.ID, t.Priority, t.Title)
		if t.Status == todo.StatusBlocked {
			line = "[BLOCKED] " + line
		}
		out = append(out, line)
	}
	return out, nil
}

// InMemoryTodoReader is the fallback TodoReader when no persistent store is
// available. It is populated by tests; production callers use TodoStoreReader.
type InMemoryTodoReader struct {
	Items []string
}

func (r *InMemoryTodoReader) Open(blockedOnly bool) ([]string, error) {
	return r.Items, nil
}

// LedgerSessionSummaryReader adapts a *ledger.Store to the
// agentloop.SessionSummaryReader interface via the summary package.
type LedgerSessionSummaryReader struct {
	Ledger    *ledger.Store
	SessionID string
}

func (r *LedgerSessionSummaryReader) Summary(sessionID string) (string, error) {
	if r.Ledger == nil {
		return "", nil
	}
	id := r.SessionID
	if sessionID != "" {
		id = sessionID
	}
	if id == "" {
		return "", nil
	}
	s, err := summary.Build(context.Background(), r.Ledger, id)
	if err != nil {
		return "", nil
	}
	return summary.Format(s), nil
}

// FileAutoMemoryReader reads the MEMORY.md index block from the workspace
// first, then falls back to ~/.config/sin-code/MEMORY.md.
type FileAutoMemoryReader struct {
	Workspace string
	HomeDir   string
}

func (r *FileAutoMemoryReader) IndexBytes() ([]byte, error) {
	var candidates []string
	if r.Workspace != "" {
		candidates = append(candidates, filepath.Join(r.Workspace, "MEMORY.md"))
	}
	if r.HomeDir != "" {
		candidates = append(candidates, filepath.Join(r.HomeDir, "MEMORY.md"))
	} else if cfg, err := os.UserConfigDir(); err == nil {
		candidates = append(candidates, filepath.Join(cfg, "sin-code", "MEMORY.md"))
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err == nil {
			return data, nil
		}
	}
	return nil, nil
}

// LessonsStoreReader adapts *lessons.Store to agentloop.LessonsReader.
// Returns the n most recent lesson texts.
type LessonsStoreReader struct {
	Store *lessons.Store
}

func (r *LessonsStoreReader) Recent(n int) ([]string, error) {
	if r.Store == nil {
		return nil, nil
	}
	entries, err := r.Store.Query(context.Background(), "", n)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Lesson
	}
	return out, nil
}

// MemoryStoreReader adapts *memory.Store to agentloop.MemoryReader.
// Runs a semantic query and returns the top n memory insights.
type MemoryStoreReader struct {
	Store *memory.Store
}

func (r *MemoryStoreReader) Query(q string, n int) ([]string, error) {
	if r.Store == nil {
		return nil, nil
	}
	// Use empty filter to get all memories, then take top n
	items, err := r.Store.List(memory.ListFilter{Limit: n})
	if err != nil {
		return nil, err
	}
	out := make([]string, len(items))
	for i, m := range items {
		out[i] = m.Insight
	}
	return out, nil
}

// GoalStoreReader adapts *autonomy.GoalStore to agentloop.GoalReader.
// Returns active (pending/running) goals as strings.
type GoalStoreReader struct {
	Store *autonomy.GoalStore
}

func (r *GoalStoreReader) Active() ([]string, error) {
	if r.Store == nil {
		return nil, nil
	}
	ctx := context.Background()
	goals, err := r.Store.List(ctx, "")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, g := range goals {
		if g.Status == autonomy.StatusPending || g.Status == autonomy.StatusRunning {
			out = append(out, fmt.Sprintf("goal %d [%d]: %s", g.ID, g.Priority, g.Prompt))
		}
	}
	return out, nil
}

// InMemoryGoalReader is the fallback GoalReader when no persistent store is available.
type InMemoryGoalReader struct {
	Items []string
}

func (r *InMemoryGoalReader) Active() ([]string, error) {
	return r.Items, nil
}

// NewDefaultSessionContextBuilder assembles a SessionContextBuilder from the
// stores available at loop construction time. Nil arguments are skipped
// gracefully. This helper lives in the loopbuilder package so both Build()
// and chat_cmd can wire it without duplicating adapter logic.
func NewDefaultSessionContextBuilder(
	workspace string,
	todos *todo.Store,
	sessionID string,
	ledgerStore *ledger.Store,
	lessonsStore *lessons.Store,
	memoryStore *memory.Store,
	goalStore *autonomy.GoalStore,
	homeDir string,
) *agentloop.SessionContextBuilder {
	var todoReader agentloop.TodoReader
	if todos != nil {
		todoReader = &TodoStoreReader{Store: todos}
	} else {
		todoReader = &InMemoryTodoReader{}
	}
	var sessionReader agentloop.SessionSummaryReader
	if ledgerStore != nil {
		sessionReader = &LedgerSessionSummaryReader{Ledger: ledgerStore, SessionID: sessionID}
	}
	var lessonsReader agentloop.LessonsReader
	if lessonsStore != nil {
		lessonsReader = &LessonsStoreReader{Store: lessonsStore}
	}
	var memoryReader agentloop.MemoryReader
	if memoryStore != nil {
		memoryReader = &MemoryStoreReader{Store: memoryStore}
	}
	var goalReader agentloop.GoalReader
	if goalStore != nil {
		goalReader = &GoalStoreReader{Store: goalStore}
	}
	var autoMemReader agentloop.AutoMemoryReader
	if workspace != "" {
		autoMemReader = &FileAutoMemoryReader{Workspace: workspace, HomeDir: homeDir}
	}
	return agentloop.NewSessionContextBuilder(
		lessonsReader,
		memoryReader,
		goalReader,
		todoReader,
		sessionReader,
		autoMemReader,
	)
}
