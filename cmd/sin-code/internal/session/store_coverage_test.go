// SPDX-License-Identifier: MIT
// Purpose: targeted coverage tests for the remaining error branches in the
// session store. These tests exercise the package-level hook vars introduced
// in store.go so that every error return is hit without mocking the SQLite
// driver itself.
package session

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func openStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "sessions.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestOpenSQLError(t *testing.T) {
	orig := sqlOpen
	sqlOpen = func(driverName, dataSourceName string) (*sql.DB, error) { return nil, errors.New("forced open") }
	defer func() { sqlOpen = orig }()

	if _, err := Open(filepath.Join(t.TempDir(), "sessions.db")); err == nil {
		t.Fatal("expected Open to fail")
	}
}

func TestOpenMigrateError(t *testing.T) {
	orig := execSchema
	execSchema = func(db *sql.DB, schema string) error { return errors.New("forced migrate") }
	defer func() { execSchema = orig }()

	if _, err := Open(filepath.Join(t.TempDir(), "sessions.db")); err == nil {
		t.Fatal("expected Open to fail")
	}
}

func TestStartOrResumeEmptyInsertError(t *testing.T) {
	store := openStore(t)
	_ = store.Close()
	if _, err := store.StartOrResume(""); err == nil {
		t.Fatal("expected insert error for closed store")
	}
}

func TestStartOrResumeQueryError(t *testing.T) {
	store := openStore(t)
	_ = store.Close()
	if _, err := store.StartOrResume("any-id"); err == nil {
		t.Fatal("expected query error for closed store")
	}
}

func TestStartOrResumeScanError(t *testing.T) {
	store := openStore(t)
	sess, _ := store.StartOrResume("")
	if err := sess.SaveHistory([]Message{{Role: "user", Content: "x"}}); err != nil {
		t.Fatal(err)
	}
	orig := rowsScan
	rowsScan = func(rows *sql.Rows, dest ...any) error { return errors.New("forced scan") }
	defer func() { rowsScan = orig }()
	if _, err := store.StartOrResume(sess.ID); err == nil {
		t.Fatal("expected scan error")
	}
}

func TestStartOrResumeRowsErrError(t *testing.T) {
	store := openStore(t)
	sess, _ := store.StartOrResume("")
	if err := sess.SaveHistory([]Message{{Role: "user", Content: "x"}}); err != nil {
		t.Fatal(err)
	}
	orig := rowsErrHook
	rowsErrHook = func(rows *sql.Rows) error { return errors.New("forced rows err") }
	defer func() { rowsErrHook = orig }()
	if _, err := store.StartOrResume(sess.ID); err == nil {
		t.Fatal("expected rows.Err error")
	}
}

func TestStartOrResumeCountError(t *testing.T) {
	store := openStore(t)
	sess, _ := store.StartOrResume("")
	if err := sess.SaveHistory([]Message{{Role: "user", Content: "x"}}); err != nil {
		t.Fatal(err)
	}
	orig := countSessions
	countSessions = func(s *Store, id string) (int, error) { return 0, errors.New("forced count") }
	defer func() { countSessions = orig }()
	if _, err := store.StartOrResume(sess.ID); err == nil {
		t.Fatal("expected count error")
	}
}

func TestSaveHistoryBeginError(t *testing.T) {
	store := openStore(t)
	sess, _ := store.StartOrResume("")
	_ = store.Close()
	if err := sess.SaveHistory([]Message{{Role: "user", Content: "x"}}); err == nil {
		t.Fatal("expected begin error")
	}
}

