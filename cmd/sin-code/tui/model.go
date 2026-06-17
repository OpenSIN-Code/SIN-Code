// SPDX-License-Identifier: MIT
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"

	agentrunner "github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/tui"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/tui/chat"
)

type Mode int

const (
	ModeNormal Mode = iota
	ModePalette
	ModeSubagents
	ModeArgInput
	ModeSessionSwitcher
	ModeModelSelector
	ModePermissionDialog
	ModeSearch
)

type PaletteState struct {
	Open   bool
	Query  string
	Items  []string
	Filter []string
	Sel    int
}

type SessionSwitcherState struct {
	Open       bool
	Query      string
	Sel        int
	Indices    []int
	Renaming   bool
	RenameInput textinput.Model
}

type ArgInputState struct {
	Open  bool
	Cmd   string
	Value string
	Input textinput.Model
}

type teaProgramIface interface {
	Send(msg any)
}

type LayoutState struct {
	Width      int
	Height     int
	Ready      bool
	RightPanel bool
}

type ThemeState struct {
	ThemeIdx int
	Styles   Styles
}

type ChatState struct {
	ChatInput     *chatInput
	ChatHistory   []ChatMessage
	ChatRunner    *chat.Runner
	ChatViewport  viewport.Model
	ChatFocusIdx  int
	SearchQuery   string
	SearchMatches []int
	SearchInput   textinput.Model
}

type AgentState struct {
	AgentRunner *agentrunner.AgentRunner
	pendingAsk  chan bool
}

type ToolState struct {
	ToolList  list.Model
	ToolItems []list.Item
}

type TodoState struct {
	TodoItems []TodoRow
	TodoSel   int
}

type NotificationState struct {
	NotificationBanner *NotificationItem
	Notifications      []NotificationItem
}

type EFMState struct {
	EFMStks []EFMStack
}

type ConfigState struct {
	Config    []ConfigEntry
	ConfigSel int
}

type HistoryState struct {
	History []HistoryEntry
}

type FilePreviewState struct {
	FilePreview     string
	FilePreviewPath string
	DiffPopupOpen   bool
}

type DAGTaskRow struct {
	ID             string
	Type           string
	Description    string
	Status         string
	Probability    float64
	PreWarmed      bool
	ExpectedOutput string
	DependsOn      []string
	AgentName      string
	TokensUsed     int
	Cost           float64
}

type DAGState struct {
	Tasks    []DAGTaskRow
	Selected int
	Prompt   string
}

type Model struct {
	ctxFn     func() context.Context
	Program   teaProgramIface
	Workspace string
	OnRun     func(name string, args []string) error

	LayoutState
	ThemeState

	ViewKind ViewKind
	Mode     Mode
	Quitting bool
	Loading  bool

	Tabs    Tabs
	Sidebar Sidebar
	Footer  Footer
	Spinner Spinner

	Palette          PaletteState
	SessionSwitcher  SessionSwitcherState
	ModelSelector    ModelSelectorState
	PermissionDialog PermissionDialogState
	ArgInput         ArgInputState

	ChatState
	AgentState
	ToolState
	TodoState
	NotificationState
	EFMState
	ConfigState
	HistoryState
	FilePreviewState
	DAGState

	// Verification gate panel
	VerifyPanel VerifyPanel

	// Tool call tree
	ToolTree *ToolCallTree

	// Session tree
	SessionTree *SessionTree
}

func (m *Model) ctx() context.Context {
	if m.ctxFn != nil {
		return m.ctxFn()
	}
	return context.Background()
}

func (m *Model) SetContextFn(fn func() context.Context) {
	m.ctxFn = fn
}

func (m *Model) appendChat(msg ChatMessage) {
	msg.Timestamp = time.Now()
	if msg.ID == 0 {
		msg.ID = msg.Timestamp.UnixNano()
	}
	m.ChatHistory = append(m.ChatHistory, msg)
	if len(m.ChatHistory) > 500 {
		m.ChatHistory = m.ChatHistory[len(m.ChatHistory)-500:]
	}
}

// chatHistoryPath returns the path for chat history persistence.
func chatHistoryPath() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".local", "share", "sin-code")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "tui-chat-history.json")
}

// saveChatHistory persists the chat history to disk as JSON.
func (m *Model) saveChatHistory() {
	if len(m.ChatHistory) == 0 {
		return
	}
	data, err := json.Marshal(m.ChatHistory)
	if err != nil {
		return
	}
	_ = os.WriteFile(chatHistoryPath(), data, 0o644)
}

// loadChatHistory loads persisted chat history from disk.
func (m *Model) loadChatHistory() {
	data, err := os.ReadFile(chatHistoryPath())
	if err != nil {
		return
	}
	var history []ChatMessage
	if err := json.Unmarshal(data, &history); err != nil {
		return
	}
	m.ChatHistory = history
}

func (m *Model) ShowFilePreview(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		m.FilePreview = fmt.Sprintf("Error reading %s: %v", path, err)
		m.FilePreviewPath = path
		return
	}
	content := string(data)
	if len(content) > 2000 {
		content = content[:2000] + "\n... (truncated)"
	}
	m.FilePreview = content
	m.FilePreviewPath = path
}

func (m *Model) ClearFilePreview() {
	m.FilePreview = ""
	m.FilePreviewPath = ""
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

	searchInput := textinput.New()
	searchInput.Placeholder = "Search chat..."
	searchInput.CharLimit = 200
	searchInput.SetWidth(60)

	m := &Model{
		LayoutState: LayoutState{
			Width:      80,
			Height:     24,
			RightPanel: true,
		},
		ThemeState: ThemeState{
			ThemeIdx: 0,
			Styles:   s,
		},
		ViewKind: ViewChat,
		Mode:     ModeNormal,
		Tabs:     NewTabs(),
		Sidebar:  NewSidebar(),
		Footer:   footer,
		Spinner:  NewSpinner(),
		Palette:  PaletteState{Open: false, Sel: 0, Items: defaultPaletteCommands(), Filter: defaultPaletteCommands()},
		ArgInput: ArgInputState{Input: ti},
		HistoryState: HistoryState{
			History: []HistoryEntry{},
		},
		EFMState: EFMState{
			EFMStks: []EFMStack{},
		},
		ConfigState: ConfigState{
			Config: DefaultConfigEntries(),
		},
		ToolState: ToolState{
			ToolList:  l,
			ToolItems: items,
		},
		ChatState: ChatState{
			ChatViewport: viewport.New(viewport.WithWidth(80), viewport.WithHeight(20)),
			SearchInput:  searchInput,
		},
	}
	m.Footer.SetView(ViewChat)
	m.Footer.ShowHints = false
	m.ApplyTheme()
	m.loadChatHistory()
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
