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
	"time"

	_ "modernc.org/sqlite"
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
}

func DefaultPath() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".local", "share", "sin-code")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "sessions.db")
}

func Open(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	schema := `
CREATE TABLE IF NOT EXISTS sessions (
  id         TEXT PRIMARY KEY,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  title      TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS messages (
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  idx        INTEGER NOT NULL,
  payload    TEXT NOT NULL,
  PRIMARY KEY (session_id, idx)
);`
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("migrate sessions db: %w", err)
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
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var m Message
		if err := json.Unmarshal([]byte(payload), &m); err != nil {
			return nil, err
		}
		history = append(history, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var exists int
	if err := s.db.QueryRow(
		`SELECT COUNT(1) FROM sessions WHERE id = ?`, id).Scan(&exists); err != nil {
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
		payload, err := json.Marshal(m)
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
	if err := tx.Commit(); err != nil {
		return err
	}
	sess.history = append([]Message(nil), msgs...)
	return nil
}

func (s *Store) List() ([]Info, error) {
	rows, err := s.db.Query(
		`SELECT id, created_at, updated_at, title FROM sessions ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Info
	for rows.Next() {
		var i Info
		if err := rows.Scan(&i.ID, &i.CreatedAt, &i.UpdatedAt, &i.Title); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (s *Store) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}
