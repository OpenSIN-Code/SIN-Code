// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type CompactMode struct {
	mu     sync.RWMutex
	active bool
}

func NewCompactMode() *CompactMode {
	return &CompactMode{}
}

func (c *CompactMode) Active() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.active
}

func (c *CompactMode) Toggle() {
	c.mu.Lock()
	c.active = !c.active
	c.mu.Unlock()
}

func (c *CompactMode) Set(active bool) {
	c.mu.Lock()
	c.active = active
	c.mu.Unlock()
}

func truncateLine(s string, max int) string {
	if max < 4 {
		max = 4
	}
	return truncateString(s, max)
}

func renderCompactMessage(msg ChatMessage, styles Styles, width int, focused bool, spinner Spinner) string {
	if width < 10 {
		width = 10
	}
	focusPrefix := ""
	if focused {
		focusPrefix = "▸ "
	}
	prefixW := len(focusPrefix)

	var b strings.Builder
	switch msg.Kind {
	case chatUser:
		text := strings.ReplaceAll(msg.Text, "\n", " ")
		b.WriteString(styles.UserMsg.Render(focusPrefix + "❯ " + truncateLine(text, width-prefixW-2)))
	case chatAssistant:
		allLines := strings.Split(strings.TrimRight(msg.Text, "\n"), "\n")
		shown := allLines
		if len(allLines) > 3 {
			shown = allLines[:3]
		}
		if focusPrefix != "" {
			b.WriteString(styles.Muted.Render(focusPrefix))
		}
		b.WriteString(strings.Join(shown, "\n"))
		if len(allLines) > 3 {
			b.WriteString("\n")
			b.WriteString(styles.Muted.Render("[show more]"))
		}
	case chatTool:
		if msg.Result {
			b.WriteString(styles.StatusOK.Render(focusPrefix + "✓ " + msg.Tool))
		} else {
			b.WriteString(styles.AccentText.Render(focusPrefix + "⚡ " + msg.Tool))
		}
		if msg.Detail != "" {
			d := truncateLine(msg.Detail, width-prefixW-len(msg.Tool)-4)
			b.WriteString(styles.Muted.Render(" → " + d))
		}
	case chatVerify:
		status := "pending"
		if strings.Contains(msg.Detail, "PASS") {
			status = "pass"
		} else if strings.Contains(msg.Detail, "FAIL") {
			status = "fail"
		}
		b.WriteString(renderVerificationCompact(status, truncateLine(msg.Detail, width-2), styles))
	case chatAsk:
		b.WriteString(styles.StatusWarn.Render(focusPrefix + "🔒 " + truncateLine(msg.Detail, width-prefixW-2)))
	case chatDone:
		b.WriteString(styles.StatusOK.Render(focusPrefix + "✓ " + truncateLine(msg.Detail, width-prefixW-2)))
	case chatError:
		errText := msg.Text
		if errText == "" && msg.Error != nil {
			errText = msg.Error.Error()
		}
		b.WriteString(styles.StatusErr.Render(focusPrefix + "❌ " + truncateLine(errText, width-prefixW-2)))
	case chatThinking:
		elapsed := time.Since(msg.Timestamp)
		b.WriteString(styles.AccentText.Render(spinner.ViewThemed(styles.Spinner, styles.Theme) + " thinking"))
		b.WriteString(styles.Muted.Render(fmt.Sprintf(" (%ds)", int(elapsed.Seconds()))))
	case chatSystem:
		b.WriteString(styles.StatusWarn.Render(focusPrefix + "⚠ " + truncateLine(msg.Text, width-prefixW-2)))
	case chatAgent:
		b.WriteString(styles.Muted.Render(focusPrefix + "⟳ " + truncateLine(msg.Text, width-prefixW-2)))
	}
	b.WriteString("\n")
	return b.String()
}
