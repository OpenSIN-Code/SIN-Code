// Purpose: sin tui — the Bubbletea program. Two-pane layout: a searchable
// command menu on the left, a live preview + output stream on the right.
// Docs: app.doc.md

package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/internal/tui/theme"
)

// AppState — the finite-state machine of the TUI.
type AppState int

const (
	StateMenu       AppState = iota // user is browsing the menu
	StateSearch                     // user is typing in the search bar
	StatePrompt                     // user is providing Args value
	StateRunning                    // a command is currently executing
	StateOutput                     // command finished; showing output
)

// listItem adapts Command -> list.Item.
type listItem struct {
	cmd Command
}

func (l listItem) Title() string       { return l.cmd.Title }
func (l listItem) Description() string { return l.cmd.Description }
func (l listItem) FilterValue() string { return l.cmd.Key + " " + l.cmd.Title + " " + l.cmd.Description }

// Model is the Bubbletea top-level model.
type Model struct {
	state    AppState
	styles   *theme.Styles
	width    int
	height   int
	quitting bool

	// Subcomponents
	list      list.Model
	search    textinput.Model
	spinner   spinner.Model
	viewport  viewport.Model
	cmdList   []Command

	// Execution context
	selected   *Command
	promptHint string
	promptFor  string // current Args placeholder
	output     strings.Builder
	err        error
	startTime  time.Time
}

// NewModel builds a fresh TUI model.
func NewModel() *Model {
	styles := theme.Default()

	// Build the initial list from the full catalog.
	items := make([]list.Item, 0, len(Commands))
	for _, c := range Commands {
		items = append(items, listItem{cmd: c})
	}

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = styles.MenuItemActive
	delegate.Styles.SelectedDesc = styles.MenuItemActive.Foreground(styles.MenuItemActive.GetBackground()).Italic(true)
	delegate.Styles.NormalTitle = styles.MenuItem
	delegate.Styles.NormalDesc = styles.MenuDesc

	l := list.New(items, delegate, 0, 0)
	l.Title = "SIN-Code Commands"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false) // we do our own search via textinput below
	l.Styles.Title = styles.Title

	ti := textinput.New()
	ti.Placeholder = "Type to filter…  ( /  to focus,  esc  to clear)"
	ti.Prompt = "🔍 "
	ti.PromptStyle = styles.SearchPrompt
	ti.TextStyle = styles.SearchInput
	ti.PlaceholderStyle = styles.Help

	sp := spinner.New(spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(styles.Spinner))

	vp := viewport.New(0, 0)
	vp.Style = styles.CodeBlock

	return &Model{
		state:   StateMenu,
		styles:  styles,
		list:    l,
		search:  ti,
		spinner: sp,
		viewport: vp,
	}
}

// Init starts the spinner.
func (m *Model) Init() tea.Cmd {
	return m.spinner.Tick
}

