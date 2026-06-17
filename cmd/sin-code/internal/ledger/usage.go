// SPDX-License-Identifier: MIT
// Purpose: Tool usage aggregation for the semantic session ledger. Records every
// local tool call, error, and permission denial with family/outcome metadata
// so the CLI can report heatmaps, coverage, and unused-tool gaps.
// Docs: ledger.doc.md
package ledger

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// UsageOutcome is the result of a tool invocation.
type UsageOutcome string

const (
	OutcomeOK     UsageOutcome = "ok"
	OutcomeError  UsageOutcome = "error"
	OutcomeDenied UsageOutcome = "denied"
)

// UsageRecord is one row in the tool_usage table.
type UsageRecord struct {
	ToolName  string
	Family    string
	Outcome   UsageOutcome
	SessionID string
	GoalID    string
	CreatedAt time.Time
}

// UsageCount is a heatmap cell: total calls for one tool, optionally broken
// down by outcome.
type UsageCount struct {
	ToolName  string
	Family    string
	Total     int64
	ByOutcome map[UsageOutcome]int64
}

// FamilyCount is a heatmap cell grouped by tool family.
type FamilyCount struct {
	Family    string
	Total     int64
	ByOutcome map[UsageOutcome]int64
}

// CoverageResult holds the coverage computation.
type CoverageResult struct {
	Coverage float64  `json:"coverage"`
	Used     []string `json:"used"`
	Unused   []string `json:"unused"`
	Total    int      `json:"total"`
}

// usageMu protects the write path so concurrent callers can RecordUsage safely.
// The underlying SQLite connection is already limited to one open connection,
// but the mutex makes the critical section explicit and race-clean (M7).
var usageMu sync.Mutex

// RecordUsage appends a single tool usage event.
func (s *Store) RecordUsage(ctx context.Context, r UsageRecord) error {
	if r.ToolName == "" || r.SessionID == "" {
		return fmt.Errorf("tool usage requires tool_name and session_id")
	}
	if r.Family == "" {
		if strings.Contains(r.ToolName, "__") {
			if server, _, ok := strings.Cut(r.ToolName, "__"); ok {
				r.Family = server
			}
		} else if strings.HasPrefix(r.ToolName, "sin_") {
			r.Family = "sin_core"
		} else {
			r.Family = "unknown"
		}
	}
	if r.Outcome == "" {
		r.Outcome = OutcomeOK
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	usageMu.Lock()
	defer usageMu.Unlock()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tool_usage (tool_name, tool_family, outcome, session_id, goal_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, r.ToolName, r.Family, string(r.Outcome), r.SessionID, r.GoalID, r.CreatedAt.Format(time.RFC3339Nano))
	return err
}

// ToolUsageCounts returns a heatmap of total calls per tool, optionally filtered
// by time range. The result is sorted by tool name for stable output.
func (s *Store) ToolUsageCounts(ctx context.Context, since, until time.Time) ([]UsageCount, error) {
	query := `
		SELECT tool_name, tool_family, outcome, COUNT(1)
		FROM tool_usage
		WHERE 1=1
	`
	var args []any
	if !since.IsZero() {
		query += " AND created_at >= ?"
		args = append(args, since.Format(time.RFC3339Nano))
	}
	if !until.IsZero() {
		query += " AND created_at <= ?"
		args = append(args, until.Format(time.RFC3339Nano))
	}
	query += " GROUP BY tool_name, tool_family, outcome"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byTool := map[string]*UsageCount{}
	for rows.Next() {
		var name, family, outcome string
		var n int64
		if err := rows.Scan(&name, &family, &outcome, &n); err != nil {
			return nil, err
		}
		uc, ok := byTool[name]
		if !ok {
			uc = &UsageCount{ToolName: name, Family: family, ByOutcome: map[UsageOutcome]int64{}}
			byTool[name] = uc
		}
		uc.Total += n
		uc.ByOutcome[UsageOutcome(outcome)] += n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]UsageCount, 0, len(byTool))
	for _, uc := range byTool {
		out = append(out, *uc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ToolName < out[j].ToolName })
	return out, nil
}

