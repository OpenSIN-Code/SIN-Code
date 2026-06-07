// SPDX-License-Identifier: MIT
// Purpose: Semantic styles for the SIN-Code TUI (built on lipgloss).
// Docs: styles.doc.md

package theme

import "github.com/charmbracelet/lipgloss"

// Styles bundles every reusable style. Components compose these via .Copy()
// rather than defining new ad-hoc styles, to keep the visual language tight.
type Styles struct {
	// App-level
	App        lipgloss.Style
	Header     lipgloss.Style
	Footer     lipgloss.Style
	Divider    lipgloss.Style
	Title      lipgloss.Style
	Subtitle   lipgloss.Style
	Breadcrumb lipgloss.Style

	// Containers
	Panel       lipgloss.Style
	PanelActive lipgloss.Style
	Card        lipgloss.Style
	Box         lipgloss.Style

	// Text
	Text      lipgloss.Style
	Muted     lipgloss.Style
	Faint     lipgloss.Style
	Bold      lipgloss.Style
	Italic    lipgloss.Style
	Code      lipgloss.Style
	CodeBlock lipgloss.Style
	Link      lipgloss.Style
	Help      lipgloss.Style

	// Status
	Success lipgloss.Style
	Warning lipgloss.Style
	Danger  lipgloss.Style
	Info    lipgloss.Style
	Running lipgloss.Style
	Skipped lipgloss.Style

	// Severity (security)
	Critical lipgloss.Style
	High     lipgloss.Style
	Medium   lipgloss.Style
	Low      lipgloss.Style
	SevInfo  lipgloss.Style

	// Menu
	MenuItem         lipgloss.Style
	MenuItemActive   lipgloss.Style
	MenuItemSelected lipgloss.Style
	MenuIcon         lipgloss.Style
	MenuShortcut     lipgloss.Style
	MenuDesc         lipgloss.Style

	// Table
	TableHeader lipgloss.Style
	TableCell   lipgloss.Style
	TableRow    lipgloss.Style

	// Progress
	ProgressBar   lipgloss.Style
	ProgressTrack lipgloss.Style
	Spinner       lipgloss.Style

	// Search
	SearchInput  lipgloss.Style
	SearchPrompt lipgloss.Style
	SearchMatch  lipgloss.Style

	// Modal
	Modal         lipgloss.Style
	ModalBackdrop lipgloss.Style

	// Button
	Button       lipgloss.Style
	ButtonActive lipgloss.Style
	ButtonDanger lipgloss.Style

	// Tab
	Tab         lipgloss.Style
	TabActive   lipgloss.Style
	TabInactive lipgloss.Style

	// Toast
	Toast lipgloss.Style
}

// Default returns the canonical SIN-Code dark theme.
func Default() *Styles {
	return buildStyles(&Palette)
}

// Light returns the warm-light theme. Same shape as Default, swapped palette.
func Light() *Styles {
	return buildStyles(&LightPalette)
}

// New returns the styles for the named theme. Unknown names fall back to dark
// so a corrupt config never makes the TUI unrenderable.
func New(name string) *Styles {
	if name == ThemeLight {
		return Light()
	}
	return Default()
}

