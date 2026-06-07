package todo

import "time"

type Status string

const (
	StatusOpen       Status = "open"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
	StatusCancelled  Status = "cancelled"
	StatusBlocked    Status = "blocked"
)

func (s Status) Valid() bool {
	switch s {
	case StatusOpen, StatusInProgress, StatusDone, StatusCancelled, StatusBlocked:
		return true
	}
	return false
}

type Priority string

const (
	PriorityP0 Priority = "P0"
	PriorityP1 Priority = "P1"
	PriorityP2 Priority = "P2"
	PriorityP3 Priority = "P3"
)

func (p Priority) Valid() bool {
	switch p {
	case PriorityP0, PriorityP1, PriorityP2, PriorityP3:
		return true
	}
	return false
}

func (p Priority) Rank() int {
	switch p {
	case PriorityP0:
		return 0
	case PriorityP1:
		return 1
	case PriorityP2:
		return 2
	case PriorityP3:
		return 3
	}
	return 99
}

type TodoType string

const (
	TypeTask     TodoType = "task"
	TypeBug      TodoType = "bug"
	TypeFeature  TodoType = "feature"
	TypeChore    TodoType = "chore"
	TypeEpic     TodoType = "epic"
	TypeQuestion TodoType = "question"
)

func (t TodoType) Valid() bool {
	switch t {
	case TypeTask, TypeBug, TypeFeature, TypeChore, TypeEpic, TypeQuestion:
		return true
	}
	return false
}

type DepType string

const (
	DepBlocks         DepType = "blocks"
	DepParentChild    DepType = "parent-child"
	DepRelated        DepType = "related"
	DepDiscoveredFrom DepType = "discovered-from"
	DepDuplicates     DepType = "duplicates"
	DepSupersedes     DepType = "supersedes"
)

func (d DepType) Valid() bool {
	switch d {
	case DepBlocks, DepParentChild, DepRelated, DepDiscoveredFrom, DepDuplicates, DepSupersedes:
		return true
	}
	return false
}

func (d DepType) IsBlocking() bool {
	return d == DepBlocks
}

type Dependency struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type DepType `json:"type"`
}

type Todo struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Status      Status    `json:"status"`
	Priority    Priority  `json:"priority"`
	Type        TodoType  `json:"type"`
	Tags        []string  `json:"tags,omitempty"`
	Assignee    string    `json:"assignee,omitempty"`
	Parent      string    `json:"parent,omitempty"`
	ExternalRef string    `json:"external_ref,omitempty"`
	Project     string    `json:"project,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
	DueAt       *time.Time `json:"due_at,omitempty"`
	Estimate    int       `json:"estimate_minutes,omitempty"`
	Notes       string    `json:"notes,omitempty"`
	Compacted   bool      `json:"compacted,omitempty"`
	Summary     string    `json:"summary,omitempty"`
}

func (t *Todo) IsOpen() bool {
	return t.Status == StatusOpen || t.Status == StatusInProgress || t.Status == StatusBlocked
}

func (t *Todo) IsClosed() bool {
	return t.Status == StatusDone || t.Status == StatusCancelled
}

type ListFilter struct {
	Status   Status
	Priority Priority
	Type     TodoType
	Tag      string
	Assignee string
	Project  string
	Search   string
}

type Stats struct {
	Total     int            `json:"total"`
	ByStatus  map[string]int `json:"by_status"`
	ByPriority map[string]int `json:"by_priority"`
	ByType    map[string]int `json:"by_type"`
	ByAssignee map[string]int `json:"by_assignee"`
	Ready     int            `json:"ready"`
	Blocked   int            `json:"blocked"`
}

type AuditEntry struct {
	ID        string    `json:"id"`
	TodoID    string    `json:"todo_id"`
	Timestamp time.Time `json:"timestamp"`
	Actor     string    `json:"actor"`
	Action    string    `json:"action"`
	From      string    `json:"from,omitempty"`
	To        string    `json:"to,omitempty"`
	Note      string    `json:"note,omitempty"`
}
