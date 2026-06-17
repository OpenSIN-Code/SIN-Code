// SPDX-License-Identifier: MIT
// Purpose: E2E tests proving handleAgentRunnerEvent actually emits
// VerifyUpdateMsg, ToolCallTreeMsg, and ToolCallUpdateMsg via Program.Send.
// The existing coverage tests (tui_coverage_test.go) only verify chat-message
// kinds and the Closed path — none wire a fakeProgram and assert the
// bubbletea messages dispatched to the render loop.
package tui

import (
	"testing"

	agentrunner "github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/tui"
)

// ── message-finder helpers ───────────────────────────────────────────────────
// These traverse the []any slice captured by fakeProgram.Send and return the
// first message of the requested concrete type.

func findToolCallTreeMsg(msgs []any) (ToolCallTreeMsg, bool) {
	for _, msg := range msgs {
		if t, ok := msg.(ToolCallTreeMsg); ok {
			return t, true
		}
	}
	return ToolCallTreeMsg{}, false
}

func findToolCallUpdateMsg(msgs []any) (ToolCallUpdateMsg, bool) {
	for _, msg := range msgs {
		if t, ok := msg.(ToolCallUpdateMsg); ok {
			return t, true
		}
	}
	return ToolCallUpdateMsg{}, false
}

func findVerifyUpdateMsg(msgs []any) (VerifyUpdateMsg, bool) {
	for _, msg := range msgs {
		if t, ok := msg.(VerifyUpdateMsg); ok {
			return t, true
		}
	}
	return VerifyUpdateMsg{}, false
}

// ── tests ─────────────────────────────────────────────────────────────────────

// TestHandleAgentRunnerEventToolStartEmitsToolCallTreeMsg verifies that a
// non-result EventTool dispatches a ToolCallTreeMsg carrying a running node.
func TestHandleAgentRunnerEventToolStartEmitsToolCallTreeMsg(t *testing.T) {
	sender := newFakeProgram()
	m := NewModel()
	m.Program = sender

	m.handleAgentRunnerEvent(AgentRunnerMsg{
		Event: agentrunner.AgentEvent{
			Kind:     agentrunner.EventTool,
			ToolName: "sin_bash",
			Detail:   "tool: sin_bash",
		},
	})

	treeMsg, ok := findToolCallTreeMsg(sender.msgs)
	if !ok {
		t.Fatalf("expected ToolCallTreeMsg to be sent, got msgs: %v", sender.msgs)
	}
	if treeMsg.Node == nil {
		t.Fatal("ToolCallTreeMsg.Node is nil")
	}
	if treeMsg.Node.Tool != "sin_bash" {
		t.Errorf("node.Tool = %q, want %q", treeMsg.Node.Tool, "sin_bash")
	}
	if treeMsg.Node.Status != "running" {
		t.Errorf("node.Status = %q, want %q", treeMsg.Node.Status, "running")
	}
	if treeMsg.Node.ID == "" {
		t.Error("node.ID should be non-empty (generated from timestamp)")
	}
	if treeMsg.ParentID != "" {
		t.Errorf("ParentID = %q, want empty (root-level call)", treeMsg.ParentID)
	}
}

// TestHandleAgentRunnerEventToolResultEmitsToolCallUpdateMsg verifies that a
// result EventTool dispatches a ToolCallUpdateMsg with status "success".
func TestHandleAgentRunnerEventToolResultEmitsToolCallUpdateMsg(t *testing.T) {
	sender := newFakeProgram()
	m := NewModel()
	m.Program = sender

	// Pre-populate the tool tree with a running node so the update has a
	// target — matches the real runtime where a start event precedes the
	// result.
	m.ToolTree = &ToolCallTree{}
	m.ToolTree.AddNode("", &ToolCallNode{
		ID:     "tool-1-sin_bash",
		Tool:   "sin_bash",
		Status: "running",
	})

	m.handleAgentRunnerEvent(AgentRunnerMsg{
		Event: agentrunner.AgentEvent{
			Kind:     agentrunner.EventTool,
			ToolName: "sin_bash",
			Detail:   "tool result: build ok",
		},
	})

	// A ToolCallTreeMsg should NOT be emitted for a result event.
	if _, ok := findToolCallTreeMsg(sender.msgs); ok {
		t.Error("did not expect ToolCallTreeMsg for a tool result event")
	}

	updMsg, ok := findToolCallUpdateMsg(sender.msgs)
	if !ok {
		t.Fatalf("expected ToolCallUpdateMsg to be sent, got msgs: %v", sender.msgs)
	}
	if updMsg.ID != "sin_bash" {
		t.Errorf("update.ID = %q, want %q", updMsg.ID, "sin_bash")
	}
	if updMsg.Status != "success" {
		t.Errorf("update.Status = %q, want %q", updMsg.Status, "success")
	}
	if updMsg.Output != "tool result: build ok" {
		t.Errorf("update.Output = %q, want %q", updMsg.Output, "tool result: build ok")
	}
}

