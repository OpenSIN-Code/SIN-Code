package tui

import (
	"fmt"
	"strings"
	"time"
)

var AgentNames = []string{"Build", "Audit", "Stats"}

type Footer struct {
	view        ViewKind
	Selection   string
	AgentIndex  int
	ModelName   string
	Tokens      int
	TokensPct   float64
	Cost        string
	Duration    time.Duration
	Streaming   bool
	Compacted   bool
	Width       int
	ShowHints   bool
	HintKeys    []HintPair
	Loading     bool
	Spinner     Spinner
	TodoOpen        int
	TodoBlocked     int
	TodoOverdue     int
	TodoReady       int
	GitBranch       string
	EstimatedCost   float64
	EstimatedTokens int
}

type HintPair struct {
	Key   string
	Label string
}

func DefaultHints(view ViewKind) []HintPair {
	switch view {
	case ViewTools:
		return []HintPair{
			{"Tab", "view"},
			{"1-6", "jump"},
			{"r", "run"},
			{"t", "theme"},
			{"ctrl+b", "side"},
			{"ctrl+p", "cmds"},
			{"q", "quit"},
		}
	case ViewEFM:
		return []HintPair{
			{"Tab", "view"},
			{"↑/↓", "navigate"},
			{"n", "new stack"},
			{"d", "destroy"},
			{"r", "refresh"},
			{"q", "quit"},
		}
	case ViewConfig:
		return []HintPair{
			{"Tab", "view"},
			{"e", "edit"},
			{"s", "save"},
			{"q", "quit"},
		}
	case ViewHistory:
		return []HintPair{
			{"Tab", "view"},
			{"↑/↓", "navigate"},
			{"c", "clear"},
			{"q", "quit"},
		}
	case ViewTodos:
		return []HintPair{
			{"Tab", "view"},
			{"↑/↓", "navigate"},
			{"o", "open"},
			{"d", "dismiss"},
			{"n", "next"},
			{"q", "quit"},
		}
	case ViewDAG:
		return []HintPair{
			{"Tab", "view"},
			{"↑/↓", "navigate"},
			{"r", "refresh"},
			{"p", "plan"},
			{"q", "quit"},
		}
	default:
		return []HintPair{
			{"Tab", "view"},
			{"1-6", "jump"},
			{"t", "theme"},
			{"q", "quit"},
		}
	}
}

func NewFooter(width int) Footer {
	return Footer{
		view:       ViewTools,
		AgentIndex: 0,
		Tokens:     0,
		TokensPct:  0,
		Cost:       "$0.00",
		Width:      width,
		ShowHints:  true,
		HintKeys:   DefaultHints(ViewTools),
		Spinner:    NewSpinner(),
	}
}

func (f *Footer) SetView(v ViewKind) {
	f.view = v
	f.HintKeys = DefaultHints(v)
}

func (f *Footer) View() ViewKind { return f.view }

func (f *Footer) CycleAgent() {
	f.AgentIndex = (f.AgentIndex + 1) % len(AgentNames)
}

func (f *Footer) AgentName() string {
	if f.AgentIndex < 0 || f.AgentIndex >= len(AgentNames) {
		return "Build"
	}
	return AgentNames[f.AgentIndex]
}

func (f *Footer) EstimateCost(inputTokens int) {
	f.EstimatedTokens = inputTokens
	outputTokens := inputTokens / 2
	f.EstimatedCost = float64(inputTokens)*0.0000005 + float64(outputTokens)*0.0000015
}

func formatEstimatedCost(cost float64) string {
	if cost < 0.01 {
		return "<$0.01"
	}
	return fmt.Sprintf("$%.2f", cost)
}

func (f Footer) ProgressBar(width int) string {
	if width <= 0 {
		return ""
	}
	filled := int(float64(width) * f.TokensPct)
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func (f Footer) Render(styles Styles) string {
	if f.view == ViewChat {
		return f.renderChatFooter(styles)
	}
	return f.renderClassicFooter(styles)
}

func (f Footer) renderChatFooter(styles Styles) string {
	var parts []string

	var status string
	if f.Streaming {
		status = styles.AccentText.Render("⟳ running")
	} else {
		status = styles.Muted.Render("● idle")
	}
	parts = append(parts, status)

	agent := f.AgentName()
	if f.ModelName != "" {
		agent = f.ModelName
	}
	if agent != "" {
		parts = append(parts, styles.FooterKey.Render(agent))
	}

	if f.GitBranch != "" {
		parts = append(parts, styles.Muted.Render(f.GitBranch))
	}

	parts = append(parts, styles.Muted.Render(fmt.Sprintf("%d tokens", f.Tokens)))

	if f.Cost != "" && f.Cost != "$0.00" {
		parts = append(parts, styles.Muted.Render(f.Cost))
	}

	if !f.Streaming && f.EstimatedTokens > 0 {
		parts = append(parts, styles.Muted.Render(fmt.Sprintf("~%s", formatTokens(f.EstimatedTokens))))
		if f.EstimatedCost > 0.01 {
			parts = append(parts, styles.Muted.Render(fmt.Sprintf("~$%.2f", f.EstimatedCost)))
		}
	}

	line := strings.Join(parts, styles.Muted.Render(" · "))
	return styles.Footer.Render(line)
}

func (f Footer) renderClassicFooter(styles Styles) string {
	var left, mid, right strings.Builder

	left.WriteString(styles.FooterKey.Render(" " + f.view.String() + " "))
	left.WriteString(" ")
	if f.Selection != "" {
		left.WriteString(styles.FooterVal.Render(f.Selection))
	} else if f.view == ViewTodos {
		open := footerCount(f, "open", '🔵')
		left.WriteString(open)
	} else {
		left.WriteString(styles.Muted.Render("(no selection)"))
	}
	left.WriteString(" ")

	if f.GitBranch != "" {
		left.WriteString(" ")
		left.WriteString(styles.Muted.Render(f.GitBranch))
	}

	agent := f.AgentName()
	if f.Streaming {
		mid.WriteString(styles.AccentText.Render("⟳ "))
	}
	mid.WriteString(styles.FooterKey.Render(agent))
	mid.WriteString(" ")
	mid.WriteString(styles.Muted.Render(fmt.Sprintf("tokens %d (%.0f%%)", f.Tokens, f.TokensPct*100)))
	mid.WriteString(" ")
	mid.WriteString(styles.FooterVal.Render(f.Cost))
	if f.Duration > 0 {
		mid.WriteString(" ")
		mid.WriteString(styles.Muted.Render(formatDuration(f.Duration)))
	}

	if f.ShowHints {
		right.WriteString(" ")
		for i, h := range f.HintKeys {
			if i > 0 {
				right.WriteString(styles.Muted.Render("·"))
			}
			right.WriteString(styles.FooterKey.Render(h.Key))
			right.WriteString(styles.Muted.Render(":" + h.Label))
		}
	}

	if f.Loading {
		left.WriteString(" ")
		left.WriteString(f.Spinner.View(styles.Spinner))
	}

	gap := f.Width - len(left.String()) - len(right.String())
	if gap > len(mid.String()) {
		mid.WriteString(strings.Repeat(" ", gap-len(mid.String())))
	}

	return styles.Footer.Render(left.String() + mid.String() + right.String())
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}

func footerCount(f Footer, label string, icon rune) string {
	if f.TodoOpen == 0 && f.TodoBlocked == 0 && f.TodoOverdue == 0 {
		return fmt.Sprintf("%c %d %s", icon, f.TodoReady, label)
	}
	return fmt.Sprintf("%c %d open", icon, f.TodoOpen)
}
