package tui

import (
	"image/color"
	"sync"

	"charm.land/lipgloss/v2"
)

type ToastKind int

const (
	ToastInfo ToastKind = iota
	ToastSuccess
	ToastWarning
	ToastError
)

func (k ToastKind) String() string {
	switch k {
	case ToastInfo:
		return "Info"
	case ToastSuccess:
		return "Success"
	case ToastWarning:
		return "Warning"
	case ToastError:
		return "Error"
	}
	return "Info"
}

func (k ToastKind) Icon() string {
	switch k {
	case ToastInfo:
		return "ℹ"
	case ToastSuccess:
		return "✓"
	case ToastWarning:
		return "⚠"
	case ToastError:
		return "✗"
	}
	return "ℹ"
}

const ToastTTL = 75

type Toast struct {
	mu      sync.Mutex
	active  bool
	kind    ToastKind
	message string
	ttl     int
}

func NewToast() *Toast {
	return &Toast{}
}

func (t *Toast) Show(kind ToastKind, message string) {
	t.mu.Lock()
	t.kind = kind
	t.message = message
	t.active = true
	t.ttl = ToastTTL
	t.mu.Unlock()
}

func (t *Toast) Active() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.active
}

func (t *Toast) Tick() {
	t.mu.Lock()
	if !t.active {
		t.mu.Unlock()
		return
	}
	t.ttl--
	if t.ttl <= 0 {
		t.active = false
		t.message = ""
	}
	t.mu.Unlock()
}

func (t *Toast) Kind() ToastKind {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.kind
}

func (t *Toast) Message() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.message
}

func (t *Toast) Render(styles Styles, width int) string {
	t.mu.Lock()
	kind := t.kind
	message := t.message
	active := t.active
	t.mu.Unlock()
	if !active {
		return ""
	}

	var borderCol color.Color
	var titleStyle lipgloss.Style
	switch kind {
	case ToastSuccess:
		borderCol = lipgloss.Color(styles.Theme.Success)
		titleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(styles.Theme.Success)).Bold(true)
	case ToastWarning:
		borderCol = lipgloss.Color(styles.Theme.Warn)
		titleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(styles.Theme.Warn)).Bold(true)
	case ToastError:
		borderCol = lipgloss.Color(styles.Theme.Error)
		titleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(styles.Theme.Error)).Bold(true)
	default:
		borderCol = lipgloss.Color(styles.Theme.Accent)
		titleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(styles.Theme.Accent)).Bold(true)
	}

	boxWidth := width / 3
	if boxWidth < 28 {
		boxWidth = 28
	}
	if boxWidth > width-2 {
		boxWidth = width - 2
	}
	if boxWidth < 24 {
		boxWidth = 24
	}

	inner := boxWidth - 6
	if inner < 16 {
		inner = 16
	}

	title := titleStyle.Render(kind.Icon() + " " + kind.String())
	msg := styles.Content.Render(truncateLine(message, inner))

	body := title + "\n" + msg

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderCol).
		Foreground(lipgloss.Color(styles.Theme.Text)).
		Background(lipgloss.Color(styles.Theme.Background)).
		Padding(0, 1).
		Width(boxWidth).
		Render(body)

	return box
}
