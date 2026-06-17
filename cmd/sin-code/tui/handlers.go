// SPDX-License-Identifier: MIT
package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

type DiffPopupMsg struct{}

func HandleDiffPopup() tea.Cmd {
	return func() tea.Msg { return DiffPopupMsg{} }
}

func RenderDiffPopupView(styles Styles, width, height int) string {
	diffs := RecentDiffs()
	if len(diffs) == 0 {
		return styles.Popup.Render(styles.Muted.Render("  No recent file changes"))
	}
	return RenderDiffPopup(diffs, styles, width, height)
}

func HandleGitRefresh(m *Model, msg GitRefreshMsg) {
	m.Footer.GitBranch = GitBranchShort(msg.Status)
}

func InitGitRefresh() tea.Cmd {
	return RefreshGitCmd()
}

type FilePreviewMsg struct {
	Path string
}

func HandleFilePreview(path string) tea.Cmd {
	return func() tea.Msg { return FilePreviewMsg{Path: path} }
}

func HandleFilePreviewMsg(m *Model, msg FilePreviewMsg) {
	m.ShowFilePreview(msg.Path)
}

func RenderFilePreview(m *Model, styles Styles, width, height int) string {
	if m.FilePreview == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString(styles.ContentHdr.Render("📄 " + m.FilePreviewPath))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-4, 10))))
	b.WriteString("\n")
	b.WriteString(styles.Content.Render(m.FilePreview))
	return styles.Popup.Render(b.String())
}
