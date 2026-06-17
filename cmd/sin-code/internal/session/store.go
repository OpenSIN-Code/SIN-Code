// SPDX-License-Identifier: MIT
// Purpose: SQLite-backed resumable agent sessions (mandate C2, AGENTS.md §8).
// CGo-free via modernc.org/sqlite (mandate M2).
package session

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Package-level hooks default to real stdlib/sqlite behaviour. They are
// swapped in tests (store_coverage_test.go) to exercise error branches
// without mocking the SQLite driver.
var (
	sqlOpen = func(driverName, dataSourceName string) (*sql.DB, error) {
		return sql.Open(driverName, dataSourceName)
	}
	execSchema = func(db *sql.DB, schema string) error {
		_, err := db.Exec(schema)
		return err
	}
	rowsScan = func(rows *sql.Rows, dest ...any) error {
		return rows.Scan(dest...)
	}
	rowsErrHook = func(rows *sql.Rows) error {
		return rows.Err()
	}
	countSessions = func(s *Store, id string) (int, error) {
		var exists int
		err := s.db.QueryRow(`SELECT COUNT(1) FROM sessions WHERE id = ?`, id).Scan(&exists)
		return exists, err
	}
	marshalMessages = func(v any) ([]byte, error) {
		return json.Marshal(v)
	}
	txCommit = func(tx *sql.Tx) error {
		return tx.Commit()
	}
	saveHistory = func(sess *Session, msgs []Message) error {
		return sess.SaveHistory(msgs)
	}
)

type Message struct {
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
}

type Store struct{ db *sql.DB }

type Session struct {
	ID      string
	store   *Store
	history []Message
}

type Info struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Title     string `json:"title"`
	// ParentID is the source-session id this session was forked from.
	// Empty for root sessions. Persisted via issue #194 (sessions fork CLI
	// surface + lineage tracking). Cross-references the source row via
	// ON DELETE SET NULL so deleting the parent does not orphan history.
	ParentID string `json:"parent_id,omitempty"`
}

func DefaultPath() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".local", "share", "sin-code")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "sessions.db")
}

