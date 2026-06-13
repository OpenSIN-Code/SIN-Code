// SPDX-License-Identifier: MIT
// Purpose: Semantic session ledger — append-only store of agent-loop events
// (prompts, tool calls, verification results, completions) used for
// summaries, audits, and cross-session learning. SQLite-based, CGo-free.
// Docs: ledger.doc.md
package ledger

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// EntryType classifies a ledger event.
type EntryType string

const (
	TypeUserPrompt       EntryType = "user_prompt"
	TypeToolCall         EntryType = "tool_call"
	TypeToolError        EntryType = "tool_error"
	TypeVerifyPass       EntryType = "verify_pass"
	TypeVerifyFail       EntryType = "verify_fail"
	TypeVerificationMode EntryType = "verification_mode"
	TypeTaskComplete     EntryType = "task_complete"
	TypeTaskAbort        EntryType = "task_abort"
)

// Entry is one row in the ledger.
type Entry struct {
	ID        string
	SessionID string
	Type      EntryType
	Data      map[string]any
	Summary   string
	CreatedAt time.Time
}

// Store is a SQLite-backed ledger.
type Store struct{ db *sql.DB }

// DefaultPath returns the default ledger SQLite path.
func DefaultPath() string {
	if h := os.Getenv("SIN_CODE_HOME"); h != "" {
		return filepath.Join(h, "ledger.db")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "ledger.db"
	}
	return filepath.Join(home, ".local", "share", "sin-code", "ledger.db")
}

// Open opens or creates the ledger at path. Parent directories are created.
func Open(path string) (*Store, error) {
	if path == "" {
		path = DefaultPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the ledger.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS ledger (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    type TEXT NOT NULL,
    data TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ledger_session ON ledger(session_id);
CREATE INDEX IF NOT EXISTS idx_ledger_type ON ledger(type);
CREATE INDEX IF NOT EXISTS idx_ledger_created ON ledger(created_at);

PRAGMA user_version = 1;
`
	_, err := s.db.Exec(schema)
	return err
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), hex.EncodeToString(b[:]))
}

// Record appends a single entry to the ledger.
func (s *Store) Record(ctx context.Context, e Entry) (string, error) {
	if e.ID == "" {
		e.ID = newID()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	data, err := json.Marshal(e.Data)
	if err != nil {
		return "", err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO ledger (id, session_id, type, data, summary, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, e.ID, e.SessionID, string(e.Type), data, e.Summary, e.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return "", err
	}
	return e.ID, nil
}

// List returns entries for a session, newest first.
func (s *Store) List(ctx context.Context, sessionID string, limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, session_id, type, data, summary, created_at
		FROM ledger
		WHERE session_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

// QueryByType returns entries for a session filtered by type.
func (s *Store) QueryByType(ctx context.Context, sessionID string, t EntryType, limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, session_id, type, data, summary, created_at
		FROM ledger
		WHERE session_id = ? AND type = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, sessionID, string(t), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

// Sessions returns all distinct session IDs, newest first.
func (s *Store) Sessions(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT session_id FROM ledger
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var sid string
		if err := rows.Scan(&sid); err != nil {
			return nil, err
		}
		out = append(out, sid)
	}
	return out, rows.Err()
}

func scanRows(rows *sql.Rows) ([]Entry, error) {
	var out []Entry
	for rows.Next() {
		var e Entry
		var data string
		var created string
		if err := rows.Scan(&e.ID, &e.SessionID, &e.Type, &data, &e.Summary, &created); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &e.Data); err != nil {
			return nil, err
		}
		var err error
		e.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
