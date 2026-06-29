// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when TUI is rewritten

package tui

import (
	tea "charm.land/bubbletea/v2"
)

func (m *Model) OpenDiffApprovalFromInlineDiff() {
	if m.DiffApproval == nil {
		m.DiffApproval = NewDiffApproval(m.Styles)
	}
	diffs := RecentDiffs()
	if len(diffs) == 0 {
		return
	}
	last := diffs[len(diffs)-1]
	diffText := computeUnifiedDiffText(last.Before, last.After, last.Path)
	filePath := last.Path
	m.DiffApproval.Styles = m.Styles
	m.DiffApproval.Width = min(m.Width-4, 80)
	m.DiffApproval.Height = min(m.Height-4, 24)
	m.DiffApproval.Show(filePath, diffText)
	m.Mode = ModeDiffApproval
}

func (m *Model) handleDiffApprovalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.DiffApproval == nil || !m.DiffApproval.Open {
		m.Mode = ModeNormal
		return m, nil
	}
	k := msg.String()
	switch k {
	case "enter":
		choice := m.DiffApproval.Choice()
		m.DiffApproval.Close()
		m.Mode = ModeNormal
		switch choice {
		case "approve":
			st := PendingDiff()
			if st.Pending {
				_ = ApplyDiff(st.FilePath, st.NewContent)
			}
			ClearPendingDiff()
			m.Footer.ShowToast(ToastSuccess, "Diff approved")
		case "reject":
			ClearPendingDiff()
			m.Footer.ShowToast(ToastInfo, "Diff rejected")
		case "edit":
			m.Footer.ShowToast(ToastInfo, "Edit mode — return to chat")
		}
		return m, nil
	case "esc":
		m.DiffApproval.Close()
		m.Mode = ModeNormal
		ClearPendingDiff()
		return m, nil
	case "tab", "right", "l":
		m.DiffApproval.Next()
		return m, nil
	case "shift+tab", "left", "h":
		m.DiffApproval.Prev()
		return m, nil
	case "up", "k":
		m.DiffApproval.Prev()
		return m, nil
	case "down", "j":
		m.DiffApproval.Next()
		return m, nil
	}
	return m, nil
}
