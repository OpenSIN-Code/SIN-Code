package tui

import (
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
)

type RiskLevel int

const (
	RiskLow RiskLevel = iota
	RiskMedium
	RiskHigh
	RiskCritical
)

func (r RiskLevel) String() string {
	switch r {
	case RiskLow:
		return "LOW"
	case RiskMedium:
		return "MEDIUM"
	case RiskHigh:
		return "HIGH"
	case RiskCritical:
		return "CRITICAL"
	}
	return "LOW"
}

func (r RiskLevel) Icon() string {
	switch r {
	case RiskLow:
		return "🟢"
	case RiskMedium:
		return "🟡"
	case RiskHigh:
		return "🔴"
	case RiskCritical:
		return "🔴"
	}
	return "🟢"
}

type PermissionRequest struct {
	Tool string
	Args string
	Risk RiskLevel
}

type PermissionPopup struct {
	mu      sync.Mutex
	active  bool
	request PermissionRequest
}

func NewPermissionPopup() *PermissionPopup {
	return &PermissionPopup{}
}

func (p *PermissionPopup) SetRequest(req PermissionRequest) {
	p.mu.Lock()
	p.request = req
	p.active = true
	p.mu.Unlock()
}

func (p *PermissionPopup) Active() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.active
}

func (p *PermissionPopup) Dismiss() {
	p.mu.Lock()
	p.active = false
	p.mu.Unlock()
}

func (p *PermissionPopup) Request() PermissionRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.request
}

func RiskFromTool(tool, detail string) RiskLevel {
	t := strings.ToLower(tool)
	d := strings.ToLower(detail)
	switch {
	case strings.Contains(t, "bash") || strings.Contains(t, "execute"):
		if strings.Contains(d, "rm -rf") || strings.Contains(d, "force") || strings.Contains(d, "delete") || strings.Contains(d, "drop") {
			return RiskCritical
		}
		return RiskHigh
	case strings.Contains(t, "git_commit") || strings.Contains(t, "git_push") || strings.Contains(t, "browser_navigate"):
		return RiskHigh
	case strings.Contains(t, "write") || strings.Contains(t, "edit") || strings.Contains(t, "test_generate"):
		return RiskMedium
	}
	return RiskLow
}

func (p *PermissionPopup) Render(styles Styles, width, height int) string {
	p.mu.Lock()
	req := p.request
	active := p.active
	p.mu.Unlock()
	if !active {
		return ""
	}

	popWidth := width / 2
	if popWidth < 44 {
		popWidth = 44
	}
	if popWidth > width-4 {
		popWidth = width - 4
	}
	if popWidth < 30 {
		popWidth = 30
	}

	borderColor := styles.Accent()
	riskStyle := styles.StatusOK
	switch req.Risk {
	case RiskHigh:
		borderColor = lipgloss.Color(styles.Theme.Error)
		riskStyle = styles.StatusErr
	case RiskCritical:
		borderColor = lipgloss.Color(styles.Theme.Error)
		riskStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(styles.Theme.Error)).Bold(true)
	case RiskMedium:
		riskStyle = styles.StatusWarn
	}

	inner := popWidth - 6
	if inner < 20 {
		inner = 20
	}

	var b strings.Builder
	b.WriteString(styles.AccentText.Render(" Permission Required"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", max(inner-2, 18))))
	b.WriteString("\n\n")

	toolName := req.Tool
	if toolName == "" {
		toolName = "(unknown)"
	}
	b.WriteString(styles.AccentText.Render("⚡ " + toolName))
	b.WriteString("\n")
	if req.Args != "" {
		b.WriteString(styles.Content.Render("$ " + truncateLine(req.Args, inner-2)))
	} else {
		b.WriteString(styles.Muted.Render("(no arguments)"))
	}
	b.WriteString("\n\n")

	b.WriteString(styles.Bold.Render("Risk: "))
	b.WriteString(riskStyle.Render(req.Risk.String() + " " + req.Risk.Icon()))
	b.WriteString("\n\n")

	hintEnter := styles.FooterKey.Render("[Enter]") + styles.Muted.Render(" Allow")
	hintEsc := styles.FooterKey.Render("[Esc]") + styles.Muted.Render(" Deny")
	hintA := styles.FooterKey.Render("[A]") + styles.Muted.Render(" Always Allow")
	b.WriteString(hintEnter + "  " + hintEsc + "  " + hintA)
	b.WriteString("\n")

	popupStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(borderColor).
		Foreground(lipgloss.Color(styles.Theme.Text)).
		Background(lipgloss.Color(styles.Theme.Background)).
		Padding(1, 2).
		Width(popWidth).
		MarginBottom(1).
		MarginRight(1).
		MarginBackground(lipgloss.Color("#000000"))

	rendered := popupStyle.Render(b.String())
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, rendered)
}
