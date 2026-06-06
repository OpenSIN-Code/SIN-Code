// Design tokens for the SIN-Code TUI (Bubbletea/Lipgloss).
//
// Centralised theme so CLI/TUI/GUI all share the same color story.
// Inspired by Charm's "Catppuccin" + SIN-OpenSIN-Code brand colors.
//
// Docs: theme/tokens.doc.md

package theme

import "github.com/charmbracelet/lipgloss"

// Palette — single source of truth for all colors.
// Each token has a meaning, not a name. Use the semantic constants below.
var Palette = struct {
	// Base
	Base       lipgloss.Color // canvas
	Surface    lipgloss.Color // cards / panels
	Subtle     lipgloss.Color // dividers / borders inactive
	Overlay    lipgloss.Color // hover / focused border

	// Text
	Text       lipgloss.Color // primary
	Muted      lipgloss.Color // secondary
	Faint      lipgloss.Color // tertiary / disabled

	// Brand
	Primary    lipgloss.Color // SIN-Code magenta (logo)
	Accent     lipgloss.Color // cyan highlight
	Accent2    lipgloss.Color // purple highlight

	// Semantic
	Success    lipgloss.Color // green
	Warning    lipgloss.Color // yellow / amber
	Danger     lipgloss.Color // red
	Info       lipgloss.Color // blue

	// Severity (for security / SCKG)
	Critical   lipgloss.Color // deep red
	High       lipgloss.Color // red
	Medium     lipgloss.Color // orange
	Low        lipgloss.Color // yellow
	InfoSev    lipgloss.Color // gray-blue
}{
	// Base (Catppuccin Mocha-ish, tuned for dark terminal)
	Base:    lipgloss.Color("#1e1e2e"),
	Surface: lipgloss.Color("#313244"),
	Subtle:  lipgloss.Color("#45475a"),
	Overlay: lipgloss.Color("#585b70"),

	// Text
	Text:    lipgloss.Color("#cdd6f4"),
	Muted:   lipgloss.Color("#a6adc8"),
	Faint:   lipgloss.Color("#6c7086"),

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
