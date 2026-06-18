// SPDX-License-Identifier: MIT
// Purpose: persistent goal queue for autonomous operation — goals survive
// restarts, carry priorities and retry budgets, and are leased atomically
// so multiple daemon workers never double-execute.
//
// Loop-engineering extensions:
//   - Definition-of-Done contracts persisted per goal (column `contract`).
//   - Recursive goal trees (`parent_id`, `depth`) so an agent can decompose a
//     goal into sub-goals that are drained depth-first; a parent finalizes
//     only once every child is verified (TryFinalize).
//   - Continuation budget (`continuations`) so a checkpointed long task is
//     re-enqueued without burning its retry budget, bounded by a max.
package autonomy

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type GoalStatus string

const (
	StatusPending   GoalStatus = "pending"
	StatusRunning   GoalStatus = "running"
	StatusVerified  GoalStatus = "verified"
	StatusFailed    GoalStatus = "failed"
	StatusExhausted GoalStatus = "exhausted"
	// StatusBlocked marks a parent goal whose own work is done but which is
	// waiting on unverified children before it can finalize.
	StatusBlocked GoalStatus = "blocked"
)

type Goal struct {
	ID            int64      `json:"id"`
	Prompt        string     `json:"prompt"`
	Workspace     string     `json:"workspace"`
	Priority      int        `json:"priority"`
	Status        GoalStatus `json:"status"`
	Attempts      int        `json:"attempts"`
	MaxRetries    int        `json:"max_retries"`
	SessionID     string     `json:"session_id,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
	Contract      string     `json:"contract,omitempty"`
	ParentID      int64      `json:"parent_id,omitempty"`
	Depth         int        `json:"depth,omitempty"`
	Continuations int        `json:"continuations,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type Queue struct {
	db *sql.DB
}

// selectColumns is the canonical column list shared by Lease/List/Get so the
// scan order never drifts from the query.
const selectColumns = `id, prompt, workspace, priority, status, attempts, max_retries,
  session_id, last_error, contract, parent_id, depth, continuations, created_at, updated_at`

func Open(path string) (*Queue, error) {
	db, err := _dbOpen("sqlite", path)
	if err != nil {
		return nil, err
	}
	// Enable WAL journal mode + a small busy-timeout so concurrent
	// readers (sub-goal poll loops) do not block each other or a
	// concurrent writer (lease / complete). WAL is the canonical
	// choice for embedded SQL with multiple in-process readers +
	// writers; modernc.org/sqlite honours it for cross-connection
	// reads-with-writer concurrency. (issue #385).
	if _, err := _dbExec(db, `PRAGMA journal_mode = WAL`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("autonomy: journal_mode=WAL: %w", err)
	}
	if _, err := _dbExec(db, `PRAGMA busy_timeout = 5000`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("autonomy: busy_timeout: %w", err)
	}
	schema := `
CREATE TABLE IF NOT EXISTS goals (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  prompt TEXT NOT NULL,
  workspace TEXT NOT NULL,
  priority INTEGER DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'pending',
  attempts INTEGER DEFAULT 0,
  max_retries INTEGER DEFAULT 3,
  session_id TEXT DEFAULT '',
  last_error TEXT DEFAULT '',
  lease_until TEXT DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_goals_status_priority ON goals(status, priority DESC);
`
	if _, err := _dbExec(db, schema); err != nil {
		return nil, err
	}
	q := &Queue{db: db}
	if err := q.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return q, nil
}

