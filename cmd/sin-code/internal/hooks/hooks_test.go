// SPDX-License-Identifier: MIT
// Purpose: hook engine tests (mandate C7, AGENTS.md §8).
package hooks

import (
	"context"
	"testing"
)

func TestCommandHookBlocking(t *testing.T) {
	e := New([]Hook{
		{Event: "tool.pre", Matcher: "sin_bash", Type: "command",
			Command: `echo "rm -rf is forbidden"; exit 2`},
	})
	res := e.Fire(context.Background(), Payload{Event: ToolPre, Name: "sin_bash"})
	if !res.Blocked {
		t.Fatal("expected block on exit 2")
	}
	if res.BlockReason != "rm -rf is forbidden" {
		t.Fatalf("unexpected reason: %q", res.BlockReason)
	}
}

func TestNonBlockableEventIgnoresExit2(t *testing.T) {
	e := New([]Hook{
		{Event: "tool.post", Type: "command", Command: `exit 2`},
	})
	res := e.Fire(context.Background(), Payload{Event: ToolPost, Name: "sin_edit"})
	if res.Blocked {
		t.Fatal("tool.post must not be blockable")
	}
}

func TestGlobEventAndMatcher(t *testing.T) {
	e := New([]Hook{
		{Event: "tool.*", Matcher: "sin_*", Type: "prompt", Text: "remember the style guide"},
	})
	res := e.Fire(context.Background(), Payload{Event: ToolPre, Name: "sin_edit"})
	if len(res.PromptInjects) != 1 {
		t.Fatal("glob hook did not fire")
	}
	res = e.Fire(context.Background(), Payload{Event: ToolPre, Name: "browser__click"})
	if len(res.PromptInjects) != 0 {
		t.Fatal("matcher should have filtered non-sin tool")
	}
}

func TestFailingHookDegradesToWarning(t *testing.T) {
	e := New([]Hook{
		{Event: "session.start", Type: "command", Command: `exit 1`},
	})
	res := e.Fire(context.Background(), Payload{Event: SessionStart})
	if res.Blocked {
		t.Fatal("exit 1 must warn, not block")
	}
}

func TestPromptHookCollects(t *testing.T) {
	e := New([]Hook{
		{Event: "session.start", Type: "prompt", Text: "first"},
		{Event: "session.*", Type: "prompt", Text: "second"},
	})
	res := e.Fire(context.Background(), Payload{Event: SessionStart})
	if len(res.PromptInjects) != 2 {
		t.Fatalf("want 2 prompt injects, got %d", len(res.PromptInjects))
	}
}

func TestUnknownTypeIgnored(t *testing.T) {
	e := New([]Hook{
		{Event: "session.start", Type: "garbage", Text: "ignored"},
	})
	res := e.Fire(context.Background(), Payload{Event: SessionStart})
	if len(res.PromptInjects) != 0 {
		t.Fatal("unknown type should not inject")
	}
	if res.Blocked {
		t.Fatal("unknown type should not block")
	}
}

func TestWebhookFiresWithoutBlocking(t *testing.T) {
	// Local httptest server that returns 200 — proves the path was called.
	// We don't actually start a server (kept off the test surface) but the
	// hook must not crash on a refused connection.
	e := New([]Hook{
		{Event: "task.complete", Type: "webhook", URL: "http://127.0.0.1:1/none"},
	})
	res := e.Fire(context.Background(), Payload{Event: TaskComplete})
	if res.Blocked {
		t.Fatal("webhook must never block (best-effort)")
	}
}

func TestFirstBlockWins(t *testing.T) {
	e := New([]Hook{
		{Event: "push.pre", Type: "command", Command: `echo a; exit 2`},
		{Event: "push.pre", Type: "command", Command: `echo b; exit 2`},
	})
	res := e.Fire(context.Background(), Payload{Event: PushPre})
	if !res.Blocked || res.BlockReason != "a" {
		t.Fatalf("first blocker must win: %+v", res)
	}
}
