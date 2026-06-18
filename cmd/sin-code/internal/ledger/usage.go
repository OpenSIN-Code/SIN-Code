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

// ── Tool Latency ───────────────────────────────────────────────────

// LatencyRecord is one latency measurement for a tool call.
type LatencyRecord struct {
	ToolName   string        `json:"tool_name"`
	DurationMs int64         `json:"duration_ms"`
	Outcome    UsageOutcome  `json:"outcome"`
	SessionID  string        `json:"session_id"`
	CreatedAt  time.Time     `json:"created_at"`
}

// AvgLatency is the average latency and count for a tool.
type AvgLatency struct {
	ToolName    string  `json:"tool_name"`
	AvgDuration float64 `json:"avg_duration_ms"`
	Count       int64   `json:"count"`
}

// ToolErrorRate holds success/error rate per tool.
type ToolErrorRate struct {
	ToolName   string  `json:"tool_name"`
	TotalCalls int64   `json:"total_calls"`
	Errors     int64   `json:"errors"`
	ErrorRate  float64 `json:"error_rate"`
}

// RecordToolLatency records a single tool latency measurement.
func (s *Store) RecordToolLatency(ctx context.Context, toolName string, durationMs int64, outcome UsageOutcome, sessionID string) error {
	if toolName == "" || sessionID == "" {
		return fmt.Errorf("tool latency requires tool_name and session_id")
	}
	if outcome == "" {
		outcome = OutcomeOK
	}
	usageMu.Lock()
	defer usageMu.Unlock()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tool_latency (tool_name, duration_ms, outcome, session_id, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, toolName, durationMs, string(outcome), sessionID, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

// ToolAvgLatencies returns average latency per tool, optionally filtered by time range.
func (s *Store) ToolAvgLatencies(ctx context.Context, since, until time.Time) ([]AvgLatency, error) {
	query := `
		SELECT tool_name, AVG(duration_ms), COUNT(1)
		FROM tool_latency
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
	query += " GROUP BY tool_name ORDER BY tool_name"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AvgLatency
	for rows.Next() {
		var al AvgLatency
		if err := rows.Scan(&al.ToolName, &al.AvgDuration, &al.Count); err != nil {
			return nil, err
		}
		out = append(out, al)
	}
	return out, rows.Err()
}

// ToolLatencyByPeriod buckets average latency by day/week/month.
func (s *Store) ToolLatencyByPeriod(ctx context.Context, period string, since, until time.Time) (map[string]float64, error) {
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
		SELECT strftime(%q, created_at) AS bucket, AVG(duration_ms)
		FROM tool_latency
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

	out := map[string]float64{}
	for rows.Next() {
		var bucket string
		var avg float64
		if err := rows.Scan(&bucket, &avg); err != nil {
			return nil, err
		}
		out[bucket] = avg
	}
	return out, rows.Err()
}

// ToolErrorRates computes error rates per tool.
func (s *Store) ToolErrorRates(ctx context.Context, since, until time.Time) ([]ToolErrorRate, error) {
	query := `
		SELECT tool_name,
			COUNT(1) AS total,
			SUM(CASE WHEN outcome = 'error' THEN 1 ELSE 0 END) AS errors
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
	query += " GROUP BY tool_name ORDER BY tool_name"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ToolErrorRate
	for rows.Next() {
		var ter ToolErrorRate
		if err := rows.Scan(&ter.ToolName, &ter.TotalCalls, &ter.Errors); err != nil {
			return nil, err
		}
		if ter.TotalCalls > 0 {
			ter.ErrorRate = float64(ter.Errors) / float64(ter.TotalCalls)
		}
		out = append(out, ter)
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
