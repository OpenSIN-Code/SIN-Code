// SPDX-License-Identifier: MIT
// Purpose: tests that need to reach into the lessons package's SQL
// internals but not modify them. Centralizes `openLessonsAt`,
// `fingerprintFor`, `insertRawLesson`, and the bare `contextBg()` used
// by the test suite.
package compress

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
)

// openLessonsAt is a thin wrapper that opens a lessons-compatible
// SQLite file *without* going through the lessons package's Open()
// (which would form a test-time import cycle). Same schema, same
// indexes, no behavior expectation differences for the compressor.
func openLessonsAt(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
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
`
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}
	return db, nil
}

// fingerprintFor delegates to lessons.Fingerprint so the test seeds
// share the same ID scheme that applyLessonsAtomic derives on Apply.
// Without this consistency the test would insert under custom IDs and
// Apply would re-derive under production IDs, deleting the original.
// We can't import the private lessons.Fingerprint... oh wait — it
// IS exported (see internal/lessons/store.go:174). Use it directly.
func fingerprintFor(t, ws string, ctx_ map[string]any) string {
	return lessons.Fingerprint(lessons.EntryType(t), ws, ctx_)
}

// lessonFpBody kept for documentation purposes only — was the
// pre-fix shed-shape. We keep a stub so the tests compile.
func lessonFpBody(t, ws string, ctx_ map[string]any) string {
	out, _ := json.Marshal(map[string]any{"type": t, "ws": ws, "ctx": ctx_})
	return string(out)
}

// stableHashCtx returns 8 bytes of SHA-256 over the joined keys/values
// (deterministic regardless of map iteration order).
func stableHashCtx(t, ws string, ctx_ map[string]any) [8]byte {
	keys := make([]string, 0, len(ctx_))
	for k := range ctx_ {
		keys = append(keys, k)
	}
	// Insertion sort.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	out := fmt.Sprintf("%s|%s|%v", t, ws, sortedValues(keys, ctx_))
	hash := sha256Sum([]byte(out))
	return [8]byte{hash[0], hash[1], hash[2], hash[3], hash[4], hash[5], hash[6], hash[7]}
}

// sortedValues returns ctx_'s values in key-sorted order.
func sortedValues(keys []string, ctx_ map[string]any) []any {
	out := make([]any, len(keys))
	for i, k := range keys {
		out[i] = ctx_[k]
	}
	return out
}

// sha256Sum exposes a hash helper without forcing an import in callers.
func sha256Sum(b []byte) []byte {
	return sha256SumImpl(b)
}

// insertRawLesson inserts a row matching the lessons schema. We
// pre-compute the fingerprint with the context that the compressor
// later re-derives via lessons.Fingerprint, so the apply step works.
func insertRawLesson(db *sql.DB, typ, lesson string, occ int, ctx map[string]any) (string, error) {
	id := fingerprintFor(typ, "*", ctx)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO lessons (id, type, workspace, context, lesson, occurrences, first_seen, last_seen) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, typ, "*", encodedCtxJSON(ctx), lesson, occ, now, now); err != nil {
		return "", err
	}
	return id, nil
}

// encodedCtxJSON returns a stable JSON byte slice — at minimum we
// require keys to lex-sort. The map is small enough that we serialize
// "key=value" pairs in sorted order.
func encodedCtxJSON(ctx map[string]any) string {
	if len(ctx) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(ctx))
	for k := range ctx {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	out := "{"
	for i, k := range keys {
		if i > 0 {
			out += ","
		}
		out += fmt.Sprintf("%q:%v", k, ctx[k])
	}
	out += "}"
	return out
}

// contextBg is a lazy alias to context.Background() used by tests.
func contextBg() context.Context { return context.Background() }
