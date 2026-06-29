// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when TUI is rewritten
package tui

import (
	"strings"
)

func renderThinkingIndicator(spinner Spinner, styles Styles) string {
	anim := NewThinkingAnimation()
	anim.SetFrame(spinner.Frame() % len(thinkingFrames))
	return anim.Render(styles)
}

func renderMarkdownWithCodeBlocks(text string, highlighter *SyntaxHighlighter, styles Styles, width int) string {
	if text == "" {
		return ""
	}

	lines := strings.Split(text, "\n")
	var b strings.Builder
	var textBuf strings.Builder
	inBlock := false
	var blockLang string
	var blockBuf strings.Builder

	flushText := func() {
		if textBuf.Len() == 0 {
			return
		}
		rendered := renderMarkdownSimple(textBuf.String(), styles, width)
		rendered = LinkifyText(rendered, styles)
		b.WriteString(strings.TrimRight(rendered, "\n"))
		textBuf.Reset()
	}

	flushCode := func() {
		code := strings.TrimSuffix(blockBuf.String(), "\n")
		code = strings.TrimPrefix(code, "\n")
		if code != "" {
			rendered := renderCodeBlock(code, blockLang, highlighter, styles, width, false)
			b.WriteString(rendered)
		}
		blockBuf.Reset()
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if !inBlock {
				flushText()
				if b.Len() > 0 {
					b.WriteString("\n\n")
				}
				inBlock = true
				blockLang = strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
			} else {
				flushCode()
				b.WriteString("\n")
				inBlock = false
				blockLang = ""
			}
		} else if inBlock {
			blockBuf.WriteString(line)
			blockBuf.WriteString("\n")
		} else {
			textBuf.WriteString(line)
			textBuf.WriteString("\n")
		}
	}

	if inBlock {
		flushCode()
	} else {
		flushText()
	}

	return b.String()
}

func renderMarkdownSimple(text string, styles Styles, width int) string {
	if text == "" {
		return ""
	}
	if !hasMarkdownSyntax(text) {
		return strings.TrimRight(text, "\n")
	}
	r := getCachedRenderer(width)
	if r == nil {
		return strings.TrimRight(text, "\n")
	}
	rendered, err := r.Render(text)
	if err != nil {
		return strings.TrimRight(text, "\n")
	}
	return strings.TrimSpace(rendered)
}