// ── Update ──────────────────────────────────────────────────────────

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layoutComponents()
		return m, nil

	case spinner.TickMsg:
		var c tea.Cmd
		m.spinner, c = m.spinner.Update(msg)
		return m, c

	case runFinishedMsg:
		m.state = StateOutput
		m.err = msg.err
		m.appendOutput(fmt.Sprintf("\n%s  (in %s)\n",
			makeStatusLine(msg.err == nil, time.Since(m.startTime)),
			time.Since(m.startTime).Round(time.Millisecond)))
		m.refreshOutput()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Delegate updates for focused subcomponents.
	if m.state == StateSearch {
		var c tea.Cmd
		m.search, c = m.search.Update(msg)
		cmds = append(cmds, c)
		m.applyFilter(m.search.Value())
	}
	if m.state == StateMenu {
		var c tea.Cmd
		m.list, c = m.list.Update(msg)
		cmds = append(cmds, c)
	}
	if m.state == StateOutput || m.state == StateRunning {
		var c tea.Cmd
		m.viewport, c = m.viewport.Update(msg)
		cmds = append(cmds, c)
	}
	return m, tea.Batch(cmds...)
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global keys
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "q":
		if m.state == StateMenu || m.state == StateOutput {
			m.quitting = true
			return m, tea.Quit
		}
	}

	switch m.state {
	case StateMenu:
		switch msg.String() {
		case "/":
			m.state = StateSearch
			m.search.Focus()
			return m, nil
		case "enter":
			it, ok := m.list.SelectedItem().(listItem)
			if !ok {
				return m, nil
			}
			m.selected = &it.cmd
			return m, m.startCommand(it.cmd)
		case "esc":
			if m.search.Value() != "" {
				m.search.SetValue("")
				m.applyFilter("")
			}
		}

	case StateSearch:
		switch msg.String() {
		case "esc":
			m.state = StateMenu
			m.search.Blur()
			m.search.SetValue("")
			m.applyFilter("")
			return m, nil
		case "enter":
			it, ok := m.list.SelectedItem().(listItem)
			if !ok {
				return m, nil
			}
			m.state = StateMenu
			m.search.Blur()
			m.selected = &it.cmd
			return m, m.startCommand(it.cmd)
		}

	case StatePrompt:
		switch msg.String() {
		case "esc":
			m.state = StateMenu
			m.search.Blur()
			return m, nil
		case "enter":
			value := m.search.Value()
			m.search.Blur()
			m.search.SetValue("")
			m.state = StateRunning
			cmd := m.buildShell(*m.selected, value)
			return m, tea.Batch(m.spinner.Tick, runCommand(cmd, m.appendStream))
		}

	case StateRunning:
		// Allow the user to copy the viewport while running.
		var c tea.Cmd
		m.viewport, c = m.viewport.Update(msg)
		return m, c

	case StateOutput:
		switch msg.String() {
		case "esc", "backspace", "left":
			m.state = StateMenu
			m.output.Reset()
			return m, nil
		}
		var c tea.Cmd
		m.viewport, c = m.viewport.Update(msg)
		return m, c
	}

	return m, nil
}

// ── View ───────────────────────────────────────────────────────────

func (m *Model) View() string {
	if m.quitting {
		return ""
	}
	if m.width == 0 {
		return "Initialising…"
	}

	header := m.viewHeader()
	footer := m.viewFooter()
	body := m.viewBody()

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		body,
		footer,
	)
}

func (m *Model) viewHeader() string {
	left := m.styles.Title.Render("⚡ sin tui")
	right := m.styles.Muted.Render("OpenSIN-Code · " + time.Now().Format("15:04:05"))
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 4
	if gap < 1 {
		gap = 1
	}
	return m.styles.Header.Width(m.width - 2).Render(
		lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", gap), right),
	)
}

func (m *Model) viewFooter() string {
	hint := ""
	switch m.state {
	case StateMenu:
		hint = "↑/↓ navigate · enter run · / search · q quit"
	case StateSearch:
		hint = "type to filter · enter run · esc clear"
	case StatePrompt:
		hint = "type value · enter run · esc back"
	case StateRunning:
		hint = "running… · ctrl+c cancel"
	case StateOutput:
		hint = "↑/↓ scroll · esc back · q quit"
	}
	return m.styles.Footer.Width(m.width - 2).Render(m.styles.Help.Render(hint))
}

func (m *Model) viewBody() string {
	// Reserve header (3) + footer (3) lines.
	bodyH := m.height - 6
	if bodyH < 5 {
		bodyH = 5
	}
	leftW := m.width * 40 / 100
	if leftW < 32 {
		leftW = 32
	}
	rightW := m.width - leftW - 3
	if rightW < 20 {
		rightW = 20
	}

	switch m.state {
	case StateSearch, StateMenu:
		return m.viewMenu(leftW, rightW, bodyH)
	case StatePrompt:
		return m.viewPrompt(leftW, rightW, bodyH)
	case StateRunning, StateOutput:
		return m.viewRun(leftW, rightW, bodyH)
	}
	return ""
}

func (m *Model) viewMenu(leftW, rightW, bodyH int) string {
	m.list.SetSize(leftW, bodyH-2)
	search := m.styles.SearchInput.Width(leftW).Render(m.search.View())
	left := m.styles.PanelActive.Width(leftW).Height(bodyH - 2).Render(
		lipgloss.JoinVertical(lipgloss.Left, search, m.list.View()),
	)

	preview := m.viewPreview(rightW, bodyH-2)
	right := m.styles.Panel.Width(rightW).Height(bodyH - 2).Render(preview)

	return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
}

func (m *Model) viewPrompt(leftW, rightW, bodyH int) string {
	left := m.styles.PanelActive.Width(leftW).Height(bodyH - 2).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			m.styles.Title.Render(m.selected.Title),
			"",
			m.styles.Muted.Render(m.selected.Description),
			"",
			m.styles.Bold.Render("Argument: ")+m.styles.Muted.Render(m.promptFor),
			"",
			m.search.View(),
		),
	)
	preview := m.viewPreview(rightW, bodyH-2)
	right := m.styles.Panel.Width(rightW).Height(bodyH - 2).Render(preview)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
}

