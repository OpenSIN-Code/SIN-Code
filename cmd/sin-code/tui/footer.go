package tui

import (
	"fmt"
	"strings"
)

var AgentNames = []string{"Build", "Audit", "Stats"}

type Footer struct {
	view       ViewKind
	Selection  string
	AgentIndex int
	Tokens     int
	TokensPct  float64
	Cost       string
	Width      int
	ShowHints  bool
	HintKeys   []HintPair
	Loading    bool
	Spinner    Spinner
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
			{"1-5", "jump"},
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
	default:
		return []HintPair{
			{"Tab", "view"},
			{"1-5", "jump"},
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
	var left, mid, right strings.Builder

	left.WriteString(styles.FooterKey.Render(" " + f.view.String() + " "))
	left.WriteString(" ")
	if f.Selection != "" {
		left.WriteString(styles.FooterVal.Render(f.Selection))
	} else {
		left.WriteString(styles.Muted.Render("(no selection)"))
	}
	left.WriteString(" ")

	agent := f.AgentName()
	mid.WriteString(styles.FooterKey.Render(agent))
	mid.WriteString(" ")
	mid.WriteString(styles.Muted.Render(fmt.Sprintf("tokens %d (%.0f%%)", f.Tokens, f.TokensPct*100)))
	mid.WriteString(" ")
	mid.WriteString(styles.FooterVal.Render(f.Cost))

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
