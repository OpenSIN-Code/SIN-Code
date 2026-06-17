// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"
)

const maxViewerLines = 500

type FileViewer struct {
	mu       sync.Mutex
	path     string
	content  string
	lines    []string
	binary   bool
	scroll   int
	truncated bool
	totalLines int
	highlighter *SyntaxHighlighter
}

func NewFileViewer() *FileViewer {
	return &FileViewer{}
}

func (v *FileViewer) Load(path string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.path = path
	v.content = ""
	v.lines = nil
	v.binary = false
	v.scroll = 0
	v.truncated = false
	v.totalLines = 0

	data, err := os.ReadFile(path)
	if err != nil {
		v.lines = []string{fmt.Sprintf("Error: %v", err)}
		return err
	}

	if isBinary(data) {
		v.binary = true
		v.lines = []string{"(binary file)"}
		return nil
	}

	content := string(data)
	if !utf8.ValidString(content) {
		v.binary = true
		v.lines = []string{"(binary file)"}
		return nil
	}

	v.content = content
	allLines := strings.Split(content, "\n")
	v.totalLines = len(allLines)

	if len(allLines) > maxViewerLines {
		v.lines = allLines[:maxViewerLines]
		v.truncated = true
	} else {
		v.lines = allLines
	}

	return nil
}

func (v *FileViewer) Render(styles Styles, width, height int) string {
	v.mu.Lock()
	defer v.mu.Unlock()

	if width < 10 {
		width = 10
	}
	if height < 3 {
		height = 3
	}

	var bldr strings.Builder

	header := "📄 " + v.path
	if v.path == "" {
		header = "📄 (no file)"
	}
	bldr.WriteString(styles.ContentHdr.Render(truncateHeader(header, width-2)))
	bldr.WriteString("\n")
	bldr.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-2, 8))))
	bldr.WriteString("\n")

	if len(v.lines) == 0 {
		bldr.WriteString(styles.Muted.Render("  (empty)"))
		bldr.WriteString("\n")
		return bldr.String()
	}

	listHeight := height - 3
	if listHeight < 1 {
		listHeight = 1
	}

	if v.binary {
		bldr.WriteString(styles.Muted.Render("  (binary file)"))
		bldr.WriteString("\n")
		return bldr.String()
	}

	if v.scroll < 0 {
		v.scroll = 0
	}
	if v.scroll > len(v.lines)-listHeight && len(v.lines) > listHeight {
		v.scroll = len(v.lines) - listHeight
	}

	lineNumWidth := len(fmt.Sprintf("%d", len(v.lines)))
	if lineNumWidth < 3 {
		lineNumWidth = 3
	}
	if lineNumWidth > 6 {
		lineNumWidth = 6
	}

	contentWidth := width - lineNumWidth - 2
	if contentWidth < 5 {
		contentWidth = 5
	}

	lang := languageFromPath(v.path)
	useHighlight := v.highlighter != nil && v.highlighter.SupportsLanguage(lang)

	end := v.scroll + listHeight
	if end > len(v.lines) {
		end = len(v.lines)
	}

	if useHighlight {
		highlighted := v.highlighter.Highlight(strings.Join(v.lines[v.scroll:end], "\n"), lang)
		hlLines := strings.Split(highlighted, "\n")
		for j := 0; j < listHeight && j < len(hlLines); j++ {
			num := fmt.Sprintf("%*d", lineNumWidth, v.scroll+j+1)
			hl := hlLines[j]
			if len(hl) > contentWidth {
				hl = hl[:contentWidth]
			}
			bldr.WriteString(styles.Muted.Render(num + " │ "))
			bldr.WriteString(padRight(hl, contentWidth))
			bldr.WriteString("\n")
		}
	} else {
		for i := v.scroll; i < end; i++ {
			numStr := fmt.Sprintf("%*d", lineNumWidth, i+1)
			line := v.lines[i]
			if len(line) > contentWidth {
				line = line[:contentWidth]
			}
			bldr.WriteString(styles.Muted.Render(numStr + " │ "))
			bldr.WriteString(padRight(line, contentWidth))
			bldr.WriteString("\n")
		}
	}

	if v.truncated {
		remaining := v.totalLines - maxViewerLines
		bldr.WriteString(styles.Muted.Render(fmt.Sprintf("  ... (%d more lines)", remaining)))
		bldr.WriteString("\n")
	}

	return bldr.String()
}

func (v *FileViewer) CurrentPath() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.path
}

func (v *FileViewer) ScrollUp(n int) {
	v.mu.Lock()
	v.scroll -= n
	if v.scroll < 0 {
		v.scroll = 0
	}
	v.mu.Unlock()
}

func (v *FileViewer) ScrollDown(n int) {
	v.mu.Lock()
	v.scroll += n
	v.mu.Unlock()
}

func (v *FileViewer) SetHighlighter(h *SyntaxHighlighter) {
	v.mu.Lock()
	v.highlighter = h
	v.mu.Unlock()
}

func (v *FileViewer) IsBinary() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.binary
}

func (v *FileViewer) LineCount() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.lines)
}

func (v *FileViewer) IsTruncated() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.truncated
}

func (v *FileViewer) Clear() {
	v.mu.Lock()
	v.path = ""
	v.content = ""
	v.lines = nil
	v.binary = false
	v.scroll = 0
	v.truncated = false
	v.totalLines = 0
	v.mu.Unlock()
}

func isBinary(data []byte) bool {
	limit := 512
	if len(data) < limit {
		limit = len(data)
	}
	for i := 0; i < limit; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

func languageFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".pyw":
		return "python"
	default:
		return ext
	}
}

func truncateHeader(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 6 {
		return s[:maxLen]
	}
	return "..." + s[len(s)-maxLen+3:]
}
