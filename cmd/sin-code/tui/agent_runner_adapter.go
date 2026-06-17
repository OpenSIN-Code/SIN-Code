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
	// Ensure tool tree exists before sending any tool-call messages.
	if m.ToolTree == nil {
		m.ToolTree = &ToolCallTree{}
	}
	ev := msg.Event
	var cm ChatMessage
	switch ev.Kind {
	case agentrunner.EventTurn:
		cm = ChatMessage{Kind: chatAgent, Text: "turn start", Detail: ev.Detail}
	case agentrunner.EventTool:
		isResult := strings.HasPrefix(ev.Detail, "tool result")
		cm = ChatMessage{
			Kind:   chatTool,
			Tool:   ev.ToolName,
			Detail: ev.Detail,
			Result: isResult,
		}
		toolName := ev.ToolName
		if toolName == "" && strings.HasPrefix(ev.Detail, "tool: ") {
			toolName = strings.TrimPrefix(ev.Detail, "tool: ")
		}
		if !isResult && toolName != "" {
			// New tool call starting — add node to tree.
			nodeID := fmt.Sprintf("tool-%d-%s", time.Now().UnixNano(), toolName)
			if m.Program != nil {
				m.Program.Send(ToolCallTreeMsg{
					ParentID: "",
					Node: &ToolCallNode{
						ID:        nodeID,
						Tool:      toolName,
						Status:    "running",
						StartTime: time.Now(),
						Expanded:  false,
					},
				})
			}
		} else if isResult && toolName != "" {
			// Tool call result — best-effort update by tool name.
			if m.Program != nil {
				m.Program.Send(ToolCallUpdateMsg{
					ID:     toolName,
					Status: "success",
					Output: ev.Detail,
				})
			}
		}
	case agentrunner.EventVerify:
		cm = ChatMessage{Kind: chatVerify, Detail: ev.Detail}
		vState := VerifyRunning
		d := ev.Detail
		if strings.Contains(d, "PASSED") || strings.Contains(d, "pass") {
			vState = VerifyPassed
		} else if strings.Contains(d, "FAILED") || strings.Contains(d, "fail") {
			vState = VerifyFailed
		} else if strings.Contains(d, "BLOCKED") || strings.Contains(d, "blocked") {
			vState = VerifyBlocked
		}
		if m.Program != nil {
			m.Program.Send(VerifyUpdateMsg{
				State:    vState,
				Mode:     "poc",
				Target:   ev.Detail,
				Evidence: ev.Result,
			})
		}
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
		if ev.Tokens > 0 {
			m.Footer.Tokens += ev.Tokens
			m.Footer.TokensPct = float64(m.Footer.Tokens) / 128000.0
			if m.Footer.TokensPct > 1.0 {
				m.Footer.TokensPct = 1.0
			}
		}
		if strings.Contains(strings.ToLower(ev.Result), "verified") && m.Program != nil {
			m.Program.Send(VerifyUpdateMsg{
				State:    VerifyPassed,
				Mode:     "poc",
				Target:   "agent run complete",
				Evidence: ev.Result,
			})
		}
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
