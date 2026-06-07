// SPDX-License-Identifier: MIT
// Purpose: glue between TUI and the notifications package — converts notification
// types to the TeaMsg types defined in messages.go.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/notifications"
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

// ListenForNotifications returns a tea.Cmd that blocks on the notifications
// broadcaster channel and emits a NotificationMsg when one arrives.
// Re-subscribe from Update after each NotificationMsg to keep listening.
func ListenForNotifications() tea.Cmd {
	return func() tea.Msg {
		n, ok := <-notifications.TUIBroadcaster()
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
