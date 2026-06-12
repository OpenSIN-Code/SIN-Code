// SPDX-License-Identifier: MIT
// Purpose: lift internal/session to >=85% coverage (issue #50).
// Focus: the tx.Rollback path in SaveHistory via an induced PK-constraint
// violation mid-loop, plus Delete/List/resume-error paths.
package session

import (
	"path/filepath"
	"strings"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "sessions.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// TestSaveHistoryRollbackOnConstraintViolation induces a PRIMARY KEY
// (session_id, idx) violation mid-loop via a BEFORE INSERT trigger that
// rejects idx >= 1, then verifies the deferred tx.Rollback restored the
// pre-existing rows.
func TestSaveHistoryRollbackOnConstraintViolation(t *testing.T) {
	store := openTestStore(t)
	sess, err := store.StartOrResume("")
	if err != nil {
		t.Fatal(err)
	}

	seed := []Message{
		{Role: "user", Content: "original-1"},
		{Role: "assistant", Content: "original-2"},
	}
	if err := sess.SaveHistory(seed); err != nil {
		t.Fatal(err)
	}

	if _, err := store.db.Exec(`
CREATE TRIGGER reject_idx BEFORE INSERT ON messages
WHEN NEW.idx >= 1
BEGIN
  SELECT RAISE(ABORT, 'induced constraint violation');
END;`); err != nil {
		t.Fatal(err)
	}

	bad := []Message{
		{Role: "user", Content: "replacement-1"},
		{Role: "assistant", Content: "replacement-2"},
	}
	err = sess.SaveHistory(bad)
	if err == nil {
		t.Fatal("expected SaveHistory to fail on induced constraint violation")
	}
	if !strings.Contains(err.Error(), "induced constraint violation") {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := store.db.Exec(`DROP TRIGGER reject_idx`); err != nil {
		t.Fatal(err)
	}
	resumed, err := store.StartOrResume(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := resumed.History()
	if len(got) != 2 || got[0].Content != "original-1" || got[1].Content != "original-2" {
		t.Fatalf("rollback failed — DB state corrupted: %+v", got)
	}

	mem := sess.History()
	if len(mem) != 2 || mem[0].Content != "original-1" {
		t.Fatalf("in-memory history mutated despite rollback: %+v", mem)
	}
}

func TestSaveHistoryEmptyReplacesAll(t *testing.T) {
	store := openTestStore(t)
	sess, err := store.StartOrResume("")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.SaveHistory([]Message{{Role: "user", Content: "x"}}); err != nil {
		t.Fatal(err)
	}
	if err := sess.SaveHistory(nil); err != nil {
		t.Fatal(err)
	}
	resumed, err := store.StartOrResume(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(resumed.History()) != 0 {
		t.Fatalf("expected empty history, got %d messages", len(resumed.History()))
	}
}

func TestResumeUnknownSessionFails(t *testing.T) {
	store := openTestStore(t)
	if _, err := store.StartOrResume("does-not-exist"); err == nil {
		t.Fatal("expected error for unknown session id")
	}
}

func TestResumeCorruptPayloadFails(t *testing.T) {
	store := openTestStore(t)
	sess, err := store.StartOrResume("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(
		`INSERT INTO messages (session_id, idx, payload) VALUES (?, 0, 'not-json{')`,
		sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartOrResume(sess.ID); err == nil {
		t.Fatal("expected JSON unmarshal error for corrupt payload")
	}
}

func TestListOrderingAndDelete(t *testing.T) {
	store := openTestStore(t)
	a, _ := store.StartOrResume("")
	b, _ := store.StartOrResume("")

	if err := b.SaveHistory([]Message{{Role: "user", Content: "b"}}); err != nil {
		t.Fatal(err)
	}
	if err := a.SaveHistory([]Message{{Role: "user", Content: "a"}}); err != nil {
		t.Fatal(err)
	}

	infos, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(infos))
	}

	if err := store.Delete(a.ID); err != nil {
		t.Fatal(err)
	}
	infos, err = store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 || infos[0].ID != b.ID {
		t.Fatalf("expected only session %s, got %+v", b.ID, infos)
	}
}

func TestDefaultPath(t *testing.T) {
	p := DefaultPath()
	if p == "" || !strings.HasSuffix(p, "sessions.db") {
		t.Fatalf("unexpected default path: %q", p)
	}
}
