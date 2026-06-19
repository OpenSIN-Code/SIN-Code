// SPDX-License-Identifier: MIT
// Purpose: knowledge base for self-improvement (Einstein: "insanity is
// doing the same thing and expecting different results"). Failed
// verifications and tool errors accumulate with occurrence counts; the
// agent queries before the first turn to avoid repeating mistakes.
package lessons

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
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

CREATE TABLE IF NOT EXISTS lesson_index (
  token TEXT NOT NULL,
  lesson_id TEXT NOT NULL,
  weight REAL NOT NULL,
  PRIMARY KEY (token, lesson_id)
);
CREATE INDEX IF NOT EXISTS idx_lesson_index_token ON lesson_index(token);

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
	if err := s.rebuildIndex(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// rebuildIndex reconstructs the lesson_index table from lessons. Idempotent.
func (s *Store) rebuildIndex() error {
	rows, err := s.db.Query(`SELECT id, type, workspace, context, lesson, occurrences FROM lessons`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type row struct {
		id, typ, ws, ctx, lesson string
		occurrences              int
	}
	var entries []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.typ, &r.ws, &r.ctx, &r.lesson, &r.occurrences); err != nil {
			return err
		}
		entries = append(entries, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM lesson_index`); err != nil {
		return err
	}
	for _, r := range entries {
		var ctxMap map[string]any
		_ = json.Unmarshal([]byte(r.ctx), &ctxMap)
		e := Entry{ID: r.id, Type: EntryType(r.typ), Workspace: r.ws, Context: ctxMap, Lesson: r.lesson, Occurrences: r.occurrences}
		if err := s.indexLessonTx(tx, e); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// indexLessonTx upserts lesson tokens into the index within an existing transaction.
func (s *Store) indexLessonTx(tx *sql.Tx, e Entry) error {
	weight := float64(e.Occurrences)
	for _, tok := range Tokens(e) {
		if _, err := tx.Exec(`
			INSERT INTO lesson_index (token, lesson_id, weight)
			VALUES (?, ?, ?)
			ON CONFLICT(token, lesson_id) DO UPDATE SET weight = excluded.weight
		`, tok, e.ID, weight); err != nil {
			return err
		}
	}
	return nil
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
		newID := LessonFingerprint(EntryType(r.typ), r.ws, ctxMap)
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
		e.ID = LessonFingerprint(e.Type, e.Workspace, e.Context)
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
	occurrences := 1
	for attempt := 0; attempt < 10; attempt++ {
		var existing struct {
			typ, ws, ctx, lesson string
			occurrences          int
		}
		err := tx.QueryRowContext(ctx, `
			SELECT type, workspace, context, lesson, occurrences FROM lessons WHERE id = ?
		`, id).Scan(&existing.typ, &existing.ws, &existing.ctx, &existing.lesson, &existing.occurrences)
		if err == sql.ErrNoRows {
			// Insert new lesson.
			_, err = tx.ExecContext(ctx, `
				INSERT INTO lessons (id, type, workspace, context, lesson, first_seen, last_seen)
				VALUES (?, ?, ?, ?, ?, ?, ?)
			`, id, e.Type, e.Workspace, ctxJSON, e.Lesson, now, now)
			if err != nil {
				return err
			}
			occurrences = 1
			if err := s.indexLessonTx(tx, Entry{ID: id, Type: e.Type, Workspace: e.Workspace, Context: e.Context, Lesson: e.Lesson, Occurrences: occurrences}); err != nil {
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
			occurrences = existing.occurrences + 1
			if err := s.indexLessonTx(tx, Entry{ID: id, Type: e.Type, Workspace: e.Workspace, Context: e.Context, Lesson: e.Lesson, Occurrences: occurrences}); err != nil {
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

// QueryTopK returns the top-K lessons relevant to a workspace and context,
// scored by token matches from the lesson_index. This avoids scanning the full
// lessons table per briefing call (issue #341).
func (s *Store) QueryTopK(ctx context.Context, workspace string, context map[string]any, limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = 25
	}
	tok := Tokens(Entry{Type: "", Workspace: workspace, Context: context, Lesson: ""})
	if len(tok) == 0 {
		return s.Query(ctx, workspace, limit)
	}
	placeholders := make([]string, len(tok))
	args := make([]any, 0, len(tok)+1)
	for i, t := range tok {
		placeholders[i] = "?"
		args = append(args, t)
	}
	args = append(args, workspace)
	query := fmt.Sprintf(`
SELECT l.id, l.type, l.workspace, l.context, l.lesson, l.occurrences, l.first_seen, l.last_seen
FROM lessons l
JOIN (
	SELECT lesson_id, SUM(weight) AS score
	FROM lesson_index
	WHERE token IN (%s)
	GROUP BY lesson_id
) idx ON l.id = idx.lesson_id
WHERE l.workspace = ? OR l.workspace = '*'
ORDER BY idx.score DESC, l.occurrences DESC, l.last_seen DESC
LIMIT ?
`, strings.Join(placeholders, ","))
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
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

// BriefingForContext renders the top-K briefing for a workspace+context using
// the token index instead of scanning all lessons.
func (s *Store) BriefingForContext(ctx context.Context, workspace string, context map[string]any, maxLessons int, maxBytes int) (string, error) {
	entries, err := s.QueryTopK(ctx, workspace, context, maxLessons)
	if err != nil {
		return "", err
	}
	return Briefing(entries, maxLessons, maxBytes), nil
}

// Delete removes a lesson by ID and its index entries.
func (s *Store) Delete(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM lessons WHERE id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM lesson_index WHERE lesson_id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// Prune removes entries with occurrence count == 1 and last_seen older than
// ageDays. It also cleans up the lesson_index of any orphaned rows.
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
	// Cleanup orphaned index entries (bounded, idempotent).
	_, _ = s.db.ExecContext(ctx, `DELETE FROM lesson_index WHERE lesson_id NOT IN (SELECT id FROM lessons)`)
	return int(n), nil
}

// LessonFingerprint is the stable identity of a lesson (type+workspace+context).
// Uses full 64-hex SHA-256 to avoid 64-bit birthday collisions.
func LessonFingerprint(t EntryType, ws string, ctx map[string]any) string {
	data, _ := json.Marshal(map[string]any{"type": t, "ws": ws, "ctx": ctx})
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// Fingerprint computes a 40-hex SHA-1 digest of the given content string.
// This is the generic fingerprint function used for collision-resistant
// content hashing (issue #340). SHA-1 provides 160 bits (40 hex chars),
// reducing the birthday-bound collision risk to ~2^80 entries.
func Fingerprint(content string) string {
	h := sha1.Sum([]byte(content))
	return hex.EncodeToString(h[:])
}

var tokenRe = regexp.MustCompile(`[a-zA-Z0-9_]+`)

// Tokens extracts searchable tokens from a lesson entry. Tokens are used by
// the lesson_index table to retrieve top-K candidates for a context without
// scanning the entire lessons table.
func Tokens(e Entry) []string {
	seen := make(map[string]bool)
	add := func(s string) {
		for _, t := range tokenRe.FindAllString(s, -1) {
			t = strings.ToLower(t)
			if len(t) >= 3 && !seen[t] {
				seen[t] = true
			}
		}
	}
	add(string(e.Type))
	add(e.Workspace)
	add(e.Lesson)
	ctxJSON, _ := json.Marshal(e.Context)
	add(string(ctxJSON))
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

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
