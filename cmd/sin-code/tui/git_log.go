// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"strings"
	"sync"
)

type GitLogEntry struct {
	Hash    string
	Author  string
	Date    string
	Message string
}

type GitLogView struct {
	mu       sync.Mutex
	runner   GitRunner
	entries  []GitLogEntry
	selected int
	scroll   int
	loaded   bool
}

func NewGitLogView() *GitLogView {
	return &GitLogView{
		runner: defaultGitRunner,
	}
}

func (v *GitLogView) SetRunner(r GitRunner) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if r != nil {
		v.runner = r
	}
}

func (v *GitLogView) Load(count int) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if count <= 0 {
		count = 20
	}

	format := "--format=%H|%an|%ad|%s"
	out, err := v.runner("log", "--oneline", fmt.Sprintf("-%d", count), format, "--date=short")
	if err != nil {
		return err
	}

	v.entries = parseLogEntries(out)
	v.selected = 0
	v.scroll = 0
	v.loaded = true
	return nil
}

func (v *GitLogView) Render(styles Styles, width, height int) string {
	v.mu.Lock()
	defer v.mu.Unlock()

	if !v.loaded {
		return styles.Muted.Render("  No log loaded.")
	}

	if len(v.entries) == 0 {
		return styles.Muted.Render("  No commits.")
	}

	if width < 40 {
		width = 40
	}

	var b strings.Builder

	b.WriteString(styles.ContentHdr.Render("git log"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-2, 10))))
	b.WriteString("\n")

	maxRows := height - 4
	if maxRows < 3 {
		maxRows = 3
	}

	start := v.scroll
	end := start + maxRows
	if end > len(v.entries) {
		end = len(v.entries)
	}

	for i := start; i < end; i++ {
		e := v.entries[i]
		hash := e.Hash
		if len(hash) > 7 {
			hash = hash[:7]
		}

		author := truncate(e.Author, 15)
		msg := truncate(e.Message, max(width-45, 20))

		line := fmt.Sprintf("  %s · %s · %s · %s",
			styles.AccentText.Render(hash),
			styles.Muted.Render(author),
			styles.Muted.Render(e.Date),
			msg,
		)

		if i == v.selected {
			fullLine := fmt.Sprintf("  %s · %s · %s · %s", hash, author, e.Date, msg)
			b.WriteString(styles.SidebarSel.Render(padRight(fullLine, width-2)))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(fmt.Sprintf("  %d commits · ↑↓ navigate", len(v.entries))))

	return b.String()
}

func (v *GitLogView) MoveUp() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.selected > 0 {
		v.selected--
	}
	if v.selected < v.scroll {
		v.scroll = v.selected
	}
}

func (v *GitLogView) MoveDown() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.selected < len(v.entries)-1 {
		v.selected++
	}
	if v.selected >= v.scroll+10 {
		v.scroll = v.selected - 9
	}
	if v.scroll < 0 {
		v.scroll = 0
	}
}

func (v *GitLogView) Entries() []GitLogEntry {
	v.mu.Lock()
	defer v.mu.Unlock()
	cp := make([]GitLogEntry, len(v.entries))
	copy(cp, v.entries)
	return cp
}

func (v *GitLogView) Selected() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.selected
}

func (v *GitLogView) Loaded() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.loaded
}

func (v *GitLogView) SelectedEntry() *GitLogEntry {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.selected < 0 || v.selected >= len(v.entries) {
		return nil
	}
	e := v.entries[v.selected]
	return &e
}

func parseLogEntries(out string) []GitLogEntry {
	var entries []GitLogEntry
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		idx1 := strings.Index(line, "|")
		if idx1 < 0 {
			continue
		}
		rest := line[idx1+1:]

		idx2 := strings.Index(rest, "|")
		if idx2 < 0 {
			continue
		}

		idx3 := strings.Index(rest[idx2+1:], "|")
		if idx3 < 0 {
			continue
		}

		hash := line[:idx1]
		author := rest[:idx2]
		date := rest[idx2+1 : idx2+1+idx3]
		message := rest[idx2+1+idx3+1:]

		entries = append(entries, GitLogEntry{
			Hash:    strings.TrimSpace(hash),
			Author:  strings.TrimSpace(author),
			Date:    strings.TrimSpace(date),
			Message: strings.TrimSpace(message),
		})
	}
	return entries
}