// buildStyles is the single place where every style is defined. Both Default
// and Light call into it with their respective palette so the visual language
// stays in sync across themes.
func buildStyles(p *paletteSet) *Styles {
	s := &Styles{}

	// App-level
	s.App = lipgloss.NewStyle().
		Background(p.Base).
		Foreground(p.Text)

	s.Header = lipgloss.NewStyle().
		Background(p.Surface).
		Foreground(p.Text).
		Bold(true).
		Padding(0, Space2).
		MarginBottom(Space1)

	s.Footer = lipgloss.NewStyle().
		Background(p.Surface).
		Foreground(p.Muted).
		Padding(0, Space2).
		MarginTop(Space1)

	s.Divider = lipgloss.NewStyle().
		Foreground(p.Subtle)

	s.Title = lipgloss.NewStyle().
		Foreground(p.Primary).
		Bold(true)

	s.Subtitle = lipgloss.NewStyle().
		Foreground(p.Accent).
		Italic(true)

	s.Breadcrumb = lipgloss.NewStyle().
		Foreground(p.Muted).
		Italic(true)

	// Containers
	s.Panel = lipgloss.NewStyle().
		Border(BorderRounded).
		BorderForeground(p.Subtle).
		Padding(Space1, Space2).
		MarginBottom(Space1)

	s.PanelActive = lipgloss.NewStyle().
		Border(BorderRounded).
		BorderForeground(p.Primary).
		Padding(Space1, Space2).
		MarginBottom(Space1)

	s.Card = lipgloss.NewStyle().
		Border(BorderHair).
		BorderForeground(p.Overlay).
		Padding(Space1, Space2)

	s.Box = lipgloss.NewStyle().
		Border(BorderRounded).
		BorderForeground(p.Subtle).
		Padding(Space1, Space2)

	// Text
	s.Text = lipgloss.NewStyle().Foreground(p.Text)
	s.Muted = lipgloss.NewStyle().Foreground(p.Muted)
	s.Faint = lipgloss.NewStyle().Foreground(p.Faint)
	s.Bold = lipgloss.NewStyle().Foreground(p.Text).Bold(true)
	s.Italic = lipgloss.NewStyle().Foreground(p.Text).Italic(true)
	s.Code = lipgloss.NewStyle().
		Foreground(p.Accent).
		Background(p.Surface).
		Padding(0, Space1)
	s.CodeBlock = lipgloss.NewStyle().
		Foreground(p.Text).
		Background(p.Surface).
		Padding(Space1, Space2)
	s.Link = lipgloss.NewStyle().
		Foreground(p.Info).
		Underline(true)
	s.Help = lipgloss.NewStyle().Foreground(p.Faint).Italic(true)

	// Status
	s.Success = lipgloss.NewStyle().Foreground(p.Success).Bold(true)
	s.Warning = lipgloss.NewStyle().Foreground(p.Warning).Bold(true)
	s.Danger = lipgloss.NewStyle().Foreground(p.Danger).Bold(true)
	s.Info = lipgloss.NewStyle().Foreground(p.Info).Bold(true)
	s.Running = lipgloss.NewStyle().Foreground(p.Accent).Bold(true)
	s.Skipped = lipgloss.NewStyle().Foreground(p.Faint)

	// Severity
	s.Critical = lipgloss.NewStyle().Foreground(p.Critical).Bold(true)
	s.High = lipgloss.NewStyle().Foreground(p.High).Bold(true)
	s.Medium = lipgloss.NewStyle().Foreground(p.Medium)
	s.Low = lipgloss.NewStyle().Foreground(p.Low)
	s.SevInfo = lipgloss.NewStyle().Foreground(p.InfoSev)

	// Menu
	s.MenuItem = lipgloss.NewStyle().
		Foreground(p.Text).
		Padding(0, Space2)

	s.MenuItemActive = lipgloss.NewStyle().
		Foreground(p.Base).
		Background(p.Primary).
		Bold(true).
		Padding(0, Space2)

	s.MenuItemSelected = lipgloss.NewStyle().
		Foreground(p.Success).
		Bold(true).
		Padding(0, Space2)

	s.MenuIcon = lipgloss.NewStyle().
		Foreground(p.Accent).
		Width(Space3)

	s.MenuShortcut = lipgloss.NewStyle().
		Foreground(p.Muted).
		Width(Space6)

	s.MenuDesc = lipgloss.NewStyle().Foreground(p.Muted).Italic(true)

	// Table
	s.TableHeader = lipgloss.NewStyle().
		Foreground(p.Primary).
		Bold(true).
		BorderBottom(true).
		BorderForeground(p.Subtle).
		Padding(0, Space1)

	s.TableCell = lipgloss.NewStyle().
		Foreground(p.Text).
		Padding(0, Space1)

	s.TableRow = lipgloss.NewStyle().
		Foreground(p.Muted).
		Padding(0, Space1)

	// Progress
	s.ProgressBar = lipgloss.NewStyle().
		Foreground(p.Primary)

	s.ProgressTrack = lipgloss.NewStyle().
		Foreground(p.Subtle)

	s.Spinner = lipgloss.NewStyle().
		Foreground(p.Accent).
		Bold(true)

	// Search
	s.SearchInput = lipgloss.NewStyle().
		Foreground(p.Text).
		Background(p.Surface).
		Padding(0, Space2)

	s.SearchPrompt = lipgloss.NewStyle().
		Foreground(p.Primary).
		Bold(true)

	s.SearchMatch = lipgloss.NewStyle().
		Foreground(p.Base).
		Background(p.Accent).
		Bold(true)

	// Modal
	s.Modal = lipgloss.NewStyle().
		Border(BorderRounded).
		BorderForeground(p.Primary).
		Background(p.Base).
		Foreground(p.Text).
		Padding(Space2, Space3)

	s.ModalBackdrop = lipgloss.NewStyle().
		Foreground(p.Faint)

	// Button
	s.Button = lipgloss.NewStyle().
		Foreground(p.Text).
		Background(p.Surface).
		Padding(0, Space2)

	s.ButtonActive = lipgloss.NewStyle().
		Foreground(p.Base).
		Background(p.Primary).
		Bold(true).
		Padding(0, Space2)

	s.ButtonDanger = lipgloss.NewStyle().
		Foreground(p.Base).
		Background(p.Danger).
		Bold(true).
		Padding(0, Space2)

	// Tab
	s.Tab = lipgloss.NewStyle().
		Foreground(p.Muted).
		Padding(0, Space2)
	s.TabActive = lipgloss.NewStyle().
		Foreground(p.Primary).
		Bold(true).
		Background(p.Surface).
		Padding(0, Space2)
	s.TabInactive = lipgloss.NewStyle().
		Foreground(p.Muted).
		Padding(0, Space2)

	// Toast — short-lived inline status (e.g. "Copied!"). Bold + success
	// color so it pops without needing its own line.
	s.Toast = lipgloss.NewStyle().
		Foreground(p.Base).
		Background(p.Success).
		Bold(true).
		Padding(0, Space1)

	return s
}
