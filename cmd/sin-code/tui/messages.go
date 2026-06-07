package tui

import "time"

type ViewKind int

const (
	ViewTools ViewKind = iota
	ViewSessions
	ViewEFM
	ViewConfig
	ViewHistory
	ViewTodos
	ViewChat
)

func (v ViewKind) String() string {
	switch v {
	case ViewTools:
		return "Tools"
	case ViewSessions:
		return "Sessions"
	case ViewEFM:
		return "EFM"
	case ViewConfig:
		return "Config"
	case ViewHistory:
		return "History"
	case ViewTodos:
		return "Todos"
	case ViewChat:
		return "Chat"
	}
	return "Unknown"
}

func (v ViewKind) Short() string {
	switch v {
	case ViewTools:
		return "1·Tools"
	case ViewSessions:
		return "2·Sessions"
	case ViewEFM:
		return "3·EFM"
	case ViewConfig:
		return "4·Config"
	case ViewHistory:
		return "5·History"
	case ViewTodos:
		return "6·Todos"
	case ViewChat:
		return "7·Chat"
	}
	return "?·"
}

// NotificationMsg is fired when the TUI receives a new notification
// from the sin-code notifications broadcaster.
type NotificationMsg struct {
	N NotificationSource
}

// CountsMsg is fired when todo counts are recomputed (open/blocked/overdue).
type CountsMsg struct {
	Open    int
	Blocked int
	Overdue int
	Ready   int
}

// TodosLoadedMsg is fired when the todo list is reloaded.
type TodosLoadedMsg struct {
	Items []TodoRow
}

// TodoRow is a lightweight struct for displaying todos in the TUI.
type TodoRow struct {
	ID       string
	Title    string
	Priority string
	Status   string
	Type     string
	Assignee string
}

type EFMStack struct {
	Name      string
	Status    string
	URL       string
	CreatedAt time.Time
	TTL       int
}

type HistoryEntry struct {
	Time    time.Time
	View    string
	Action  string
	Detail  string
	Success bool
}

type SpinnerTickMsg time.Time

type ViewSwitchMsg struct {
	View ViewKind
}

type ThemeChangedMsg struct {
	Index int
}

type SidebarToggleMsg struct{}

type PaletteOpenMsg struct {
	Open bool
}

type SubagentsOpenMsg struct {
	Open bool
}

type InterruptMsg struct{}

type ToolRunMsg struct {
	Name string
	Args []string
}

type EFMRefreshMsg struct {
	Stacks []EFMStack
}

type HistoryAppendMsg struct {
	Entry HistoryEntry
}

type AgentCycleMsg struct {
	Index int
}

type SessionAddMsg struct {
	Name string
}

type SessionCloseMsg struct {
	Index int
}

type SessionSelectMsg struct {
	Index int
}
