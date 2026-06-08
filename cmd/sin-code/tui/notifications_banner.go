// SPDX-License-Identifier: MIT
// Purpose: Notification banner rendering + keybindings (o=open, d=dismiss, n=next).
package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

type NotificationItem struct {
	ID      string
	Title   string
	Message string
	Type    string
	Dismissed bool
}

func (n NotificationItem) GetID() string      { return n.ID }
func (n NotificationItem) GetTitle() string   { return n.Title }
func (n NotificationItem) GetMessage() string { return n.Message }
func (n NotificationItem) GetType() string    { return n.Type }

func (m *Model) SetBanner(n *NotificationItem) {
	m.NotificationBanner = n
	if n != nil {
		m.Notifications = append(m.Notifications, *n)
	}
}

func (m *Model) DismissBanner() {
	if m.NotificationBanner == nil {
		return
	}
	for i := range m.Notifications {
		if m.Notifications[i].ID == m.NotificationBanner.ID {
			m.Notifications[i].Dismissed = true
		}
	}
	m.NotificationBanner = nil
	m.BannerNext()
}

func (m *Model) BannerNext() {
	for i := range m.Notifications {
		if !m.Notifications[i].Dismissed {
			n := m.Notifications[i]
			m.NotificationBanner = &n
			return
		}
	}
	m.NotificationBanner = nil
}

func (m *Model) RenderBanner(styles Styles, width int) string {
	if m.NotificationBanner == nil {
		return ""
	}
	if width < 20 {
		width = 20
	}
	icon := "🔔"
	switch m.NotificationBanner.Type {
	case "todo_completed":
		icon = "✓"
	case "todo_assigned", "todo_claimed":
		icon = "📌"
	case "todo_blocked":
		icon = "⛔"
	case "todo_unblocked":
		icon = "✅"
	case "todo_deleted", "todo_cancelled":
		icon = "✗"
	}
	innerWidth := width - 4
	if innerWidth < 10 {
		innerWidth = 10
	}
	var b strings.Builder
	b.WriteString(styles.AccentText.Render("╭─ " + icon + " " + m.NotificationBanner.Title + " "))
	b.WriteString(styles.Muted.Render(strings.Repeat("─", innerWidth-len(m.NotificationBanner.Title)-6)))
	b.WriteString("╮")
	b.WriteString("\n")
	msg := m.NotificationBanner.Message
	if len(msg) > innerWidth {
		msg = msg[:innerWidth-1] + "…"
	}
	b.WriteString(styles.Content.Render("│  " + msg))
	b.WriteString(strings.Repeat(" ", innerWidth-len(msg)-2))
	b.WriteString("│")
	b.WriteString("\n")
	actions := "[o] open  [d] dismiss  [n] next"
	b.WriteString(styles.Muted.Render("╰─ " + actions + " "))
	b.WriteString(strings.Repeat("─", innerWidth-len(actions)-4))
	b.WriteString("╯")
	b.WriteString("\n")
	return b.String()
}

type BannerKeyMsg struct {
	Action string
}

func BannerOpenCmd(id string) tea.Cmd {
	return func() tea.Msg { return BannerKeyMsg{Action: "open:" + id} }
}

func BannerDismissCmd(id string) tea.Cmd {
	return func() tea.Msg { return BannerKeyMsg{Action: "dismiss:" + id} }
}
