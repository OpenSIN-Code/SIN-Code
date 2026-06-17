// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"sync"
)

const (
	defaultRevealRate  = 50
	streamingCursorRune = "▋"
	streamingMinWidth   = 10
)

type StreamingBuffer struct {
	mu          sync.Mutex
	text        strings.Builder
	revealed    int
	revealRate  int
	completed   bool
	cursorOn    bool
	cursorBlink bool
	maxLines    int
}

func NewStreamingBuffer() *StreamingBuffer {
	return &StreamingBuffer{
		revealRate: defaultRevealRate,
		cursorOn:   true,
		maxLines:   0,
	}
}

func (b *StreamingBuffer) Append(text string) {
	if text == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.completed {
		b.completed = false
	}
	b.text.WriteString(text)
}

func (b *StreamingBuffer) Tick() {
	b.mu.Lock()
	defer b.mu.Unlock()
	total := b.text.Len()
	if b.revealed < total {
		b.revealed += b.revealRate
		if b.revealed > total {
			b.revealed = total
		}
	}
	b.cursorBlink = !b.cursorBlink
}

func (b *StreamingBuffer) SetRevealRate(n int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if n < 1 {
		n = 1
	}
	b.revealRate = n
}

func (b *StreamingBuffer) SetMaxLines(n int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.maxLines = n
}

func (b *StreamingBuffer) SetCursorOn(on bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cursorOn = on
}

func (b *StreamingBuffer) Complete() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.revealed = b.text.Len()
	b.completed = true
	b.cursorBlink = false
}

func (b *StreamingBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.text.Reset()
	b.revealed = 0
	b.completed = false
	b.cursorBlink = false
	b.cursorOn = true
}

func (b *StreamingBuffer) Pending() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	total := b.text.Len()
	if b.revealed > total {
		return 0
	}
	return total - b.revealed
}

func (b *StreamingBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.text.Len()
}

func (b *StreamingBuffer) Revealed() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.revealed
}

func (b *StreamingBuffer) FullText() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.text.String()
}

func (b *StreamingBuffer) IsCompleted() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.completed
}

func (b *StreamingBuffer) Render(styles Styles, width int) string {
	b.mu.Lock()
	total := b.text.Len()
	revealed := b.revealed
	completed := b.completed
	cursorOn := b.cursorOn
	cursorBlink := b.cursorBlink
	maxLines := b.maxLines
	full := b.text.String()
	b.mu.Unlock()

	if width < streamingMinWidth {
		width = streamingMinWidth
	}

	if total == 0 {
		if cursorOn && !completed && cursorBlink {
			return streamingCursorRune
		}
		return ""
	}

	if revealed > total {
		revealed = total
	}
	if revealed < 0 {
		revealed = 0
	}

	runes := []rune(full)
	if revealed > len(runes) {
		revealed = len(runes)
	}
	visible := string(runes[:revealed])

	wrapped := wrapStreamText(visible, width)

	if maxLines > 0 {
		lines := strings.Split(wrapped, "\n")
		if len(lines) > maxLines {
			lines = lines[len(lines)-maxLines:]
		}
		wrapped = strings.Join(lines, "\n")
	}

	if cursorOn && !completed {
		if cursorBlink {
			wrapped += streamingCursorRune
		} else {
			wrapped += " "
		}
	}

	return wrapped
}

func wrapStreamText(text string, width int) string {
	if width < 1 {
		width = 1
	}
	var b strings.Builder
	for _, line := range strings.Split(text, "\n") {
		b.WriteString(wrapStreamLineRunes(line, width))
		b.WriteString("\n")
	}
	out := b.String()
	return strings.TrimSuffix(out, "\n")
}

func wrapStreamLineRunes(line string, width int) string {
	runes := []rune(line)
	if len(runes) <= width {
		return line
	}
	var b strings.Builder
	for i, r := range runes {
		if i > 0 && i%width == 0 {
			b.WriteString("\n")
		}
		b.WriteRune(r)
	}
	return b.String()
}