// TestHandleAgentRunnerEventVerifyEmitsVerifyUpdateMsg verifies that an
// EventVerify with "PASSED" in the detail dispatches a VerifyUpdateMsg with
// State = VerifyPassed.
func TestHandleAgentRunnerEventVerifyEmitsVerifyUpdateMsg(t *testing.T) {
	sender := newFakeProgram()
	m := NewModel()
	m.Program = sender

	m.handleAgentRunnerEvent(AgentRunnerMsg{
		Event: agentrunner.AgentEvent{
			Kind:   agentrunner.EventVerify,
			Detail: "verify: PASSED - all checks green",
			Result: "evidence: 3/3 tests passed",
		},
	})

	vMsg, ok := findVerifyUpdateMsg(sender.msgs)
	if !ok {
		t.Fatalf("expected VerifyUpdateMsg to be sent, got msgs: %v", sender.msgs)
	}
	if vMsg.State != VerifyPassed {
		t.Errorf("State = %v, want %v (VerifyPassed)", vMsg.State, VerifyPassed)
	}
	if vMsg.Mode != "poc" {
		t.Errorf("Mode = %q, want %q", vMsg.Mode, "poc")
	}
	if vMsg.Target != "verify: PASSED - all checks green" {
		t.Errorf("Target = %q, want detail string", vMsg.Target)
	}
	if vMsg.Evidence != "evidence: 3/3 tests passed" {
		t.Errorf("Evidence = %q, want result string", vMsg.Evidence)
	}
}

// TestHandleAgentRunnerEventVerifyFailed verifies that an EventVerify with
// "FAILED" in the detail dispatches a VerifyUpdateMsg with State =
// VerifyFailed.
func TestHandleAgentRunnerEventVerifyFailed(t *testing.T) {
	sender := newFakeProgram()
	m := NewModel()
	m.Program = sender

	m.handleAgentRunnerEvent(AgentRunnerMsg{
		Event: agentrunner.AgentEvent{
			Kind:   agentrunner.EventVerify,
			Detail: "verify: FAILED - 2 tests failed",
		},
	})

	vMsg, ok := findVerifyUpdateMsg(sender.msgs)
	if !ok {
		t.Fatalf("expected VerifyUpdateMsg to be sent, got msgs: %v", sender.msgs)
	}
	if vMsg.State != VerifyFailed {
		t.Errorf("State = %v, want %v (VerifyFailed)", vMsg.State, VerifyFailed)
	}
}

// TestHandleAgentRunnerEventDoneWithVerified verifies that an EventDone whose
// Result contains "verified" dispatches a VerifyUpdateMsg with State =
// VerifyPassed.
func TestHandleAgentRunnerEventDoneWithVerified(t *testing.T) {
	sender := newFakeProgram()
	m := NewModel()
	m.Program = sender

	m.handleAgentRunnerEvent(AgentRunnerMsg{
		Event: agentrunner.AgentEvent{
			Kind:   agentrunner.EventDone,
			Result: "task completed and verified",
		},
	})

	vMsg, ok := findVerifyUpdateMsg(sender.msgs)
	if !ok {
		t.Fatalf("expected VerifyUpdateMsg to be sent, got msgs: %v", sender.msgs)
	}
	if vMsg.State != VerifyPassed {
		t.Errorf("State = %v, want %v (VerifyPassed)", vMsg.State, VerifyPassed)
	}
	if vMsg.Target != "agent run complete" {
		t.Errorf("Target = %q, want %q", vMsg.Target, "agent run complete")
	}
	if vMsg.Evidence != "task completed and verified" {
		t.Errorf("Evidence = %q, want result string", vMsg.Evidence)
	}
}

// TestHandleAgentRunnerEventDoneWithoutVerified verifies that an EventDone
// whose Result does NOT contain "verified" does NOT dispatch a VerifyUpdateMsg.
func TestHandleAgentRunnerEventDoneWithoutVerified(t *testing.T) {
	sender := newFakeProgram()
	m := NewModel()
	m.Program = sender

	m.handleAgentRunnerEvent(AgentRunnerMsg{
		Event: agentrunner.AgentEvent{
			Kind:   agentrunner.EventDone,
			Result: "task completed",
		},
	})

	if _, ok := findVerifyUpdateMsg(sender.msgs); ok {
		t.Errorf("did not expect VerifyUpdateMsg when result lacks 'verified', got msgs: %v", sender.msgs)
	}
}

// TestHandleAgentRunnerEventToolTreeInitialized verifies that
// handleAgentRunnerEvent lazily initialises m.ToolTree before dispatching any
// tool or verify messages.
func TestHandleAgentRunnerEventToolTreeInitialized(t *testing.T) {
	sender := newFakeProgram()
	m := NewModel()
	m.Program = sender
	if m.ToolTree != nil {
		t.Fatal("ToolTree should start nil for this test")
	}

	m.handleAgentRunnerEvent(AgentRunnerMsg{
		Event: agentrunner.AgentEvent{
			Kind:     agentrunner.EventTool,
			ToolName: "sin_read",
			Detail:   "tool: sin_read",
		},
	})

	if m.ToolTree == nil {
		t.Error("expected ToolTree to be lazily initialised after handling an event")
	}
}

// TestHandleAgentRunnerEventClosedClearsRunner verifies that a Closed message
// sets m.AgentRunner to nil and returns early without dispatching any Program
// messages.
func TestHandleAgentRunnerEventClosedClearsRunner(t *testing.T) {
	sender := newFakeProgram()
	m := NewModel()
	m.Program = sender
	m.AgentRunner = &agentrunner.AgentRunner{}

	m.handleAgentRunnerEvent(AgentRunnerMsg{Closed: true})

	if m.AgentRunner != nil {
		t.Error("expected AgentRunner to be nil after Closed event")
	}
	if len(sender.msgs) != 0 {
		t.Errorf("expected no messages to be sent on Closed, got %d: %v", len(sender.msgs), sender.msgs)
	}
}
