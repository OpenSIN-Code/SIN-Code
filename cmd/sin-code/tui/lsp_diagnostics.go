// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
)

type Diagnostic = LSPDiagnostic

type LSPDiagnostics struct {
	mu       sync.Mutex
	diags    []Diagnostic
	selected int
	errors   int
	warnings int
	infos    int
	hints    int
}

func NewLSPDiagnostics() *LSPDiagnostics {
	return &LSPDiagnostics{}
}

func (d *LSPDiagnostics) Update(diags []Diagnostic) {
	sorted := make([]Diagnostic, len(diags))
	copy(sorted, diags)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].File != sorted[j].File {
			return sorted[i].File < sorted[j].File
		}
		if sorted[i].Line != sorted[j].Line {
			return sorted[i].Line < sorted[j].Line
		}
		return severityRank(sorted[i].Severity) < severityRank(sorted[j].Severity)
	})

	d.mu.Lock()
	defer d.mu.Unlock()
	d.diags = sorted
	d.errors, d.warnings, d.infos, d.hints = 0, 0, 0, 0
	for _, di := range sorted {
		switch di.Severity {
		case "error":
			d.errors++
		case "warning":
			d.warnings++
		case "info":
			d.infos++
		case "hint":
			d.hints++
		}
	}
	if d.selected >= len(d.diags) {
		d.selected = 0
	}
	if d.selected < 0 {
		d.selected = 0
	}
}

func severityRank(sev string) int {
	switch sev {
	case "error":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	case "hint":
		return 3
	default:
		return 4
	}
}

func (d *LSPDiagnostics) Render(styles Styles, width, height int) string {
	d.mu.Lock()
	defer d.mu.Unlock()

	if width < 10 {
		width = 10
	}
	if height < 3 {
		height = 3
	}

	var b strings.Builder
	b.WriteString(styles.ContentHdr.Render(" LSP Diagnostics"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-2, 10))))
	b.WriteString("\n")

	if len(d.diags) == 0 {
		b.WriteString(styles.StatusOK.Render("  ✅ No diagnostics — all clear!"))
		b.WriteString("\n")
		return b.String()
	}

	summary := fmt.Sprintf("  %d errors · %d warnings · %d info", d.errors, d.warnings, d.infos)
	if d.hints > 0 {
		summary += fmt.Sprintf(" · %d hints", d.hints)
	}
	b.WriteString(summary)
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-2, 10))))
	b.WriteString("\n")

	listHeight := height - 6
	if listHeight < 1 {
		listHeight = 1
	}

	maxShow := listHeight
	if len(d.diags) < maxShow {
		maxShow = len(d.diags)
	}

	for i := 0; i < maxShow; i++ {
		di := d.diags[i]
		icon := severityGlyph(di.Severity)
		sevStyle := severityStyle(di.Severity, styles)

		fileShort := di.File
		if len(fileShort) > 28 {
			fileShort = "..." + fileShort[len(fileShort)-25:]
		}

		loc := styles.Muted.Render(fmt.Sprintf("%s:%d:%d", fileShort, di.Line, di.Col))

		msg := di.Message
		budget := width - 48
		if budget > 0 && len(msg) > budget {
			msg = msg[:budget-3] + "..."
		}

		line := fmt.Sprintf("  %s %s  %s  %s",
			sevStyle.Render(icon),
			loc,
			msg,
			styles.Muted.Render("["+di.Source+"]"))

		if i == d.selected {
			b.WriteString(styles.SidebarSel.Render(padRight(line, max(width-2, 0))))
		} else {
			b.WriteString(styles.Content.Render(line))
		}
		b.WriteString("\n")
	}

	if len(d.diags) > maxShow {
		b.WriteString(styles.Muted.Render(fmt.Sprintf("  ... %d more", len(d.diags)-maxShow)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  ↑/↓ navigate · enter open file · r refresh"))
	b.WriteString("\n")

	return b.String()
}

func (d *LSPDiagnostics) MoveUp() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.selected > 0 {
		d.selected--
	}
}

func (d *LSPDiagnostics) MoveDown() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.selected < len(d.diags)-1 {
		d.selected++
	}
}

func (d *LSPDiagnostics) Selected() *Diagnostic {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.selected < 0 || d.selected >= len(d.diags) {
		return nil
	}
	di := d.diags[d.selected]
	return &di
}

func (d *LSPDiagnostics) ErrorCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.errors
}

func (d *LSPDiagnostics) WarningCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.warnings
}

func (d *LSPDiagnostics) InfoCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.infos
}

func (d *LSPDiagnostics) HintCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.hints
}

func (d *LSPDiagnostics) Count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.diags)
}

func (d *LSPDiagnostics) All() []Diagnostic {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]Diagnostic, len(d.diags))
	copy(out, d.diags)
	return out
}

func severityGlyph(severity string) string {
	switch severity {
	case "error":
		return "🔴"
	case "warning":
		return "🟡"
	case "info":
		return "🔵"
	case "hint":
		return "⚪"
	default:
		return "○"
	}
}

func severityStyle(severity string, styles Styles) lipgloss.Style {
	switch severity {
	case "error":
		return styles.StatusErr
	case "warning":
		return styles.StatusWarn
	case "info":
		return styles.AccentText
	case "hint":
		return styles.Muted
	default:
		return styles.Muted
	}
}
