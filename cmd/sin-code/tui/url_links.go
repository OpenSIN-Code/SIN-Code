// SPDX-License-Identifier: MIT
package tui

import (
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
)

// urlRegex matches http(s) URLs in text.
var urlRegex = regexp.MustCompile(`https?://[^\s<>"]+[^\s<>".,)!;]`)

// OSC 8 hyperlink escape sequences.
const (
	osc8Begin = "\x1b]8;;"
	osc8Sep   = "\x1b\\"
	osc8End   = "\x1b]8;;\x1b\\"
)

// LinkifyText detects URLs in text and wraps them with terminal hyperlinks.
// On supporting terminals (iTerm2, WezTerm, kitty, Windows Terminal),
// URLs become clickable via OSC 8 escape sequences.
// On all terminals, URLs are rendered in accent color + underlined.
//
// URLs inside fenced code blocks (```...```) are left untouched.
func LinkifyText(text string, styles Styles) string {
	if !HasURLs(text) {
		return text
	}

	var b strings.Builder
	lines := strings.Split(text, "\n")
	inCodeBlock := false

	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inCodeBlock = !inCodeBlock
			b.WriteString(line)
			if i < len(lines)-1 {
				b.WriteString("\n")
			}
			continue
		}

		if inCodeBlock {
			b.WriteString(line)
			if i < len(lines)-1 {
				b.WriteString("\n")
			}
			continue
		}

		b.WriteString(linkifyLine(line, styles))
		if i < len(lines)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// linkifyLine processes a single line, replacing URLs with styled hyperlinks.
func linkifyLine(line string, styles Styles) string {
	matches := urlRegex.FindAllStringIndex(line, -1)
	if len(matches) == 0 {
		return line
	}

	linkStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Accent)).
		Underline(true)

	var b strings.Builder
	last := 0
	for _, m := range matches {
		start, end := m[0], m[1]
		b.WriteString(line[last:start])

		url := line[start:end]
		styled := linkStyle.Render(url)
		hyperlink := osc8Begin + url + osc8Sep + styled + osc8End
		b.WriteString(hyperlink)
		last = end
	}
	b.WriteString(line[last:])
	return b.String()
}

// HasURLs checks if the text contains any URLs.
func HasURLs(text string) bool {
	return urlRegex.MatchString(text)
}

// ExtractURLs returns all URLs found in text.
func ExtractURLs(text string) []string {
	return urlRegex.FindAllString(text, -1)
}
