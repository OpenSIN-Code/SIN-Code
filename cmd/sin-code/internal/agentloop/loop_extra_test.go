// SPDX-License-Identifier: MIT
// Purpose: additional agentloop tests to cover error paths and wiring
// branches not exercised by loop_test.go / loop_hooks_test.go.
package agentloop

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

func TestRun_WithLedger(t *testing.T) {
	s := setupSession(t)
	ws := t.TempDir()
	ldb, err := ledger.Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ldb.Close()

	loop := &Loop{
		Gate:      verify.NewGate("off", nil, nil),
		Workspace: ws,
		SessionID: s.ID,
		Ledger:    ldb,
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	res, err := loop.Run(context.Background(), s, "task")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Verified {
		t.Fatal("expected verified")
	}

	entries, err := ldb.List(context.Background(), s.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected ledger entries")
	}
}

func TestRun_RunOverride(t *testing.T) {
	s := setupSession(t)
	loop := &Loop{
		RunOverride: func(ctx context.Context, sess *session.Session, prompt string) (*Result, error) {
			return &Result{SessionID: sess.ID, Summary: "override", Verified: true, Turns: 1}, nil
		},
	}
	res, err := loop.Run(context.Background(), s, "task")
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary != "override" || !res.Verified {
		t.Fatalf("unexpected result %+v", res)
	}
}

func TestRun_CompletionNil(t *testing.T) {
	s := setupSession(t)
	loop := &Loop{Gate: verify.NewGate("off", nil, nil)}
	_, err := loop.Run(context.Background(), s, "task")
	if err == nil || !strings.Contains(err.Error(), "Completion func not wired") {
		t.Fatalf("expected Completion nil error, got %v", err)
	}
}

func TestRun_ContextCanceled(t *testing.T) {
	s := setupSession(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	loop := &Loop{
		Gate: verify.NewGate("off", nil, nil),
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	_, err := loop.Run(ctx, s, "task")
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestRun_SessionIDAssigned(t *testing.T) {
	s := setupSession(t)
	loop := &Loop{
		Gate:      verify.NewGate("off", nil, nil),
		Workspace: t.TempDir(),
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	if loop.SessionID != "" {
		t.Fatal("precondition: SessionID should be empty")
	}
	if _, err := loop.Run(context.Background(), s, "task"); err != nil {
		t.Fatal(err)
	}
	if loop.SessionID != s.ID {
		t.Fatalf("SessionID not assigned: got %q want %q", loop.SessionID, s.ID)
	}
}

func TestExecute_NoLocalTool(t *testing.T) {
	loop := &Loop{Gate: verify.NewGate("off", nil, nil)}
	out, _ := loop.execute(context.Background(), ToolCall{ID: "x", Name: "sin_read"})
	if !strings.Contains(out, "no LocalTool registered") {
		t.Fatalf("expected no LocalTool error, got %q", out)
	}
}

func TestExecute_LocalToolError(t *testing.T) {
	loop := &Loop{
		Workspace: t.TempDir(),
		LocalTool: func(ctx context.Context, name string, args map[string]any) (string, error) {
			return "", errors.New("boom")
		},
	}
	out, _ := loop.execute(context.Background(), ToolCall{ID: "x", Name: "sin_read"})
	if !strings.Contains(out, "TOOL ERROR: boom") {
		t.Fatalf("expected tool error, got %q", out)
	}
}

func TestExecute_LocalToolErrorWithLessons(t *testing.T) {
	ws := t.TempDir()
	mem, err := lessons.Open(filepath.Join(t.TempDir(), "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer mem.Close()

	loop := &Loop{
		Workspace: ws,
		Lessons:   mem,
		LocalTool: func(ctx context.Context, name string, args map[string]any) (string, error) {
			return "", errors.New("boom")
		},
	}
	out, _ := loop.execute(context.Background(), ToolCall{ID: "x", Name: "sin_read"})
	if !strings.Contains(out, "TOOL ERROR: boom") {
		t.Fatalf("expected tool error, got %q", out)
	}
	entries, err := mem.Query(context.Background(), ws, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected lesson recorded on tool error")
	}
}

func TestExecute_PermissionAskBlocked(t *testing.T) {
	loop := &Loop{
		Workspace: t.TempDir(),
		Perm: permission.New([]permission.Rule{
			{Tool: "*", Policy: "ask"},
		}),
		Ask: func(ToolCall) bool { return true },
		Hooks: hooks.New([]hooks.Hook{{
			Event: hooks.PermissionAsk, Type: "command",
			Command: "echo no-ask; exit 2",
		}}),
	}
	out, _ := loop.execute(context.Background(), ToolCall{ID: "x", Name: "sin_read"})
	if !strings.Contains(out, "DENIED by hook: no-ask") {
		t.Fatalf("expected ask hook block, got %q", out)
	}
}

func TestRun_VerifyPreBlocked(t *testing.T) {
	s := setupSession(t)
	ws := t.TempDir()
	marker := filepath.Join(ws, "verify-pre-blocked.marker")
	turns := 0
	loop := &Loop{
		Gate:      verify.NewGate("off", nil, nil),
		Workspace: ws,
		Hooks: hooks.New([]hooks.Hook{{
			Event: hooks.VerifyPre, Type: "command",
			Command: "if [ ! -f " + marker + " ]; then echo verify-blocked; touch " + marker + "; exit 2; fi",
		}}),
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			turns++
			if turns == 1 {
				return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
			}
			return &Completion{Text: "retry", Raw: session.Message{Role: "assistant", Content: "retry"}}, nil
		},
	}
	res, err := loop.Run(context.Background(), s, "task")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Verified || res.Turns != 2 {
		t.Fatalf("expected verified after 2 turns, got %+v", res)
	}
	found := false
	for _, m := range s.History() {
		if m.Role == "user" && strings.Contains(m.Content, "verify-blocked") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected verify block reason in history")
	}
}

func TestRun_VerifyFailPromptInjects(t *testing.T) {
	s := setupSession(t)
	ws := t.TempDir()
	attempts := 0
	loop := &Loop{
		Gate: verify.NewGate("poc", func(ctx context.Context, ws string) (bool, string, error) {
			attempts++
			if attempts == 1 {
				return false, "fail", nil
			}
			return true, "ok", nil
		}, nil),
		Workspace: ws,
		Hooks: hooks.New([]hooks.Hook{{
			Event: hooks.VerifyFail, Type: "prompt",
			Text: "INJECT: please double-check the tests",
		}}),
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	res, err := loop.Run(context.Background(), s, "task")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Verified || res.Turns != 2 {
		t.Fatalf("expected verified after 2 turns, got %+v", res)
	}
	found := false
	for _, m := range s.History() {
		if m.Role == "user" && strings.Contains(m.Content, "INJECT: please double-check the tests") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected injected prompt in history")
	}
}

func TestRun_VerifyFailRecordsLesson(t *testing.T) {
	s := setupSession(t)
	ws := t.TempDir()
	mem, err := lessons.Open(filepath.Join(t.TempDir(), "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer mem.Close()

	attempts := 0
	loop := &Loop{
		Gate: verify.NewGate("poc", func(ctx context.Context, ws string) (bool, string, error) {
			attempts++
			if attempts == 1 {
				return false, "fail", nil
			}
			return true, "ok", nil
		}, nil),
		Workspace: ws,
		Lessons:   mem,
		Completion: func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	if _, err := loop.Run(context.Background(), s, "task"); err != nil {
		t.Fatal(err)
	}
	entries, err := mem.Query(context.Background(), ws, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected lesson recorded on verify failure")
	}
}

func TestRun_VerifyPreBlocked_SaveHistoryError(t *testing.T) {
	s := setupSession(t)
	ws := t.TempDir()
	marker := filepath.Join(ws, "verify-pre-blocked.marker")
	loop := &Loop{
		Gate:      verify.NewGate("off", nil, nil),
		Workspace: ws,
		Hooks: hooks.New([]hooks.Hook{{
			Event: hooks.VerifyPre, Type: "command",
			Command: "if [ ! -f " + marker + " ]; then echo blocked; touch " + marker + "; exit 2; fi",
		}}),
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	saveHistoryHook = func(sess *session.Session, msgs []session.Message) error {
		return errors.New("save failed")
	}
	defer func() { saveHistoryHook = nil }()

	_, err := loop.Run(context.Background(), s, "task")
	if err == nil || !strings.Contains(err.Error(), "save failed") {
		t.Fatalf("expected save history error, got %v", err)
	}
}

func TestRun_VerifyFail_SaveHistoryError(t *testing.T) {
	s := setupSession(t)
	loop := &Loop{
		Gate:      verify.NewGate("poc", func(ctx context.Context, ws string) (bool, string, error) { return false, "fail", nil }, nil),
		Workspace: t.TempDir(),
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "x", Raw: session.Message{Role: "assistant", Content: "x"}}, nil
		},
	}
	saveHistoryHook = func(sess *session.Session, msgs []session.Message) error {
		return errors.New("save failed")
	}
	defer func() { saveHistoryHook = nil }()

	_, err := loop.Run(context.Background(), s, "task")
	if err == nil || !strings.Contains(err.Error(), "save failed") {
		t.Fatalf("expected save history error, got %v", err)
	}
}

func TestRun_VerifyPass_SaveHistoryError(t *testing.T) {
	s := setupSession(t)
	loop := &Loop{
		Gate:      verify.NewGate("poc", func(ctx context.Context, ws string) (bool, string, error) { return true, "ok", nil }, nil),
		Workspace: t.TempDir(),
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	saveHistoryHook = func(sess *session.Session, msgs []session.Message) error {
		return errors.New("save failed")
	}
	defer func() { saveHistoryHook = nil }()

	_, err := loop.Run(context.Background(), s, "task")
	if err == nil || !strings.Contains(err.Error(), "save failed") {
		t.Fatalf("expected save history error, got %v", err)
	}
}

func TestRun_ToolCalls_SaveHistoryError(t *testing.T) {
	s := setupSession(t)
	loop := &Loop{
		Gate:      verify.NewGate("off", nil, nil),
		Workspace: t.TempDir(),
		LocalTool: func(ctx context.Context, name string, args map[string]any) (string, error) { return "ok", nil },
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{
				ToolCalls: []ToolCall{{ID: "t1", Name: "sin_read", Args: map[string]any{}}},
				Raw:       session.Message{Role: "assistant", Content: ""},
			}, nil
		},
	}
	saveHistoryHook = func(sess *session.Session, msgs []session.Message) error {
		return errors.New("save failed")
	}
	defer func() { saveHistoryHook = nil }()

	_, err := loop.Run(context.Background(), s, "task")
	if err == nil || !strings.Contains(err.Error(), "save failed") {
		t.Fatalf("expected save history error, got %v", err)
	}
}
