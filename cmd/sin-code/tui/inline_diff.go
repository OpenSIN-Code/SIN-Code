package tui

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
)

type DiffLineKind int

const (
	DiffLineContext DiffLineKind = iota
	DiffLineAdded
	DiffLineRemoved
	DiffLineHunk
	DiffLineEmpty
)

type DiffLine struct {
	Kind    DiffLineKind
	Content string
	LineNum int
	OldLine int
	NewLine int
}

type DiffRenderer struct {
	mu     sync.Mutex
	styles Styles
}

func NewDiffRenderer(styles Styles) *DiffRenderer {
	return &DiffRenderer{styles: styles}
}

func (d *DiffRenderer) ParseDiff(diffText string) []DiffLine {
	d.mu.Lock()
	defer d.mu.Unlock()
	return parseDiffLocked(diffText)
}

func parseDiffLocked(diffText string) []DiffLine {
	if diffText == "" {
		return nil
	}

	lines := strings.Split(diffText, "\n")
	result := make([]DiffLine, 0, len(lines))

	oldLine := 0
	newLine := 0

	for _, line := range lines {
		if line == "" {
			result = append(result, DiffLine{Kind: DiffLineEmpty, Content: ""})
			continue
		}

		if strings.HasPrefix(line, "@@") {
			oldStart, newStart := parseHunkHeader(line)
			oldLine = oldStart
			newLine = newStart
			result = append(result, DiffLine{Kind: DiffLineHunk, Content: line})
			continue
		}

		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
			result = append(result, DiffLine{
				Kind:    DiffLineContext,
				Content: line,
				OldLine: oldLine,
				NewLine: newLine,
			})
			continue
		}

		if strings.HasPrefix(line, "+") {
			result = append(result, DiffLine{
				Kind:    DiffLineAdded,
				Content: line[1:],
				NewLine: newLine,
			})
			newLine++
			continue
		}

		if strings.HasPrefix(line, "-") {
			result = append(result, DiffLine{
				Kind:    DiffLineRemoved,
				Content: line[1:],
				OldLine: oldLine,
			})
			oldLine++
			continue
		}

		content := line
		if strings.HasPrefix(line, " ") {
			content = line[1:]
		}
		result = append(result, DiffLine{
			Kind:    DiffLineContext,
			Content: content,
			OldLine: oldLine,
			NewLine: newLine,
		})
		oldLine++
		newLine++
	}

	return result
}

func parseHunkHeader(line string) (int, int) {
	oldStart := 0
	newStart := 0
	parts := strings.Fields(line)
	for _, p := range parts {
		if strings.HasPrefix(p, "-") && len(p) > 1 {
			num := strings.TrimPrefix(p, "-")
			if idx := strings.Index(num, ","); idx >= 0 {
				num = num[:idx]
			}
			oldStart, _ = strconv.Atoi(num)
		}
		if strings.HasPrefix(p, "+") && len(p) > 1 {
			num := strings.TrimPrefix(p, "+")
			if idx := strings.Index(num, ","); idx >= 0 {
				num = num[:idx]
			}
			newStart, _ = strconv.Atoi(num)
		}
	}
	return oldStart, newStart
}

