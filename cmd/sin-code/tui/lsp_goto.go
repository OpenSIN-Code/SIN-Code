// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"strings"
	"sync"
)

type DefinitionResult struct {
	File string
	Line int
	Col  int
}

var lspGotoDefinitionHook = func(file string, line, col int) (DefinitionResult, error) {
	return DefinitionResult{}, fmt.Errorf("LSP client not configured")
}

type GotoDefinition struct {
	mu      sync.Mutex
	result  *DefinitionResult
	lastErr error
	pending bool
	viewer  *FileViewer
}

func NewGotoDefinition() *GotoDefinition {
	return &GotoDefinition{}
}

func (g *GotoDefinition) SetViewer(v *FileViewer) {
	g.mu.Lock()
	g.viewer = v
	g.mu.Unlock()
}

func (g *GotoDefinition) Request(file string, line, col int) error {
	if file == "" {
		return fmt.Errorf("file path required")
	}
	if line < 1 || col < 1 {
		return fmt.Errorf("line and col must be 1-based and positive")
	}

	g.mu.Lock()
	g.pending = true
	g.result = nil
	g.lastErr = nil
	g.mu.Unlock()

	res, err := lspGotoDefinitionHook(file, line, col)

	g.mu.Lock()
	defer g.mu.Unlock()
	g.pending = false
	if err != nil {
		g.lastErr = err
		return err
	}
	g.lastErr = nil
	g.result = &res
	return nil
}

func (g *GotoDefinition) Result() (*DefinitionResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pending {
		return nil, fmt.Errorf("request in progress")
	}
	if g.lastErr != nil {
		return nil, g.lastErr
	}
	if g.result == nil {
		return nil, fmt.Errorf("no result available")
	}
	out := *g.result
	return &out, nil
}

func (g *GotoDefinition) JumpToResult() error {
	g.mu.Lock()
	viewer := g.viewer
	res := g.result
	g.mu.Unlock()

	if viewer == nil {
		return fmt.Errorf("no file viewer attached")
	}
	if res == nil {
		return fmt.Errorf("no definition result to jump to")
	}
	if err := viewer.Load(res.File); err != nil {
		return fmt.Errorf("jump failed: %w", err)
	}
	viewer.SetCursor(res.Line)
	return nil
}

func (g *GotoDefinition) IsPending() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.pending
}

func (g *GotoDefinition) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.result = nil
	g.lastErr = nil
	g.pending = false
}

func (g *GotoDefinition) Render(styles Styles, width int) string {
	g.mu.Lock()
	defer g.mu.Unlock()

	if width < 10 {
		width = 10
	}

	var b strings.Builder
	b.WriteString(styles.ContentHdr.Render(" Go to Definition"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-2, 10))))
	b.WriteString("\n")

	if g.pending {
		b.WriteString(styles.AccentText.Render("  ⟳ Requesting definition..."))
		b.WriteString("\n")
		return b.String()
	}

	if g.lastErr != nil {
		b.WriteString(styles.StatusErr.Render("  ✗ " + g.lastErr.Error()))
		b.WriteString("\n")
		return b.String()
	}

	if g.result == nil {
		b.WriteString(styles.Muted.Render("  (no definition requested)"))
		b.WriteString("\n")
		return b.String()
	}

	fileShort := g.result.File
	if len(fileShort) > width-12 {
		fileShort = "..." + fileShort[len(fileShort)-(width-15):]
	}
	b.WriteString(styles.AccentText.Render(fmt.Sprintf("  ➜ %s:%d:%d", fileShort, g.result.Line, g.result.Col)))
	b.WriteString("\n")
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  enter to jump · esc to dismiss"))
	b.WriteString("\n")
	return b.String()
}
