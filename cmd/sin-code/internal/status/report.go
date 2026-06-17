// SPDX-License-Identifier: MIT
// Purpose: collect a deterministic readiness snapshot from every SIN-Code
// subsystem that exposes state: goals, todos, sessions, ledger, sin-debt,
// and installed skills. Missing or empty stores are reported as "No data yet"
// rather than failing the report.
package status

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/sindept"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/skillmgr"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/todo"
)

// maxItems limits the number of detail rows rendered per section.
const maxItems = 10

// Report is the unit rendered by RenderMarkdown / RenderJSON. Every field is
// populated best-effort; an Error string means the section is unavailable.
type Report struct {
	GeneratedAt time.Time `json:"generated_at"`
	Workspace   string    `json:"workspace"`

	Goals    GoalsSection    `json:"goals"`
	Todos    TodosSection    `json:"todos"`
	Sessions SessionsSection `json:"sessions"`
	Ledger   LedgerSection   `json:"ledger"`
	Debt     DebtSection     `json:"debt"`
	Skills   SkillsSection   `json:"skills"`
	Warnings []string        `json:"warnings"`
}

// GoalsSection holds a snapshot of the autonomous goal queue.
type GoalsSection struct {
	Total    int            `json:"total"`
	ByStatus map[string]int `json:"by_status"`
	Items    []GoalItem     `json:"items,omitempty"`
	Error    string         `json:"error,omitempty"`
	Empty    bool           `json:"empty"`
}

// GoalItem is a slim, stable view of autonomy.Goal.
type GoalItem struct {
	ID         int64  `json:"id"`
	Status     string `json:"status"`
	Priority   int    `json:"priority"`
	Workspace  string `json:"workspace"`
	Prompt     string `json:"prompt"`
	Attempts   int    `json:"attempts"`
	MaxRetries int    `json:"max_retries"`
}

// TodosSection holds a snapshot of the bbolt todo store.
type TodosSection struct {
	Total      int            `json:"total"`
	Open       int            `json:"open"`
	Ready      int            `json:"ready"`
	Blocked    int            `json:"blocked"`
	ByStatus   map[string]int `json:"by_status"`
	ByPriority map[string]int `json:"by_priority"`
	Items      []TodoItem     `json:"items,omitempty"`
	Error      string         `json:"error,omitempty"`
	Empty      bool           `json:"empty"`
}

// TodoItem is a slim, stable view of todo.Todo.
type TodoItem struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Status   string   `json:"status"`
	Priority string   `json:"priority"`
	Type     string   `json:"type"`
	Tags     []string `json:"tags,omitempty"`
	Assignee string   `json:"assignee,omitempty"`
}

// SessionsSection holds a snapshot of the SQLite session store.
type SessionsSection struct {
	Total int           `json:"total"`
	Items []SessionItem `json:"items,omitempty"`
	Error string        `json:"error,omitempty"`
	Empty bool          `json:"empty"`
}

// SessionItem is a slim, stable view of session.Info.
type SessionItem struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Title     string `json:"title"`
	ParentID  string `json:"parent_id,omitempty"`
}

// LedgerSection holds a snapshot of tool usage from the ledger.
type LedgerSection struct {
	DistinctSessions int               `json:"distinct_sessions"`
	ToolUsage        []ToolUsageItem   `json:"tool_usage,omitempty"`
	FamilyUsage      []FamilyUsageItem `json:"family_usage,omitempty"`
	Error            string            `json:"error,omitempty"`
	Empty            bool              `json:"empty"`
}

// ToolUsageItem is a stable rendering of ledger.UsageCount.
type ToolUsageItem struct {
	Name   string `json:"name"`
	Family string `json:"family"`
	Total  int64  `json:"total"`
	OK     int64  `json:"ok"`
	Error  int64  `json:"error"`
	Denied int64  `json:"denied"`
}

// FamilyUsageItem is a stable rendering of ledger.FamilyCount.
type FamilyUsageItem struct {
	Family string `json:"family"`
	Total  int64  `json:"total"`
	OK     int64  `json:"ok"`
	Error  int64  `json:"error"`
	Denied int64  `json:"denied"`
}

// DebtSection holds a snapshot of sin-debt markers.
type DebtSection struct {
	Total    int    `json:"total"`
	RotRisk  int    `json:"rot_risk"`
	ByReason []KV   `json:"by_reason,omitempty"`
	Error    string `json:"error,omitempty"`
	Empty    bool   `json:"empty"`
}

// KV is a sorted key+count pair.
type KV struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

// SkillsSection holds a snapshot of installed skills.
type SkillsSection struct {
	Total     int         `json:"total"`
	Installed int         `json:"installed"`
	Runnable  int         `json:"runnable"`
	Items     []SkillItem `json:"items,omitempty"`
	Error     string      `json:"error,omitempty"`
	Empty     bool        `json:"empty"`
}

// SkillItem is a slim, stable view of skillmgr.SkillStatus.
type SkillItem struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Runnable  bool   `json:"runnable"`
	Detail    string `json:"detail,omitempty"`
}

