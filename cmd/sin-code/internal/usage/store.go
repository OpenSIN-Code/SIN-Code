// SPDX-License-Identifier: MIT
// Purpose: persist and aggregate LLM token usage (issue #168). One row per
// chat completion with prompt/completion/total counts, model, source, and
// session. Aggregations: per session, per day, per lifetime, per model.
// SQLite-backed, CGo-free (modernc.org/sqlite) — preserves M2 (single static
// binary).
// Docs: usage.doc.md
package usage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Source categorises which agent subsystem emitted the LLM call. Fixed set,
// included in every Event so `tokens aggregate --by source` does the right
// thing. Stored as TEXT for forward-compatibility.
type Source string

const (
	SourceChat    Source = "chat"    // primary chat completion in agentloop
	SourceVerify  Source = "verify"  // PoC / Oracle verify runs (none yet)
	SourceJudge   Source = "judge"   // stop-gate LLM judge
	SourceSummary Source = "summary" // summary builder fallback (none yet)
	SourcePlan    Source = "plan"    // orchestrator plan stage
	SourceAdHoc   Source = "adhoc"   // catch-all: spec author, adapters, tui chat, etc.
)

// Event is one LLM completion call. Prompt/Completion/Total tokens come
// directly from the upstream ChatResponse.Usage struct (parsed but dropped
// before this work — see cmd/sin-code/internal/llm/provider.go:42-46 prior
// to issue #168).
type Event struct {
	SessionID    string    `json:"session_id"`
	Model        string    `json:"model"`
	Source       Source    `json:"source"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	TotalTokens  int       `json:"total_tokens"`
	CostUSD      float64   `json:"cost_usd,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	HasUsage     bool      `json:"has_usage"` // some providers omit usage; keep row but flag
}

// Store is the SQLite-backed event store. Safe for concurrent use when Record
// is the only writer; reads tolerate late-arriving writes via the shared
// *sql.DB.
type Store struct {
	db *sql.DB
	mu sync.Mutex // guards Record ID allocation + Cost calculation

	// pricingPer1K maps "model" → USD per 1000 tokens. Optional; if empty
	// the built-in DefaultPricing() is consulted, falling through to 0.
	pricingPer1K map[string]float64
}

// Package-level hooks for error paths that are otherwise impossible to
// trigger in a portable test (no real filesystem errors, missing driver, etc.).
var (
	userHomeDir = os.UserHomeDir
	mkdirAll    = os.MkdirAll
	sqlOpen     = sql.Open
	migrateExec = func(db *sql.DB, schema string) error { _, err := db.Exec(schema); return err }
	queryRows   = func(ctx context.Context, db *sql.DB, query string, args ...any) (*sql.Rows, error) {
		return db.QueryContext(ctx, query, args...)
	}
	rowsClose      = func(r *sql.Rows) error { return r.Close() }
	aggregateQuery = func(ctx context.Context, db *sql.DB, query string, args ...any) (*sql.Rows, error) {
		return db.QueryContext(ctx, query, args...)
	}
)

// DefaultPath returns the canonical on-disk location:
//
//	$XDG_DATA_HOME/sin-code/tokens.db  (else ~/.local/share/sin-code/tokens.db)
//
// Override per-process with SIN_CODE_TOKENS_DB.
//
// See AGENTS.md §7 (Configuration and DB locations) — issue #168 deliberately
// uses os.UserConfigDir() / XDG_DATA_HOME (NOT a gitignored subdir like
// cmd/sin-code/tui/.sin-code) so the file does not get committed by accident.
func DefaultPath() string {
	if env := os.Getenv("SIN_CODE_TOKENS_DB"); env != "" {
		return env
	}
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, _ := userHomeDir()
		if home == "" {
			return "tokens.db"
		}
		dir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dir, "sin-code", "tokens.db")
}