func Open(dbPath string) (*Store, error) {
	// Use WAL mode + busy timeout for concurrent access safety (issue #284:
	// parallel sub-agents each get their own session and write concurrently).
	// The DSN pragmas are modernc.org/sqlite specific (key=value pairs).
	// busy_timeout(5000) makes concurrent writers wait up to 5s instead of
	// immediately returning SQLITE_BUSY.
	db, err := sqlOpen("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	schema := `
CREATE TABLE IF NOT EXISTS sessions (
  id         TEXT PRIMARY KEY,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  title      TEXT NOT NULL DEFAULT '',
  parent_id  TEXT REFERENCES sessions(id) ON DELETE SET NULL
);
CREATE TABLE IF NOT EXISTS messages (
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  idx        INTEGER NOT NULL,
  payload    TEXT NOT NULL,
  PRIMARY KEY (session_id, idx)
);`
	if err := execSchema(db, schema); err != nil {
		return nil, fmt.Errorf("migrate sessions db: %w", err)
	}
	// Idempotent column migration for DBs created before issue #194
	// (parent_id did not exist in v3.19.x sessions DBs). SQLite returns
	// "duplicate column name: parent_id" when the column already exists;
	// we swallow that case and propagate any other error.
	if _, err := db.Exec(`ALTER TABLE sessions ADD COLUMN parent_id TEXT REFERENCES sessions(id) ON DELETE SET NULL`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return nil, fmt.Errorf("migrate parent_id column: %w", err)
		}
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return time.Now().UTC().Format("20060102") + "-" + hex.EncodeToString(b)
}

func (s *Store) StartOrResume(id string) (*Session, error) {
	if id == "" {
		id = newID()
		now := time.Now().UTC().Format(time.RFC3339)
		if _, err := s.db.Exec(
			`INSERT INTO sessions (id, created_at, updated_at) VALUES (?, ?, ?)`,
			id, now, now); err != nil {
			return nil, err
		}
		return &Session{ID: id, store: s}, nil
	}

	rows, err := s.db.Query(
		`SELECT payload FROM messages WHERE session_id = ? ORDER BY idx`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []Message
	for rows.Next() {
		var payload string
		if err := rowsScan(rows, &payload); err != nil {
			return nil, err
		}
		var m Message
		if err := json.Unmarshal([]byte(payload), &m); err != nil {
			return nil, err
		}
		history = append(history, m)
	}
	if err := rowsErrHook(rows); err != nil {
		return nil, err
	}
	exists, err := countSessions(s, id)
	if err != nil {
		return nil, err
	}
	if exists == 0 && history == nil {
		return nil, fmt.Errorf("session %q not found", id)
	}
	return &Session{ID: id, store: s, history: history}, nil
}

func (sess *Session) History() []Message {
	return append([]Message(nil), sess.history...)
}

func (sess *Session) SaveHistory(msgs []Message) error {
	tx, err := sess.store.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM messages WHERE session_id = ?`, sess.ID); err != nil {
		return err
	}
	for i, m := range msgs {
		payload, err := marshalMessages(m)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(
			`INSERT INTO messages (session_id, idx, payload) VALUES (?, ?, ?)`,
			sess.ID, i, string(payload)); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(
		`UPDATE sessions SET updated_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), sess.ID); err != nil {
		return err
	}
	if err := txCommit(tx); err != nil {
		return err
	}
	sess.history = append([]Message(nil), msgs...)
	return nil
}

func (s *Store) List() ([]Info, error) {
	rows, err := s.db.Query(
		`SELECT id, created_at, updated_at, title, COALESCE(parent_id, '') FROM sessions ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Info
	for rows.Next() {
		var i Info
		if err := rowsScan(rows, &i.ID, &i.CreatedAt, &i.UpdatedAt, &i.Title, &i.ParentID); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rowsErrHook(rows)
}

func (s *Store) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

// Fork creates a new session whose history is the first `turn` messages
// of the source session identified by `src`. If turn is negative or
// exceeds the source history length, it is clamped. The source session
// must exist. Returns the new *Session (already persisted) and an error
// if the source is not found. Mandate C2; called by the WebUI v2 fork
// endpoint (issue #52).
func (s *Store) Fork(src string, turn int) (*Session, error) {
	rows, err := s.db.Query(
		`SELECT payload FROM messages WHERE session_id = ? ORDER BY idx`, src)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var history []Message
	for rows.Next() {
		var payload string
		if err := rowsScan(rows, &payload); err != nil {
			return nil, err
		}
		var m Message
		if err := json.Unmarshal([]byte(payload), &m); err != nil {
			return nil, err
		}
		history = append(history, m)
	}
	if err := rowsErrHook(rows); err != nil {
		return nil, err
	}
	exists, err := countSessions(s, src)
	if err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, fmt.Errorf("session %q not found", src)
	}
	if turn < 0 {
		turn = 0
	}
	if turn > len(history) {
		turn = len(history)
	}
	forked := history[:turn]

	child := &Session{ID: newID(), store: s, history: forked}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(
		`INSERT INTO sessions (id, created_at, updated_at, parent_id) VALUES (?, ?, ?, ?)`,
		child.ID, now, now, src); err != nil {
		return nil, err
	}
	if err := saveHistory(child, forked); err != nil {
		return nil, err
	}
	return child, nil
}

// ForkEx is Fork with an optional title applied to the new session. The
// parent_id lineage is recorded automatically (issue #194).
//
// CLI convention: turn < 0 means "copy entire history" (resolves to a
// value that gets clamped to len(history) inside Fork itself). Fork()
// alone still clamps negative turn to 0 for the existing WebUI hook
// caller (apiweb/api.go:33) — its contract predates the CLI.
func (s *Store) ForkEx(src string, turn int, title string) (*Session, error) {
	if turn < 0 {
		turn = 1 << 30 // clamps to len(history) inside Fork
	}
	child, err := s.Fork(src, turn)
	if err != nil {
		return nil, err
	}
	if title != "" {
		if _, err := s.db.Exec(
			`UPDATE sessions SET title = ?, updated_at = ? WHERE id = ?`,
			title, time.Now().UTC().Format(time.RFC3339), child.ID); err != nil {
			return nil, fmt.Errorf("set fork title: %w", err)
		}
	}
	return child, nil
}

// Tree returns the ancestry chain for a session, ordered root → ... → self.
// Walks parent_id upward and terminates on missing parent, empty parent,
// cycle (defensive), or self-reference. Returns ErrSessionNotFound when
// the queried id does not exist.
func (s *Store) Tree(id string) ([]Info, error) {
	var exists int
	if err := s.db.QueryRow(
		`SELECT COUNT(1) FROM sessions WHERE id = ?`, id,
	).Scan(&exists); err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, ErrSessionNotFound{id: id}
	}
	seen := map[string]bool{}
	chain := []Info{}
	cur := id
	for cur != "" && !seen[cur] {
		seen[cur] = true
		var i Info
		if err := s.db.QueryRow(
			`SELECT id, created_at, updated_at, title, COALESCE(parent_id, '') FROM sessions WHERE id = ?`, cur,
		).Scan(&i.ID, &i.CreatedAt, &i.UpdatedAt, &i.Title, &i.ParentID); err != nil {
			return nil, err
		}
		chain = append([]Info{i}, chain...)
		cur = i.ParentID
	}
	return chain, nil
}

// ErrSessionNotFound is returned by Tree when the queried session id is
// not present in the store.
type ErrSessionNotFound struct{ id string }

func (e ErrSessionNotFound) Error() string { return fmt.Sprintf("session %q not found", e.id) }
