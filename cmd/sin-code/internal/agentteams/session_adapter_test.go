// SPDX-License-Identifier: MIT
// Purpose: Tests for cross-harness session normalization (issue #331).
package agentteams

import (
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

func TestNormalizeSinCode(t *testing.T) {
	adapter := NewSessionAdapter()
	sess := &session.Session{
		ID: "sin-001",
	}
	// Populate history via SaveHistory would require a real DB.
	// Instead, test with empty history (valid edge case).
	ns, err := adapter.NormalizeSinCode(sess)
	if err != nil {
		t.Fatal(err)
	}
	if ns.ID != "sin-001" || ns.Harness != "sin-code" {
		t.Fatalf("unexpected: %+v", ns)
	}
	if len(ns.Messages) != 0 {
		t.Fatalf("want 0 messages, got %d", len(ns.Messages))
	}
}

func TestNormalizeSinCodeNil(t *testing.T) {
	adapter := NewSessionAdapter()
	if _, err := adapter.NormalizeSinCode(nil); err == nil {
		t.Fatal("nil session must error")
	}
}

func TestNormalizeClaudeCode(t *testing.T) {
	adapter := NewSessionAdapter()
	data := []byte(`{"session_id":"claude-123","role":"user","content":"hello","timestamp":"2026-06-18T10:00:00Z"}
{"session_id":"claude-123","role":"assistant","content":"hi there","timestamp":"2026-06-18T10:00:01Z"}`)
	ns, err := adapter.NormalizeClaudeCode(data)
	if err != nil {
		t.Fatal(err)
	}
	if ns.ID != "claude-123" || ns.Harness != "claude-code" {
		t.Fatalf("unexpected: %+v", ns)
	}
	if len(ns.Messages) != 2 {
		t.Fatalf("want 2 messages, got %d", len(ns.Messages))
	}
	if ns.Messages[0].Role != "user" || ns.Messages[0].Content != "hello" {
		t.Fatalf("unexpected first message: %+v", ns.Messages[0])
	}
	if ns.Messages[1].Role != "assistant" || ns.Messages[1].Content != "hi there" {
		t.Fatalf("unexpected second message: %+v", ns.Messages[1])
	}
}

func TestNormalizeClaudeCodeEmpty(t *testing.T) {
	adapter := NewSessionAdapter()
	ns, err := adapter.NormalizeClaudeCode([]byte{})
	if err != nil {
		t.Fatal(err)
	}
	if ns.Harness != "claude-code" {
		t.Fatalf("harness should be claude-code: %+v", ns)
	}
	if len(ns.Messages) != 0 {
		t.Fatalf("want 0 messages, got %d", len(ns.Messages))
	}
}

func TestNormalizeClaudeCodeInvalid(t *testing.T) {
	adapter := NewSessionAdapter()
	if _, err := adapter.NormalizeClaudeCode([]byte("not json\nalso not json")); err == nil {
		t.Fatal("invalid JSON must error")
	}
}

func TestNormalizeOpenCode(t *testing.T) {
	adapter := NewSessionAdapter()
	data := []byte(`{
		"id": "oc-001",
		"title": "test session",
		"messages": [
			{"role": "user", "content": "write a function"},
			{"role": "assistant", "content": "func foo() {}", "tool_calls": ["write", "edit"]}
		]
	}`)
	ns, err := adapter.NormalizeOpenCode(data)
	if err != nil {
		t.Fatal(err)
	}
	if ns.ID != "oc-001" || ns.Harness != "opencode" || ns.Title != "test session" {
		t.Fatalf("unexpected: %+v", ns)
	}
	if len(ns.Messages) != 2 {
		t.Fatalf("want 2 messages, got %d", len(ns.Messages))
	}
	if ns.Messages[0].Role != "user" || ns.Messages[0].Content != "write a function" {
		t.Fatalf("unexpected first message: %+v", ns.Messages[0])
	}
	if len(ns.Messages[1].ToolCalls) != 2 || ns.Messages[1].ToolCalls[0] != "write" {
		t.Fatalf("unexpected tool calls: %+v", ns.Messages[1].ToolCalls)
	}
}

func TestNormalizeOpenCodeInvalid(t *testing.T) {
	adapter := NewSessionAdapter()
	if _, err := adapter.NormalizeOpenCode([]byte("not json")); err == nil {
		t.Fatal("invalid JSON must error")
	}
}

func TestDetectHarnessClaudeCode(t *testing.T) {
	adapter := NewSessionAdapter()
	data := []byte(`{"session_id":"x","role":"user","content":"hi"}
{"session_id":"x","role":"assistant","content":"hello"}`)
	if got := adapter.DetectHarness(data); got != "claude-code" {
		t.Fatalf("want claude-code, got %q", got)
	}
}

func TestDetectHarnessOpenCode(t *testing.T) {
	adapter := NewSessionAdapter()
	data := []byte(`{"id":"x","messages":[{"role":"user","content":"hi"}]}`)
	if got := adapter.DetectHarness(data); got != "opencode" {
		t.Fatalf("want opencode, got %q", got)
	}
}

func TestDetectHarnessUnknown(t *testing.T) {
	adapter := NewSessionAdapter()
	if got := adapter.DetectHarness([]byte("")); got != "unknown" {
		t.Fatalf("want unknown, got %q", got)
	}
	if got := adapter.DetectHarness([]byte("random text")); got != "unknown" {
		t.Fatalf("want unknown, got %q", got)
	}
}
