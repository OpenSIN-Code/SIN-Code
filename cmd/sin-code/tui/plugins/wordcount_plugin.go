// SPDX-License-Identifier: MIT
package plugins

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/tui"
)

type WordCountPlugin struct {
	mu       sync.RWMutex
	filePath string
	words    int
	lines    int
	chars    int
	config   map[string]string
}

func NewWordCountPlugin(config map[string]string) *WordCountPlugin {
	p := &WordCountPlugin{config: config}
	if path, ok := config["file"]; ok {
		p.filePath = path
		p.refresh()
	}
	return p
}

func (w *WordCountPlugin) Name() string { return "wordcount" }

func (w *WordCountPlugin) Render(styles tui.Styles, width, height int) string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if width < 20 {
		width = 20
	}
	label := "Word Count"
	if w.filePath != "" {
		label = "Word Count: " + shortenPath(w.filePath, 20)
	}
	hdr := styles.ContentHdr.Render(label)
	body := styles.Content.Render(fmt.Sprintf("Words: %d · Lines: %d · Chars: %d", w.words, w.lines, w.chars))
	return hdr + "\n" + body
}

func (w *WordCountPlugin) Update(msg tea.Msg) (handled bool) {
	switch msg.(type) {
	case tui.ToolRunMsg:
		w.mu.Lock()
		w.refresh()
		w.mu.Unlock()
		return true
	case tea.KeyMsg:
		return true
	}
	return false
}

func (w *WordCountPlugin) Keybindings() []tui.HintPair {
	return []tui.HintPair{{Key: "ctrl+r", Label: "refresh"}}
}

func (w *WordCountPlugin) SidebarItem() tui.SidebarItem {
	return tui.SidebarItem{View: -1, Icon: "W", Label: "Word Count", Shortcut: "w"}
}

func (w *WordCountPlugin) SetFile(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.filePath = path
	w.refresh()
}

func (w *WordCountPlugin) Stats() (words, lines, chars int) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.words, w.lines, w.chars
}

func (w *WordCountPlugin) refresh() {
	if w.filePath == "" {
		w.words, w.lines, w.chars = 0, 0, 0
		return
	}
	data, err := os.ReadFile(w.filePath)
	if err != nil {
		w.words, w.lines, w.chars = 0, 0, 0
		return
	}
	text := string(data)
	w.chars = len(data)
	w.lines = strings.Count(text, "\n") + 1
	if len(strings.TrimSpace(text)) == 0 {
		w.words = 0
	} else {
		w.words = countWords(text)
	}
}

func countWords(s string) int {
	count := 0
	inWord := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			if !inWord {
				count++
				inWord = true
			}
		} else {
			inWord = false
		}
	}
	return count
}

func shortenPath(p string, maxLen int) string {
	if len(p) <= maxLen {
		return p
	}
	parts := strings.Split(p, "/")
	if len(parts) <= 2 {
		return p
	}
	return "…/" + parts[len(parts)-1]
}

func init() {
	tui.RegisterBuiltin("wordcount", func(config map[string]string) tui.TUIPlugin {
		return NewWordCountPlugin(config)
	})
}
