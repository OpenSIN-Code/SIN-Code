// SPDX-License-Identifier: MIT
package tui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/attachments"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/tui/chat"
)

type chatInput = chat.Input

var newChatRunnerHook = func() (*chat.Runner, error) { return chat.NewRunner() }

var newAttachmentStoreHook = func() (*attachments.Store, error) { return attachments.NewStore() }

var chatRunnerRunHook = func(r *chat.Runner, ctx context.Context, prompt string, history []string) (string, error) {
	return r.Run(ctx, prompt, history)
}

var chatRunnerStreamHook = func(r *chat.Runner, ctx context.Context, prompt string, history []string, onChunk func(string)) (string, error) {
	return r.RunStream(ctx, prompt, history, onChunk)
}

func newChatInput() *chatInput {
	store, err := newAttachmentStoreHook()
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

func (m *Model) initChatRunner() {
	if m.ChatRunner != nil {
		return
	}
	r, err := newChatRunnerHook()
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

func chatMessageToString(msg ChatMessage) string {
	switch msg.Kind {
	case chatUser:
		return msg.Text
	case chatAssistant:
		return "assistant: " + msg.Text
	case chatSystem:
		return "assistant: " + msg.Text
	case chatError:
		if msg.Error != nil {
			return "assistant: (error: " + msg.Error.Error() + ")"
		}
		return "assistant: (error: " + msg.Text + ")"
	case chatThinking:
		return "assistant: thinking..."
	default:
		if msg.Text != "" {
			return "assistant: " + msg.Text
		}
		return "assistant: " + msg.Detail
	}
}

func chatHistoryToStrings(history []ChatMessage) []string {
	out := make([]string, 0, len(history))
	for _, msg := range history {
		out = append(out, chatMessageToString(msg))
	}
	return out
}

func handleChatSubmit(m *Model, submit chat.SubmitMsg) tea.Cmd {
	entry := submit.Text

	if strings.TrimSpace(entry) == "/clear" {
		m.ChatHistory = nil
		m.setStreaming(false)
		m.Footer.Tokens = 0
		m.Footer.TokensPct = 0
		m.Footer.Cost = ""
		m.Footer.Compacted = false
		m.ChatFocusIdx = 0
		m.AppendHistory(ViewChat.String(), "chat-clear", "chat history cleared", true)
		return nil
	}

	if len(submit.Attachments) > 0 {
		entry += "\n[attachments:"
		for _, a := range submit.Attachments {
			entry += " " + a.Marker()
		}
		entry += "]"
	}
	m.appendChat(ChatMessage{Kind: chatUser, Text: entry})
	m.AppendHistory(ViewChat.String(), "chat-submit", entry, true)

	m.initChatRunner()
	if m.ChatRunner == nil {
		m.appendChat(ChatMessage{Kind: chatSystem, Text: "(no API key — set SIN_NIM_API_KEY)"})
	}

	agentCmd := m.submitAgentPrompt(submit.Text)

	if m.ChatRunner == nil {
		return agentCmd
	}

	m.appendChat(ChatMessage{Kind: chatThinking})
	thinkingIdx := len(m.ChatHistory) - 1
	m.setStreaming(true)

	runner := m.ChatRunner
	historySnapshot := chatHistoryToStrings(m.ChatHistory[:thinkingIdx])
	prompt := submit.Text

	prog := m.Program
	if prog == nil {
		text, err := chatRunnerRunHook(runner, m.ctx(), prompt, historySnapshot)
		applyChatResponseMsg(m, chat.ChatResponseMsg{Text: text, Error: err}, thinkingIdx)
		return agentCmd
	}
	go func() {
		text, err := chatRunnerStreamHook(runner, m.ctx(), prompt, historySnapshot, func(chunk string) {
			prog.Send(ChatChunkMsg{Text: chunk, Idx: thinkingIdx})
		})
		prog.Send(chat.ChatResponseMsg{Text: text, Error: err})
	}()
	return agentCmd
}

func applyChatResponseMsg(m *Model, msg chat.ChatResponseMsg, idx int) {
	if idx < 0 || idx >= len(m.ChatHistory) {
		return
	}
	if msg.Error != nil {
		m.ChatHistory[idx] = ChatMessage{Kind: chatError, Text: msg.Error.Error(), Error: msg.Error}
		return
	}
	text := msg.Text
	if text == "" {
		text = "(empty response)"
	}
	if m.ChatHistory[idx].Kind == chatAssistant && m.ChatHistory[idx].Text != "" {
		m.ChatHistory[idx].Text = text
	} else {
		m.ChatHistory[idx] = ChatMessage{Kind: chatAssistant, Text: text}
	}
	m.updateSessionPreview()
}

func (m *Model) updateChat(msg tea.Msg) tea.Cmd {
	if m.ChatInput == nil {
		return nil
	}
	cmd, submit := m.ChatInput.Update(msg)
	if submit != nil {
		agentCmd := handleChatSubmit(m, *submit)
		m.ChatInput.Clear()
		if agentCmd != nil {
			return tea.Batch(cmd, agentCmd)
		}
		return cmd
	}
	return cmd
}
