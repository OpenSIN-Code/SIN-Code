// SPDX-License-Identifier: MIT
// Purpose: deterministic markdown and JSON rendering for the status report.
package status

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// RenderMarkdown renders a Report as a deterministic markdown document.
func RenderMarkdown(r *Report) string {
	var b strings.Builder

	b.WriteString("# SIN-Code Status Snapshot\n\n")
	b.WriteString(fmt.Sprintf("- **Generated:** %s\n", r.GeneratedAt.UTC().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- **Workspace:** %s\n", r.Workspace))
	b.WriteString("\n")

	renderReadinessTable(&b, r)
	renderGoalsSection(&b, r.Goals)
	renderTodosSection(&b, r.Todos)
	renderSessionsSection(&b, r.Sessions)
	renderLedgerSection(&b, r.Ledger)
	renderDebtSection(&b, r.Debt)
	renderSkillsSection(&b, r.Skills)
	renderWarningsSection(&b, r.Warnings)
	return b.String()
}

// RenderJSON renders a Report as indented JSON with sorted map keys.
func RenderJSON(r *Report) ([]byte, error) {
	out, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	out = append(out, '\n')
	return out, nil
}

func renderReadinessTable(b *strings.Builder, r *Report) {
	b.WriteString("## Readiness\n\n")
	b.WriteString("| Signal | Value |\n")
	b.WriteString("| --- | --- |\n")
	b.WriteString(fmt.Sprintf("| Goals | %s |\n", readinessGoals(r.Goals)))
	b.WriteString(fmt.Sprintf("| Todos | %s |\n", readinessTodos(r.Todos)))
	b.WriteString(fmt.Sprintf("| Sessions | %s |\n", readinessSessions(r.Sessions)))
	b.WriteString(fmt.Sprintf("| Ledger Sessions | %s |\n", readinessLedger(r.Ledger)))
	b.WriteString(fmt.Sprintf("| Debt | %s |\n", readinessDebt(r.Debt)))
	b.WriteString(fmt.Sprintf("| Skills | %s |\n", readinessSkills(r.Skills)))
	b.WriteString(fmt.Sprintf("| Warnings | %s |\n", readinessWarnings(r.Warnings)))
	b.WriteString("\n")
}

func readinessGoals(g GoalsSection) string {
	if g.Error != "" {
		return "unavailable"
	}
	if g.Empty {
		return "no data yet"
	}
	pending := g.ByStatus["pending"] + g.ByStatus["running"]
	return fmt.Sprintf("%d total (%d pending/running, %d verified, %d failed/exhausted)",
		g.Total, pending, g.ByStatus["verified"], g.ByStatus["failed"]+g.ByStatus["exhausted"])
}

func readinessTodos(t TodosSection) string {
	if t.Error != "" {
		return "unavailable"
	}
	if t.Empty {
		return "no data yet"
	}
	return fmt.Sprintf("%d total (%d open, %d blocked, %d closed)", t.Total, t.Open, t.Blocked, t.Total-t.Open)
}

func readinessSessions(s SessionsSection) string {
	if s.Error != "" {
		return "unavailable"
	}
	if s.Empty {
		return "no data yet"
	}
	return fmt.Sprintf("%d session(s)", s.Total)
}

func readinessLedger(l LedgerSection) string {
	if l.Error != "" {
		return "unavailable"
	}
	if l.Empty {
		return "no data yet"
	}
	return fmt.Sprintf("%d distinct session(s), %d tool call row(s)", l.DistinctSessions, len(l.ToolUsage))
}

func readinessDebt(d DebtSection) string {
	if d.Error != "" {
		return "unavailable"
	}
	if d.Empty {
		return "no data yet"
	}
	return fmt.Sprintf("%d marker(s), %d rot-risk", d.Total, d.RotRisk)
}

func readinessSkills(s SkillsSection) string {
	if s.Error != "" {
		return "unavailable"
	}
	if s.Empty {
		return "no data yet"
	}
	return fmt.Sprintf("%d known (%d installed, %d runnable)", s.Total, s.Installed, s.Runnable)
}

func readinessWarnings(warnings []string) string {
	if len(warnings) == 0 {
		return "none"
	}
	return fmt.Sprintf("%d", len(warnings))
}

func renderGoalsSection(b *strings.Builder, g GoalsSection) {
	b.WriteString("## Goals\n\n")
	if g.Error != "" {
		b.WriteString(fmt.Sprintf("No data yet — %s\n\n", g.Error))
		return
	}
	if g.Empty || g.Total == 0 {
		b.WriteString("No data yet.\n\n")
		return
	}
	b.WriteString(fmt.Sprintf("**Total:** %d\n\n", g.Total))
	renderMapTable(b, "Status", g.ByStatus)
	if len(g.Items) > 0 {
		b.WriteString("\n| ID | Status | Priority | Attempts | Prompt |\n")
		b.WriteString("| --- | --- | --- | --- | --- |\n")
		for _, it := range g.Items {
			b.WriteString(fmt.Sprintf("| %d | %s | %d | %d/%d | %s |\n",
				it.ID, it.Status, it.Priority, it.Attempts, it.MaxRetries, escapeMarkdown(it.Prompt)))
		}
	}
	b.WriteString("\n")
}

func renderTodosSection(b *strings.Builder, t TodosSection) {
	b.WriteString("## Todos\n\n")
	if t.Error != "" {
		b.WriteString(fmt.Sprintf("No data yet — %s\n\n", t.Error))
		return
	}
	if t.Empty || t.Total == 0 {
		b.WriteString("No data yet.\n\n")
		return
	}
	b.WriteString(fmt.Sprintf("**Total:** %d | **Open:** %d | **Blocked:** %d | **Ready:** %d\n\n", t.Total, t.Open, t.Blocked, t.Ready))
	renderMapTable(b, "Status", t.ByStatus)
	renderMapTable(b, "Priority", t.ByPriority)
	if len(t.Items) > 0 {
		b.WriteString("\n| ID | Title | Status | Priority | Type | Assignee |\n")
		b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
		for _, it := range t.Items {
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n",
				it.ID, escapeMarkdown(it.Title), it.Status, it.Priority, it.Type, coalesce(it.Assignee)))
		}
	}
	b.WriteString("\n")
}

