package tui

import "github.com/charmbracelet/lipgloss"

type Theme struct {
	Name       string
	Accent     string
	AccentDim  string
	Text       string
	TextDim    string
	Background string
	Border     string
	Success    string
	Warn       string
	Error      string
}

var Themes = []Theme{
	{
		Name:       "default",
		Accent:     "#7D56F4",
		AccentDim:  "#5E40BF",
		Text:       "#FFFFFF",
		TextDim:    "#808080",
		Background: "#1E1E2E",
		Border:     "#7D56F4",
		Success:    "#50FA7B",
		Warn:       "#F1FA8C",
		Error:      "#FF5555",
	},
	{
		Name:       "Dracula",
		Accent:     "#FF79C6",
		AccentDim:  "#BD93F9",
		Text:       "#F8F8F2",
		TextDim:    "#6272A4",
		Background: "#282A36",
		Border:     "#FF79C6",
		Success:    "#50FA7B",
		Warn:       "#F1FA8C",
		Error:      "#FF5555",
	},
	{
		Name:       "Nord",
		Accent:     "#88C0D0",
		AccentDim:  "#5E81AC",
		Text:       "#ECEFF4",
		TextDim:    "#4C566A",
		Background: "#2E3440",
		Border:     "#88C0D0",
		Success:    "#A3BE8C",
		Warn:       "#EBCB8B",
		Error:      "#BF616A",
	},
	{
		Name:       "Solarized",
		Accent:     "#B58900",
		AccentDim:  "#CB4B16",
		Text:       "#FDF6E3",
		TextDim:    "#93A1A1",
		Background: "#002B36",
		Border:     "#B58900",
		Success:    "#859900",
		Warn:       "#B58900",
		Error:      "#DC322F",
	},
	{
		Name:       "Monokai",
		Accent:     "#A6E22E",
		AccentDim:  "#66D9EF",
		Text:       "#F8F8F2",
		TextDim:    "#75715E",
		Background: "#272822",
		Border:     "#A6E22E",
		Success:    "#A6E22E",
		Warn:       "#E6DB74",
		Error:      "#F92672",
	},
}

type Styles struct {
	Theme Theme

	Header     lipgloss.Style
	TabActive  lipgloss.Style
	TabIdle    lipgloss.Style
	TabAdd     lipgloss.Style
	Sidebar    lipgloss.Style
	SidebarSel lipgloss.Style
	SidebarHdr lipgloss.Style
	Content    lipgloss.Style
	ContentHdr lipgloss.Style
	Footer     lipgloss.Style
	FooterKey  lipgloss.Style
	FooterVal  lipgloss.Style
	Popup      lipgloss.Style
	PopupItem  lipgloss.Style
	PopupSel   lipgloss.Style
	Spinner    lipgloss.Style
	Progress   lipgloss.Style
	StatusOK   lipgloss.Style
	StatusWarn lipgloss.Style
	StatusErr  lipgloss.Style
	Hint       lipgloss.Style
	AccentText lipgloss.Style
	Bold       lipgloss.Style
	Muted      lipgloss.Style
}

func c(s string) lipgloss.TerminalColor { return lipgloss.Color(s) }

func NewStyles(theme Theme) Styles {
	t := theme
	s := Styles{Theme: t}

	s.Header = lipgloss.NewStyle().
		Bold(true).
		Foreground(c(t.Accent)).
		Padding(0, 1)

	s.TabActive = lipgloss.NewStyle().
		Foreground(c(t.Background)).
		Background(c(t.Accent)).
		Bold(true).
		Padding(0, 2)

	s.TabIdle = lipgloss.NewStyle().
		Foreground(c(t.TextDim)).
		Background(c(t.Background)).
		Padding(0, 2)

	s.TabAdd = lipgloss.NewStyle().
		Foreground(c(t.Accent)).
		Background(c(t.Background)).
		Bold(true).
		Padding(0, 1)

	s.Sidebar = lipgloss.NewStyle().
		Foreground(c(t.Text)).
		Background(c(t.Background)).
		Padding(0, 1)

	s.SidebarSel = lipgloss.NewStyle().
		Foreground(c(t.Background)).
		Background(c(t.Accent)).
		Bold(true).
		Padding(0, 1)

	s.SidebarHdr = lipgloss.NewStyle().
		Foreground(c(t.Accent)).
		Bold(true).
		Padding(0, 1)

	s.Content = lipgloss.NewStyle().
		Foreground(c(t.Text)).
		Background(c(t.Background)).
		Padding(0, 1)

	s.ContentHdr = lipgloss.NewStyle().
		Foreground(c(t.Accent)).
		Bold(true).
		Padding(0, 1)

	s.Footer = lipgloss.NewStyle().
		Foreground(c(t.TextDim)).
		Background(c(t.Background)).
		Padding(0, 1)

	s.FooterKey = lipgloss.NewStyle().
		Foreground(c(t.Accent)).
		Bold(true)

	s.FooterVal = lipgloss.NewStyle().
		Foreground(c(t.Text))

	s.Popup = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(c(t.Accent)).
		Foreground(c(t.Text)).
		Background(c(t.Background)).
		Padding(1, 2)

	s.PopupItem = lipgloss.NewStyle().
		Foreground(c(t.Text)).
		Padding(0, 1)

	s.PopupSel = lipgloss.NewStyle().
		Foreground(c(t.Background)).
		Background(c(t.Accent)).
		Bold(true).
		Padding(0, 1)

	s.Spinner = lipgloss.NewStyle().
		Foreground(c(t.Accent)).
		Bold(true)

	s.Progress = lipgloss.NewStyle().
		Foreground(c(t.Accent))

	s.StatusOK = lipgloss.NewStyle().Foreground(c(t.Success))
	s.StatusWarn = lipgloss.NewStyle().Foreground(c(t.Warn))
	s.StatusErr = lipgloss.NewStyle().Foreground(c(t.Error))

	s.Hint = lipgloss.NewStyle().Foreground(c(t.TextDim))
	s.AccentText = lipgloss.NewStyle().Foreground(c(t.Accent)).Bold(true)
	s.Bold = lipgloss.NewStyle().Bold(true)
	s.Muted = lipgloss.NewStyle().Foreground(c(t.TextDim))

	return s
}

func (s Styles) BorderColor() lipgloss.TerminalColor {
	return lipgloss.Color(s.Theme.Border)
}

func (s Styles) Accent() lipgloss.TerminalColor {
	return lipgloss.Color(s.Theme.Accent)
}

func (s Styles) Text() lipgloss.TerminalColor {
	return lipgloss.Color(s.Theme.Text)
}

func (s Styles) TextDim() lipgloss.TerminalColor {
	return lipgloss.Color(s.Theme.TextDim)
}
