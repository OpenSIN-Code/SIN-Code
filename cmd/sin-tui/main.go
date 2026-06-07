// SPDX-License-Identifier: MIT
// Purpose: sin tui entry point — a separate Go binary that the Python
// `sin` CLI can shell out to. Keep the runtime self-contained; no Python deps.
// Docs: main.doc.md

package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/internal/tui"
)

var version = "0.1.0"

func main() {
	m := tui.NewModel()
	p := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "sin tui: %s\n", err)
		os.Exit(1)
	}
}

func init() {
	// Tag the binary at build time via -ldflags "-X main.version=..."
	_ = version
}
