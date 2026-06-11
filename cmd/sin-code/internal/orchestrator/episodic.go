// SPDX-License-Identifier: MIT
// Purpose: Episodic Replay — persist verified plans as searchable episodes.
// Uses FTS5 over the existing SQLite memory DB. When a new task arrives,
// similar past episodes are injected as a planning prior so the agent
// stops re-deriving known-good strategies from scratch.
package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Episode struct {
	ID        int64           `json:"id"`
	Intent    string          `json:"intent"`
	TaskTitle string          `json:"task_title"`
	PlanJSON  json.RawMessage `json:"plan"`
	Diff      string          `json:"diff,omitempty"`
	Score     float64         `json:"score"`
	Passed    bool            `json:"passed"`
	Rounds    int             `json:"rounds"`
	CreatedAt time.Time       `json:"created_at"`
}

type EpisodeStore struct {
	db        *sql.DB
	hasSchema bool
}

func NewEpisodeStore(db *sql.DB) (*EpisodeStore, error) {
	if db == nil {
		return &EpisodeStore{hasSchema: false}, nil
	}
	const schema = `
CREATE TABLE IF NOT EXISTS episodes (
	id INTEGER PRIMARY KEY,
	intent TEXT NOT NULL,
	task_title TEXT NOT NULL,
	plan_json TEXT NOT NULL,
	diff TEXT,
	score REAL NOT NULL,
	passed INTEGER NOT NULL,
	rounds INTEGER NOT NULL,
	created_at TEXT NOT NULL
);
CREATE VIRTUAL TABLE IF NOT EXISTS episodes_fts USING fts5(
	task_title, intent, content='episodes', content_rowid='id'
);
CREATE TRIGGER IF NOT EXISTS episodes_ai AFTER INSERT ON episodes BEGIN
	INSERT INTO episodes_fts(rowid, task_title, intent)
	VALUES (new.id, new.task_title, new.intent);
END;`
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("episodes schema: %w", err)
	}
	return &EpisodeStore{db: db, hasSchema: true}, nil
}

func (s *EpisodeStore) Record(ctx context.Context, ep *Episode) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO episodes (intent, task_title, plan_json, diff, score, passed, rounds, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ep.Intent, ep.TaskTitle, string(ep.PlanJSON), ep.Diff,
		ep.Score, boolToInt(ep.Passed), ep.Rounds,
		timeNow().Format(time.RFC3339),
	)
	return err
}

func (s *EpisodeStore) Similar(ctx context.Context, taskTitle string, k int) ([]*Episode, error) {
	if s == nil || s.db == nil || k <= 0 {
		return nil, nil
	}
	query := ftsQuery(taskTitle)
	if query == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.id, e.intent, e.task_title, e.plan_json, e.score, e.passed, e.rounds, e.created_at
		FROM episodes_fts f JOIN episodes e ON e.id = f.rowid
		WHERE episodes_fts MATCH ?
		ORDER BY e.passed DESC, bm25(episodes_fts) ASC
		LIMIT ?`, query, k)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Episode
	for rows.Next() {
		ep := &Episode{}
		var planStr, createdStr string
		var passedInt int
		if err := rows.Scan(&ep.ID, &ep.Intent, &ep.TaskTitle, &planStr,
			&ep.Score, &passedInt, &ep.Rounds, &createdStr); err != nil {
			return nil, err
		}
		ep.PlanJSON = json.RawMessage(planStr)
		ep.Passed = passedInt == 1
		ep.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		out = append(out, ep)
	}
	return out, rows.Err()
}

func PlanningPrior(episodes []*Episode) string {
	if len(episodes) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Prior episodes (similar past tasks)\n")
	for _, ep := range episodes {
		status := "SUCCEEDED"
		if !ep.Passed {
			status = "FAILED — avoid this approach"
		}
		fmt.Fprintf(&b, "- [%s, score %.2f, %d repair rounds] %q (intent: %s)\n",
			status, ep.Score, ep.Rounds, ep.TaskTitle, ep.Intent)
	}
	b.WriteString("\nPrefer strategies from succeeded episodes; do not repeat failed approaches.\n")
	return b.String()
}

func ftsQuery(s string) string {
	fields := strings.Fields(s)
	terms := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, f)
		if len(f) >= 3 {
			terms = append(terms, `"`+f+`"`)
		}
	}
	return strings.Join(terms, " OR ")
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
