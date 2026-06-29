// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	tea "charm.land/bubbletea/v2"
)

type StatusPopup struct {
	Open bool
}

func NewStatusPopup() *StatusPopup {
	return &StatusPopup{}
}

func (s *StatusPopup) Open_() {
	s.Open = true
}

func (s *StatusPopup) Close() {
	s.Open = false
}

func (s *StatusPopup) IsOpen() bool {
	return s.Open
}

func (m *Model) OpenStatusPopup() {
	if m.StatusPopup == nil {
		m.StatusPopup = NewStatusPopup()
	}
	m.StatusPopup.Open_()
	m.Mode = ModeStatus
}

func (m *Model) CloseStatusPopup() {
	if m.StatusPopup != nil {
		m.StatusPopup.Close()
	}
	m.Mode = ModeNormal
}

func (m *Model) handleStatusPopupKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc", "enter", "q":
		m.CloseStatusPopup()
		return m, nil
	}
	return m, nil
}

func (m *Model) RenderStatusPopup(styles Styles, width, height int) string {
	if m.StatusPopup == nil || !m.StatusPopup.Open {
		return ""
	}

	popupWidth := 56
	if popupWidth > width-4 {
		popupWidth = width - 4
	}
	if popupWidth < 36 {
		popupWidth = 36
	}

	model := m.Footer.ModelName
	if model == "" {
		model = m.AgentConfig.Model
	}
	if model == "" {
		model = "(not set)"
	}

	baseURL := os.Getenv("SIN_LLM_BASE_URL")
	if baseURL == "" {
		baseURL = os.Getenv("SIN_BASE_URL")
	}
	if baseURL == "" {
		baseURL = "(default)"
	}

	apiKeySet := "not set"
	keyStyle := styles.StatusErr
	if os.Getenv("SIN_NIM_API_KEY") != "" || os.Getenv("SIN_LLM_API_KEY") != "" || os.Getenv("FIREWORKS_API_KEY") != "" {
		apiKeySet = "set"
		keyStyle = styles.StatusOK
	}

	verifyMode := m.AgentConfig.VerifyMode
	if verifyMode == "" {
		verifyMode = m.Footer.VerifyMode
	}
	if verifyMode == "" {
		verifyMode = "poc"
	}

	sessionID := "(none)"
	if m.SessionInfo != nil {
		if sid := m.SessionInfo.SessionID(); sid != "" {
			sessionID = sid
		}
	}

	tokens := m.Footer.Tokens
	cost := m.Footer.Cost
	if cost == "" {
		cost = "$0.00"
	}

	vgStatus := m.Footer.VerifyGate.String()
	vgRender := styles.Muted
	switch m.Footer.VerifyGate {
	case VerifyGatePassed:
		vgRender = styles.StatusOK
	case VerifyGateFailed:
		vgRender = styles.StatusErr
	case VerifyGateRunning:
		vgRender = styles.AccentText
	case VerifyGateIdle:
		vgRender = styles.StatusWarn
	}

	mcpConnected := m.Footer.MCPConnected
	mcpTotal := m.Footer.MCPTotal
	mcpStr := fmt.Sprintf("%d / %d", mcpConnected, mcpTotal)
	if mcpTotal == 0 {
		mcpStr = "(none)"
	}

	labelWidth := 16

	var b strings.Builder
	b.WriteString(styles.AccentText.Render(" Status"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("-", popupWidth-4)))
	b.WriteString("\n\n")

	writeRow := func(label string, value string, valStyle lipgloss.Style) {
		b.WriteString(styles.Muted.Render(fmt.Sprintf("  %-*s", labelWidth, label)))
		b.WriteString(valStyle.Render(value))
		b.WriteString("\n")
	}

	writeRow("Model:", model, styles.FooterVal)
	writeRow("Base URL:", truncateStr(baseURL, popupWidth-labelWidth-6), styles.Muted)
	writeRow("API Key:", apiKeySet, keyStyle)
	writeRow("Verify Mode:", verifyMode, styles.FooterVal)
	writeRow("Verify Gate:", vgStatus, vgRender)
	writeRow("MCP Servers:", mcpStr, styles.Muted)
	writeRow("Session ID:", sessionID, styles.Muted)
	writeRow("Tokens:", fmt.Sprintf("%d", tokens), styles.Muted)
	writeRow("Cost:", cost, styles.Muted)

	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(" esc/enter/q close"))
	b.WriteString("\n")

	return styles.Popup.Render(b.String())
}

func truncateStr(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