func TestSaveHistoryDeleteError(t *testing.T) {
	store := openStore(t)
	sess, _ := store.StartOrResume("")
	if err := sess.SaveHistory([]Message{{Role: "user", Content: "x"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`
		CREATE TRIGGER reject_delete BEFORE DELETE ON messages
		BEGIN
			SELECT RAISE(ABORT, 'delete rejected');
		END;`); err != nil {
		t.Fatal(err)
	}
	if err := sess.SaveHistory([]Message{{Role: "user", Content: "y"}}); err == nil {
		t.Fatal("expected delete error")
	}
}

func TestSaveHistoryMarshalError(t *testing.T) {
	store := openStore(t)
	sess, _ := store.StartOrResume("")
	orig := marshalMessages
	marshalMessages = func(v any) ([]byte, error) { return nil, errors.New("forced marshal") }
	defer func() { marshalMessages = orig }()
	if err := sess.SaveHistory([]Message{{Role: "user", Content: "x"}}); err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestSaveHistoryUpdateError(t *testing.T) {
	store := openStore(t)
	sess, _ := store.StartOrResume("")
	if err := sess.SaveHistory([]Message{{Role: "user", Content: "x"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`
		CREATE TRIGGER reject_update BEFORE UPDATE ON sessions
		BEGIN
			SELECT RAISE(ABORT, 'update rejected');
		END;`); err != nil {
		t.Fatal(err)
	}
	if err := sess.SaveHistory([]Message{{Role: "user", Content: "y"}}); err == nil {
		t.Fatal("expected update error")
	}
}

func TestSaveHistoryCommitError(t *testing.T) {
	store := openStore(t)
	sess, _ := store.StartOrResume("")
	orig := txCommit
	txCommit = func(tx *sql.Tx) error { return errors.New("forced commit") }
	defer func() { txCommit = orig }()
	if err := sess.SaveHistory([]Message{{Role: "user", Content: "x"}}); err == nil {
		t.Fatal("expected commit error")
	}
}

func TestListQueryError(t *testing.T) {
	store := openStore(t)
	_ = store.Close()
	if _, err := store.List(); err == nil {
		t.Fatal("expected list query error")
	}
}

func TestListScanError(t *testing.T) {
	store := openStore(t)
	_, _ = store.StartOrResume("")
	orig := rowsScan
	rowsScan = func(rows *sql.Rows, dest ...any) error { return errors.New("forced scan") }
	defer func() { rowsScan = orig }()
	if _, err := store.List(); err == nil {
		t.Fatal("expected list scan error")
	}
}

func TestForkQueryError(t *testing.T) {
	store := openStore(t)
	sess, _ := store.StartOrResume("")
	_ = store.Close()
	if _, err := store.Fork(sess.ID, 0); err == nil {
		t.Fatal("expected fork query error")
	}
}

func TestForkScanError(t *testing.T) {
	store := openStore(t)
	sess, _ := store.StartOrResume("")
	if err := sess.SaveHistory([]Message{{Role: "user", Content: "x"}}); err != nil {
		t.Fatal(err)
	}
	orig := rowsScan
	rowsScan = func(rows *sql.Rows, dest ...any) error { return errors.New("forced scan") }
	defer func() { rowsScan = orig }()
	if _, err := store.Fork(sess.ID, 1); err == nil {
		t.Fatal("expected fork scan error")
	}
}

func TestForkUnmarshalError(t *testing.T) {
	store := openStore(t)
	sess, _ := store.StartOrResume("")
	if err := sess.SaveHistory([]Message{{Role: "user", Content: "x"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(
		`INSERT INTO messages (session_id, idx, payload) VALUES (?, 1, 'not-json')`,
		sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Fork(sess.ID, 2); err == nil {
		t.Fatal("expected fork unmarshal error")
	}
}

func TestForkRowsErrError(t *testing.T) {
	store := openStore(t)
	sess, _ := store.StartOrResume("")
	if err := sess.SaveHistory([]Message{{Role: "user", Content: "x"}}); err != nil {
		t.Fatal(err)
	}
	orig := rowsErrHook
	rowsErrHook = func(rows *sql.Rows) error { return errors.New("forced rows err") }
	defer func() { rowsErrHook = orig }()
	if _, err := store.Fork(sess.ID, 1); err == nil {
		t.Fatal("expected fork rows.Err error")
	}
}

func TestForkCountError(t *testing.T) {
	store := openStore(t)
	sess, _ := store.StartOrResume("")
	if err := sess.SaveHistory([]Message{{Role: "user", Content: "x"}}); err != nil {
		t.Fatal(err)
	}
	orig := countSessions
	countSessions = func(s *Store, id string) (int, error) { return 0, errors.New("forced count") }
	defer func() { countSessions = orig }()
	if _, err := store.Fork(sess.ID, 1); err == nil {
		t.Fatal("expected fork count error")
	}
}

func TestForkChildInsertError(t *testing.T) {
	store := openStore(t)
	sess, _ := store.StartOrResume("")
	if err := sess.SaveHistory([]Message{{Role: "user", Content: "x"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`
		CREATE TRIGGER reject_insert BEFORE INSERT ON sessions
		BEGIN
			SELECT RAISE(ABORT, 'insert rejected');
		END;`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Fork(sess.ID, 1); err == nil {
		t.Fatal("expected fork child insert error")
	}
}

func TestForkSaveHistoryError(t *testing.T) {
	store := openStore(t)
	sess, _ := store.StartOrResume("")
	if err := sess.SaveHistory([]Message{{Role: "user", Content: "x"}}); err != nil {
		t.Fatal(err)
	}
	orig := saveHistory
	saveHistory = func(sess *Session, msgs []Message) error { return errors.New("forced save") }
	defer func() { saveHistory = orig }()
	if _, err := store.Fork(sess.ID, 1); err == nil {
		t.Fatal("expected fork save history error")
	}
}