// FamilyUsageCounts returns a heatmap grouped by tool family.
func (s *Store) FamilyUsageCounts(ctx context.Context, since, until time.Time) ([]FamilyCount, error) {
	query := `
		SELECT tool_family, outcome, COUNT(1)
		FROM tool_usage
		WHERE 1=1
	`
	var args []any
	if !since.IsZero() {
		query += " AND created_at >= ?"
		args = append(args, since.Format(time.RFC3339Nano))
	}
	if !until.IsZero() {
		query += " AND created_at <= ?"
		args = append(args, until.Format(time.RFC3339Nano))
	}
	query += " GROUP BY tool_family, outcome"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byFamily := map[string]*FamilyCount{}
	for rows.Next() {
		var family, outcome string
		var n int64
		if err := rows.Scan(&family, &outcome, &n); err != nil {
			return nil, err
		}
		fc, ok := byFamily[family]
		if !ok {
			fc = &FamilyCount{Family: family, ByOutcome: map[UsageOutcome]int64{}}
			byFamily[family] = fc
		}
		fc.Total += n
		fc.ByOutcome[UsageOutcome(outcome)] += n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]FamilyCount, 0, len(byFamily))
	for _, fc := range byFamily {
		out = append(out, *fc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Family < out[j].Family })
	return out, nil
}

// ToolCoverage computes unique_tools_used / len(knownTools). Returns sorted
// used and unused slices for stable reporting.
func (s *Store) ToolCoverage(ctx context.Context, knownTools []string) (*CoverageResult, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT tool_name FROM tool_usage
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	usedSet := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		usedSet[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	used := make([]string, 0, len(usedSet))
	unused := make([]string, 0, len(knownTools))
	for name := range usedSet {
		used = append(used, name)
	}
	sort.Strings(used)

	knownSet := map[string]bool{}
	for _, t := range knownTools {
		knownSet[t] = true
	}
	for _, t := range knownTools {
		if !usedSet[t] {
			unused = append(unused, t)
		}
	}
	sort.Strings(unused)

	coverage := 0.0
	if len(knownSet) > 0 {
		coverage = float64(len(usedSet)) / float64(len(knownSet))
	}
	return &CoverageResult{
		Coverage: coverage,
		Used:     used,
		Unused:   unused,
		Total:    len(knownSet),
	}, nil
}

// UnusedTools returns the tools in knownTools that have never been recorded.
func (s *Store) UnusedTools(ctx context.Context, knownTools []string) ([]string, error) {
	res, err := s.ToolCoverage(ctx, knownTools)
	if err != nil {
		return nil, err
	}
	return res.Unused, nil
}

// ToolUsageByPeriod buckets usage counts by day/week/month. The period argument
// must be "day", "week", or "month".
func (s *Store) ToolUsageByPeriod(ctx context.Context, period string, since, until time.Time) (map[string]int64, error) {
	var timeFmt string
	switch period {
	case "day":
		timeFmt = "%Y-%m-%d"
	case "week":
		timeFmt = "%Y-%W"
	case "month":
		timeFmt = "%Y-%m"
	default:
		return nil, fmt.Errorf("unsupported period %q (use day, week, month)", period)
	}
	query := fmt.Sprintf(`
		SELECT strftime(%q, created_at) AS bucket, COUNT(1)
		FROM tool_usage
		WHERE 1=1
	`, timeFmt)
	var args []any
	if !since.IsZero() {
		query += " AND created_at >= ?"
		args = append(args, since.Format(time.RFC3339Nano))
	}
	if !until.IsZero() {
		query += " AND created_at <= ?"
		args = append(args, until.Format(time.RFC3339Nano))
	}
	query += " GROUP BY bucket ORDER BY bucket"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]int64{}
	for rows.Next() {
		var bucket string
		var n int64
		if err := rows.Scan(&bucket, &n); err != nil {
			return nil, err
		}
		out[bucket] = n
	}
	return out, rows.Err()
}

// OutcomeTotals returns total counts by outcome.
func (s *Store) OutcomeTotals(ctx context.Context, since, until time.Time) (map[UsageOutcome]int64, error) {
	query := `
		SELECT outcome, COUNT(1) FROM tool_usage WHERE 1=1
	`
	var args []any
	if !since.IsZero() {
		query += " AND created_at >= ?"
		args = append(args, since.Format(time.RFC3339Nano))
	}
	if !until.IsZero() {
		query += " AND created_at <= ?"
		args = append(args, until.Format(time.RFC3339Nano))
	}
	query += " GROUP BY outcome"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[UsageOutcome]int64{}
	for rows.Next() {
		var o string
		var n int64
		if err := rows.Scan(&o, &n); err != nil {
			return nil, err
		}
		out[UsageOutcome(o)] = n
	}
	return out, rows.Err()
}