// Collect gathers data from every available subsystem. Errors are captured
// per section so the report can still be produced when one store is down.
func Collect(ctx context.Context, cfg Config) (*Report, error) {
	rep := &Report{
		GeneratedAt: time.Now().UTC(),
		Workspace:   cfg.Workspace,
	}
	if rep.Workspace == "" {
		rep.Workspace = "."
	}

	rep.Goals = collectGoals(ctx, cfg.Workspace)
	rep.Todos = collectTodos()
	rep.Sessions = collectSessions()
	rep.Ledger = collectLedger(ctx, cfg.Since)
	rep.Debt = collectDebt(cfg.Workspace)
	rep.Skills = collectSkills(ctx)
	rep.Warnings = buildWarnings(rep)
	return rep, nil
}

func collectGoals(ctx context.Context, workspace string) GoalsSection {
	q, err := autonomy.Open(autonomy.DefaultPath())
	if err != nil {
		return GoalsSection{Error: fmt.Sprintf("Goal store unavailable: %v", err)}
	}
	defer q.Close()

	goals, err := q.List(ctx, "")
	if err != nil {
		return GoalsSection{Error: fmt.Sprintf("Goal store unavailable: %v", err)}
	}

	if workspace != "" && workspace != "." {
		absWorkspace, werr := filepath.Abs(workspace)
		if werr == nil {
			filtered := goals[:0]
			for _, g := range goals {
				absGoal, gerr := filepath.Abs(g.Workspace)
				if gerr != nil {
					absGoal = g.Workspace
				}
				if absGoal == absWorkspace || strings.HasPrefix(absGoal, absWorkspace+string(filepath.Separator)) {
					filtered = append(filtered, g)
				}
			}
			goals = filtered
		}
	}

	section := GoalsSection{
		Total:    len(goals),
		ByStatus: map[string]int{},
		Empty:    len(goals) == 0,
	}
	for _, g := range goals {
		section.ByStatus[string(g.Status)]++
	}
	for _, g := range goals {
		if len(section.Items) >= maxItems {
			break
		}
		section.Items = append(section.Items, GoalItem{
			ID:         g.ID,
			Status:     string(g.Status),
			Priority:   g.Priority,
			Workspace:  g.Workspace,
			Prompt:     g.Prompt,
			Attempts:   g.Attempts,
			MaxRetries: g.MaxRetries,
		})
	}
	return section
}

func collectTodos() TodosSection {
	store, err := todo.Open("")
	if err != nil {
		return TodosSection{Error: fmt.Sprintf("Todo store unavailable: %v", err)}
	}
	defer store.Close()

	all, err := store.List()
	if err != nil {
		return TodosSection{Error: fmt.Sprintf("Todo store unavailable: %v", err)}
	}

	section := TodosSection{
		Total:      len(all),
		ByStatus:   map[string]int{},
		ByPriority: map[string]int{},
		Empty:      len(all) == 0,
	}
	for _, t := range all {
		section.ByStatus[string(t.Status)]++
		section.ByPriority[string(t.Priority)]++
		if t.IsOpen() {
			section.Open++
		}
		if t.Status == todo.StatusBlocked {
			section.Blocked++
		}
	}
	section.Ready = section.Open - section.Blocked

	for _, t := range all {
		if len(section.Items) >= maxItems {
			break
		}
		section.Items = append(section.Items, TodoItem{
			ID:       t.ID,
			Title:    t.Title,
			Status:   string(t.Status),
			Priority: string(t.Priority),
			Type:     string(t.Type),
			Tags:     sortedCopy(t.Tags),
			Assignee: t.Assignee,
		})
	}
	return section
}

func collectSessions() SessionsSection {
	store, err := session.Open(session.DefaultPath())
	if err != nil {
		return SessionsSection{Error: fmt.Sprintf("Session store unavailable: %v", err)}
	}
	defer store.Close()

	infos, err := store.List()
	if err != nil {
		return SessionsSection{Error: fmt.Sprintf("Session store unavailable: %v", err)}
	}

	section := SessionsSection{
		Total: len(infos),
		Empty: len(infos) == 0,
	}
	for _, i := range infos {
		if len(section.Items) >= maxItems {
			break
		}
		section.Items = append(section.Items, SessionItem{
			ID:        i.ID,
			CreatedAt: i.CreatedAt,
			UpdatedAt: i.UpdatedAt,
			Title:     i.Title,
			ParentID:  i.ParentID,
		})
	}
	return section
}