func renderSessionsSection(b *strings.Builder, s SessionsSection) {
	b.WriteString("## Sessions\n\n")
	if s.Error != "" {
		b.WriteString(fmt.Sprintf("No data yet — %s\n\n", s.Error))
		return
	}
	if s.Empty {
		b.WriteString("No data yet.\n\n")
		return
	}
	b.WriteString(fmt.Sprintf("**Total:** %d\n\n", s.Total))
	if len(s.Items) > 0 {
		b.WriteString("| ID | Created | Updated | Title | Parent |\n")
		b.WriteString("| --- | --- | --- | --- | --- |\n")
		for _, it := range s.Items {
			parent := it.ParentID
			if parent == "" {
				parent = "-"
			}
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
				it.ID, it.CreatedAt, it.UpdatedAt, escapeMarkdown(it.Title), parent))
		}
	}
	b.WriteString("\n")
}

func renderLedgerSection(b *strings.Builder, l LedgerSection) {
	b.WriteString("## Ledger / Tool Usage\n\n")
	if l.Error != "" {
		b.WriteString(fmt.Sprintf("No data yet — %s\n\n", l.Error))
		return
	}
	if l.Empty || (l.DistinctSessions == 0 && len(l.ToolUsage) == 0) {
		b.WriteString("No data yet.\n\n")
		return
	}
	b.WriteString(fmt.Sprintf("**Distinct sessions:** %d\n\n", l.DistinctSessions))
	if len(l.ToolUsage) > 0 {
		b.WriteString("| Tool | Family | Total | OK | Error | Denied |\n")
		b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
		for _, it := range l.ToolUsage {
			b.WriteString(fmt.Sprintf("| %s | %s | %d | %d | %d | %d |\n",
				it.Name, it.Family, it.Total, it.OK, it.Error, it.Denied))
		}
		b.WriteString("\n")
	}
	if len(l.FamilyUsage) > 0 {
		b.WriteString("| Family | Total | OK | Error | Denied |\n")
		b.WriteString("| --- | --- | --- | --- | --- |\n")
		for _, it := range l.FamilyUsage {
			b.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %d |\n",
				it.Family, it.Total, it.OK, it.Error, it.Denied))
		}
		b.WriteString("\n")
	}
}

func renderDebtSection(b *strings.Builder, d DebtSection) {
	b.WriteString("## Debt\n\n")
	if d.Error != "" {
		b.WriteString(fmt.Sprintf("No data yet — %s\n\n", d.Error))
		return
	}
	if d.Empty {
		b.WriteString("No data yet.\n\n")
		return
	}
	b.WriteString(fmt.Sprintf("**Total markers:** %d | **Rot-risk:** %d\n\n", d.Total, d.RotRisk))
	if len(d.ByReason) > 0 {
		b.WriteString("| Reason | Count |\n")
		b.WriteString("| --- | --- |\n")
		for _, kv := range d.ByReason {
			b.WriteString(fmt.Sprintf("| %s | %d |\n", escapeMarkdown(kv.Key), kv.Count))
		}
		b.WriteString("\n")
	}
}

func renderSkillsSection(b *strings.Builder, s SkillsSection) {
	b.WriteString("## Skills\n\n")
	if s.Error != "" {
		b.WriteString(fmt.Sprintf("No data yet — %s\n\n", s.Error))
		return
	}
	if s.Empty {
		b.WriteString("No data yet.\n\n")
		return
	}
	b.WriteString(fmt.Sprintf("**Total:** %d | **Installed:** %d | **Runnable:** %d\n\n", s.Total, s.Installed, s.Runnable))
	if len(s.Items) > 0 {
		b.WriteString("| Skill | Installed | Runnable | Detail |\n")
		b.WriteString("| --- | --- | --- | --- |\n")
		for _, it := range s.Items {
			b.WriteString(fmt.Sprintf("| %s | %v | %v | %s |\n",
				it.Name, yesNo(it.Installed), yesNo(it.Runnable), escapeMarkdown(it.Detail)))
		}
	}
	b.WriteString("\n")
}

func renderWarningsSection(b *strings.Builder, warnings []string) {
	b.WriteString("## Warnings\n\n")
	if len(warnings) == 0 {
		b.WriteString("No warnings.\n\n")
		return
	}
	for _, w := range warnings {
		b.WriteString(fmt.Sprintf("- %s\n", w))
	}
	b.WriteString("\n")
}

func renderMapTable(b *strings.Builder, title string, m map[string]int) {
	if len(m) == 0 {
		return
	}
	keys := sortedKeys(m)
	b.WriteString(fmt.Sprintf("\n**%s**\n\n", title))
	b.WriteString("| Key | Count |\n")
	b.WriteString("| --- | --- |\n")
	for _, k := range keys {
		b.WriteString(fmt.Sprintf("| %s | %d |\n", escapeMarkdown(k), m[k]))
	}
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func escapeMarkdown(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	return strings.TrimSpace(s)
}

func coalesce(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
