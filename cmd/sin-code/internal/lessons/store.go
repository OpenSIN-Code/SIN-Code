// SPDX-License-Identifier: MIT
// Purpose: knowledge base for self-improvement (Einstein: "insanity is
// doing the same thing and expecting different results"). Failed
// verifications and tool errors accumulate with occurrence counts; the
// agent queries before the first turn to avoid repeating mistakes.
package lessons

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Package-level hooks for testing error branches without mocking the
// SQLite driver. Production defaults are nil and never alter behavior.
var (
	errTestOpen       = errors.New("forced open")
	testOpenDBErr     error
	testOpenPragmaErr error
	testOpenSchemaErr error
)

type EntryType string

const (
	TypeFailedVerification EntryType = "failed_verification"
	TypeSuccessPattern     EntryType = "success_pattern"
	TypeConstraint         EntryType = "constraint"
	TypeToolError          EntryType = "tool_error"
)

type Entry struct {
	ID          string         `json:"id"`
	Type        EntryType      `json:"type"`
	Workspace   string         `json:"workspace"`
	Context     map[string]any `json:"context"`
	Lesson      string         `json:"lesson"`
	Occurrences int            `json:"occurrences"`
	FirstSeen   time.Time      `json:"first_seen"`
	LastSeen    time.Time      `json:"last_seen"`
}

type Store struct {
	db *sql.DB
}

// DefaultPath returns ~/.local/share/sin-code/lessons.db
func DefaultPath() string {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".local", "share")
	}
	base := filepath.Join(dir, "sin-code")
	_ = os.MkdirAll(base, 0o755)
	return filepath.Join(base, "lessons.db")
}