func (m *Model) viewRun(leftW, rightW, bodyH int) string {
	var left string
	if m.state == StateRunning {
		left = m.styles.PanelActive.Width(leftW).Height(bodyH - 2).Render(
			lipgloss.JoinVertical(lipgloss.Left,
				m.styles.Title.Render(m.selected.Title),
				"",
				m.styles.Running.Render(m.spinner.View()+" running…"),
				"",
				m.styles.Muted.Render("Started: "+m.startTime.Format("15:04:05")),
			),
		)
	} else {
		left = m.styles.PanelActive.Width(leftW).Height(bodyH - 2).Render(
			lipgloss.JoinVertical(lipgloss.Left,
				m.styles.Title.Render(m.selected.Title),
				"",
				func() string {
					if m.err != nil {
						return m.styles.Danger.Render("✗ failed")
					}
					return m.styles.Success.Render("✓ completed")
				}(),
				"",
				m.styles.Muted.Render("esc to return to menu"),
			),
		)
	}
	m.viewport.Width = rightW - 2
	m.viewport.Height = bodyH - 4
	right := m.styles.Panel.Width(rightW).Height(bodyH - 2).Render(m.viewport.View())
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
}

func (m *Model) viewPreview(w, h int) string {
	if m.selected == nil {
		if it, ok := m.list.SelectedItem().(listItem); ok {
			m.selected = &it.cmd
		}
	}
	if m.selected == nil {
		return m.styles.Muted.Render("(no command selected)")
	}
	c := *m.selected
	header := m.styles.Title.Render(c.Title)
	desc := m.styles.Muted.Render(c.Description)
	args := m.styles.Help.Render("Args: ") + m.styles.Code.Render(c.Args)
	group := m.styles.Help.Render("Group: ") + m.styles.Accent(c.Group).Render(c.Group)
	cmd := m.styles.Help.Render("Command: ") + m.styles.Bold.Render("sin "+c.Key+" "+c.Args)

	return lipgloss.JoinVertical(lipgloss.Left,
		header, "", desc, "", group, args, "", cmd,
	)
}

// makeAccent returns a style for an inline accent (workaround helper).
func (m *Model) Accent(s string) lipgloss.Style {
	return m.styles.Text.Foreground(theme.Palette.Accent)
}

// ── Helpers ────────────────────────────────────────────────────────

func (m *Model) layoutComponents() {
	// search/list sized dynamically in viewMenu; nothing to do here yet.
}

func (m *Model) applyFilter(query string) {
	cmds := Filter(query)
	items := make([]list.Item, 0, len(cmds))
	for _, c := range cmds {
		items = append(items, listItem{cmd: c})
	}
	m.cmdList = cmds
	m.list.SetItems(items)
	if len(items) > 0 {
		m.list.Select(0)
	}
}

func (m *Model) startCommand(c Command) tea.Cmd {
	if c.Args != "" {
		// Prompt the user.
		m.promptFor = c.Args
		m.promptHint = c.Description
		m.state = StatePrompt
		m.search.Focus()
		m.search.Placeholder = c.Args + "  (esc to cancel)"
		return nil
	}
	m.state = StateRunning
	m.output.Reset()
	m.refreshOutput()
	cmd := m.buildShell(c, "")
	return tea.Batch(m.spinner.Tick, runCommand(cmd, m.appendStream))
}

func (m *Model) buildShell(c Command, arg string) string {
	parts := []string{"sin", c.Key}
	if arg != "" {
		parts = append(parts, arg)
	}
	return strings.Join(parts, " ")
}

func (m *Model) appendStream(line string) {
	m.appendOutput(line)
	m.refreshOutput()
}

func (m *Model) appendOutput(line string) {
	m.output.WriteString(line)
}

func (m *Model) refreshOutput() {
	m.viewport.SetContent(m.output.String())
	m.viewport.GotoBottom()
}

func makeStatusLine(ok bool, d time.Duration) string {
	if ok {
		return fmt.Sprintf("✓ done in %s", d.Round(time.Millisecond))
	}
	return fmt.Sprintf("✗ failed after %s", d.Round(time.Millisecond))
}
