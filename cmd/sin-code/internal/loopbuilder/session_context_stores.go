// SPDX-License-Identifier: MIT
// Purpose: concrete store adapters for SessionContextBuilder (issue #379).
// These small wrappers bridge the agentloop reader interfaces to the real
// todo, ledger/summary, and auto-memory surfaces. Any nil store is treated
// as "source unavailable" and returns empty results so the builder can
// still assemble the remaining sections.
package loopbuilder

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
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
// agentloop.SessionSummaryReader interface via the summary package. The
// sessionID is captured at construction time because the builder currently
// calls Summary(""); the wrapper prefers a non-empty explicit argument if
// one is ever provided.
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
// first, then falls back to ~/.config/sin-code/MEMORY.md. This matches the
// task contract (issue #379) without depending on the auto_mem hashed store.
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

// NewDefaultSessionContextBuilder assembles a SessionContextBuilder from the
// stores available at loop construction time. Nil arguments are skipped
// gracefully. This helper lives in the loopbuilder package so both Build()
// and chat_cmd can wire it without duplicating adapter logic.
func NewDefaultSessionContextBuilder(
	workspace string,
	todos *todo.Store,
	sessionID string,
	ledgerStore *ledger.Store,
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
	var autoMemReader agentloop.AutoMemoryReader
	if workspace != "" {
		autoMemReader = &FileAutoMemoryReader{Workspace: workspace, HomeDir: homeDir}
	}
	return agentloop.NewSessionContextBuilder(nil, nil, nil, todoReader, sessionReader, autoMemReader)
}
