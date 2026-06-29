// SPDX-License-Identifier: MIT
package decision

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Decision represents an architectural decision made during a session.
type Decision struct {
	ID           string
	SessionID    string
	Timestamp    time.Time
	Decision     string
	Rationale    string
	Alternatives string
	Files        string
	Workspace    string
}

// Store persists architectural decisions in SQLite.
type Store struct {
	mu   sync.Mutex
	db   *sql.DB
	path string
}

// Open creates or opens the decision store.
func Open(workspace string) (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dbPath := filepath.Join(home, ".local", "share", "sin-code", "decisions.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("decision: mkdir: %w", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("decision: open: %w", err)
	}
	s := &Store{db: db, path: dbPath}
	if err := s.init(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) init() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS decisions (
		id TEXT PRIMARY KEY,
		session_id TEXT,
		timestamp TEXT NOT NULL,
		decision TEXT NOT NULL,
		rationale TEXT,
		alternatives TEXT,
		files TEXT,
		workspace TEXT
	)`)
	return err
}

func (s *Store) Record(ctx context.Context, d Decision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d.Timestamp.IsZero() {
		d.Timestamp = time.Now()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO decisions (id, session_id, timestamp, decision, rationale, alternatives, files, workspace)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.SessionID, d.Timestamp.Format(time.RFC3339),
		d.Decision, d.Rationale, d.Alternatives, d.Files, d.Workspace)
	return err
}

func (s *Store) List(ctx context.Context, workspace string, limit int) ([]Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, timestamp, decision, rationale, alternatives, files, workspace
		 FROM decisions WHERE workspace = ? ORDER BY timestamp DESC LIMIT ?`,
		workspace, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Decision
	for rows.Next() {
		var d Decision
		var ts string
		if err := rows.Scan(&d.ID, &d.SessionID, &ts, &d.Decision, &d.Rationale, &d.Alternatives, &d.Files, &d.Workspace); err != nil {
			return nil, err
		}
		d.Timestamp, _ = time.Parse(time.RFC3339, ts)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) Search(ctx context.Context, workspace, query string) ([]Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := "%" + strings.ToLower(query) + "%"
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, timestamp, decision, rationale, alternatives, files, workspace
		 FROM decisions WHERE workspace = ? AND (LOWER(decision) LIKE ? OR LOWER(rationale) LIKE ?)
		 ORDER BY timestamp DESC LIMIT 50`,
		workspace, q, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Decision
	for rows.Next() {
		var d Decision
		var ts string
		if err := rows.Scan(&d.ID, &d.SessionID, &ts, &d.Decision, &d.Rationale, &d.Alternatives, &d.Files, &d.Workspace); err != nil {
			return nil, err
		}
		d.Timestamp, _ = time.Parse(time.RFC3339, ts)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Close()
}