// Open opens or creates the usage store at path. Parent directories are
// created with 0o755. Path ""→DefaultPath().
func Open(path string) (*Store, error) {
	if path == "" {
		path = DefaultPath()
	}
	if err := mkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("usage: mkdir: %w", err)
	}
	db, err := sqlOpen("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // modernc/sqlite is single-writer; cheaper than per-call pooling
	s := &Store{
		db:           db,
		pricingPer1K: DefaultPricing(),
	}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// OpenWithPricing opens the store AND overrides the per-model pricing
// (USD per 1k tokens) with the supplied map. Used by `sin-code tokens` to
// surface user-overrides from `~/.config/sin/sin-code.toml`
// (`llm.pricing_per_1k`).
func OpenWithPricing(path string, per1K map[string]float64) (*Store, error) {
	s, err := Open(path)
	if err != nil {
		return nil, err
	}
	if per1K != nil {
		s.mu.Lock()
		for k, v := range per1K {
			s.pricingPer1K[k] = v
		}
		s.mu.Unlock()
	}
	return s, nil
}

// Close releases the underlying DB handle.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS usage_events (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'adhoc',
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    cost_usd REAL NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_usage_session ON usage_events(session_id);
CREATE INDEX IF NOT EXISTS idx_usage_model ON usage_events(model);
CREATE INDEX IF NOT EXISTS idx_usage_source ON usage_events(source);
CREATE INDEX IF NOT EXISTS idx_usage_created ON usage_events(created_at);
PRAGMA user_version = 1;
`
	return migrateExec(s.db, schema)
}

// Record persists one event. SessionID/Model/Source default to safe
// sentinels rather than empty so the row remains queryable. The Cost field
// on the event is ignored — cost is always derived from per-1k pricing at
// write time so historical rows stay consistent with the user's current
// override map.
func (s *Store) Record(ctx context.Context, e Event) error {
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	cost := s.computeCost(e)
	s.mu.Lock()
	id := newEventID(e)
	s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, `
INSERT INTO usage_events (id, session_id, model, source, input_tokens, output_tokens, total_tokens, cost_usd, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
`, id, e.SessionID, e.Model, string(e.Source), e.InputTokens, e.OutputTokens, e.TotalTokens, cost, e.CreatedAt.Format(time.RFC3339Nano))
	return err
}

// RecordFromChatUsage is a convenience that builds an Event from a typical
// chat-completion Usage struct and writes it. Pass sessionID = "" if the
// call is not session-scoped (e.g. spec author). HasUsage is set true when
// any of the token fields are > 0 (most NIM / OpenAI endpoints always
// populate usage; some free endpoints leave it empty — we record the row
// but flag it so the aggregator can decide).
func (s *Store) RecordFromChatUsage(ctx context.Context, sessionID, model string, src Source, input, output, total int) error {
	if input == 0 && output == 0 && total == 0 {
		return nil // skip empty usage rows so we do not pollute aggregates
	}
	return s.Record(ctx, Event{
		SessionID:    sessionID,
		Model:        model,
		Source:       src,
		InputTokens:  input,
		OutputTokens: output,
		TotalTokens:  total,
		HasUsage:     true,
	})
}

func newEventID(e Event) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s|%s|%s|%s|%d|%d|%d",
		e.CreatedAt.UTC().Format(time.RFC3339Nano),
		e.SessionID, e.Model, e.Source,
		e.InputTokens, e.OutputTokens, e.TotalTokens)
	h := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(h[:16])
}

// computeCost returns USD cost from per-1k pricing. Falls through to 0 when
// the model is unknown so the user is never blocked by missing pricing.
func (s *Store) computeCost(e Event) float64 {
	s.mu.Lock()
	rate, ok := s.pricingPer1K[e.Model]
	s.mu.Unlock()
	if !ok {
		// substring fallback: try to match the leaf (e.g. "llama-3.1-70b" matches
		// "nvidia/llama-3.1-70b-instruct"). Sorted longest-first to avoid best-fit.
		s.mu.Lock()
		keys := make([]string, 0, len(s.pricingPer1K))
		for k := range s.pricingPer1K {
			keys = append(keys, k)
		}
		s.mu.Unlock()
		sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
		for _, k := range keys {
			if strings.Contains(e.Model, k) {
				s.mu.Lock()
				rate = s.pricingPer1K[k]
				s.mu.Unlock()
				break
			}
		}
	}
	if rate == 0 {
		return 0
	}
	return float64(e.TotalTokens) * rate / 1000.0
}

// Aggregation is the rolled-up view returned by Aggregate. It can be filtered
// by session_id / today / month via a SQL WHERE clause built by Aggregate().
type Aggregation struct {
	Group         string         `json:"group"` // session_id, "today", "month:YYYY-MM", "model:X", etc.
	InputTokens   int            `json:"input_tokens"`
	OutputTokens  int            `json:"output_tokens"`
	TotalTokens   int            `json:"total_tokens"`
	CostUSD       float64        `json:"cost_usd"`
	EventCount    int            `json:"event_count"`
	ByModel       map[string]int `json:"by_model,omitempty"`  // model → total tokens
	BySource      map[string]int `json:"by_source,omitempty"` // source → total tokens
	FirstEvent    time.Time      `json:"first_event,omitempty"`
	LastEvent     time.Time      `json:"last_event,omitempty"`
	SessionsCount int            `json:"sessions_count,omitempty"`
}

// Filter narrows the aggregation. Zero values mean "no filter".
type Filter struct {
	SessionID string
	Since     time.Time // inclusive
	Until     time.Time // exclusive
	Source    Source
	Model     string
}

// Aggregate returns one Aggregation row plus a per-key map (sessions,
// days, or models depending on GroupBy).
func (s *Store) Aggregate(ctx context.Context, f Filter, groupBy string) (*Aggregation, []Aggregation, error) {
	if s == nil {
		return nil, nil, errors.New("usage: store is nil")
	}
	where, args := buildWhere(f)

	// Top-level roll-up.
	topSQL := `
SELECT
    COALESCE(SUM(input_tokens), 0),
    COALESCE(SUM(output_tokens), 0),
    COALESCE(SUM(total_tokens), 0),
    COALESCE(SUM(cost_usd), 0),
    COUNT(*),
    COALESCE(MIN(created_at), ''),
    COALESCE(MAX(created_at), ''),
    COUNT(DISTINCT session_id)
FROM usage_events
` + where
	row := s.db.QueryRowContext(ctx, topSQL, args...)
	top := &Aggregation{
		ByModel:  map[string]int{},
		BySource: map[string]int{},
	}
	var first, last string
	if err := row.Scan(&top.InputTokens, &top.OutputTokens, &top.TotalTokens,
		&top.CostUSD, &top.EventCount, &first, &last, &top.SessionsCount); err != nil {
		return nil, nil, err
	}
	if first != "" {
		t, _ := time.Parse(time.RFC3339Nano, first)
		top.FirstEvent = t
	}
	if last != "" {
		t, _ := time.Parse(time.RFC3339Nano, last)
		top.LastEvent = t
	}
	_ = scanBreakdowns(ctx, s.db, f, where, args, top)

	if groupBy == "" {
		return top, nil, nil
	}
	top.Group = groupBy

	// Sub-aggregations: group rows by an expression.
	var subSQL string
	switch groupBy {
	case "day":
		subSQL = `SELECT substr(created_at, 1, 10) AS g, SUM(input_tokens), SUM(output_tokens), SUM(total_tokens), SUM(cost_usd), COUNT(*), MIN(created_at), MAX(created_at) FROM usage_events ` + where + ` GROUP BY g ORDER BY g DESC`
	case "month":
		subSQL = `SELECT substr(created_at, 1, 7) AS g, SUM(input_tokens), SUM(output_tokens), SUM(total_tokens), SUM(cost_usd), COUNT(*), MIN(created_at), MAX(created_at) FROM usage_events ` + where + ` GROUP BY g ORDER BY g DESC`
	case "model":
		subSQL = `SELECT model AS g, SUM(input_tokens), SUM(output_tokens), SUM(total_tokens), SUM(cost_usd), COUNT(*), MIN(created_at), MAX(created_at) FROM usage_events ` + where + ` GROUP BY g ORDER BY SUM(total_tokens) DESC`
	case "source":
		subSQL = `SELECT source AS g, SUM(input_tokens), SUM(output_tokens), SUM(total_tokens), SUM(cost_usd), COUNT(*), MIN(created_at), MAX(created_at) FROM usage_events ` + where + ` GROUP BY g ORDER BY SUM(total_tokens) DESC`
	case "session":
		subSQL = `SELECT CASE WHEN session_id = '' THEN '(no-session)' ELSE session_id END AS g, SUM(input_tokens), SUM(output_tokens), SUM(total_tokens), SUM(cost_usd), COUNT(*), MIN(created_at), MAX(created_at) FROM usage_events ` + where + ` GROUP BY g ORDER BY MAX(created_at) DESC`
	default:
		return top, nil, fmt.Errorf("usage: unknown group_by %q (want day|month|model|source|session)", groupBy)
	}
	rows, err := aggregateQuery(ctx, s.db, subSQL, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var subs []Aggregation
	for rows.Next() {
		var g Aggregation
		var first, last string
		if err := rows.Scan(&g.Group, &g.InputTokens, &g.OutputTokens, &g.TotalTokens, &g.CostUSD, &g.EventCount, &first, &last); err != nil {
			return nil, nil, err
		}
		if first != "" {
			g.FirstEvent, _ = time.Parse(time.RFC3339Nano, first)
		}
		if last != "" {
			g.LastEvent, _ = time.Parse(time.RFC3339Nano, last)
		}
		subs = append(subs, g)
	}
	return top, subs, rows.Err()
}

func scanBreakdowns(ctx context.Context, db *sql.DB, f Filter, where string, args []any, top *Aggregation) error {
	rows, err := queryRows(ctx, db, `SELECT model, SUM(total_tokens) FROM usage_events `+where+` GROUP BY model`, args...)
	if err != nil {
		return err
	}
	for rows.Next() {
		var m string
		var t int
		if err := rows.Scan(&m, &t); err != nil {
			_ = rowsClose(rows)
			return err
		}
		top.ByModel[m] = t
	}
	if err := rowsClose(rows); err != nil {
		return err
	}

	rows, err = queryRows(ctx, db, `SELECT source, SUM(total_tokens) FROM usage_events `+where+` GROUP BY source`, args...)
	if err != nil {
		return err
	}
	defer rowsClose(rows)
	for rows.Next() {
		var src string
		var t int
		if err := rows.Scan(&src, &t); err != nil {
			return err
		}
		top.BySource[src] = t
	}
	return rows.Err()
}

func buildWhere(f Filter) (string, []any) {
	var clauses []string
	var args []any
	if f.SessionID != "" {
		clauses = append(clauses, "session_id = ?")
		args = append(args, f.SessionID)
	}
	if !f.Since.IsZero() {
		clauses = append(clauses, "created_at >= ?")
		args = append(args, f.Since.UTC().Format(time.RFC3339Nano))
	}
	if !f.Until.IsZero() {
		clauses = append(clauses, "created_at < ?")
		args = append(args, f.Until.UTC().Format(time.RFC3339Nano))
	}
	if f.Source != "" {
		clauses = append(clauses, "source = ?")
		args = append(args, string(f.Source))
	}
	if f.Model != "" {
		clauses = append(clauses, "model = ?")
		args = append(args, f.Model)
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

// Tail returns the most recent N events matching f, newest first.
func (s *Store) Tail(ctx context.Context, f Filter, n int) ([]Event, error) {
	if n <= 0 {
		n = 20
	}
	where, args := buildWhere(f)
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, model, source, input_tokens, output_tokens, total_tokens, cost_usd, created_at
FROM usage_events `+where+` ORDER BY created_at DESC, id DESC LIMIT ?`,
		append(args, n)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var id, created string
		var src string
		if err := rows.Scan(&id, &e.SessionID, &e.Model, &src, &e.InputTokens, &e.OutputTokens, &e.TotalTokens, &e.CostUSD, &created); err != nil {
			return nil, err
		}
		e.Source = Source(src)
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		e.HasUsage = e.TotalTokens > 0
		out = append(out, e)
	}
	return out, rows.Err()
}

