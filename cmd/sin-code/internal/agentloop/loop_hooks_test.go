// SPDX-License-Identifier: MIT
// Purpose: integration tests for hook wiring (#46) and permission
// transitions (#47): tool.pre block, verify.fail feedback, task.complete
// payload, allow / ask-yes / ask-no / deny, --yolo, headless.
package agentloop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

func stubCompletion(captured *[]session.Message) func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
	calls := 0
	return func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
		*captured = append([]session.Message(nil), history...)
		calls++
		if calls == 1 {
			return &Completion{
				ToolCalls: []ToolCall{{ID: "tc1", Name: "sin_bash", Args: map[string]any{"command": "echo hi"}}},
				Raw:       session.Message{Role: "assistant", Content: ""},
			}, nil
		}
		return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
	}
}

func testSession(t *testing.T) *session.Session {
	t.Helper()
	store, err := session.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	sess, err := store.StartOrResume("")
	if err != nil {
		t.Fatal(err)
	}
	return sess
}

func TestToolPreHookBlocks(t *testing.T) {
	ws := t.TempDir()
	var hist []session.Message
	loop := &Loop{
		Gate:       verify.NewGate("off", nil, nil),
		Workspace:  ws,
		Completion: stubCompletion(&hist),
		LocalTool: func(ctx context.Context, name string, args map[string]any) (string, error) {
			t.Fatal("tool must not run when blocked by tool.pre hook")
			return "", nil
		},
		Hooks: hooks.New([]hooks.Hook{{
			Event: hooks.ToolPre, Type: "command",
			Command: "echo no-bash-allowed; exit 2",
		}}),
	}
	res, err := loop.Run(context.Background(), testSession(t), "do something")
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || !res.Verified {
		t.Fatalf("expected verified result, got %+v", res)
	}
	found := false
	for _, m := range hist {
		if m.Role == "tool" && strings.Contains(m.Content, "BLOCKED by hook: no-bash-allowed") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'BLOCKED by hook' tool message in history")
	}
}

func TestVerifyFailFiresHookAndFeedsBack(t *testing.T) {
	ws := t.TempDir()
	marker := filepath.Join(ws, "verify-fail.marker")
	attempt := 0
	runner := func(ctx context.Context, workspace string) (bool, string, error) {
		attempt++
		if attempt == 1 {
			return false, "tests failed: 1 of 3", nil
		}
		return true, "all green", nil
	}
	var hist []session.Message
	loop := &Loop{
		Gate:      verify.NewGate("poc", runner, nil),
		Workspace: ws,
		LocalTool: func(ctx context.Context, name string, args map[string]any) (string, error) { return "ok", nil },
		Hooks: hooks.New([]hooks.Hook{{
			Event: hooks.VerifyFail, Type: "command",
			Command: "touch verify-fail.marker",
		}}),
	}
	loop.Completion = func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
		hist = append([]session.Message(nil), history...)
		return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
	}
	res, err := loop.Run(context.Background(), testSession(t), "task")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Verified || res.Turns != 2 {
		t.Fatalf("expected verified after retry, got %+v", res)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("expected verify.fail hook to fire (marker file missing)")
	}
	feedback := false
	for _, m := range hist {
		if m.Role == "user" && strings.Contains(m.Content, "VERIFICATION FAILED (poc)") {
			feedback = true
		}
	}
	if !feedback {
		t.Fatal("expected verification failure feedback message")
	}
}

func TestTaskCompleteHookFires(t *testing.T) {
	ws := t.TempDir()
	marker := filepath.Join(ws, "complete.marker")
	loop := &Loop{
		Gate:      verify.NewGate("off", nil, nil),
		Workspace: ws,
		Hooks: hooks.New([]hooks.Hook{{
			Event: hooks.TaskComplete, Type: "command",
			Command: "cat > complete.marker",
		}}),
	}
	loop.Completion = func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
		return &Completion{Text: "summary text", Raw: session.Message{Role: "assistant", Content: "summary text"}}, nil
	}
	if _, err := loop.Run(context.Background(), testSession(t), "task"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal("expected task.complete hook to fire:", err)
	}
	payload := string(data)
	for _, want := range []string{"summary text", "turns", "verified"} {
		if !strings.Contains(payload, want) {
			t.Fatalf("task.complete payload missing %q: %s", want, payload)
		}
	}
}

func TestPermissionTransitions(t *testing.T) {
	rules := []permission.Rule{
		{Tool: "tool_allow", Policy: "allow"},
		{Tool: "tool_deny", Policy: "deny"},
		{Tool: "*", Policy: "ask"},
	}
	cases := []struct {
		name     string
		tool     string
		askReply bool
		askSet   bool
		yolo     bool
		headless bool
		want     string
		toolRuns bool
	}{
		{name: "allow", tool: "tool_allow", askSet: true, want: "TOOL-OK", toolRuns: true},
		{name: "ask-yes", tool: "tool_ask", askSet: true, askReply: true, want: "TOOL-OK", toolRuns: true},
		{name: "ask-no", tool: "tool_ask", askSet: true, askReply: false, want: "DENIED by user"},
		{name: "deny", tool: "tool_deny", askSet: true, askReply: true, want: "DENIED by permission policy"},
		{name: "yolo-bypasses-ask", tool: "tool_ask", yolo: true, want: "TOOL-OK", toolRuns: true},
		{name: "headless-ask-denies", tool: "tool_ask", headless: true, want: "DENIED by permission policy"},
		{name: "yolo-never-bypasses-deny", tool: "tool_deny", yolo: true, want: "DENIED by permission policy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			perm := permission.New(rules)
			perm.Yolo = tc.yolo
			perm.Headless = tc.headless
			ran := false
			loop := &Loop{
				Workspace: t.TempDir(),
				Perm:      perm,
				LocalTool: func(ctx context.Context, name string, args map[string]any) (string, error) {
					ran = true
					return "TOOL-OK", nil
				},
			}
			if tc.askSet {
				loop.Ask = func(ToolCall) bool { return tc.askReply }
			}
			out, _ := loop.execute(context.Background(), ToolCall{ID: "x", Name: tc.tool})
			if !strings.Contains(out, tc.want) {
				t.Fatalf("got %q, want substring %q", out, tc.want)
			}
			if ran != tc.toolRuns {
				t.Fatalf("tool ran=%v, want %v", ran, tc.toolRuns)
			}
		})
	}
}
