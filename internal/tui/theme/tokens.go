// Design tokens for the SIN-Code TUI (Bubbletea/Lipgloss).
//
// Centralised theme so CLI/TUI/GUI all share the same color story.
// Inspired by Charm's "Catppuccin" + SIN-OpenSIN-Code brand colors.
//
// Docs: theme/tokens.doc.md

package theme

import "github.com/charmbracelet/lipgloss"

// Theme name constants. Two palettes are shipped — keep this list in sync
// with the cycle in app.go (cycleTheme) and the catalog in PaletteFor.
const (
	ThemeDark  = "dark"
	ThemeLight = "light"
)

// paletteSet is the named type for every palette variant. Keeping the type
// explicit (rather than an anonymous struct) lets us pass palettes around
// and define multiple variants without copy/paste of the field list.
type paletteSet struct {
	// Base
	Base    lipgloss.Color // canvas
	Surface lipgloss.Color // cards / panels
	Subtle  lipgloss.Color // dividers / borders inactive
	Overlay lipgloss.Color // hover / focused border

	// Text
	Text  lipgloss.Color // primary
	Muted lipgloss.Color // secondary
	Faint lipgloss.Color // tertiary / disabled

	// Brand
	Primary lipgloss.Color // SIN-Code magenta (logo)
	Accent  lipgloss.Color // cyan highlight
	Accent2 lipgloss.Color // purple highlight

	// Semantic
	Success lipgloss.Color // green
	Warning lipgloss.Color // yellow / amber
	Danger  lipgloss.Color // red
	Info    lipgloss.Color // blue

	// Severity (for security / SCKG)
	Critical lipgloss.Color // deep red
	High     lipgloss.Color // red
	Medium   lipgloss.Color // orange
	Low      lipgloss.Color // yellow
	InfoSev  lipgloss.Color // gray-blue
}

// Palette — the dark (default) palette. Single source of truth for all
// colors when no theme override is active. Catppuccin Mocha tuned for
// dark terminals.
var Palette = paletteSet{
	// Base
	Base:    lipgloss.Color("#1e1e2e"),
	Surface: lipgloss.Color("#313244"),
	Subtle:  lipgloss.Color("#45475a"),
	Overlay: lipgloss.Color("#585b70"),

	// Text
	Text:  lipgloss.Color("#cdd6f4"),
	Muted: lipgloss.Color("#a6adc8"),
	Faint: lipgloss.Color("#6c7086"),

	// Brand
	Primary: lipgloss.Color("#cba6f7"), // SIN magenta
	Accent:  lipgloss.Color("#94e2d5"), // cyan
	Accent2: lipgloss.Color("#b4befe"), // periwinkle

	// Semantic
	Success: lipgloss.Color("#a6e3a1"),
	Warning: lipgloss.Color("#f9e2af"),
	Danger:  lipgloss.Color("#f38ba8"),
	Info:    lipgloss.Color("#89b4fa"),

	// Severity
	Critical: lipgloss.Color("#ff5555"),
	High:     lipgloss.Color("#fab387"),
	Medium:   lipgloss.Color("#f9e2af"),
	Low:      lipgloss.Color("#a6adc8"),
	InfoSev:  lipgloss.Color("#74c7ec"),
}

// LightPalette — warm light theme based on Solarized Light. Picked for
// daytime / projector use; contrast checked at WCAG AA for body text.
var LightPalette = paletteSet{
	// Base — cream / parchment
	Base:    lipgloss.Color("#fdf6e3"),
	Surface: lipgloss.Color("#eee8d5"),
	Subtle:  lipgloss.Color("#d8c890"),
	Overlay: lipgloss.Color("#93a1a1"),

	// Text — dark blue-black on cream stays readable indoors and outdoors
	Text:  lipgloss.Color("#073642"),
	Muted: lipgloss.Color("#586e75"),
	Faint: lipgloss.Color("#93a1a1"),

	// Brand — warm gold + warm orange (avoid the cold magenta on cream)
	Primary: lipgloss.Color("#b58900"),
	Accent:  lipgloss.Color("#cb4b16"),
	Accent2: lipgloss.Color("#d33682"),

	// Semantic
	Success: lipgloss.Color("#859900"),
	Warning: lipgloss.Color("#cb4b16"),
	Danger:  lipgloss.Color("#dc322f"),
	Info:    lipgloss.Color("#268bd2"),

	// Severity
	Critical: lipgloss.Color("#dc322f"),
	High:     lipgloss.Color("#cb4b16"),
	Medium:   lipgloss.Color("#b58900"),
	Low:      lipgloss.Color("#586e75"),
	InfoSev:  lipgloss.Color("#268bd2"),
}

// PaletteFor returns the palette matching the named theme. Unknown names
// fall back to the dark palette so a stale config never wedges the TUI.
func PaletteFor(name string) *paletteSet {
	if name == ThemeLight {
		return &LightPalette
	}
	return &Palette
}

// Spacing tokens (in terminal cells / chars).
const (
	Space0 = 0
	Space1 = 1
	Space2 = 2
	Space3 = 3
	Space4 = 4
	Space6 = 6
	Space8 = 8
)

// Border styles.
var (
	BorderNone    = lipgloss.HiddenBorder()
	BorderHair    = lipgloss.NormalBorder()
	BorderThin    = lipgloss.ThickBorder() // use sparingly
	BorderRounded = lipgloss.RoundedBorder()
	BorderDouble  = lipgloss.DoubleBorder()
)
