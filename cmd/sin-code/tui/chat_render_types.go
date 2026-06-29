// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when TUI is rewritten
package tui

import (
	"strings"
	"time"
)

type chatMsgKind int

const (
	chatUser chatMsgKind = iota
	chatAssistant
	chatAgent
	chatTool
	chatVerify
	chatAsk
	chatDone
	chatError
	chatThinking
	chatSystem
)

func chatKindString(k chatMsgKind) string {
	switch k {
	case chatUser:
		return "user"
	case chatAssistant:
		return "assistant"
	case chatAgent:
		return "agent"
	case chatTool:
		return "tool"
	case chatVerify:
		return "verify"
	case chatAsk:
		return "ask"
	case chatDone:
		return "done"
	case chatError:
		return "error"
	case chatThinking:
		return "thinking"
	case chatSystem:
		return "system"
	default:
		return "msg"
	}
}

type ChatMessage struct {
	ID         int64
	Kind       chatMsgKind
	Text       string
	Tool       string
	ToolInput  string
	ToolOutput string
	Detail     string
	Result     bool
	Timestamp  time.Time
	Tokens     int
	Error      error
	Expanded   bool
}

func formatTimestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("15:04:05")
}

func countUserTurns(history []ChatMessage) int {
	n := 0
	for _, msg := range history {
		if msg.Kind == chatUser {
			n++
		}
	}
	return n
}

func looksLikeGoCode(s string) bool {
	indicators := []string{"func ", "package ", "import ", "type ", "var ", "const "}
	count := 0
	for _, ind := range indicators {
		if strings.Contains(s, ind) {
			count++
		}
	}
	return count >= 2 || (strings.Contains(s, "func ") && strings.Contains(s, "{"))
}