func (d *DiffRenderer) Render(lines []DiffLine, styles Styles, width int) string {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(lines) == 0 {
		return ""
	}

	if width < 20 {
		width = 20
	}

	added := 0
	removed := 0
	for _, l := range lines {
		switch l.Kind {
		case DiffLineAdded:
			added++
		case DiffLineRemoved:
			removed++
		}
	}

	var b strings.Builder

	b.WriteString(styles.StatusOK.Render(fmt.Sprintf("+%d", added)))
	b.WriteString(" ")
	b.WriteString(styles.StatusErr.Render(fmt.Sprintf("-%d", removed)))
	b.WriteString("\n")

	addedStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Success)).
		Background(lipgloss.Color("#1b3320"))
	removedStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Error)).
		Background(lipgloss.Color("#331b1b"))
	hunkStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Accent)).
		Bold(true)

	lnWidth := 4
	contentWidth := width - lnWidth*2 - 3
	if contentWidth < 10 {
		contentWidth = 10
	}

	for _, l := range lines {
		switch l.Kind {
		case DiffLineHunk:
			b.WriteString(hunkStyle.Render(l.Content))
			b.WriteString("\n")
			b.WriteString(styles.Muted.Render(strings.Repeat("─", width)))
		case DiffLineEmpty:
			b.WriteString("\n")
		case DiffLineAdded:
			oldNum := fmt.Sprintf("%*s", lnWidth, "")
			newNum := fmt.Sprintf("%*d", lnWidth, l.NewLine)
			content := "+" + l.Content
			if len(content) > contentWidth {
				content = truncateString(content, contentWidth)
			}
			b.WriteString(fmt.Sprintf("%s %s %s\n", oldNum, newNum, addedStyle.Render(content)))
		case DiffLineRemoved:
			oldNum := fmt.Sprintf("%*d", lnWidth, l.OldLine)
			newNum := fmt.Sprintf("%*s", lnWidth, "")
			content := "-" + l.Content
			if len(content) > contentWidth {
				content = truncateString(content, contentWidth)
			}
			b.WriteString(fmt.Sprintf("%s %s %s\n", oldNum, newNum, removedStyle.Render(content)))
		default:
			oldNum := fmt.Sprintf("%*d", lnWidth, l.OldLine)
			newNum := fmt.Sprintf("%*d", lnWidth, l.NewLine)
			content := " " + l.Content
			if len(content) > contentWidth {
				content = truncateString(content, contentWidth)
			}
			b.WriteString(fmt.Sprintf("%s %s %s\n", oldNum, newNum, styles.Muted.Render(content)))
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

func (d *DiffRenderer) RenderCompact(diffText string, styles Styles, width int) string {
	d.mu.Lock()
	defer d.mu.Unlock()

	if diffText == "" {
		return ""
	}

	if width < 20 {
		width = 20
	}

	lines := parseDiffLocked(diffText)

	fileName := extractDiffFileName(diffText)

	var changes []DiffLine
	for _, l := range lines {
		if l.Kind == DiffLineAdded || l.Kind == DiffLineRemoved {
			changes = append(changes, l)
		}
	}

	if len(changes) == 0 {
		return ""
	}

	var b strings.Builder

	b.WriteString(styles.AccentText.Render("📄 " + fileName))
	b.WriteString("\n")

	added := 0
	removed := 0
	for _, l := range changes {
		if l.Kind == DiffLineAdded {
			added++
		} else {
			removed++
		}
	}
	b.WriteString(styles.StatusOK.Render(fmt.Sprintf("+%d", added)))
	b.WriteString(" ")
	b.WriteString(styles.StatusErr.Render(fmt.Sprintf("-%d", removed)))
	b.WriteString("\n")

	addedStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Success)).
		Background(lipgloss.Color("#1b3320"))
	removedStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Error)).
		Background(lipgloss.Color("#331b1b"))

	maxWidth := width - 4
	if maxWidth < 20 {
		maxWidth = 20
	}

	collapseThreshold := 50
	if len(changes) > collapseThreshold {
		first := changes[:3]
		last := changes[len(changes)-3:]
		middle := len(changes) - 6

		for _, l := range first {
			renderCompactDiffLine(&b, l, addedStyle, removedStyle, maxWidth)
		}
		b.WriteString(styles.Muted.Render(fmt.Sprintf("  ... %d more ...", middle)))
		b.WriteString("\n")
		for _, l := range last {
			renderCompactDiffLine(&b, l, addedStyle, removedStyle, maxWidth)
		}
	} else {
		for _, l := range changes {
			renderCompactDiffLine(&b, l, addedStyle, removedStyle, maxWidth)
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

func renderCompactDiffLine(b *strings.Builder, l DiffLine, addedStyle, removedStyle lipgloss.Style, maxWidth int) {
	var content string
	switch l.Kind {
	case DiffLineAdded:
		content = "+ " + l.Content
		if len(content) > maxWidth {
			content = truncateString(content, maxWidth)
		}
		b.WriteString(addedStyle.Render(content))
	case DiffLineRemoved:
		content = "- " + l.Content
		if len(content) > maxWidth {
			content = truncateString(content, maxWidth)
		}
		b.WriteString(removedStyle.Render(content))
	}
	b.WriteString("\n")
}

func extractDiffFileName(diffText string) string {
	lines := strings.Split(diffText, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "+++ ") {
			file := strings.TrimPrefix(line, "+++ ")
			file = strings.TrimPrefix(file, "b/")
			return file
		}
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "--- ") {
			file := strings.TrimPrefix(line, "--- ")
			file = strings.TrimPrefix(file, "a/")
			return file
		}
	}
	return "file"
}

func isFileModifyingTool(toolName string) bool {
	switch toolName {
	case "sin_edit", "sin_write", "sin_bash":
		return true
	}
	return false
}

func extractDiffFromOutput(output string) string {
	if output == "" {
		return ""
	}
	if !strings.Contains(output, "+++") && !strings.Contains(output, "@@") {
		return ""
	}
	idx := strings.Index(output, "--- ")
	if idx >= 0 {
		return output[idx:]
	}
	idx = strings.Index(output, "@@ ")
	if idx >= 0 {
		return output[idx:]
	}
	return ""
}