func collectLedger(ctx context.Context, since time.Time) LedgerSection {
	path := ledger.DefaultPath()
	if env := os.Getenv("SIN_CODE_LEDGER"); env != "" {
		path = env
	}
	store, err := ledger.Open(path)
	if err != nil {
		return LedgerSection{Error: fmt.Sprintf("Ledger store unavailable: %v", err)}
	}
	defer store.Close()

	var section LedgerSection
	sessions, err := store.DistinctSessions(ctx, 0)
	if err != nil {
		section.Error = fmt.Sprintf("Ledger store unavailable: %v", err)
		return section
	}
	section.DistinctSessions = len(sessions)

	counts, err := store.ToolUsageCounts(ctx, since, time.Time{})
	if err != nil {
		section.Error = fmt.Sprintf("Ledger tool usage unavailable: %v", err)
		return section
	}
	for _, c := range counts {
		section.ToolUsage = append(section.ToolUsage, ToolUsageItem{
			Name:   c.ToolName,
			Family: c.Family,
			Total:  c.Total,
			OK:     c.ByOutcome[ledger.OutcomeOK],
			Error:  c.ByOutcome[ledger.OutcomeError],
			Denied: c.ByOutcome[ledger.OutcomeDenied],
		})
	}
	section.ToolUsage = sortedToolUsage(section.ToolUsage)

	families, err := store.FamilyUsageCounts(ctx, since, time.Time{})
	if err != nil {
		section.Error = fmt.Sprintf("Ledger family usage unavailable: %v", err)
		return section
	}
	for _, f := range families {
		section.FamilyUsage = append(section.FamilyUsage, FamilyUsageItem{
			Family: f.Family,
			Total:  f.Total,
			OK:     f.ByOutcome[ledger.OutcomeOK],
			Error:  f.ByOutcome[ledger.OutcomeError],
			Denied: f.ByOutcome[ledger.OutcomeDenied],
		})
	}
	section.FamilyUsage = sortedFamilyUsage(section.FamilyUsage)
	section.Empty = section.DistinctSessions == 0 && len(section.ToolUsage) == 0
	return section
}

func collectDebt(workspace string) DebtSection {
	root, err := filepath.Abs(workspace)
	if err != nil {
		return DebtSection{Error: fmt.Sprintf("Debt scan unavailable: %v", err)}
	}
	markers, err := sindept.ParseDir(root, sindept.DefaultOptions())
	if err != nil {
		return DebtSection{Error: fmt.Sprintf("Debt scan unavailable: %v", err)}
	}

	stats := sindept.AggregateStats(markers)
	section := DebtSection{
		Total:   stats.Total,
		RotRisk: stats.WithoutUpgrade,
		Empty:   stats.Total == 0,
	}
	for _, kv := range stats.ByReason {
		section.ByReason = append(section.ByReason, KV{Key: kv.Key, Count: kv.Count})
	}
	return section
}

func collectSkills(ctx context.Context) SkillsSection {
	items := skillmgr.Status(ctx)
	section := SkillsSection{
		Total: len(items),
		Empty: len(items) == 0,
	}
	for _, s := range items {
		if s.Installed {
			section.Installed++
		}
		if s.Runnable {
			section.Runnable++
		}
		section.Items = append(section.Items, SkillItem{
			Name:      s.Name,
			Installed: s.Installed,
			Runnable:  s.Runnable,
			Detail:    s.Detail,
		})
	}
	section.Items = sortedSkills(section.Items)
	return section
}

func buildWarnings(r *Report) []string {
	var warnings []string
	if r.Goals.Total > 0 {
		pending := r.Goals.ByStatus["pending"] + r.Goals.ByStatus["running"]
		if pending > 0 {
			warnings = append(warnings, fmt.Sprintf("%d goal(s) pending or running", pending))
		}
	}
	if r.Todos.Open > 0 {
		warnings = append(warnings, fmt.Sprintf("%d open todo(s)", r.Todos.Open))
	}
	if r.Debt.RotRisk > 0 {
		warnings = append(warnings, fmt.Sprintf("%d rot-risk debt marker(s) without upgrade clause", r.Debt.RotRisk))
	}
	if r.Skills.Total > 0 && r.Skills.Runnable < r.Skills.Total {
		warnings = append(warnings, fmt.Sprintf("%d/%d skills not runnable", r.Skills.Total-r.Skills.Runnable, r.Skills.Total))
	}
	if r.Goals.Error != "" {
		warnings = append(warnings, "goals: "+r.Goals.Error)
	}
	if r.Todos.Error != "" {
		warnings = append(warnings, "todos: "+r.Todos.Error)
	}
	if r.Sessions.Error != "" {
		warnings = append(warnings, "sessions: "+r.Sessions.Error)
	}
	if r.Ledger.Error != "" {
		warnings = append(warnings, "ledger: "+r.Ledger.Error)
	}
	if r.Debt.Error != "" {
		warnings = append(warnings, "debt: "+r.Debt.Error)
	}
	if r.Skills.Error != "" {
		warnings = append(warnings, "skills: "+r.Skills.Error)
	}
	sort.Strings(warnings)
	return warnings
}

func sortedCopy(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	sort.Strings(out)
	return out
}

func sortedToolUsage(in []ToolUsageItem) []ToolUsageItem {
	out := make([]ToolUsageItem, len(in))
	copy(out, in)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func sortedFamilyUsage(in []FamilyUsageItem) []FamilyUsageItem {
	out := make([]FamilyUsageItem, len(in))
	copy(out, in)
	sort.Slice(out, func(i, j int) bool { return out[i].Family < out[j].Family })
	return out
}

func sortedSkills(in []SkillItem) []SkillItem {
	out := make([]SkillItem, len(in))
	copy(out, in)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
