// SPDX-License-Identifier: MIT
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	agentrunner "github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/tui"
)

type AgentRunnerMsg struct {
	Event  agentrunner.AgentEvent
	Closed bool
}

var newAgentRunnerHook = func(ctx context.Context, cfg agentrunner.Config) (*agentrunner.AgentRunner, error) {
	return agentrunner.NewAgentRunner(ctx, cfg)
}

var submitAgentRunnerHook = func(r *agentrunner.AgentRunner, ctx context.Context, prompt string) (<-chan struct{}, error) {
	return r.Submit(ctx, prompt)
}

func (m *Model) initAgentRunner() *agentrunner.AgentRunner {
	if m.AgentRunner != nil {
		return m.AgentRunner
	}
	ws := m.Workspace
	if ws == "" {
		ws = "."
	}
	r, err := newAgentRunnerHook(m.ctx(), agentrunner.Config{
		Workspace:   ws,
		Headless:    false,
		Yolo:        false,
		MaxTurns:    20,
		ToolFactory: tuiToolFactory(ws),
	})
	if err != nil {
		return nil
	}
	m.AgentRunner = r
	return r
}

func listenAgentRunnerCmd(r *agentrunner.AgentRunner) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-r.EventsChannel()
		if !ok {
			return AgentRunnerMsg{Closed: true}
		}
		return AgentRunnerMsg{Event: ev}
	}
}

func (m *Model) handleAgentRunnerEvent(msg AgentRunnerMsg) {
	if msg.Closed {
		m.AgentRunner = nil
		return
	}
	ev := msg.Event
	var cm ChatMessage
	switch ev.Kind {
	case agentrunner.EventTurn:
		cm = ChatMessage{Kind: chatAgent, Text: "turn start", Detail: ev.Detail}
	case agentrunner.EventTool:
		isResult := strings.HasPrefix(ev.Detail, "tool result: ")
		cm = ChatMessage{
			Kind:   chatTool,
			Tool:   ev.ToolName,
			Detail: ev.Detail,
			Result: isResult,
		}
	case agentrunner.EventVerify:
		cm = ChatMessage{Kind: chatVerify, Detail: ev.Detail}
	case agentrunner.EventAsk:
		m.pendingAsk = ev.AskReply
		m.OpenPermissionDialog(ev.ToolName, ev.Detail, "")
		m.setStreaming(false)
		// Don't add to chat history — the permission dialog IS the
		// visual feedback. A 🔒 entry would clutter the scrollback.
		return
	case agentrunner.EventDone:
		cm = ChatMessage{Kind: chatDone, Detail: ev.Result}
		m.setStreaming(false)
	case agentrunner.EventError:
		cm = ChatMessage{Kind: chatError, Text: ev.Detail, Error: ev.Err}
		m.setStreaming(false)
	default:
		cm = ChatMessage{Kind: chatSystem, Text: ev.Detail}
	}
	m.appendChat(cm)
	m.AppendHistory(ViewChat.String(), "agent-event", cm.Detail, ev.Err == nil)
}

func (m *Model) answerPendingAsk(allow bool) {
	if m.pendingAsk == nil {
		return
	}
	ch := m.pendingAsk
	m.pendingAsk = nil
	select {
	case ch <- allow:
	case <-time.After(3 * time.Second):
	}
}

func (m *Model) submitAgentPrompt(prompt string) tea.Cmd {
	r := m.initAgentRunner()
	if r == nil {
		return nil
	}
	if _, err := submitAgentRunnerHook(r, m.ctx(), prompt); err != nil {
		m.appendChat(ChatMessage{Kind: chatSystem, Text: "(agent runner unavailable: " + err.Error() + ")"})
		return nil
	}
	return listenAgentRunnerCmd(r)
}

func (m *Model) runAgentSkillPrompt(skill, args string) tea.Cmd {
	r := m.initAgentRunner()
	if r == nil {
		hint := fmt.Sprintf("run: sin-code mcp call %s %q", skill, args)
		m.appendChat(ChatMessage{Kind: chatAssistant, Text: hint})
		return nil
	}
	prompt := fmt.Sprintf("use the %s tool to %s", skill, args)
	if _, err := submitAgentRunnerHook(r, m.ctx(), prompt); err != nil {
		m.appendChat(ChatMessage{Kind: chatSystem, Text: "(agent runner error: " + err.Error() + ")"})
		return nil
	}
	return listenAgentRunnerCmd(r)
}
