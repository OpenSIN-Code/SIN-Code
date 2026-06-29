// SPDX-License-Identifier: MIT
package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/attachments"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/tui/chat"
)

type chatInput = chat.Input

var newChatRunnerHook = func() (*chat.Runner, error) { return chat.NewRunner() }

var newAttachmentStoreHook = func() (*attachments.Store, error) { return attachments.NewStore() }

var chatRunnerRunHook = func(r *chat.Runner, ctx context.Context, prompt string, history []string) (string, int, error) {
	return r.Run(ctx, prompt, history)
}

var chatRunnerStreamHook = func(r *chat.Runner, ctx context.Context, prompt string, history []string, onChunk func(string, int)) (string, int, error) {
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
	case "/export":
		path := DefaultExportPath()
		if err := ExportChat(m.ChatHistory, path); err != nil {
			m.appendChat(ChatMessage{Kind: chatError, Text: "Export failed: " + err.Error()})
		} else {
			m.appendChat(ChatMessage{Kind: chatSystem, Text: "Chat exported to " + path})
		}
		m.AppendHistory(ViewChat.String(), "chat-export", path, true)
		return nil
	case "/clear":
		m.CancelPrompt()
		m.ChatHistory = nil
		m.setStreaming(false)
		m.Footer.Tokens = 0
		m.Footer.TokensPct = 0
		m.Footer.Cost = ""
		m.Footer.Compacted = false
		m.ChatFocusIdx = 0
		m.AppendHistory(ViewChat.String(), "chat-clear", "chat history cleared", true)
		return nil
	case "/model":
		m.OpenModelSwitcher()
		return nil
	case "/status":
		m.OpenStatusPopup()
		return nil
	case "/help":
		if m.HelpOverlay != nil {
			m.HelpOverlay.Open()
			m.Mode = ModeHelpOverlay
		}
		return nil
	case "/compact":
		m.manualCompactContext()
		return nil
	case "/dag":
		m.SwitchView(ViewDAG)
		return nil
	case "/ctx-viz":
		m.SwitchView(ViewContextViz)
		return nil
	case "/dashboard":
		m.SwitchView(ViewAgentDashboard)
		return nil
	case "/sessions":
		m.OpenSessionSwitcher()
		return nil
	case "/tools":
		m.SwitchView(ViewTools)
		return nil
	case "/btw":
		m.appendChat(ChatMessage{Kind: chatSystem, Text: "usage: /btw <question> — ask a side question without breaking the main context"})
		return nil
	case "/undercover":
		if m.UndercoverMode != nil {
			on := m.UndercoverMode.Toggle()
			status := "undercover mode: OFF — AI identity visible in commits"
			if on {
				status = "undercover mode: ON — AI identity hidden in commits"
			}
			m.appendChat(ChatMessage{Kind: chatSystem, Text: status})
			m.AppendHistory(ViewChat.String(), "undercover-toggle", status, true)
		}
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
	if strings.HasPrefix(trimmed, "/model ") {
		modelName := strings.TrimSpace(strings.TrimPrefix(trimmed, "/model "))
		if modelName != "" {
			m.AgentConfig.Model = modelName
			m.Footer.ModelName = modelName
			m.appendChat(ChatMessage{Kind: chatSystem, Text: fmt.Sprintf("Switched to model: %s", modelName)})
			m.AppendHistory(ViewChat.String(), "model-switch", modelName, true)
			return nil
		}
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
	if strings.HasPrefix(trimmed, "/btw ") {
		question := strings.TrimSpace(strings.TrimPrefix(trimmed, "/btw "))
		if question == "" {
			m.appendChat(ChatMessage{Kind: chatSystem, Text: "usage: /btw <question> — ask a side question without breaking the main context"})
		} else {
			m.appendChat(ChatMessage{Kind: chatSystem, Text: "BTW side-question requires an LLM client to be wired. Question: " + question})
		}
		return nil
	}
	if strings.HasPrefix(trimmed, "/undercover ") {
		arg := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(trimmed, "/undercover ")))
		if m.UndercoverMode != nil {
			switch arg {
			case "on", "enable", "true":
				m.UndercoverMode.Enable()
				m.appendChat(ChatMessage{Kind: chatSystem, Text: "undercover mode: ON — AI identity hidden in commits"})
			case "off", "disable", "false":
				m.UndercoverMode.Disable()
				m.appendChat(ChatMessage{Kind: chatSystem, Text: "undercover mode: OFF — AI identity visible in commits"})
			case "status", "show":
				status := "undercover mode: OFF — AI identity visible in commits"
				if m.UndercoverMode.Enabled() {
					status = "undercover mode: ON — AI identity hidden in commits"
				}
				m.appendChat(ChatMessage{Kind: chatSystem, Text: status})
			default:
				m.appendChat(ChatMessage{Kind: chatSystem, Text: "usage: /undercover [on|off|status] — toggle undercover mode"})
			}
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
	m.userScrolledUp = false
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
	ctx := m.startPromptContext()
	if prog == nil {
		text, tokens, err := chatRunnerRunHook(runner, ctx, prompt, historySnapshot)
		m.resetPromptContext()
		applyChatResponseMsg(m, chat.ChatResponseMsg{Text: text, Error: err, Tokens: tokens}, thinkingIdx)
		return nil
	}
	go func() {
		text, tokens, err := chatRunnerStreamHook(runner, ctx, prompt, historySnapshot, func(chunk string, estimatedTokens int) {
			prog.Send(ChatChunkMsg{Text: chunk, Idx: thinkingIdx, EstimatedTokens: estimatedTokens})
		})
		m.resetPromptContext()
		prog.Send(chat.ChatResponseMsg{Text: text, Error: err, Tokens: tokens})
	}()
	// Start the live footer ticker; it will stop once IsStreaming() becomes false.
	return streamTickCmd()
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
			active := m.CompactMode.Active()
			toggleMsg := "Compact mode off"
			if active {
				toggleMsg = "Compact mode on — messages rendered in condensed format"
			}
			m.appendChat(ChatMessage{Kind: chatSystem, Text: toggleMsg})
			return nil
		}
	}

	// Handle SlashMenu navigation when open — takes priority over autocomplete.
	if m.SlashMenu != nil && m.SlashMenu.Open {
		if kp, ok := msg.(tea.KeyPressMsg); ok {
			key := kp.String()
			switch key {
			case "up", "k":
				m.SlashMenu.Prev()
				return nil
			case "down", "j":
				m.SlashMenu.Next()
				return nil
			case "tab", "enter":
				sel := m.SlashMenu.Selected()
				if sel.Name != "" {
					insertText := sel.Name + " "
					m.ChatInput.SetValue(insertText)
					m.SlashMenu.Close()
					return nil
				}
			case "esc":
				m.SlashMenu.Close()
				return nil
			}
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
		if m.IsStreaming() {
			m.appendChat(ChatMessage{Kind: chatSystem, Text: "⏳ Wait for the current response to finish (Esc to interrupt)"})
			m.ChatInput.Clear()
			return nil
		}
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
	if m.ChatInput == nil {
		return
	}
	raw := m.ChatInput.RawValue()
	if strings.HasPrefix(raw, "/") && !strings.Contains(raw, "\n") {
		// Open and drive the richer SlashMenu.
		if m.SlashMenu != nil {
			if !m.SlashMenu.Open {
				m.SlashMenu.OpenMenu()
			}
			m.SlashMenu.Filter_(raw)
		}
		// Keep the legacy autocomplete in sync for backward compatibility.
		if m.SlashAutocomplete != nil {
			if !m.SlashAutocomplete.Active() {
				m.SlashAutocomplete.SetActive(true)
			}
			m.SlashAutocomplete.Filter(raw)
		}
	} else {
		if m.SlashMenu != nil && m.SlashMenu.Open {
			m.SlashMenu.Close()
		}
		if m.SlashAutocomplete != nil && m.SlashAutocomplete.Active() {
			m.SlashAutocomplete.SetActive(false)
		}
	}
}

func (m *Model) OpenChatSearch() {
	m.Mode = ModeSearch
	m.SearchQuery = ""
	m.SearchMatches = nil
	m.ScrollToMatchIdx = -1
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
	m.ScrollToMatchIdx = -1
	if m.ChatSearch != nil {
		m.ChatSearch.Clear()
	}
}
