// SPDX-License-Identifier: MIT
// Purpose: verify the closed learning loop — recorded failures from a
// previous run are injected as a briefing into the next run.
package agentloop

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

func TestKnowledgeBriefingInjectedOnNextRun(t *testing.T) {
	ws := t.TempDir()
	mem, err := lessons.Open(filepath.Join(t.TempDir(), "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer mem.Close()
	ctx := context.Background()

	lesson := lessons.Entry{
		Type: lessons.TypeFailedVerification, Workspace: ws,
		Context: map[string]any{"mode": "poc"},
		Lesson:  "tests fail when DB migrations are skipped",
	}
	_ = mem.Record(ctx, lesson)
	_ = mem.Record(ctx, lesson)

	var captured []session.Message
	loop := &Loop{
		Gate:      verify.NewGate("off", nil, nil),
		Workspace: ws,
		Lessons:   mem,
	}
	loop.Completion = func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
		captured = append([]session.Message(nil), history...)
		return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
	}

	if _, err := loop.Run(ctx, testSession(t), "new task"); err != nil {
		t.Fatal(err)
	}

	found := false
	for _, m := range captured {
		if strings.Contains(m.Content, "WORKSPACE KNOWLEDGE") &&
			strings.Contains(m.Content, "DB migrations are skipped") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected knowledge briefing in first-turn history")
	}
}

func TestSingleOccurrenceIsNotInjected(t *testing.T) {
	ws := t.TempDir()
	mem, err := lessons.Open(filepath.Join(t.TempDir(), "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer mem.Close()
	_ = mem.Record(context.Background(), lessons.Entry{
		Type: lessons.TypeToolError, Workspace: ws, Lesson: "one-off noise",
	})

	loop := &Loop{Gate: verify.NewGate("off", nil, nil), Workspace: ws, Lessons: mem}
	var captured []session.Message
	loop.Completion = func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
		captured = append([]session.Message(nil), history...)
		return &Completion{Text: "ok", Raw: session.Message{Role: "assistant", Content: "ok"}}, nil
	}
	if _, err := loop.Run(context.Background(), testSession(t), "task"); err != nil {
		t.Fatal(err)
	}
	for _, m := range captured {
		if strings.Contains(m.Content, "one-off noise") {
			t.Fatal("single-occurrence entry must not be injected (noise filter)")
		}
	}
}
