// SPDX-License-Identifier: MIT
// Purpose: persistent goal queue for autonomous operation — goals survive
// restarts, carry priorities and retry budgets, and are leased atomically
// so multiple daemon workers never double-execute.
package autonomy

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
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
)

type Goal struct {
	ID         int64      `json:"id"`
	Prompt     string     `json:"prompt"`
	Workspace  string     `json:"workspace"`
	Priority   int        `json:"priority"`
	Status     GoalStatus `json:"status"`
	Attempts   int        `json:"attempts"`
	MaxRetries int        `json:"max_retries"`
	SessionID  string     `json:"session_id,omitempty"`
	LastError  string     `json:"last_error,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type Queue struct {
	db *sql.DB
}

func Open(path string) (*Queue, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
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
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}
	return &Queue{db: db}, nil
}

func (q *Queue) Close() error { return q.db.Close() }

// Add enqueues a goal. Returns its ID.
func (q *Queue) Add(ctx context.Context, prompt, workspace string, priority, maxRetries int) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := q.db.ExecContext(ctx, `
INSERT INTO goals (prompt, workspace, priority, max_retries, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)`, prompt, workspace, priority, maxRetries, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Lease atomically claims the highest-priority pending or stale-running goal.
func (q *Queue) Lease(ctx context.Context, leaseDur time.Duration) (*Goal, error) {
	now := time.Now().UTC()
	leaseUntil := now.Add(leaseDur).Format(time.RFC3339)
	nowStr := now.Format(time.RFC3339)

	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var g Goal
	var created, updated string
	err = tx.QueryRowContext(ctx, `
SELECT id, prompt, workspace, priority, status, attempts, max_retries, session_id, last_error, created_at, updated_at
FROM goals
WHERE (status = 'pending')
   OR (status = 'running' AND lease_until < ?)
ORDER BY priority DESC, id ASC
LIMIT 1`, nowStr).Scan(&g.ID, &g.Prompt, &g.Workspace, &g.Priority, &g.Status,
		&g.Attempts, &g.MaxRetries, &g.SessionID, &g.LastError, &created, &updated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE goals SET status = 'running', attempts = attempts + 1, lease_until = ?, updated_at = ?
WHERE id = ?`, leaseUntil, nowStr, g.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	g.Status = StatusRunning
	g.Attempts++
	g.CreatedAt, _ = time.Parse(time.RFC3339, created)
	g.UpdatedAt = now
	return &g, nil
}

// Complete marks a goal verified.
func (q *Queue) Complete(ctx context.Context, id int64, sessionID string) error {
	return q.setStatus(ctx, id, StatusVerified, sessionID, "")
}

// Fail records a failure; the goal returns to pending until the retry
// budget is spent, then becomes exhausted.
func (q *Queue) Fail(ctx context.Context, id int64, sessionID, errMsg string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := q.db.ExecContext(ctx, `
UPDATE goals SET
  status = CASE WHEN attempts >= max_retries THEN 'exhausted' ELSE 'pending' END,
  session_id = ?, last_error = ?, lease_until = '', updated_at = ?
WHERE id = ?`, sessionID, errMsg, now, id)
	return err
}

func (q *Queue) setStatus(ctx context.Context, id int64, s GoalStatus, sessionID, errMsg string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := q.db.ExecContext(ctx, `
UPDATE goals SET status = ?, session_id = ?, last_error = ?, lease_until = '', updated_at = ?
WHERE id = ?`, s, sessionID, errMsg, now, id)
	return err
}

// List returns goals filtered by status ("" = all), newest first.
func (q *Queue) List(ctx context.Context, status GoalStatus) ([]Goal, error) {
	query := `SELECT id, prompt, workspace, priority, status, attempts, max_retries, session_id, last_error, created_at, updated_at FROM goals`
	args := []any{}
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY id DESC LIMIT 200`
	rows, err := q.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Goal
	for rows.Next() {
		var g Goal
		var created, updated string
		if err := rows.Scan(&g.ID, &g.Prompt, &g.Workspace, &g.Priority, &g.Status,
			&g.Attempts, &g.MaxRetries, &g.SessionID, &g.LastError, &created, &updated); err != nil {
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
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".local", "share")
	}
	base := filepath.Join(dir, "sin-code")
	_ = os.MkdirAll(base, 0o755)
	return filepath.Join(base, "goals.db")
}

// formatForLog helper kept private — not used externally
var _ = fmt.Sprintf
