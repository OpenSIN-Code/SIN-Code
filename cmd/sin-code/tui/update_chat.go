// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when TUI is rewritten

package tui

import (
	"fmt"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/usage"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/tui/chat"
)

func (m *Model) handleChatResponse(msg chat.ChatResponseMsg) {
	m.updatePromptDuration()
	m.setStreaming(false)
	if msg.Tokens > 0 {
		m.Footer.Tokens += msg.Tokens
		m.Footer.TokensPct = clamp(float64(m.Footer.Tokens)/128000.0, 0, 1)
		m.Footer.Cost = fmt.Sprintf("$%.2f", usage.ComputeCost(m.Footer.ModelName, m.Footer.Tokens))
	}
	if len(m.ChatHistory) == 0 {
		return
	}
	idx := len(m.ChatHistory) - 1
	last := m.ChatHistory[idx]
	if last.Kind != chatThinking {
		if msg.Error != nil {
			m.appendChat(ChatMessage{Kind: chatError, Text: msg.Error.Error(), Error: msg.Error})
		} else if msg.Text == "" {
			m.appendChat(ChatMessage{Kind: chatAssistant, Text: "(empty response)"})
		} else {
			m.appendChat(ChatMessage{Kind: chatAssistant, Text: msg.Text})
		}
		m.updateSessionPreview()
		return
	}
	if msg.Error != nil {
		m.ChatHistory[idx] = ChatMessage{Kind: chatError, Text: msg.Error.Error(), Error: msg.Error}
		m.updateSessionPreview()
		return
	}
	text := msg.Text
	if text == "" {
		text = "(empty response)"
	}
	m.ChatHistory[idx] = ChatMessage{Kind: chatAssistant, Text: text}
	m.updateSessionPreview()
}
