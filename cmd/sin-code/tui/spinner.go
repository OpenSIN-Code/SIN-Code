package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

var spinnerFramesAlt = []string{"⠁", "⠂", "⠄", "⡀", "⢀", "⠠", "⠐", "⠈"}

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

var pulseFrames = []string{"●", "◉", "○", "◉"}

func spinnerTick() tea.Cmd {
	return tea.Tick(60*time.Millisecond, func(t time.Time) tea.Msg {
		return SpinnerTickMsg(t)
	})
}

type SpinnerStyle int

const (
	SpinnerStyleBolt   SpinnerStyle = iota
	SpinnerStyleRing
	SpinnerStyleBraille
	SpinnerStylePulse
)

type Spinner struct {
	frame int
	ring  int
	bolt  int
	pulse int
	style SpinnerStyle
}

func NewSpinner() Spinner {
	return Spinner{style: SpinnerStyleBolt}
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
		s.pulse = (s.pulse + 1) % len(pulseFrames)
		return s, spinnerTick()
	}
	return s, nil
}

func (s Spinner) View(style lipgloss.Style) string {
	switch s.style {
	case SpinnerStyleRing:
		frame := spinnerRingFrames[s.ring]
		if s.pulse%2 == 0 {
			return style.Render(frame)
		}
		return style.Bold(false).Render(frame)
	case SpinnerStyleBraille:
		frame := s.brailleFrame()
		if s.pulse%2 == 0 {
			return style.Render(frame)
		}
		return style.Bold(false).Render(frame)
	case SpinnerStylePulse:
		frame := pulseFrames[s.pulse]
		if s.pulse%2 == 0 {
			return style.Render(frame)
		}
		return style.Bold(false).Render(frame)
	default:
		frame := spinnerFrames[s.frame]
		ring := spinnerRingFrames[s.ring]
		bolt := boltFrames[s.bolt]
		if s.pulse%2 == 0 {
			return style.Render(bolt + " " + ring + frame)
		}
		return style.Bold(false).Render(bolt + " " + ring + frame)
	}
}

func (s Spinner) ViewPlain() string {
	switch s.style {
	case SpinnerStyleRing:
		return spinnerRingFrames[s.ring]
	case SpinnerStyleBraille:
		return s.brailleFrame()
	case SpinnerStylePulse:
		return pulseFrames[s.pulse]
	default:
		frame := spinnerFrames[s.frame]
		ring := spinnerRingFrames[s.ring]
		bolt := boltFrames[s.bolt]
		return bolt + " " + ring + frame
	}
}

func (s Spinner) ViewThemed(style lipgloss.Style, theme Theme) string {
	accent := style.Foreground(lipgloss.Color(theme.Accent))
	primary := style.Foreground(lipgloss.Color(theme.AccentDim))

	switch s.style {
	case SpinnerStyleRing:
		frame := spinnerRingFrames[s.ring]
		if s.pulse%2 == 0 {
			return accent.Bold(true).Render(frame)
		}
		return primary.Render(frame)
	case SpinnerStyleBraille:
		frame := s.brailleFrame()
		if s.pulse%2 == 0 {
			return accent.Bold(true).Render(frame)
		}
		return primary.Render(frame)
	case SpinnerStylePulse:
		frame := pulseFrames[s.pulse]
		if s.pulse%2 == 0 {
			return accent.Bold(true).Render(frame)
		}
		return primary.Render(frame)
	default:
		frame := spinnerFrames[s.frame]
		ring := spinnerRingFrames[s.ring]
		bolt := boltFrames[s.bolt]
		if s.pulse%2 == 0 {
			return accent.Render(bolt) + " " + primary.Render(ring + frame)
		}
		return primary.Render(bolt) + " " + accent.Render(ring + frame)
	}
}

func (s Spinner) Frame() int { return s.frame }

func (s *Spinner) SetStyle(style SpinnerStyle) {
	s.style = style
}

func (s Spinner) brailleFrame() string {
	if s.bolt%2 == 0 {
		return spinnerFrames[s.frame]
	}
	return spinnerFramesAlt[s.frame%len(spinnerFramesAlt)]
}
