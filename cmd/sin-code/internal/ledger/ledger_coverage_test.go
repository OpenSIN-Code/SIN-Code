// SPDX-License-Identifier: MIT
// Purpose: targeted coverage tests for the remaining error branches in the
// semantic session ledger. These tests use the package-level hook vars
// introduced in store.go to hit every uncovered statement without a full
// SQLite mock driver.
package ledger

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func openLedger(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestDefaultPathSinCodeHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIN_CODE_HOME", dir)
	if got := DefaultPath(); got != filepath.Join(dir, "ledger.db") {
		t.Fatalf("DefaultPath = %q, want %q", got, filepath.Join(dir, "ledger.db"))
	}
}

func TestDefaultPathHome(t *testing.T) {
	t.Setenv("SIN_CODE_HOME", "")
	dir := t.TempDir()
	orig := userHomeDir
	userHomeDir = func() (string, error) { return dir, nil }
	defer func() { userHomeDir = orig }()
	if got := DefaultPath(); got != filepath.Join(dir, ".local", "share", "sin-code", "ledger.db") {
		t.Fatalf("DefaultPath = %q", got)
	}
}

func TestDefaultPathUserHomeDirError(t *testing.T) {
	t.Setenv("SIN_CODE_HOME", "")
	orig := userHomeDir
	userHomeDir = func() (string, error) { return "", errors.New("no home") }
	defer func() { userHomeDir = orig }()
	if got := DefaultPath(); got != "ledger.db" {
		t.Fatalf("DefaultPath = %q, want ledger.db", got)
	}
}

func TestOpenDefaultPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIN_CODE_HOME", dir)
	s, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Record(context.Background(), Entry{SessionID: "s", Type: TypeUserPrompt, Data: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
}

func TestOpenMkdirAllError(t *testing.T) {
	f := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// MkdirAll must fail because f is a file, not a directory.
	if _, err := Open(filepath.Join(f, "ledger.db")); err == nil {
		t.Fatal("expected MkdirAll error")
	}
}

func TestOpenSQLError(t *testing.T) {
	orig := sqlOpen
	sqlOpen = func(driverName, dataSourceName string) (*sql.DB, error) { return nil, errors.New("forced open") }
	defer func() { sqlOpen = orig }()
	if _, err := Open(filepath.Join(t.TempDir(), "ledger.db")); err == nil {
		t.Fatal("expected sql.Open error")
	}
}

func TestOpenMigrateError(t *testing.T) {
	orig := migrateFn
	migrateFn = func(s *Store) error { return errors.New("forced migrate") }
	defer func() { migrateFn = orig }()
	if _, err := Open(filepath.Join(t.TempDir(), "ledger.db")); err == nil {
		t.Fatal("expected migrate error")
	}
}

func TestListDefaultLimit(t *testing.T) {
	s := openLedger(t)
	ctx := context.Background()
	sid := "default-limit"
	if _, err := s.Record(ctx, Entry{SessionID: sid, Type: TypeUserPrompt, Data: map[string]any{"x": 1}}); err != nil {
		t.Fatal(err)
	}
	entries, err := s.List(ctx, sid, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
}

func TestQueryByTypeDefaultLimit(t *testing.T) {
	s := openLedger(t)
	ctx := context.Background()
	sid := "qbt-default"
	if _, err := s.Record(ctx, Entry{SessionID: sid, Type: TypeUserPrompt, Data: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	entries, err := s.QueryByType(ctx, sid, TypeUserPrompt, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
}

func TestSessionsDefaultLimit(t *testing.T) {
	s := openLedger(t)
	ctx := context.Background()
	if _, err := s.Record(ctx, Entry{SessionID: "s1", Type: TypeUserPrompt, Data: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	sessions, err := s.Sessions(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("want 1 session, got %d", len(sessions))
	}
}

func TestListQueryError(t *testing.T) {
	s := openLedger(t)
	_ = s.Close()
	if _, err := s.List(context.Background(), "x", 10); err == nil {
		t.Fatal("expected list query error")
	}
}

func TestQueryByTypeQueryError(t *testing.T) {
	s := openLedger(t)
	_ = s.Close()
	if _, err := s.QueryByType(context.Background(), "x", TypeUserPrompt, 10); err == nil {
		t.Fatal("expected query error")
	}
}

func TestSessionsQueryError(t *testing.T) {
	s := openLedger(t)
	_ = s.Close()
	if _, err := s.Sessions(context.Background(), 10); err == nil {
		t.Fatal("expected sessions query error")
	}
}

func TestSessionsRowsScanError(t *testing.T) {
	s := openLedger(t)
	ctx := context.Background()
	if _, err := s.Record(ctx, Entry{SessionID: "s", Type: TypeUserPrompt, Data: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	orig := rowsScan
	rowsScan = func(rows *sql.Rows, dest ...any) error { return errors.New("forced scan") }
	defer func() { rowsScan = orig }()
	if _, err := s.Sessions(ctx, 10); err == nil {
		t.Fatal("expected sessions scan error")
	}
}

func TestScanRowsScanError(t *testing.T) {
	s := openLedger(t)
	ctx := context.Background()
	if _, err := s.Record(ctx, Entry{SessionID: "s", Type: TypeUserPrompt, Data: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	orig := rowsScan
	rowsScan = func(rows *sql.Rows, dest ...any) error { return errors.New("forced scan") }
	defer func() { rowsScan = orig }()
	if _, err := s.List(ctx, "s", 10); err == nil {
		t.Fatal("expected scanRows scan error")
	}
}

func TestScanRowsUnmarshalError(t *testing.T) {
	s := openLedger(t)
	if _, err := s.db.Exec(`
		INSERT INTO ledger (id, session_id, type, data, summary, created_at)
		VALUES ('id1', 's1', 'user_prompt', 'not-json', '', '2023-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(context.Background(), "s1", 10); err == nil {
		t.Fatal("expected unmarshal error")
	}
}

func TestScanRowsTimeParseError(t *testing.T) {
	s := openLedger(t)
	if _, err := s.db.Exec(`
		INSERT INTO ledger (id, session_id, type, data, summary, created_at)
		VALUES ('id2', 's2', 'user_prompt', '{}', '', 'not-a-time')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(context.Background(), "s2", 10); err == nil {
		t.Fatal("expected time parse error")
	}
}

func TestRecordMarshalError(t *testing.T) {
	s := openLedger(t)
	if _, err := s.Record(context.Background(), Entry{
		SessionID: "s",
		Type:      TypeUserPrompt,
		Data:      map[string]any{"bad": make(chan int)},
	}); err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestRecordExecError(t *testing.T) {
	s := openLedger(t)
	_ = s.Close()
	if _, err := s.Record(context.Background(), Entry{SessionID: "s", Type: TypeUserPrompt, Data: map[string]any{}}); err == nil {
		t.Fatal("expected exec error")
	}
}
