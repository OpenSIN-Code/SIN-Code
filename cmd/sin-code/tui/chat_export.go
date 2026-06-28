// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
)

// ExportChat exports the chat history to a markdown file.
func ExportChat(history []ChatMessage, outputPath string) error {
	content := ExportChatToString(history)
	return os.WriteFile(outputPath, []byte(content), filemode.Default())
}

// ExportChatToString converts chat history to a markdown string.
func ExportChatToString(history []ChatMessage) string {
	var b strings.Builder

	b.WriteString("# SIN-Code Chat Export\n\n")

	now := time.Now()
	b.WriteString(fmt.Sprintf("**Date:** %s\n", now.Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("**Messages:** %d\n", len(history)))
	b.WriteString("\n---\n\n")

	for _, msg := range history {
		header, body := formatExportMessage(msg)
		if header == "" {
			continue
		}
		b.WriteString("## ")
		b.WriteString(header)
		b.WriteString("\n\n")
		if body != "" {
			b.WriteString(body)
			b.WriteString("\n")
		}
		b.WriteString("---\n\n")
	}

	return b.String()
}

// formatExportMessage returns a section header and body for a chat message.
func formatExportMessage(msg ChatMessage) (header, body string) {
	switch msg.Kind {
	case chatUser:
		return "User", msg.Text
	case chatAssistant:
		return "Assistant", msg.Text
	case chatAgent:
		return "Assistant (agent)", msg.Text
	case chatTool:
		hdr := "Tool: " + msg.Tool
		var bb strings.Builder
		if msg.ToolInput != "" {
			bb.WriteString("**Input:**\n\n```\n")
			bb.WriteString(msg.ToolInput)
			bb.WriteString("\n```\n\n")
		}
		if msg.ToolOutput != "" {
			bb.WriteString("**Output:**\n\n```\n")
			bb.WriteString(msg.ToolOutput)
			bb.WriteString("\n```")
		}
		return hdr, bb.String()
	case chatSystem:
		return "System", msg.Text
	case chatError:
		text := msg.Text
		if msg.Error != nil && text == "" {
			text = msg.Error.Error()
		}
		return "⚠ Error", text
	case chatVerify:
		if msg.Result {
			return "✓ Verified", msg.Text
		}
		return "✗ Verification Failed", msg.Text
	case chatDone:
		return "Done", msg.Text
	case chatAsk:
		return "Permission Request", msg.Text
	case chatThinking:
		return "", ""
	default:
		return "", ""
	}
}

// DefaultExportPath returns a sensible default path for chat export:
// ~/Desktop/sin-code-chat-YYYYMMDD-HHMMSS.md
func DefaultExportPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	desktop := filepath.Join(home, "Desktop")
	if _, err := os.Stat(desktop); err != nil {
		desktop = home
	}
	ts := time.Now().Format("20060102-150405")
	return filepath.Join(desktop, fmt.Sprintf("sin-code-chat-%s.md", ts))
}
