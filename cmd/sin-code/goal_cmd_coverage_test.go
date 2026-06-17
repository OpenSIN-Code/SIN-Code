// SPDX-License-Identifier: MIT
// Purpose: coverage tests for goal_cmd.go — exercises add/list subcommands
// using package-level hooks so tests never need a real goals.db.
// Docs: goal_cmd.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
)

type goalErrWriter struct{ err error }

func (e goalErrWriter) Write(p []byte) (int, error) { return 0, e.err }

func saveGoalHooks(t *testing.T) {
	t.Helper()
	origOpen := goalAutonomyOpenHook
	origDefaultPath := goalAutonomyDefaultPathHook
	origGetwd := goalOsGetwdHook
	t.Cleanup(func() {
		goalAutonomyOpenHook = origOpen
		goalAutonomyDefaultPathHook = origDefaultPath
		goalOsGetwdHook = origGetwd
	})
}

func runGoalCmd(t *testing.T, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	cmd := NewGoalCmd()
	var out bytes.Buffer
	setOutAll(cmd, &out)
	cmd.SetArgs(args)
	return &out, cmd.Execute()
}

func openClosedQueue(t *testing.T) *autonomy.Queue {
	t.Helper()
	q, err := autonomy.Open(filepath.Join(t.TempDir(), "goals.db"))
	if err != nil {
		t.Fatal(err)
	}
	q.Close()
	return q
}

func TestGoalAddSuccess(t *testing.T) {
	saveGoalHooks(t)
	db := filepath.Join(t.TempDir(), "goals.db")
	goalAutonomyDefaultPathHook = func() string { return db }
	out, err := runGoalCmd(t, "add", "hello", "--priority", "5", "--retries", "2")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "enqueued") {
		t.Errorf("expected enqueued output, got %q", out.String())
	}
}

func TestGoalAddOpenError(t *testing.T) {
	saveGoalHooks(t)
	goalAutonomyOpenHook = func(string) (*autonomy.Queue, error) { return nil, errors.New("open boom") }
	_, err := runGoalCmd(t, "add", "hello")
	if err == nil || !strings.Contains(err.Error(), "open boom") {
		t.Fatalf("expected open error, got %v", err)
	}
}

func TestGoalAddAddError(t *testing.T) {
	saveGoalHooks(t)
	q := openClosedQueue(t)
	goalAutonomyOpenHook = func(string) (*autonomy.Queue, error) { return q, nil }
	_, err := runGoalCmd(t, "add", "hello")
	if err == nil {
		t.Fatal("expected add error on closed queue")
	}
}

func TestGoalListEmpty(t *testing.T) {
	saveGoalHooks(t)
	db := filepath.Join(t.TempDir(), "goals.db")
	goalAutonomyDefaultPathHook = func() string { return db }
	out, err := runGoalCmd(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "no goals") {
		t.Errorf("expected no goals output, got %q", out.String())
	}
}

func TestGoalListGoals(t *testing.T) {
	saveGoalHooks(t)
	db := filepath.Join(t.TempDir(), "goals.db")
	goalAutonomyDefaultPathHook = func() string { return db }
	q, err := autonomy.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	_, err = q.Add(context.Background(), "prompt", "/tmp", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	out, err := runGoalCmd(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "prompt") || !strings.Contains(out.String(), "ID") {
		t.Errorf("expected goal output, got %q", out.String())
	}
}

func TestGoalListOpenError(t *testing.T) {
	saveGoalHooks(t)
	goalAutonomyOpenHook = func(string) (*autonomy.Queue, error) { return nil, errors.New("open boom") }
	_, err := runGoalCmd(t, "list")
	if err == nil || !strings.Contains(err.Error(), "open boom") {
		t.Fatalf("expected open error, got %v", err)
	}
}

func TestGoalListJSON(t *testing.T) {
	saveGoalHooks(t)
	db := filepath.Join(t.TempDir(), "goals.db")
	goalAutonomyDefaultPathHook = func() string { return db }
	out, err := runGoalCmd(t, "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var result []map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty list, got %v", result)
	}
}

func TestGoalListJSONEncodeError(t *testing.T) {
	saveGoalHooks(t)
	db := filepath.Join(t.TempDir(), "goals.db")
	goalAutonomyDefaultPathHook = func() string { return db }
	cmd := NewGoalCmd()
	cmd.SetArgs([]string{"list", "--json"})
	setOutAll(cmd, goalErrWriter{err: errors.New("encode boom")})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "encode boom") {
		t.Fatalf("expected encode error, got %v", err)
	}
}

func TestGoalListFilter(t *testing.T) {
	saveGoalHooks(t)
	db := filepath.Join(t.TempDir(), "goals.db")
	goalAutonomyDefaultPathHook = func() string { return db }
	q, err := autonomy.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	_, err = q.Add(context.Background(), "pending", "/tmp", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	out, err := runGoalCmd(t, "list", "--status", "pending")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "pending") {
		t.Errorf("expected pending output, got %q", out.String())
	}
	_ = out
}

func TestGoalListError(t *testing.T) {
	saveGoalHooks(t)
	q := openClosedQueue(t)
	goalAutonomyOpenHook = func(string) (*autonomy.Queue, error) { return q, nil }
	_, err := runGoalCmd(t, "list")
	if err == nil {
		t.Fatal("expected list error on closed queue")
	}
}

func TestGoalAddGetwdError(t *testing.T) {
	saveGoalHooks(t)
	db := filepath.Join(t.TempDir(), "goals.db")
	goalAutonomyDefaultPathHook = func() string { return db }
	goalOsGetwdHook = func() (string, error) { return "", errors.New("getwd boom") }
	out, err := runGoalCmd(t, "add", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "enqueued") {
		t.Errorf("expected enqueued output, got %q", out.String())
	}
	_ = out
}

func TestGoalAddDefaultPathOverride(t *testing.T) {
	saveGoalHooks(t)
	db := filepath.Join(t.TempDir(), "goals.db")
	goalAutonomyDefaultPathHook = func() string { return db }
	out, err := runGoalCmd(t, "add", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "enqueued") {
		t.Errorf("expected enqueued output, got %q", out.String())
	}
	if _, err := os.Stat(db); err != nil {
		t.Errorf("expected db to exist: %v", err)
	}
	_ = out
}