// Count returns the number of stored events matching f. Cheap (uses
// table-wide COUNT — no large scan).
func (s *Store) Count(ctx context.Context, f Filter) (int, error) {
	where, args := buildWhere(f)
	row := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM usage_events `+where, args...)
	var n int
	err := row.Scan(&n)
	return n, err
}

// SetPricing overrides per-1k pricing at runtime. Used by `sin-code tokens
// show --cost` and the daemon when it reloads config.
func (s *Store) SetPricing(per1K map[string]float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pricingPer1K = make(map[string]float64, len(per1K))
	for k, v := range per1K {
		s.pricingPer1K[k] = v
	}
}

// Pricing returns a snapshot of the current per-1k pricing map.
func (s *Store) Pricing() map[string]float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]float64, len(s.pricingPer1K))
	for k, v := range s.pricingPer1K {
		out[k] = v
	}
	return out
}

// DefaultPricing is the built-in price table (USD per 1k tokens, combined
// input + output). As of 2026-06-16 — check provider pages for current
// prices. Override per-model via `llm.pricing_per_1k` in the user config.
//
// Values are deliberately compact: a single rate per model stands in for the
// input+output difference. SIN-Code is a tool, not a billing system; users
// who bill on asymmetry should supply their own map.
func DefaultPricing() map[string]float64 {
	return map[string]float64{
		// NIM (NVIDIA) — common-open catalogue, ~2026-06 pricing.
		"meta/llama-3.3-70b-instruct":          0.0009,
		"meta/llama-3.1-70b-instruct":          0.0009,
		"meta/llama-3.1-8b-instruct":           0.0002,
		"nvidia/llama-3.1-nemotron-nano-8b-v1": 0.0002,
		"qwen/qwen3-coder-480b-a35b-instruct":  0.0010,
		"openai/gpt-oss-120b":                  0.0008,
		"moonshotai/kimi-k2.6":                 0.0012,
		"nvidia/nemotron-3-nano-30b-a3b":       0.0004,
		// Anthropic — Claude 4.x and 3.x.
		"claude-opus-4":               0.0150,
		"claude-sonnet-4":             0.0030,
		"claude-haiku-4":              0.0008,
		"claude-3-opus":               0.0150,
		"claude-3-7-sonnet":           0.0030,
		"claude-3-5-sonnet":           0.0030,
		"claude-3-5-haiku":            0.0008,
		"anthropic/claude-sonnet-4-5": 0.0030,
		"anthropic/claude-haiku-4-5":  0.0008,
		"anthropic/claude-opus-4-5":   0.0150,
		// OpenAI — GPT-4o family.
		"gpt-4o":       0.0050,
		"gpt-4o-mini":  0.0002,
		"gpt-4.1":      0.0050,
		"gpt-4.1-mini": 0.0004,
		"gpt-4-turbo":  0.0100,
		"o1":           0.0150,
		"o1-mini":      0.0030,
		// Fireworks AI — common alias.
		"fireworks/llama-3.3-70b":              0.0009,
		"fireworks/llama-3.1-70b":              0.0009,
		"fireworks/deepseek-v3":                0.0014,
		"accounts/fireworks/models/minimax-m3": 0.0010, // developer opencode default
		// Generic catch-all groups (submatched by computeCost).
		"llama-3.3-70b": 0.0009,
		"llama-3.1-70b": 0.0009,
		"llama-3.1-8b":  0.0002,
		"nemotron":      0.0004,
		"gpt-oss":       0.0008,
		"kimi":          0.0012,
		"minimax":       0.0010,
	}
}
