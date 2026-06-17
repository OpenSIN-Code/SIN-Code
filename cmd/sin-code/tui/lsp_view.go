// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lsp"
)

// LSPDiagnostic is the TUI-facing representation of a single LSP diagnostic.
// It is deliberately a flat, render-friendly struct — the heavier
// lsp.Diagnostic (with Range/Code/etc.) is converted into this at the
// boundary so the view layer never imports the JSON-RPC wire types.
type LSPDiagnostic struct {
	File     string
	Line     int
	Col      int
	Severity string // "error", "warning", "info", "hint"
	Message  string
	Source   string // "gopls", "pyright", etc.
}

// LSPState is the mutable view state for the diagnostics panel.
// It is intended to be embedded in (or referenced by) the main Model
// struct and updated by HandleLSPDiagnostics on each
// textDocument/publishDiagnostics notification. Selection state
// (Selected) is owned here so j/k navigation stays local to the view.
type LSPState struct {
	Diagnostics []LSPDiagnostic
	Selected    int
	Loading     bool
	ServerName  string
}

// LSPDiagnosticsMsg is the bubbletea message carrying a fresh batch of
// diagnostics from the LSP client goroutine to the main model. The
// command that polls the lsp.Client's notification channel should
// emit this message; HandleLSPDiagnostics applies it to LSPState.
type LSPDiagnosticsMsg struct {
	Diagnostics []LSPDiagnostic
}

