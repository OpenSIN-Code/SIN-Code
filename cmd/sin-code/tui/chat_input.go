// SPDX-License-Identifier: MIT
package tui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/key"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/attachments"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/tui/chat"
)

type chatInput = chat.Input

var newChatRunnerHook = func() (*chat.Runner, error) { return chat.NewRunner() }

var newAttachmentStoreHook = func() (*attachments.Store, error) { return attachments.NewStore() }

var chatRunnerRunHook = func(r *chat.Runner, ctx context.Context, prompt string, history []string) (string, int, error) {
	return r.Run(ctx, prompt, history)
}

var chatRunnerStreamHook = func(r *chat.Runner, ctx context.Context, prompt string, history []string, onChunk func(string)) (string, int, error) {
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
	trimmed := strings.TrimSpace(entry)

	// Model-level slash commands
	switch trimmed {
	case "/clear":
		m.ChatHistory = nil
		m.setStreaming(false)
		m.Footer.Tokens = 0
		m.Footer.TokensPct = 0
		m.Footer.Cost = ""
		m.Footer.Compacted = false
		m.ChatFocusIdx = 0
		m.AppendHistory(ViewChat.String(), "chat-clear", "chat history cleared", true)
		return nil
	case "/help":
		helpText := "Available commands:\n\n" +
			"  /attach <path>  Attach a file to the next message\n" +
			"  /attach-glob <pattern>  Attach files matching a glob pattern\n" +
			"  /detach <name|index>  Remove an attachment\n" +
			"  /clear  Clear chat history\n" +
			"  /help   Show this help message\n" +
			"  /theme custom <path>  Load custom theme from JSON\n" +
			"  /theme export <path>  Export current theme to JSON\n\n" +
			"Keys:\n" +
			"  Enter        Send message\n" +
			"  Shift+Enter  Insert newline\n" +
			"  Ctrl+S       Send message (alternative)\n" +
			"  Ctrl+C/X     Quit\n" +
			"  Ctrl+M       Switch model\n" +
			"  Ctrl+G       Switch session\n" +
			"  Esc          Interrupt\n" +
			"  y/n          Allow/deny permission dialog\n" +
			"  PgUp/PgDn    Scroll chat history\n" +
			"  1-7          Jump to view (Tools, Sessions, EFM, Config, History, Todos, Chat)"
		m.appendChat(ChatMessage{Kind: chatSystem, Text: helpText})
		return nil
	}

	if strings.HasPrefix(trimmed, "/theme custom ") {
		path := strings.TrimSpace(strings.TrimPrefix(trimmed, "/theme custom "))
		if path == "" {
			m.appendChat(ChatMessage{Kind: chatSystem, Text: "Usage: /theme custom <path>"})
			return nil
		}
		if err := m.LoadCustomThemeFromPath(path); err != nil {
			m.appendChat(ChatMessage{Kind: chatError, Text: "Failed to load theme: " + err.Error()})
		} else {
			m.appendChat(ChatMessage{Kind: chatSystem, Text: "Custom theme loaded from " + path})
		}
		return nil
	}
	if strings.HasPrefix(trimmed, "/theme export ") {
		path := strings.TrimSpace(strings.TrimPrefix(trimmed, "/theme export "))
		if path == "" {
			m.appendChat(ChatMessage{Kind: chatSystem, Text: "Usage: /theme export <path>"})
			return nil
		}
		if err := m.ExportThemeToPath(path); err != nil {
			m.appendChat(ChatMessage{Kind: chatError, Text: "Failed to export theme: " + err.Error()})
		} else {
			m.appendChat(ChatMessage{Kind: chatSystem, Text: "Theme exported to " + path})
		}
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

	agentCmd := m.submitAgentPrompt(submit.Text)

	// If the AgentRunner is active, it IS the full agent loop (LLM +
	// tools + verification). Running ChatRunner in parallel produces
	// duplicate, interleaved responses. Skip ChatRunner entirely.
	if agentCmd != nil {
		m.setStreaming(true)
		return agentCmd
	}

	// No AgentRunner — fall back to raw ChatRunner (LLM call only).
	if m.ChatRunner == nil {
		m.appendChat(ChatMessage{Kind: chatSystem, Text: "(no API key — set SIN_NIM_API_KEY)"})
		return nil
	}

	m.appendChat(ChatMessage{Kind: chatThinking})
	thinkingIdx := len(m.ChatHistory) - 1
	m.setStreaming(true)

	runner := m.ChatRunner
	historySnapshot := chatHistoryToStrings(m.ChatHistory[:thinkingIdx])
	prompt := submit.Text

	prog := m.Program
	if prog == nil {
		text, tokens, err := chatRunnerRunHook(runner, m.ctx(), prompt, historySnapshot)
		applyChatResponseMsg(m, chat.ChatResponseMsg{Text: text, Error: err, Tokens: tokens}, thinkingIdx)
		return nil
	}
	go func() {
		text, tokens, err := chatRunnerStreamHook(runner, m.ctx(), prompt, historySnapshot, func(chunk string) {
			prog.Send(ChatChunkMsg{Text: chunk, Idx: thinkingIdx})
		})
		prog.Send(chat.ChatResponseMsg{Text: text, Error: err, Tokens: tokens})
	}()
	return nil
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

	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		if m.CompactMode != nil && key.Matches(kmsg, keymap.CompactToggle) {
			m.CompactMode.Toggle()
			return nil
		}
	}

	if m.SlashAutocomplete != nil && m.SlashAutocomplete.Active() {
		if kp, ok := msg.(tea.KeyPressMsg); ok {
			key := kp.String()
			switch key {
			case "up", "k":
				m.SlashAutocomplete.MoveUp()
				return nil
			case "down", "j":
				m.SlashAutocomplete.MoveDown()
				return nil
			case "tab", "enter":
				sel := m.SlashAutocomplete.Selected()
				if sel != nil {
					insertText := sel.Name
					if sel.Args != "" {
						insertText += " "
					} else {
						insertText += " "
					}
					m.ChatInput.SetValue(insertText)
					m.SlashAutocomplete.SetActive(false)
					if sel.Name == "/search" {
						m.OpenChatSearch()
					}
					return nil
				}
			case "esc":
				m.SlashAutocomplete.SetActive(false)
				return nil
			}
		}
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

	m.updateSlashAutocompleteFromInput()

	return cmd
}

func (m *Model) updateSlashAutocompleteFromInput() {
	if m.SlashAutocomplete == nil || m.ChatInput == nil {
		return
	}
	raw := m.ChatInput.RawValue()
	if strings.HasPrefix(raw, "/") && !strings.Contains(raw, "\n") {
		if !m.SlashAutocomplete.Active() {
			m.SlashAutocomplete.SetActive(true)
		}
		m.SlashAutocomplete.Filter(raw)
	} else if m.SlashAutocomplete.Active() {
		m.SlashAutocomplete.SetActive(false)
	}
}

func (m *Model) OpenChatSearch() {
	m.Mode = ModeSearch
	m.SearchQuery = ""
	m.SearchMatches = nil
	m.SearchInput.SetValue("")
	m.SearchInput.Placeholder = "Search chat..."
	m.SearchInput.Focus()
	if m.ChatSearch != nil {
		m.ChatSearch.Clear()
	}
}

func (m *Model) CloseChatSearch() {
	m.Mode = ModeNormal
	m.SearchInput.Blur()
	m.SearchQuery = ""
	m.SearchMatches = nil
	if m.ChatSearch != nil {
		m.ChatSearch.Clear()
	}
}
