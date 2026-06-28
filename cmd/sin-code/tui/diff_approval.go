// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

const diffApprovalChoices = 3

type DiffApproval struct {
	Open      bool
	FilePath  string
	Diff      string
	Additions int
	Deletions int
	Selected  int
	Styles    Styles
	Width     int
	Height    int
}

func NewDiffApproval(styles Styles) *DiffApproval {
	return &DiffApproval{
		Styles:   styles,
		Selected: 0,
		Width:    60,
		Height:   15,
	}
}

func (d *DiffApproval) Show(filePath, diff string) {
	d.Open = true
	d.FilePath = filePath
	d.Diff = diff
	d.Selected = 0
	d.Additions = 0
	d.Deletions = 0

	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			d.Additions++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			d.Deletions++
		}
	}
}

func (d *DiffApproval) Close() {
	d.Open = false
	d.FilePath = ""
	d.Diff = ""
	d.Selected = 0
	d.Additions = 0
	d.Deletions = 0
}

func (d *DiffApproval) Next() {
	d.Selected = (d.Selected + 1) % diffApprovalChoices
}

func (d *DiffApproval) Prev() {
	d.Selected = (d.Selected - 1 + diffApprovalChoices) % diffApprovalChoices
}

func (d *DiffApproval) Choice() string {
	switch d.Selected {
	case 0:
		return "approve"
	case 1:
		return "reject"
	case 2:
		return "edit"
	}
	return "reject"
}

func (d *DiffApproval) Render() string {
	if !d.Open {
		return ""
	}

	styles := d.Styles
	width := d.Width
	if width < 40 {
		width = 40
	}

	innerWidth := width - 4
	if innerWidth < 20 {
		innerWidth = 20
	}

	var b strings.Builder

	header := fmt.Sprintf(" File Change: %s ", truncateString(d.FilePath, innerWidth-16))
	b.WriteString(styles.ContentHdr.Render(header))
	b.WriteString("\n")

	b.WriteString(styles.Muted.Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n\n")

	b.WriteString(styles.StatusOK.Render(fmt.Sprintf("+%d", d.Additions)))
	b.WriteString(" ")
	b.WriteString(styles.StatusErr.Render(fmt.Sprintf("-%d", d.Deletions)))
	b.WriteString(styles.Muted.Render(" lines"))
	b.WriteString("\n\n")

	diffLines := d.renderDiffBody(styles, innerWidth)
	b.WriteString(diffLines)
	b.WriteString("\n")

	b.WriteString(styles.Muted.Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")

	b.WriteString(d.renderButtons(styles, innerWidth))

	popupStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(c(styles.Theme.Accent)).
		Foreground(c(styles.Theme.Text)).
		Background(c(styles.Theme.Background)).
		Padding(1, 2).
		Width(width)

	return popupStyle.Render(b.String())
}

func (d *DiffApproval) renderDiffBody(styles Styles, innerWidth int) string {
	if d.Diff == "" {
		return styles.Muted.Render("  (no changes)")
	}

	lines := strings.Split(d.Diff, "\n")
	maxLines := d.Height - 8
	if maxLines < 5 {
		maxLines = 5
	}

	addedStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Success)).
		Background(lipgloss.Color("#1b3320"))
	removedStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Error)).
		Background(lipgloss.Color("#331b1b"))
	hunkStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Accent)).
		Bold(true)

	var b strings.Builder
	count := 0
	for _, line := range lines {
		if count >= maxLines {
			b.WriteString(styles.Muted.Render("  ... (truncated)\n"))
			break
		}

		display := truncateString(line, innerWidth-2)

		switch {
		case strings.HasPrefix(line, "@@"):
			b.WriteString(hunkStyle.Render(display))
			b.WriteString("\n")
			count++
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			b.WriteString(styles.Muted.Render(display))
			b.WriteString("\n")
			count++
		case strings.HasPrefix(line, "+"):
			b.WriteString(addedStyle.Render(display))
			b.WriteString("\n")
			count++
		case strings.HasPrefix(line, "-"):
			b.WriteString(removedStyle.Render(display))
			b.WriteString("\n")
			count++
		default:
			if strings.TrimSpace(line) != "" {
				b.WriteString(styles.Content.Render(display))
				b.WriteString("\n")
				count++
			}
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

func (d *DiffApproval) renderButtons(styles Styles, innerWidth int) string {
	labels := []string{"✓ Approve", "✗ Reject", "✎ Edit"}
	var parts []string

	for i, label := range labels {
		if i == d.Selected {
			parts = append(parts, styles.PopupSel.Render("["+label+"]"))
		} else {
			parts = append(parts, styles.Muted.Render("["+label+"]"))
		}
	}

	row := strings.Join(parts, "  ")
	return "  " + row
}

func computeUnifiedDiffText(before, after, filePath string) string {
	lines := computeUnifiedDiff(before, after)
	var b strings.Builder
	b.WriteString("--- a/" + filePath + "\n")
	b.WriteString("+++ b/" + filePath + "\n")
	b.WriteString("@@ -1," + itoa(len(strings.Split(before, "\n"))) + " +1," + itoa(len(strings.Split(after, "\n"))) + " @@\n")
	for _, dl := range lines {
		b.WriteString(dl.prefix + dl.content + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}