// migrate adds loop-engineering columns to pre-existing databases. Each ADD
// COLUMN is idempotent: SQLite errors with "duplicate column name" when the
// column already exists, which we tolerate so the migration is safe to run on
// every Open.
func (q *Queue) migrate() error {
	addCols := []string{
		`ALTER TABLE goals ADD COLUMN contract TEXT DEFAULT ''`,
		`ALTER TABLE goals ADD COLUMN parent_id INTEGER DEFAULT 0`,
		`ALTER TABLE goals ADD COLUMN depth INTEGER DEFAULT 0`,
		`ALTER TABLE goals ADD COLUMN continuations INTEGER DEFAULT 0`,
		`ALTER TABLE goals ADD COLUMN dedup_key TEXT DEFAULT ''`,
	}
	for _, stmt := range addCols {
		if _, err := _dbExec(q.db, stmt); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
				continue
			}
			return fmt.Errorf("autonomy migrate: %q: %w", stmt, err)
		}
	}
	_, _ = _dbExec(q.db, `CREATE INDEX IF NOT EXISTS idx_goals_parent ON goals(parent_id)`)
	_, _ = _dbExec(q.db, `CREATE INDEX IF NOT EXISTS idx_goals_dedup ON goals(dedup_key)`)
	return nil
}

// AddDiscovered enqueues a goal found by the autonomous backlog scanner,
// keyed by dedupKey. If an unfinished (non-terminal) goal with the same
// dedupKey already exists, it is NOT re-enqueued and (0, false) is returned —
// so a recurring scan does not pile up duplicates of the same TODO/issue.
func (q *Queue) AddDiscovered(ctx context.Context, prompt, workspace, dedupKey string, priority, maxRetries int, contract string) (int64, bool, error) {
	if dedupKey != "" {
		var n int
		err := q.db.QueryRowContext(ctx,
			`SELECT COUNT(1) FROM goals WHERE dedup_key = ? AND status NOT IN ('verified','failed','exhausted')`,
			dedupKey).Scan(&n)
		if err != nil {
			return 0, false, err
		}
		if n > 0 {
			return 0, false, nil
		}
	}
	now := _timeNow().UTC().Format(time.RFC3339)
	res, err := _dbExecContext(q.db, ctx, `
INSERT INTO goals (prompt, workspace, priority, max_retries, contract, dedup_key, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, prompt, workspace, priority, maxRetries, contract, dedupKey, now, now)
	if err != nil {
		return 0, false, err
	}
	id, err := res.LastInsertId()
	return id, true, err
}

func (q *Queue) Close() error { return q.db.Close() }

// Add enqueues a top-level goal. Returns its ID.
func (q *Queue) Add(ctx context.Context, prompt, workspace string, priority, maxRetries int) (int64, error) {
	return q.add(ctx, prompt, workspace, priority, maxRetries, "", 0, 0)
}

// AddWithContract enqueues a top-level goal carrying a Definition-of-Done
// contract (serialized JSON; empty means none).
func (q *Queue) AddWithContract(ctx context.Context, prompt, workspace string, priority, maxRetries int, contract string) (int64, error) {
	return q.add(ctx, prompt, workspace, priority, maxRetries, contract, 0, 0)
}

// AddSub enqueues a child goal under parentID. The child inherits the parent's
// workspace and runs at depth+1 with a higher effective priority so trees
// drain depth-first (children before their parent).
func (q *Queue) AddSub(ctx context.Context, parentID int64, prompt string, priority, maxRetries int, contract string) (int64, error) {
	parent, err := q.Get(ctx, parentID)
	if err != nil {
		return 0, err
	}
	if parent == nil {
		return 0, fmt.Errorf("autonomy: parent goal %d not found", parentID)
	}
	return q.add(ctx, prompt, parent.Workspace, priority, maxRetries, contract, parentID, parent.Depth+1)
}

func (q *Queue) add(ctx context.Context, prompt, workspace string, priority, maxRetries int, contract string, parentID int64, depth int) (int64, error) {
	now := _timeNow().UTC().Format(time.RFC3339)
	res, err := _dbExecContext(q.db, ctx, `
INSERT INTO goals (prompt, workspace, priority, max_retries, contract, parent_id, depth, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, prompt, workspace, priority, maxRetries, contract, parentID, depth, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Lease atomically claims the highest-priority leasable goal. Goal trees are
// drained depth-first: deeper goals are leased before shallower ones so a
// parent is never finalized before its children. A goal is leasable when it is
// pending (and has no unverified children) or a running lease has expired.
func (q *Queue) Lease(ctx context.Context, leaseDur time.Duration) (*Goal, error) {
	now := _timeNow().UTC()
	leaseUntil := now.Add(leaseDur).Format(time.RFC3339)
	nowStr := now.Format(time.RFC3339)

	tx, err := _dbBeginTx(q.db, ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var g Goal
	var created, updated string
	err = _txQueryRowContext(tx, ctx, `
SELECT `+selectColumns+`
FROM goals g
WHERE (
        (status = 'pending')
     OR (status = 'running' AND lease_until < ?)
      )
  AND NOT EXISTS (
        SELECT 1 FROM goals c
        WHERE c.parent_id = g.id
          AND c.status NOT IN ('verified')
      )
ORDER BY depth DESC, priority DESC, id ASC
LIMIT 1`, nowStr).Scan(scanArgs(&g, &created, &updated)...)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err := _txExecContext(tx, ctx, `
UPDATE goals SET status = 'running', attempts = attempts + 1, lease_until = ?, updated_at = ?
WHERE id = ?`, leaseUntil, nowStr, g.ID); err != nil {
		return nil, err
	}
	if err := _txCommit(tx); err != nil {
		return nil, err
	}
	g.Status = StatusRunning
	g.Attempts++
	g.CreatedAt, _ = time.Parse(time.RFC3339, created)
	g.UpdatedAt = now
	return &g, nil
}

// Get returns a single goal by ID, or (nil, nil) if it does not exist.
func (q *Queue) Get(ctx context.Context, id int64) (*Goal, error) {
	var g Goal
	var created, updated string
	err := q.db.QueryRowContext(ctx, `SELECT `+selectColumns+` FROM goals WHERE id = ?`, id).
		Scan(scanArgs(&g, &created, &updated)...)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	g.CreatedAt, _ = time.Parse(time.RFC3339, created)
	g.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &g, nil
}

// GetStatus returns just the goal's status, or an error if the ID does
// not resolve. Polled by synchronous sub-goal callers (`spawn_subgoal`,
// issue #385); a typed wrapper around Get so the polling loop does not
// materialise a full Goal on every tick.
func (q *Queue) GetStatus(ctx context.Context, id int64) (GoalStatus, error) {
	g, err := q.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if g == nil {
		return "", fmt.Errorf("autonomy: goal %d not found", id)
	}
	return g.Status, nil
}

// Children returns the direct children of parentID, oldest first.
func (q *Queue) Children(ctx context.Context, parentID int64) ([]Goal, error) {
	rows, err := _dbQueryContext(q.db, ctx, `SELECT `+selectColumns+` FROM goals WHERE parent_id = ? ORDER BY id ASC`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGoals(rows)
}

// Complete marks a goal verified. If the goal has unverified children it is
// instead marked blocked (its own work is done, but the tree is not).
func (q *Queue) Complete(ctx context.Context, id int64, sessionID string) error {
	pending, err := q.unverifiedChildCount(ctx, id)
	if err != nil {
		return err
	}
	if pending > 0 {
		return q.setStatus(ctx, id, StatusBlocked, sessionID, "")
	}
	if err := q.setStatus(ctx, id, StatusVerified, sessionID, ""); err != nil {
		return err
	}
	return q.bubbleUp(ctx, id)
}

// TryFinalize re-checks a (possibly blocked) goal: when all its children are
// verified it transitions to verified and recurses up the tree. Safe to call
// repeatedly; a no-op when children remain.
func (q *Queue) TryFinalize(ctx context.Context, id int64) error {
	g, err := q.Get(ctx, id)
	if err != nil || g == nil {
		return err
	}
	if g.Status == StatusVerified {
		return nil
	}
	pending, err := q.unverifiedChildCount(ctx, id)
	if err != nil {
		return err
	}
	if pending > 0 {
		return nil
	}
	// Only finalize goals whose own work already succeeded (blocked) — never
	// resurrect a failed/exhausted goal just because its children passed.
	if g.Status != StatusBlocked {
		return nil
	}
	if err := q.setStatus(ctx, id, StatusVerified, g.SessionID, ""); err != nil {
		return err
	}
	return q.bubbleUp(ctx, id)
}

// bubbleUp re-finalizes the parent (if any) after a child verifies.
func (q *Queue) bubbleUp(ctx context.Context, id int64) error {
	g, err := q.Get(ctx, id)
	if err != nil || g == nil || g.ParentID == 0 {
		return err
	}
	return q.TryFinalize(ctx, g.ParentID)
}

func (q *Queue) unverifiedChildCount(ctx context.Context, id int64) (int, error) {
	var n int
	err := q.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM goals WHERE parent_id = ? AND status NOT IN ('verified')`, id).Scan(&n)
	return n, err
}

// Fail records a failure; the goal returns to pending until the retry
// budget is spent, then becomes exhausted.
func (q *Queue) Fail(ctx context.Context, id int64, sessionID, errMsg string) error {
	now := _timeNow().UTC().Format(time.RFC3339)
	_, err := _dbExecContext(q.db, ctx, `
UPDATE goals SET
  status = CASE WHEN attempts >= max_retries THEN 'exhausted' ELSE 'pending' END,
  session_id = ?, last_error = ?, lease_until = '', updated_at = ?
WHERE id = ?`, sessionID, errMsg, now, id)
	return err
}

// Continue re-enqueues a checkpointed goal for resumption WITHOUT consuming
// its retry budget: the lease's attempt increment is refunded and the
// continuation counter is bumped instead. Returns the new continuation count
// so the caller can enforce a ceiling.
func (q *Queue) Continue(ctx context.Context, id int64, sessionID, note string) (int, error) {
	now := _timeNow().UTC().Format(time.RFC3339)
	_, err := _dbExecContext(q.db, ctx, `
UPDATE goals SET
  status = 'pending',
  attempts = CASE WHEN attempts > 0 THEN attempts - 1 ELSE 0 END,
  continuations = continuations + 1,
  session_id = ?, last_error = ?, lease_until = '', updated_at = ?
WHERE id = ?`, sessionID, note, now, id)
	if err != nil {
		return 0, err
	}
	var n int
	err = q.db.QueryRowContext(ctx, `SELECT continuations FROM goals WHERE id = ?`, id).Scan(&n)
	return n, err
}

func (q *Queue) setStatus(ctx context.Context, id int64, s GoalStatus, sessionID, errMsg string) error {
	now := _timeNow().UTC().Format(time.RFC3339)
	_, err := _dbExecContext(q.db, ctx, `
UPDATE goals SET status = ?, session_id = ?, last_error = ?, lease_until = '', updated_at = ?
WHERE id = ?`, s, sessionID, errMsg, now, id)
	return err
}

// List returns goals filtered by status ("" = all), newest first.
func (q *Queue) List(ctx context.Context, status GoalStatus) ([]Goal, error) {
	query := `SELECT ` + selectColumns + ` FROM goals`
	args := []any{}
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY id DESC LIMIT 200`
	rows, err := _dbQueryContext(q.db, ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGoals(rows)
}

// scanArgs returns the Scan destination list matching selectColumns. created
// and updated are scanned as strings then parsed by the caller.
func scanArgs(g *Goal, created, updated *string) []any {
	return []any{
		&g.ID, &g.Prompt, &g.Workspace, &g.Priority, &g.Status,
		&g.Attempts, &g.MaxRetries, &g.SessionID, &g.LastError, &g.Contract,
		&g.ParentID, &g.Depth, &g.Continuations, created, updated,
	}
}

func scanGoals(rows *sql.Rows) ([]Goal, error) {
	var out []Goal
	for rows.Next() {
		var g Goal
		var created, updated string
		if err := rows.Scan(scanArgs(&g, &created, &updated)...); err != nil {
			return nil, err
		}
		g.CreatedAt, _ = time.Parse(time.RFC3339, created)
		g.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		out = append(out, g)
	}
	return out, rows.Err()
}

// DefaultPath returns ~/.local/share/sin-code/goals.db
func DefaultPath() string {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, _ := _userHomeDir()
		dir = filepath.Join(home, ".local", "share")
	}
	base := filepath.Join(dir, "sin-code")
	_ = _mkdirAll(base, 0o755)
	return filepath.Join(base, "goals.db")
}
