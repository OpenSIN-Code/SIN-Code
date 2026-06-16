// SPDX-License-Identifier: MIT
// Purpose: tests for the loop health dashboard (loop-004) — status icons and
// the JSON tree builder over a parent/child goal set.
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
)

func TestStatusIcon(t *testing.T) {
	cases := map[autonomy.GoalStatus]string{
		autonomy.StatusVerified:  "[x]",
		autonomy.StatusRunning:   "[>]",
		autonomy.StatusBlocked:   "[~]",
		autonomy.StatusFailed:    "[!]",
		autonomy.StatusExhausted: "[X]",
		autonomy.StatusPending:   "[ ]",
	}
	for s, want := range cases {
		if got := statusIcon(s); got != want {
			t.Errorf("statusIcon(%q)=%q want %q", s, got, want)
		}
	}
}

func TestPrintStatusJSONTree(t *testing.T) {
	roots := []autonomy.Goal{{ID: 1, Status: autonomy.StatusBlocked, Prompt: "parent", Depth: 0}}
	children := map[int64][]autonomy.Goal{
		1: {
			{ID: 2, ParentID: 1, Status: autonomy.StatusVerified, Prompt: "child-a", Depth: 1},
			{ID: 3, ParentID: 1, Status: autonomy.StatusPending, Prompt: "child-b", Depth: 1, Continuations: 2},
		},
	}
	counts := map[autonomy.GoalStatus]int{
		autonomy.StatusBlocked: 1, autonomy.StatusVerified: 1, autonomy.StatusPending: 1,
	}

	out := captureStdout(t, func() {
		if err := printStatusJSON(roots, children, counts); err != nil {
			t.Fatalf("printStatusJSON: %v", err)
		}
	})

	var parsed struct {
		Summary map[string]int `json:"summary"`
		Goals   []struct {
			ID       int64 `json:"id"`
			Children []struct {
				ID            int64 `json:"id"`
				Continuations int   `json:"continuations"`
			} `json:"children"`
		} `json:"goals"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(parsed.Goals) != 1 || parsed.Goals[0].ID != 1 {
		t.Fatalf("expected one root goal #1, got %+v", parsed.Goals)
	}
	if len(parsed.Goals[0].Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(parsed.Goals[0].Children))
	}
	if parsed.Summary["blocked"] != 1 || parsed.Summary["verified"] != 1 {
		t.Fatalf("unexpected summary: %+v", parsed.Summary)
	}
	if parsed.Goals[0].Children[1].Continuations != 2 {
		t.Fatalf("continuation count not preserved: %+v", parsed.Goals[0].Children[1])
	}
}

// captureStdout runs fn while capturing everything written to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	fn()
	_ = w.Close()
	os.Stdout = orig
	return <-done
}
