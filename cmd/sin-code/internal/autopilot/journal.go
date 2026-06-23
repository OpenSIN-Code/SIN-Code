// SPDX-License-Identifier: MIT
// Purpose: SQLite experiment journal — the durable log of every autonomous
// experiment (proposal, metric before/after, kept/reverted, commit, lesson).
// This is what you read in the morning after an overnight run.
package autopilot

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Outcome is the terminal state of an experiment.
type Outcome string

const (
	OutcomeKept       Outcome = "kept"        // verified AND metric improved
	OutcomeReverted   Outcome = "reverted"    // regressed or no improvement
	OutcomeVerifyFail Outcome = "verify_fail" // never passed the gate
)

// testJournalDBErr and testJournalExecErr are injected by coverage tests to
// exercise the OpenJournal error paths. errJournalOpen is the sentinel value
// the tests expect back. journalOpen and journalExec wrap the real sql.DB
// calls so the error branches after the calls are also exercised.
var (
	testJournalDBErr   error
	testJournalExecErr error
	errJournalOpen     = errors.New("journal: open failed")
	journalOpen        = func(driverName, dataSourceName string) (*sql.DB, error) {
		if testJournalDBErr != nil {
			return nil, testJournalDBErr
		}
		return sql.Open(driverName, dataSourceName)
	}
	journalExec = func(db *sql.DB, query string, args ...any) (sql.Result, error) {
		if testJournalExecErr != nil {
			return nil, testJournalExecErr
		}
		return db.Exec(query, args...)
	}
)

// Experiment is one row of the journal.
type Experiment struct {
	ID           int64     `json:"id"`
	Objective    string    `json:"objective"`
	Proposal     string    `json:"proposal"`
	Outcome      Outcome   `json:"outcome"`
	MetricBefore float64   `json:"metric_before"`
	MetricAfter  float64   `json:"metric_after"`
	MetricFound  bool      `json:"metric_found"`
	Commit       string    `json:"commit,omitempty"`
	SessionID    string    `json:"session_id,omitempty"`
	Note         string    `json:"note,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// Journal is the experiment store.
type Journal struct {
	db *sql.DB
}

// OpenJournal opens (and migrates) the journal at path.
func OpenJournal(path string) (*Journal, error) {
	db, err := journalOpen("sqlite", path)
	if err != nil {
		return nil, err
	}
	schema := `
CREATE TABLE IF NOT EXISTS experiments (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  objective TEXT NOT NULL,
  proposal TEXT NOT NULL,
  outcome TEXT NOT NULL,
  metric_before REAL,
  metric_after REAL,
  metric_found INTEGER DEFAULT 0,
  commit_hash TEXT DEFAULT '',
  session_id TEXT DEFAULT '',
  note TEXT DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_experiments_outcome ON experiments(outcome);
`
	if _, err := journalExec(db, schema); err != nil {
		return nil, err
	}
	return &Journal{db: db}, nil
}

// Close closes the underlying database.
func (j *Journal) Close() error { return j.db.Close() }

// Record persists one experiment and returns its ID.
func (j *Journal) Record(ctx context.Context, e Experiment) (int64, error) {
	found := 0
	if e.MetricFound {
		found = 1
	}
	res, err := j.db.ExecContext(ctx, `
INSERT INTO experiments
  (objective, proposal, outcome, metric_before, metric_after, metric_found, commit_hash, session_id, note, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Objective, e.Proposal, string(e.Outcome), e.MetricBefore, e.MetricAfter, found,
		e.Commit, e.SessionID, e.Note, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Recent returns the newest experiments, up to limit.
func (j *Journal) Recent(ctx context.Context, limit int) ([]Experiment, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := j.db.QueryContext(ctx, `
SELECT id, objective, proposal, outcome, metric_before, metric_after, metric_found, commit_hash, session_id, note, created_at
FROM experiments ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Experiment
	for rows.Next() {
		var e Experiment
		var outcome, created string
		var found int
		if err := rows.Scan(&e.ID, &e.Objective, &e.Proposal, &outcome,
			&e.MetricBefore, &e.MetricAfter, &found, &e.Commit, &e.SessionID, &e.Note, &created); err != nil {
			return nil, err
		}
		e.Outcome = Outcome(outcome)
		e.MetricFound = found == 1
		e.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, e)
	}
	return out, rows.Err()
}

// BestKept returns the metric value of the best kept experiment, or NaN.
func (j *Journal) BestKept(ctx context.Context, dir Direction) float64 {
	order := "ASC"
	if dir == Maximize {
		order = "DESC"
	}
	var v sql.NullFloat64
	row := j.db.QueryRowContext(ctx, `
SELECT metric_after FROM experiments
WHERE outcome = 'kept' AND metric_found = 1
ORDER BY metric_after `+order+` LIMIT 1`)
	if err := row.Scan(&v); err != nil || !v.Valid {
		return NoMetric()
	}
	return v.Float64
}

// Count returns the number of experiments with the given outcome ("" = all).
func (j *Journal) Count(ctx context.Context, outcome Outcome) (int, error) {
	q := `SELECT COUNT(*) FROM experiments`
	args := []any{}
	if outcome != "" {
		q += ` WHERE outcome = ?`
		args = append(args, string(outcome))
	}
	var n int
	err := j.db.QueryRowContext(ctx, q, args...).Scan(&n)
	return n, err
}

// DefaultJournalPath returns <workspace>/.sin-code/autopilot.db.
func DefaultJournalPath(workspace string) string {
	dir := filepath.Join(workspace, ".sin-code")
	_ = os.MkdirAll(dir, 0o755) // #nosec G301
	return filepath.Join(dir, "autopilot.db")
}