// RenderLSPView renders the diagnostics panel into a string suitable
// for placement in the main content area. It is pure: given the same
// (state, styles, width, height) it always produces the same bytes,
// which is a prerequisite for the snapshot/hash metric (issue #2)
// and for deterministic screenshot tests.
func RenderLSPView(state LSPState, styles Styles, width, height int) string {
	var b strings.Builder
	b.WriteString(styles.ContentHdr.Render(" LSP Diagnostics"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-2, 10))))
	b.WriteString("\n")

	if state.Loading {
		b.WriteString(styles.AccentText.Render("  ⟳ Loading diagnostics..."))
		b.WriteString("\n")
		return b.String()
	}

	if len(state.Diagnostics) == 0 {
		b.WriteString(styles.StatusOK.Render("  ✅ No diagnostics — all clear!"))
		b.WriteString("\n")
		return b.String()
	}

	// Group by severity for the summary line.
	errors, warnings, infos := 0, 0, 0
	for _, d := range state.Diagnostics {
		switch d.Severity {
		case "error":
			errors++
		case "warning":
			warnings++
		case "info":
			infos++
		}
	}

	b.WriteString(fmt.Sprintf("  %d errors · %d warnings · %d info\n", errors, warnings, infos))
	b.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-2, 10))))
	b.WriteString("\n")

	maxShow := height - 6
	if len(state.Diagnostics) < maxShow {
		maxShow = len(state.Diagnostics)
	}
	if maxShow < 0 {
		maxShow = 0
	}

	for i := 0; i < maxShow; i++ {
		d := state.Diagnostics[i]
		icon := severityIcon(d.Severity)
		sevStyle := styleForSeverity(d.Severity, styles)

		fileShort := d.File
		if len(fileShort) > 30 {
			fileShort = "..." + fileShort[len(fileShort)-27:]
		}

		msg := d.Message
		// Reserve room for icon + file:line:col + source tag. The
		// exact constant is not critical; we just need to avoid
		// wrapping past the panel edge on narrow terminals.
		budget := width - 50
		if budget > 0 && len(msg) > budget {
			msg = msg[:budget-3] + "..."
		}

		line := fmt.Sprintf("  %s %s:%d:%d  %s  %s",
			sevStyle.Render(icon),
			styles.Muted.Render(fileShort),
			d.Line, d.Col,
			msg,
			styles.Muted.Render("["+d.Source+"]"))

		if i == state.Selected {
			b.WriteString(styles.SidebarSel.Render(padRight(line, max(width-2, 0))))
		} else {
			b.WriteString(styles.Content.Render(line))
		}
		b.WriteString("\n")
	}

	if len(state.Diagnostics) > maxShow {
		b.WriteString(styles.Muted.Render(fmt.Sprintf("  ... %d more", len(state.Diagnostics)-maxShow)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  ↑/↓ navigate · enter open file · r refresh"))
	b.WriteString("\n")

	return b.String()
}

// severityIcon maps a LSPDiagnostic.Severity string to a single-glyph
// icon. The set is closed; an unknown severity falls back to a hollow
// bullet so the row still renders without panic.
func severityIcon(severity string) string {
	switch severity {
	case "error":
		return "❌"
	case "warning":
		return "⚠"
	case "info":
		return "ℹ"
	case "hint":
		return "💡"
	default:
		return "○"
	}
}

// styleForSeverity picks the lipgloss.Style for a given severity. It
// returns styles from the shared Styles struct so the panel honours
// the active theme (default / Dracula / Nord / Solarized / Monokai).
func styleForSeverity(severity string, styles Styles) lipgloss.Style {
	switch severity {
	case "error":
		return styles.StatusErr
	case "warning":
		return styles.StatusWarn
	case "info", "hint":
		return styles.AccentText
	default:
		return styles.Muted
	}
}

// HandleLSPDiagnostics applies a LSPDiagnosticsMsg to LSPState. It is
// the single mutation point for the diagnostics slice and is safe to
// call from the bubbletea Update path (single-threaded message
// dispatch). The selection cursor is clamped to the new slice so a
// shrinking diagnostic list never leaves Selected out of range.
func HandleLSPDiagnostics(state *LSPState, msg LSPDiagnosticsMsg) {
	state.Diagnostics = msg.Diagnostics
	state.Loading = false
	if state.Selected >= len(state.Diagnostics) {
		state.Selected = 0
	}
}

// MoveUp moves the selection cursor up by one, clamped at zero.
func (s *LSPState) MoveUp() {
	if s.Selected > 0 {
		s.Selected--
	}
}

// MoveDown moves the selection cursor down by one, clamped at the last
// diagnostic index. No-op when the list is empty.
func (s *LSPState) MoveDown() {
	if s.Selected < len(s.Diagnostics)-1 {
		s.Selected++
	}
}

// SeverityName maps the numeric LSP DiagnosticSeverity (per the LSP
// specification: 1=Error, 2=Warning, 3=Information, 4=Hint) to the
// lowercase string used by LSPDiagnostic.Severity. A nil or
// out-of-range severity defaults to "error" — fail loud in the UI
// rather than silently dropping a real problem.
func SeverityName(sev *int) string {
	if sev == nil {
		return "error"
	}
	switch *sev {
	case 1:
		return "error"
	case 2:
		return "warning"
	case 3:
		return "info"
	case 4:
		return "hint"
	default:
		return "error"
	}
}

// FromLSPDiagnostics converts a batch of wire-level lsp.Diagnostic
// (from a textDocument/publishDiagnostics notification) into the
// flat []LSPDiagnostic the view renders. The URI is stripped of its
// file:// scheme prefix so fileShort truncation shows a real path.
// This is the intended glue between internal/lsp.Client's
// notification handler and the TUI's LSPDiagnosticsMsg — a command
// polling the client's notification channel calls this, wraps the
// result in LSPDiagnosticsMsg, and returns it for the Update loop.
func FromLSPDiagnostics(uri string, diags []lsp.Diagnostic) []LSPDiagnostic {
	out := make([]LSPDiagnostic, 0, len(diags))
	file := strings.TrimPrefix(uri, "file://")
	for _, d := range diags {
		out = append(out, LSPDiagnostic{
			File:     file,
			Line:     d.Range.Start.Line + 1, // LSP positions are 0-based; UI is 1-based
			Col:      d.Range.Start.Character + 1,
			Severity: SeverityName(d.Severity),
			Message:  d.Message,
			Source:   d.Source,
		})
	}
	return out
}
