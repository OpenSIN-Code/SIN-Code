// SPDX-License-Identifier: MIT
// Purpose: coverage tests for session_cmd.go — exercises list/show/rm using
// package-level hooks so tests never need a real sessions.db.
// Docs: session_cmd.go
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

type sessionsErrWriter struct{ err error }

func (e sessionsErrWriter) Write(p []byte) (int, error) { return 0, e.err }

func saveSessionsHooks(t *testing.T) {
	t.Helper()
	origOpen := sessionsOpenHook
	origDefault := sessionsDefaultPathHook
	origList := sessionsStoreListHook
	origStart := sessionsStoreStartOrResumeHook
	origDelete := sessionsStoreDeleteHook
	t.Cleanup(func() {
		sessionsOpenHook = origOpen
		sessionsDefaultPathHook = origDefault
		sessionsStoreListHook = origList
		sessionsStoreStartOrResumeHook = origStart
		sessionsStoreDeleteHook = origDelete
	})
}

func runSessionsCmd(t *testing.T, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	cmd := NewSessionsCmd()
	var out bytes.Buffer
	setOutAll(cmd, &out)
	cmd.SetArgs(args)
	return &out, cmd.Execute()
}

func fakeStore(t *testing.T) *session.Store {
	t.Helper()
	// Using a real in-memory store through the session package is the simplest
	// way to get a *session.Store with valid History()/Close() behaviour.
	store, err := session.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestSessionsCmd_NewSessionsCmd(t *testing.T) {
	cmd := NewSessionsCmd()
	if cmd.Use != "sessions" {
		t.Errorf("Use = %q, want sessions", cmd.Use)
	}
	names := []string{}
	for _, c := range cmd.Commands() {
		names = append(names, c.Name())
	}
	joined := strings.Join(names, " ")
	for _, want := range []string{"list", "show", "rm"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing subcommand %q in %q", want, joined)
		}
	}
}

func TestSessionsCmd_List_OpenError(t *testing.T) {
	saveSessionsHooks(t)
	sessionsOpenHook = func(string) (*session.Store, error) { return nil, errors.New("open boom") }
	_, err := runSessionsCmd(t, "list")
	if err == nil || !strings.Contains(err.Error(), "open boom") {
		t.Fatalf("expected open error, got %v", err)
	}
}

func TestSessionsCmd_List_ListError(t *testing.T) {
	saveSessionsHooks(t)
	_, err := runSessionsCmd(t, "list", "--db", ":memory:")
	sessionsStoreListHook = func(*session.Store) ([]session.Info, error) { return nil, errors.New("list boom") }
	_, err = runSessionsCmd(t, "list", "--db", ":memory:")
	if err == nil || !strings.Contains(err.Error(), "list boom") {
		t.Fatalf("expected list error, got %v", err)
	}
}

func TestSessionsCmd_List_Empty(t *testing.T) {
	saveSessionsHooks(t)
	out, err := runSessionsCmd(t, "list", "--db", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "no sessions") {
		t.Errorf("expected no sessions, got %q", out.String())
	}
}

func TestSessionsCmd_List_JSON(t *testing.T) {
	saveSessionsHooks(t)
	out, err := runSessionsCmd(t, "list", "--db", ":memory:", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty list, got %v", got)
	}
}

func TestSessionsCmd_List_JSONEncodeError(t *testing.T) {
	saveSessionsHooks(t)
	cmd := NewSessionsCmd()
	cmd.SetArgs([]string{"list", "--db", ":memory:", "--json"})
	setOutAll(cmd, sessionsErrWriter{err: errors.New("encode boom")})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "encode boom") {
		t.Fatalf("expected encode error, got %v", err)
	}
}

func TestSessionsCmd_List_WithRows(t *testing.T) {
	saveSessionsHooks(t)
	store := fakeStore(t)
	defer store.Close()
	sess, err := store.StartOrResume("")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.SaveHistory([]session.Message{{Role: "user", Content: "hello"}}); err != nil {
		t.Fatal(err)
	}

	sessionsOpenHook = func(string) (*session.Store, error) { return store, nil }
	out, err := runSessionsCmd(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), sess.ID) {
		t.Errorf("expected session id %q in output, got %q", sess.ID, out.String())
	}
	if !strings.Contains(out.String(), "CREATED") {
		t.Errorf("expected header, got %q", out.String())
	}
}

func TestSessionsCmd_Show_OpenError(t *testing.T) {
	saveSessionsHooks(t)
	sessionsOpenHook = func(string) (*session.Store, error) { return nil, errors.New("open boom") }
	_, err := runSessionsCmd(t, "show", "abc")
	if err == nil || !strings.Contains(err.Error(), "open boom") {
		t.Fatalf("expected open error, got %v", err)
	}
}

func TestSessionsCmd_Show_StartOrResumeError(t *testing.T) {
	saveSessionsHooks(t)
	sessionsStoreStartOrResumeHook = func(*session.Store, string) (*session.Session, error) {
		return nil, errors.New("resume boom")
	}
	_, err := runSessionsCmd(t, "show", "abc", "--db", ":memory:")
	if err == nil || !strings.Contains(err.Error(), "resume boom") {
		t.Fatalf("expected resume error, got %v", err)
	}
}

func TestSessionsCmd_Show_WithHistory(t *testing.T) {
	saveSessionsHooks(t)
	store := fakeStore(t)
	defer store.Close()
	sess, err := store.StartOrResume("")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.SaveHistory([]session.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "", ToolCalls: json.RawMessage(`[{"name":"x"}]`)},
	}); err != nil {
		t.Fatal(err)
	}

	sessionsOpenHook = func(string) (*session.Store, error) { return store, nil }
	out, err := runSessionsCmd(t, "show", sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "USER") {
		t.Errorf("expected USER label, got %q", out.String())
	}
	if !strings.Contains(out.String(), "[tool calls]") {
		t.Errorf("expected tool call marker, got %q", out.String())
	}
}

func TestSessionsCmd_Rm_OpenError(t *testing.T) {
	saveSessionsHooks(t)
	sessionsOpenHook = func(string) (*session.Store, error) { return nil, errors.New("open boom") }
	_, err := runSessionsCmd(t, "rm", "abc")
	if err == nil || !strings.Contains(err.Error(), "open boom") {
		t.Fatalf("expected open error, got %v", err)
	}
}

func TestSessionsCmd_Rm_DeleteError(t *testing.T) {
	saveSessionsHooks(t)
	sessionsStoreDeleteHook = func(*session.Store, string) error { return errors.New("delete boom") }
	_, err := runSessionsCmd(t, "rm", "abc", "--db", ":memory:")
	if err == nil || !strings.Contains(err.Error(), "delete boom") {
		t.Fatalf("expected delete error, got %v", err)
	}
}

func TestSessionsCmd_Rm_Success(t *testing.T) {
	saveSessionsHooks(t)
	out, err := runSessionsCmd(t, "rm", "abc", "--db", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "deleted session abc") {
		t.Errorf("expected delete message, got %q", out.String())
	}
}

func TestSessionsCmd_DefaultPathHook(t *testing.T) {
	saveSessionsHooks(t)
	sessionsDefaultPathHook = func() string { return ":memory:" }
	out, err := runSessionsCmd(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "no sessions") {
		t.Errorf("expected no sessions, got %q", out.String())
	}
}
