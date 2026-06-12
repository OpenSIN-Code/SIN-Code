// SPDX-License-Identifier: MIT
// Purpose: TUI-side adapter for the chat.Input widget. Avoids the
// `*chat.Input` direct dep in model.go by wrapping it in a local type.
// Submits are routed through a chat.Runner (lazy-init singleton) and the
// LLM call runs in a background goroutine so the UI stays responsive.
package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/attachments"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/tui/chat"
)

type chatInput = chat.Input

func newChatInput() *chatInput {
	store, err := attachments.NewStore()
	if err != nil {
		store = nil
	}
	return chat.NewInput(store)
}

func (m *Model) initChatInput() {
	if m.ChatInput == nil {
		m.ChatInput = newChatInput()
	}
}

// initChatRunner lazily initializes the chat LLM runner. If no API key is
// configured the runner stays nil and the submit handler prints an
// in-band error rather than calling the LLM.
func (m *Model) initChatRunner() {
	if m.ChatRunner != nil {
		return
	}
	r, err := chat.NewRunner()
	if err != nil {
		m.ChatRunner = nil
		return
	}
	m.ChatRunner = r
}

type chatSubmitMsg struct {
	Text        string
	Attachments []*attachments.Attachment
}

// handleChatSubmit appends the user entry to history and, when a runner
// is available, kicks off an async LLM call. A "thinking..." placeholder
// is shown immediately; the background goroutine dispatches a
// chat.ChatResponseMsg back into the Update loop via *tea.Program.Send
// (or, when no program is set, blocks synchronously — used by tests).
//
// Returns a tea.Cmd that subscribes to the AgentRunner's event stream
// (issue #53) so the user sees the full agentloop progress in chat
// history. Returns nil when no agent runner is available.
func handleChatSubmit(m *Model, submit chat.SubmitMsg) tea.Cmd {
	entry := submit.Text
	if len(submit.Attachments) > 0 {
		entry += "\n[attachments:"
		for _, a := range submit.Attachments {
			entry += " " + a.Marker()
		}
		entry += "]"
	}
	m.ChatHistory = append(m.ChatHistory, entry)
	if len(m.ChatHistory) > 500 {
		m.ChatHistory = m.ChatHistory[len(m.ChatHistory)-500:]
	}
	m.AppendHistory(ViewChat.String(), "chat-submit", entry, true)

	m.initChatRunner()
	if m.ChatRunner == nil {
		m.ChatHistory = append(m.ChatHistory, "assistant: (no API key — set SIN_NIM_API_KEY)")
		if len(m.ChatHistory) > 500 {
			m.ChatHistory = m.ChatHistory[len(m.ChatHistory)-500:]
		}
	}

	// Issue #53: also kick off the full agentloop in parallel. The
	// agent runner emits AgentRunnerMsg events that the update loop
	// folds back into ChatHistory, so the user sees the agent's tool
	// calls, asks, and final summary alongside the LLM chat reply.
	// Falls back to nil (no-op) when the runner cannot be constructed
	// (e.g. workspace not writable).
	agentCmd := m.submitAgentPrompt(submit.Text)

	if m.ChatRunner == nil {
		return agentCmd
	}

	// Show "thinking..." placeholder right away so the user sees feedback.
	m.ChatHistory = append(m.ChatHistory, "assistant: thinking...")
	if len(m.ChatHistory) > 500 {
		m.ChatHistory = m.ChatHistory[len(m.ChatHistory)-500:]
	}
	thinkingIdx := len(m.ChatHistory) - 1

	// Snapshot the runner + history so the goroutine doesn't race the
	// Update loop's mutations.
	runner := m.ChatRunner
	historySnapshot := append([]string(nil), m.ChatHistory[:thinkingIdx]...)
	prompt := submit.Text

	prog := m.Program
	go func() {
		text, err := runner.Run(m.ctx(), prompt, historySnapshot)
		msg := chat.ChatResponseMsg{Text: text, Error: err}
		if prog != nil {
			prog.Send(msg)
			return
		}
		// No program wired up (e.g. test path): apply synchronously via
		// a tea.Cmd the next Update tick can pick up. The caller will
		// see the assistant entry in history after the next Update.
		applyChatResponseMsg(m, msg, thinkingIdx)
	}()
	return nil
}

// applyChatResponseMsg replaces the "thinking..." placeholder at idx with
// the real assistant text (or error). Used by the synchronous fallback
// path; the async path mutates m.ChatHistory directly from the goroutine
// only when there's no *tea.Program, in which case the model is single-
// threaded and there's no race.
func applyChatResponseMsg(m *Model, msg chat.ChatResponseMsg, idx int) {
	if idx < 0 || idx >= len(m.ChatHistory) {
		return
	}
	if msg.Error != nil {
		m.ChatHistory[idx] = "assistant: (error: " + msg.Error.Error() + ")"
		return
	}
	text := msg.Text
	if text == "" {
		text = "(empty response)"
	}
	m.ChatHistory[idx] = "assistant: " + text
}

func (m *Model) updateChat(msg tea.Msg) tea.Cmd {
	if m.ChatInput == nil {
		return nil
	}
	cmd, submit := m.ChatInput.Update(msg)
	if submit != nil {
		agentCmd := handleChatSubmit(m, *submit)
		m.ChatInput.Clear()
		// Combine the input's tea.Cmd with the agent-runner
		// subscription so both fire on the next tick. The agent
		// subscription re-arms itself in update.go's AgentRunnerMsg
		// handler.
		if agentCmd != nil {
			return tea.Batch(cmd, agentCmd)
		}
		return cmd
	}
	return cmd
}
