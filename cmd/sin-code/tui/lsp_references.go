// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

type ReferenceResult struct {
	File    string
	Line    int
	Col     int
	Preview string
}

var lspFindReferencesHook = func(file string, line, col int) ([]ReferenceResult, error) {
	return nil, fmt.Errorf("LSP client not configured")
}

type FindReferences struct {
	mu       sync.Mutex
	results  []ReferenceResult
	selected int
	lastErr  error
	pending  bool
	viewer   *FileViewer
}

func NewFindReferences() *FindReferences {
	return &FindReferences{}
}

func (r *FindReferences) SetViewer(v *FileViewer) {
	r.mu.Lock()
	r.viewer = v
	r.mu.Unlock()
}

func (r *FindReferences) Request(file string, line, col int) error {
	if file == "" {
		return fmt.Errorf("file path required")
	}
	if line < 1 || col < 1 {
		return fmt.Errorf("line and col must be 1-based and positive")
	}

	r.mu.Lock()
	r.pending = true
	r.results = nil
	r.lastErr = nil
	r.selected = 0
	r.mu.Unlock()

	refs, err := lspFindReferencesHook(file, line, col)

	r.mu.Lock()
	defer r.mu.Unlock()
	r.pending = false
	if err != nil {
		r.lastErr = err
		return err
	}
	sort.SliceStable(refs, func(i, j int) bool {
		if refs[i].File != refs[j].File {
			return refs[i].File < refs[j].File
		}
		return refs[i].Line < refs[j].Line
	})
	r.results = refs
	r.selected = 0
	return nil
}

func (r *FindReferences) Results() []ReferenceResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ReferenceResult, len(r.results))
	copy(out, r.results)
	return out
}

func (r *FindReferences) Render(styles Styles, width, height int) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	if width < 10 {
		width = 10
	}
	if height < 3 {
		height = 3
	}

	var b strings.Builder
	b.WriteString(styles.ContentHdr.Render(" Find References"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-2, 10))))
	b.WriteString("\n")

	if r.pending {
		b.WriteString(styles.AccentText.Render("  ⟳ Searching for references..."))
		b.WriteString("\n")
		return b.String()
	}

	if r.lastErr != nil {
		b.WriteString(styles.StatusErr.Render("  ✗ " + r.lastErr.Error()))
		b.WriteString("\n")
		return b.String()
	}

	if len(r.results) == 0 {
		b.WriteString(styles.Muted.Render("  (no references)"))
		b.WriteString("\n")
		return b.String()
	}

	b.WriteString(styles.Muted.Render(fmt.Sprintf("  %d references", len(r.results))))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-2, 10))))
	b.WriteString("\n")

	listHeight := height - 6
	if listHeight < 1 {
		listHeight = 1
	}
	maxShow := listHeight
	if len(r.results) < maxShow {
		maxShow = len(r.results)
	}

	for i := 0; i < maxShow; i++ {
		ref := r.results[i]
		fileShort := ref.File
		if len(fileShort) > 28 {
			fileShort = "..." + fileShort[len(fileShort)-25:]
		}

		preview := strings.TrimSpace(ref.Preview)
		budget := width - 44
		if budget > 0 && len(preview) > budget {
			preview = preview[:budget-3] + "..."
		}

		line := fmt.Sprintf("  %s:%s  %s",
			styles.AccentText.Render(fileShort),
			styles.Muted.Render(fmt.Sprintf("%d:%d", ref.Line, ref.Col)),
			preview)

		if i == r.selected {
			b.WriteString(styles.SidebarSel.Render(padRight(line, max(width-2, 0))))
		} else {
			b.WriteString(styles.Content.Render(line))
		}
		b.WriteString("\n")
	}

	if len(r.results) > maxShow {
		b.WriteString(styles.Muted.Render(fmt.Sprintf("  ... %d more", len(r.results)-maxShow)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  ↑/↓ navigate · enter to jump · esc to dismiss"))
	b.WriteString("\n")
	return b.String()
}

func (r *FindReferences) MoveUp() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.selected > 0 {
		r.selected--
	}
}

func (r *FindReferences) MoveDown() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.selected < len(r.results)-1 {
		r.selected++
	}
}

func (r *FindReferences) Selected() *ReferenceResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.selected < 0 || r.selected >= len(r.results) {
		return nil
	}
	res := r.results[r.selected]
	return &res
}

func (r *FindReferences) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.results)
}

func (r *FindReferences) IsPending() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pending
}

func (r *FindReferences) JumpToSelected() error {
	r.mu.Lock()
	viewer := r.viewer
	if r.selected < 0 || r.selected >= len(r.results) {
		r.mu.Unlock()
		return fmt.Errorf("no reference selected")
	}
	ref := r.results[r.selected]
	r.mu.Unlock()

	if viewer == nil {
		return fmt.Errorf("no file viewer attached")
	}
	if err := viewer.Load(ref.File); err != nil {
		return fmt.Errorf("jump failed: %w", err)
	}
	viewer.SetCursor(ref.Line)
	return nil
}

func (r *FindReferences) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results = nil
	r.selected = 0
	r.lastErr = nil
	r.pending = false
}
