// SPDX-License-Identifier: MIT
// Purpose: glue between TUI and the notifications package — converts notification
// types to the TeaMsg types defined in messages.go.
package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/notifications"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// NotificationSource is the interface used by NotificationMsg to access
// notification fields. Implemented by *notifications.Notification and by
// test doubles in todos_view_test.go.
type NotificationSource interface {
	GetID() string
	GetTitle() string
	GetMessage() string
	GetType() string
}

// tuiBroadcasterHook is a test seam for the notifications broadcaster.
var tuiBroadcasterHook = func() <-chan *notifications.Notification { return notifications.TUIBroadcaster() }

// ListenForNotifications returns a tea.Cmd that blocks on the notifications
// broadcaster channel and emits a NotificationMsg when one arrives.
// Re-subscribe from Update after each NotificationMsg to keep listening.
func ListenForNotifications() tea.Cmd {
	return func() tea.Msg {
		n, ok := <-tuiBroadcasterHook()
		if !ok || n == nil {
			return nil
		}
		return NotificationMsg{N: n}
	}
}

// RefreshTodosCmd returns a tea.Cmd that re-counts todos.
func RefreshTodosCmd() tea.Cmd {
	return func() tea.Msg {
		return CountsMsg{Open: 0, Blocked: 0, Overdue: 0, Ready: 0}
	}
}

// sessionStorePathHook is a test seam for the session DB path.
// Defaults to session.DefaultPath() (~/.local/share/sin-code/sessions.db);
// tests override to point at a temp DB.
var sessionStorePathHook = session.DefaultPath

// sessionTreeDataHook is a test seam that loads session tree data from
// the store at the given path. Returns flat SessionNodeData suitable for
// BuildSessionTree. Tests override to avoid hitting a real SQLite DB.
var sessionTreeDataHook = func(path string) ([]SessionNodeData, error) {
	store, err := session.Open(path)
	if err != nil {
		return nil, err
	}
	defer store.Close()

	infos, err := store.List()
	if err != nil {
		return nil, err
	}

	data := make([]SessionNodeData, 0, len(infos))
	for _, info := range infos {
		createdAt, _ := time.Parse(time.RFC3339, info.CreatedAt)
		updatedAt, _ := time.Parse(time.RFC3339, info.UpdatedAt)

		var msgCount int
		var preview string
		if sess, err := store.StartOrResume(info.ID); err == nil {
			hist := sess.History()
			msgCount = len(hist)
			for i := len(hist) - 1; i >= 0; i-- {
				if hist[i].Role == "assistant" && hist[i].Content != "" {
					preview = hist[i].Content
					break
				}
			}
		}

		data = append(data, SessionNodeData{
			ID:           info.ID,
			Name:         info.Title,
			ParentID:     info.ParentID,
			CreatedAt:    createdAt,
			LastActive:   updatedAt,
			MessageCount: msgCount,
			Preview:      preview,
		})
	}

	return data, nil
}

// RefreshSessionTreeCmd returns a tea.Cmd that queries the session store
// and returns a SessionTreeMsg with real session data. Returns nil (no-op)
// when the store is unavailable — graceful degradation.
func RefreshSessionTreeCmd() tea.Cmd {
	return func() tea.Msg {
		dbPath := sessionStorePathHook()
		if dbPath == "" {
			return nil
		}

		data, err := sessionTreeDataHook(dbPath)
		if err != nil {
			return nil
		}

		return SessionTreeMsg{Sessions: data}
	}
}

// lspDiagnosticsHook is a test seam for obtaining LSP diagnostics.
//
// The internal/lsp package uses an asynchronous notification model:
// servers push textDocument/publishDiagnostics via a handler registered
// with Client.SetNotificationHandler. There is no synchronous pull API
// (no Diagnostics() method), and no global registry of running clients
// that subscribe.go can reach. Starting an LSP server inside a refresh
// command would be wrong — servers are long-lived and should be managed
// by a dedicated goroutine that collects notifications and feeds them
// here.
//
// The default therefore returns nil (graceful degradation): the LSP
// panel shows "No diagnostics — all clear!" when no server is running,
// which is correct. When real LSP integration is wired up, a caller
// replaces this hook with one that reads from whatever diagnostics
// buffer the notification handler populates.
var lspDiagnosticsHook = func() ([]LSPDiagnostic, error) {
	return nil, nil
}

// RefreshLSPCmd returns a tea.Cmd that fetches diagnostics from the LSP
// client and emits an LSPDiagnosticsMsg for the Update loop to apply via
// HandleLSPDiagnostics. Returns nil (no-op) when diagnostics are
// unavailable — graceful degradation so the TUI never crashes when no
// LSP server is running.
func RefreshLSPCmd() tea.Cmd {
	return func() tea.Msg {
		diags, err := lspDiagnosticsHook()
		if err != nil || diags == nil {
			return nil
		}
		return LSPDiagnosticsMsg{Diagnostics: diags}
	}
}
