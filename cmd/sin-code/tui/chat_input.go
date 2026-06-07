// SPDX-License-Identifier: MIT
// Purpose: TUI-side adapter for the chat.Input widget. Avoids the
// `*chat.Input` direct dep in model.go by wrapping it in a local type.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/attachments"
	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/tui/chat"
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

type chatSubmitMsg struct {
	Text        string
	Attachments []*attachments.Attachment
}

func handleChatSubmit(m *Model, submit chat.SubmitMsg) {
	entry := submit.Text
	if len(submit.Attachments) > 0 {
		entry += "\n[attachments:"
		for _, a := range submit.Attachments {
			entry += " " + a.Marker()
		}
		entry += "]"
	}
	m.ChatHistory = append(m.ChatHistory, entry)
	if len(m.ChatHistory) > 200 {
		m.ChatHistory = m.ChatHistory[len(m.ChatHistory)-200:]
	}
	m.AppendHistory(ViewChat.String(), "chat-submit", entry, true)
}

func (m *Model) updateChat(msg tea.Msg) tea.Cmd {
	if m.ChatInput == nil {
		return nil
	}
	cmd, submit := m.ChatInput.Update(msg)
	if submit != nil {
		handleChatSubmit(m, *submit)
		m.ChatInput.Clear()
	}
	return cmd
}
