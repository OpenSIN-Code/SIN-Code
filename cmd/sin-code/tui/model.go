package tui

import (
	"context"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/tui/chat"
	agentrunner "github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/tui"
)

type Mode int

const (
	ModeNormal Mode = iota
	ModePalette
	ModeSubagents
	ModeArgInput
)

type PaletteState struct {
	Open   bool
	Query  string
	Items  []string
	Filter []string
	Sel    int
}

type ArgInputState struct {
	Open  bool
	Cmd   string
	Value string
	Input textinput.Model
}

// teaProgramIface is the subset of *tea.Program the chat runner needs:
// it just calls Send to push messages back into the event loop.
// Defined here as a type alias for *tea.Program (declared in
// chat_program.go) so the model file does not need to import bubbletea.
type teaProgramIface interface {
	Send(msg any)
}

// Model is the top-level TUI model. Program, ChatRunner, and ctxFn are
// optional — when nil, the chat submit path falls back to synchronous
// behavior (used by tests and headless invocations).
type Model struct {
	Width      int
	Height     int
	ThemeIdx   int
	ViewKind   ViewKind
	Mode       Mode
	Quitting   bool
	Ready      bool
	Loading    bool

	Tabs       Tabs
	Sidebar    Sidebar
	Footer     Footer
	Spinner    Spinner
	Styles     Styles
	RightPanel bool

	Palette   PaletteState
	ArgInput  ArgInputState
	History   []HistoryEntry
	EFMStks   []EFMStack
	Config    []ConfigEntry
	ConfigSel int

	ToolList  list.Model
	ToolItems []list.Item

	TodoItems []TodoRow
	TodoSel   int

	NotificationBanner *NotificationItem
	Notifications      []NotificationItem

	ChatInput   *chatInput
	ChatHistory []string
	ChatRunner  *chat.Runner
	Program     teaProgramIface
	ctxFn       func() context.Context

	// AgentRunner is the v3.3.1 (issue #53) full agentloop embed. When
	// set, chat submits and skill-palette entries route through it
	// instead of the simple LLM chat runner. Lazily initialized by
	// initAgentRunner.
	AgentRunner *agentrunner.AgentRunner
	// pendingAsk is the AskReply channel for the most recent agent
	// ask event, or nil if no ask is pending. The chat view's y/N
	// keypress handler pops it via answerPendingAsk.
	pendingAsk chan bool

	// Workspace is the directory the TUI is operating in. The agent
	// runner uses this to locate the .sin-code/sessions.db. Set by
	// main() at startup; defaults to "." if empty.
	Workspace string

	OnRun func(name string, args []string) error
}

// ctx returns a context.Context for background goroutines. Defaults to
// context.Background(); tests can override via SetContextFn.
func (m *Model) ctx() context.Context {
	if m.ctxFn != nil {
		return m.ctxFn()
	}
	return context.Background()
}

// SetContextFn lets tests inject a cancellable context.
func (m *Model) SetContextFn(fn func() context.Context) {
	m.ctxFn = fn
}

func NewModel() *Model {
	s := NewStyles(Themes[0])
	footer := NewFooter(80)
	ti := textinput.New()
	ti.Placeholder = "args..."
	ti.CharLimit = 256
	ti.SetWidth(50)

	items := makeItemsForTools()
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color(Themes[0].Background)).
		Background(lipgloss.Color(Themes[0].Accent))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(lipgloss.Color(Themes[0].Background)).
		Background(lipgloss.Color(Themes[0].Accent))
	l := list.New(items, delegate, 0, 0)
	l.Title = "Tools"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)

	m := &Model{
		Width:      80,
		Height:     24,
		ThemeIdx:   0,
		ViewKind:   ViewTools,
		Mode:       ModeNormal,
		Tabs:       NewTabs(),
		Sidebar:    NewSidebar(),
		Footer:     footer,
		Spinner:    NewSpinner(),
		Styles:     s,
		RightPanel: true,
		History:    []HistoryEntry{},
		EFMStks:    []EFMStack{},
		Config:     DefaultConfigEntries(),
		ToolList:   l,
		ToolItems:  items,
		ArgInput:   ArgInputState{Input: ti},
		Palette:    PaletteState{Open: false, Sel: 0, Items: defaultPaletteCommands(), Filter: defaultPaletteCommands()},
	}
	m.ApplyTheme()
	return m
}

func defaultPaletteCommands() []string {
	return []string{
		"discover", "execute", "map", "grasp", "scout", "harvest",
		"orchestrate", "ibd", "poc", "sckg", "adw", "oracle",
		"efm", "serve", "security", "sbom", "config", "self-update",
		"todo add", "todo list", "todo ready", "todo complete",
		"theme: next", "agent: cycle", "view: tools", "view: sessions",
		"view: efm", "view: config", "view: history", "view: todos",
		"sidebar: toggle", "quit",
	}
}

func makeItemsForTools() []list.Item {
	subs := DefaultToolSubItems()
	out := make([]list.Item, 0, len(subs))
	for _, s := range subs {
		out = append(out, listItem{
			name:        s.Name,
			description: s.Description,
			runnable:    s.Runnable,
		})
	}
	return out
}

type listItem struct {
	name        string
	description string
	runnable    bool
}

func (l listItem) Title() string       { return l.name }
func (l listItem) Description() string { return l.description }
func (l listItem) FilterValue() string { return l.name + " " + l.description }
