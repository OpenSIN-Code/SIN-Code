// SPDX-License-Identifier: MIT
// Purpose: TUI-side glue that wires cmd/sin-code/internal/tui.AgentRunner
// into the Bubbletea event loop. Issue #53: when the user submits a
// chat prompt or selects a skill-palette entry, the AgentRunner runs
// the full agentloop (with tools, permission gates, verification) and
// streams AgentEvents back into the model.
//
// This file lives in cmd/sin-code/tui/ (not internal/tui/) because the
// AgentRunner is in cmd/sin-code/internal/tui/ and Go's internal-package
// visibility rules prevent internal/tui/ from importing it.
package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	agentrunner "github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/tui"
)

// AgentRunnerMsg is the tea.Msg that carries one AgentEvent into the
// TUI's update loop. Closed is true when the runner has shut down its
// event channel; the TUI should drop the AgentRunner reference.
type AgentRunnerMsg struct {
	Event  agentrunner.AgentEvent
	Closed bool
}

// initAgentRunner lazily initializes the agent runner used for chat
// submits. Returns the runner or nil if it could not be constructed.
// The caller is responsible for calling Close() when the TUI exits.
func (m *Model) initAgentRunner() *agentrunner.AgentRunner {
	if m.AgentRunner != nil {
		return m.AgentRunner
	}
	ws := m.Workspace
	if ws == "" {
		ws = "."
	}
	r, err := agentrunner.NewAgentRunner(m.ctx(), agentrunner.Config{
		Workspace: ws,
		Headless:  false, // TUI is interactive — show the Ask dialog
		Yolo:      false, // never auto-allow in interactive mode
		MaxTurns:  20,    // tighter than CLI default to keep the UI snappy
	})
	if err != nil {
		return nil
	}
	m.AgentRunner = r
	return r
}

// listenAgentRunnerCmd returns a tea.Cmd that produces the next
// AgentRunnerMsg from the runner's event channel.
func listenAgentRunnerCmd(r *agentrunner.AgentRunner) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-r.EventsChannel()
		if !ok {
			return AgentRunnerMsg{Closed: true}
		}
		return AgentRunnerMsg{Event: ev}
	}
}

// handleAgentRunnerEvent applies one AgentEvent to the TUI's chat
// history. All event kinds map to chat history entries so the user
// can scroll back through the run.
func (m *Model) handleAgentRunnerEvent(msg AgentRunnerMsg) {
	if msg.Closed {
		m.AgentRunner = nil
		return
	}
	ev := msg.Event
	var line string
	switch ev.Kind {
	case agentrunner.EventTurn:
		line = "agent: turn start: " + ev.Detail
	case agentrunner.EventTool:
		prefix := "tool"
		if ev.ToolName != "" {
			prefix = "tool(" + ev.ToolName + ")"
		}
		line = prefix + ": " + ev.Detail
	case agentrunner.EventVerify:
		line = "verify: " + ev.Detail
	case agentrunner.EventAsk:
		m.pendingAsk = ev.AskReply
		line = "ask: " + ev.Detail + " — press y/N"
	case agentrunner.EventDone:
		line = "agent: done: " + ev.Result
	case agentrunner.EventError:
		line = "agent: ERROR: " + ev.Detail
	default:
		line = "agent: " + ev.Detail
	}
	m.ChatHistory = append(m.ChatHistory, "assistant: "+line)
	if len(m.ChatHistory) > 500 {
		m.ChatHistory = m.ChatHistory[len(m.ChatHistory)-500:]
	}
	m.AppendHistory(ViewChat.String(), "agent-event", strings.TrimSpace(line), ev.Err == nil)
}

// answerPendingAsk answers the most recent agent ask. Called from the
// chat view's y/N key handler.
func (m *Model) answerPendingAsk(allow bool) {
	if m.pendingAsk == nil {
		return
	}
	ch := m.pendingAsk
	m.pendingAsk = nil
	select {
	case ch <- allow:
	default:
	}
}

// submitAgentPrompt kicks off a Submit on the AgentRunner. Returns a
// tea.Cmd that subscribes to runner events.
func (m *Model) submitAgentPrompt(prompt string) tea.Cmd {
	r := m.initAgentRunner()
	if r == nil {
		return nil
	}
	if _, err := r.Submit(m.ctx(), prompt); err != nil {
		m.ChatHistory = append(m.ChatHistory,
			"assistant: (agent runner unavailable: "+err.Error()+")")
		if len(m.ChatHistory) > 500 {
			m.ChatHistory = m.ChatHistory[len(m.ChatHistory)-500:]
		}
		return nil
	}
	return listenAgentRunnerCmd(r)
}

// runAgentSkillPrompt formats a skill-palette entry as a directive to
// the agent and submits it through the AgentRunner. The prompt asks
// the agent to "use the X tool to ..." so the LLM resolves the skill
// via the live tool surface (websearch, browser, scheduler, etc.)
// rather than just printing a CLI hint.
func (m *Model) runAgentSkillPrompt(skill, args string) tea.Cmd {
	r := m.initAgentRunner()
	if r == nil {
		// No agent runner: fall back to the in-band "use the CLI"
		// hint so the user gets feedback.
		hint := fmt.Sprintf("run: sin-code mcp call %s %q", skill, args)
		m.ChatHistory = append(m.ChatHistory, "assistant: "+hint)
		if len(m.ChatHistory) > 500 {
			m.ChatHistory = m.ChatHistory[len(m.ChatHistory)-500:]
		}
		return nil
	}
	prompt := fmt.Sprintf("use the %s tool to %s", skill, args)
	if _, err := r.Submit(m.ctx(), prompt); err != nil {
		m.ChatHistory = append(m.ChatHistory,
			"assistant: (agent runner error: "+err.Error()+")")
		if len(m.ChatHistory) > 500 {
			m.ChatHistory = m.ChatHistory[len(m.ChatHistory)-500:]
		}
		return nil
	}
	return listenAgentRunnerCmd(r)
}