func Open(path string) (*Store, error) {
	if path == "" {
		path = DefaultPath()
	}
	var db *sql.DB
	var err error
	if testOpenDBErr != nil {
		err = testOpenDBErr
	} else {
		db, err = sql.Open("sqlite", path)
	}
	if err != nil {
		return nil, err
	}

	if testOpenPragmaErr != nil {
		err = testOpenPragmaErr
	} else {
		_, err = db.Exec(`PRAGMA foreign_keys = ON`)
	}
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	if testOpenSchemaErr != nil {
		err = testOpenSchemaErr
	} else {
		schema := `
CREATE TABLE IF NOT EXISTS lessons (
  id TEXT PRIMARY KEY,
  type TEXT NOT NULL,
  workspace TEXT NOT NULL,
  context TEXT NOT NULL,
  lesson TEXT NOT NULL,
  occurrences INTEGER DEFAULT 1,
  first_seen TEXT NOT NULL,
  last_seen TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_lessons_workspace ON lessons(workspace);
CREATE INDEX IF NOT EXISTS idx_lessons_type ON lessons(type);
CREATE INDEX IF NOT EXISTS idx_lessons_occurrences ON lessons(occurrences DESC);

PRAGMA user_version = 2;
`
		_, err = db.Exec(schema)
	}
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrateFingerprints(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// migrateFingerprints rewrites any 16-hex (64-bit) fingerprints to full 64-hex
// SHA-256 fingerprints. Idempotent: only rows with exactly 16 hex chars are touched.
func (s *Store) migrateFingerprints() error {
	rows, err := s.db.Query(`SELECT id, type, workspace, context, lesson, occurrences, first_seen, last_seen FROM lessons WHERE LENGTH(id) = 16`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type oldRow struct {
		id, typ, ws, ctx, lesson, first, last string
		occurrences                           int
	}
	var toMigrate []oldRow
	for rows.Next() {
		var r oldRow
		if err := rows.Scan(&r.id, &r.typ, &r.ws, &r.ctx, &r.lesson, &r.occurrences, &r.first, &r.last); err != nil {
			return err
		}
		toMigrate = append(toMigrate, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(toMigrate) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range toMigrate {
		var ctxMap map[string]any
		if err := json.Unmarshal([]byte(r.ctx), &ctxMap); err != nil {
			ctxMap = nil
		}
		newID := Fingerprint(EntryType(r.typ), r.ws, ctxMap)
		if _, err := tx.Exec(`
			UPDATE lessons SET id = ? WHERE id = ?
		`, newID, r.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Record upserts a lesson — same fingerprint increments the count.
// If a collision is detected (same fingerprint but different content),
// a deterministic variant ID is used so the distinct lesson is preserved.
func (s *Store) Record(ctx context.Context, e Entry) error {
	if e.ID == "" {
		e.ID = Fingerprint(e.Type, e.Workspace, e.Context)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	ctxJSON, err := json.Marshal(e.Context)
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	id := e.ID
	for attempt := 0; attempt < 10; attempt++ {
		var existing struct {
			typ, ws, ctx, lesson string
		}
		err := tx.QueryRowContext(ctx, `
			SELECT type, workspace, context, lesson FROM lessons WHERE id = ?
		`, id).Scan(&existing.typ, &existing.ws, &existing.ctx, &existing.lesson)
		if err == sql.ErrNoRows {
			// Insert new lesson.
			_, err = tx.ExecContext(ctx, `
				INSERT INTO lessons (id, type, workspace, context, lesson, first_seen, last_seen)
				VALUES (?, ?, ?, ?, ?, ?, ?)
			`, id, e.Type, e.Workspace, ctxJSON, e.Lesson, now, now)
			if err != nil {
				return err
			}
			return tx.Commit()
		}
		if err != nil {
			return err
		}
		// Collision check: compare canonical content.
		if existing.typ == string(e.Type) && existing.ws == e.Workspace && existing.ctx == string(ctxJSON) && existing.lesson == e.Lesson {
			_, err = tx.ExecContext(ctx, `
				UPDATE lessons SET occurrences = occurrences + 1, last_seen = ? WHERE id = ?
			`, now, id)
			if err != nil {
				return err
			}
			return tx.Commit()
		}
		// True collision: try a variant ID.
		id = fmt.Sprintf("%s:%d", e.ID, attempt+1)
	}
	return errors.New("too many fingerprint collisions")
}

// Query returns relevant lessons for a workspace, ordered by frequency.
func (s *Store) Query(ctx context.Context, workspace string, limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, type, workspace, context, lesson, occurrences, first_seen, last_seen
FROM lessons
WHERE workspace = ? OR workspace = '*'
ORDER BY occurrences DESC, last_seen DESC
LIMIT ?
`, workspace, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		var ctxJSON, first, last string
		if err := rows.Scan(&e.ID, &e.Type, &e.Workspace, &ctxJSON, &e.Lesson,
			&e.Occurrences, &first, &last); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(ctxJSON), &e.Context); err != nil {
			return nil, err
		}
		e.FirstSeen, _ = time.Parse(time.RFC3339, first)
		e.LastSeen, _ = time.Parse(time.RFC3339, last)
		out = append(out, e)
	}
	return out, rows.Err()
}

// Delete removes a lesson by ID.
func (s *Store) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM lessons WHERE id = ?`, id)
	return err
}

// Prune removes entries with occurrence count == 1 and last_seen older than
// ageDays.
func (s *Store) Prune(ctx context.Context, ageDays int) (int, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(ageDays) * 24 * time.Hour).Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `
DELETE FROM lessons
WHERE occurrences = 1 AND last_seen < ?
`, cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// Fingerprint is the stable identity of a lesson (type+workspace+context).
// Uses full 64-hex SHA-256 to avoid 64-bit birthday collisions.
func Fingerprint(t EntryType, ws string, ctx map[string]any) string {
	data, _ := json.Marshal(map[string]any{"type": t, "ws": ws, "ctx": ctx})
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// sin-debt: linear scan over all lessons per briefing call, upgrade: switch to top-K precomputed index when entry count > 10k
// Briefing renders the top workspace lessons as a compact prompt prefix.
// Only entries with occurrences >= 2 qualify (repetition is signal, single
// is noise). Capped at 10 lessons / ~2KB to protect the context window.
func Briefing(entries []Entry, maxLessons int, maxBytes int) string {
	if maxLessons <= 0 {
		maxLessons = 10
	}
	if maxBytes <= 0 {
		maxBytes = 2048
	}
	var b []byte
	header := []byte("WORKSPACE KNOWLEDGE (lessons from previous sessions — do NOT repeat these mistakes):\n")
	count := 0
	started := false
	for _, e := range entries {
		if e.Occurrences < 2 {
			continue
		}
		if !started {
			b = append(b, header...)
			started = true
		}
		line := fmt.Sprintf("- [%dx %s] %s\n", e.Occurrences, e.Type, e.Lesson)
		if len(b)+len(line) > maxBytes {
			break
		}
		b = append(b, line...)
		count++
		if count >= maxLessons {
			break
		}
	}
	return string(b)
}
