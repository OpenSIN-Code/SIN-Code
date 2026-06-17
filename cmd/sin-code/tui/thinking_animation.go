package tui

import (
	"fmt"
	"sync"
	"time"
)

var thinkingFrames = []string{
	"🤔 ·",
	"🤔 ··",
	"🤔 ···",
	"🤔  ·",
	"🤔  ··",
	"🤔  ···",
}

var thinkingBrailleFrames = []string{
	"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏",
}

type ThinkingAnimation struct {
	mu      sync.Mutex
	frame   int
	start   time.Time
	braille bool
}

func NewThinkingAnimation() *ThinkingAnimation {
	return &ThinkingAnimation{}
}

func (t *ThinkingAnimation) Tick() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.frame++
	max := len(thinkingFrames)
	if t.braille {
		max = len(thinkingBrailleFrames)
	}
	if t.frame >= max {
		t.frame = 0
	}
}

func (t *ThinkingAnimation) Start() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.start = time.Now()
	t.frame = 0
}

func (t *ThinkingAnimation) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.start = time.Time{}
	t.frame = 0
}

func (t *ThinkingAnimation) Elapsed() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.start.IsZero() {
		return 0
	}
	return time.Since(t.start)
}

func (t *ThinkingAnimation) Frame() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.frame
}

func (t *ThinkingAnimation) SetFrame(f int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.frame = f
}

func (t *ThinkingAnimation) SetStart(ts time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.start = ts
}

func (t *ThinkingAnimation) SetBraille(b bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.braille = b
}

func (t *ThinkingAnimation) Render(styles Styles) string {
	t.mu.Lock()
	frame := t.frame
	braille := t.braille
	start := t.start
	t.mu.Unlock()

	var indicator string
	if braille {
		idx := frame % len(thinkingBrailleFrames)
		indicator = thinkingBrailleFrames[idx]
	} else {
		idx := frame % len(thinkingFrames)
		indicator = thinkingFrames[idx]
	}

	result := styles.AccentText.Render("Thinking") + " " + styles.Muted.Render(indicator)

	if !start.IsZero() {
		elapsed := time.Since(start)
		if elapsed >= time.Second {
			secs := int(elapsed.Seconds())
			result += " " + styles.Muted.Render(fmt.Sprintf("· %ds", secs))
		}
	}

	return result
}
