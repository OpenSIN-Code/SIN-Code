package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

var spinnerRingFrames = []string{
	"◐",
	"◓",
	"◑",
	"◒",
}

var boltFrames = []string{
	"⚡",
	"✦",
	"✧",
	"✦",
}

func spinnerTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return SpinnerTickMsg(t)
	})
}

type Spinner struct {
	frame int
	ring  int
	bolt  int
}

func NewSpinner() Spinner {
	return Spinner{}
}

func (s Spinner) Init() tea.Cmd {
	return spinnerTick()
}

func (s Spinner) Update(msg tea.Msg) (Spinner, tea.Cmd) {
	switch msg.(type) {
	case SpinnerTickMsg:
		s.frame = (s.frame + 1) % len(spinnerFrames)
		s.ring = (s.ring + 1) % len(spinnerRingFrames)
		s.bolt = (s.bolt + 1) % len(boltFrames)
		return s, spinnerTick()
	}
	return s, nil
}

func (s Spinner) View(style lipgloss.Style) string {
	frame := spinnerFrames[s.frame]
	ring := spinnerRingFrames[s.ring]
	bolt := boltFrames[s.bolt]
	return style.Render(bolt + " " + ring + frame)
}

func (s Spinner) ViewPlain() string {
	frame := spinnerFrames[s.frame]
	ring := spinnerRingFrames[s.ring]
	bolt := boltFrames[s.bolt]
	return bolt + " " + ring + frame
}

func (s Spinner) Frame() int { return s.frame }
