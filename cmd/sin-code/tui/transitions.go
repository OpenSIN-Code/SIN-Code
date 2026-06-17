package tui

import (
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
)

type TransitionKind int

const (
	TransitionNone TransitionKind = iota
	TransitionSlideLeft
	TransitionSlideRight
	TransitionFade
)

const transitionTotalTicks = 5

type Transition struct {
	mu   sync.Mutex
	kind TransitionKind
	tick int
}

func NewTransition() *Transition {
	return &Transition{}
}

func (t *Transition) Start(kind TransitionKind) {
	t.mu.Lock()
	t.kind = kind
	t.tick = 0
	t.mu.Unlock()
}

func (t *Transition) Tick() {
	t.mu.Lock()
	if t.kind == TransitionNone {
		t.mu.Unlock()
		return
	}
	t.tick++
	if t.tick >= transitionTotalTicks {
		t.kind = TransitionNone
		t.tick = 0
	}
	t.mu.Unlock()
}

func (t *Transition) Active() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.kind != TransitionNone
}

func (t *Transition) Progress() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.progressLocked()
}

func (t *Transition) progressLocked() float64 {
	if t.kind == TransitionNone {
		return 1.0
	}
	p := float64(t.tick) / float64(transitionTotalTicks)
	if p < 0 {
		p = 0
	}
	if p > 1 {
		p = 1
	}
	return p
}

func (t *Transition) Kind() TransitionKind {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.kind
}

func (t *Transition) Render(content string, styles Styles, width, height int) string {
	t.mu.Lock()
	kind := t.kind
	progress := t.progressLocked()
	t.mu.Unlock()
	if kind == TransitionNone || width <= 0 || content == "" {
		return content
	}

	switch kind {
	case TransitionFade:
		return lipgloss.NewStyle().Faint(true).Render(content)
	case TransitionSlideLeft:
		shift := int((1.0 - progress) * float64(width))
		if shift > width {
			shift = width
		}
		if shift < 0 {
			shift = 0
		}
		pad := strings.Repeat(" ", shift)
		lines := strings.Split(content, "\n")
		for i, l := range lines {
			lines[i] = pad + l
		}
		return strings.Join(lines, "\n")
	case TransitionSlideRight:
		shift := int(progress * float64(width))
		if shift > width {
			shift = width
		}
		if shift < 0 {
			shift = 0
		}
		pad := strings.Repeat(" ", shift)
		lines := strings.Split(content, "\n")
		for i, l := range lines {
			lines[i] = pad + l
		}
		return strings.Join(lines, "\n")
	}
	return content
}
