// SPDX-License-Identifier: MIT
package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportChatToString(t *testing.T) {
	history := []ChatMessage{
		{Kind: chatUser, Text: "Hello, can you help me?"},
		{Kind: chatAssistant, Text: "Sure! What do you need?"},
		{Kind: chatTool, Tool: "sin_edit", ToolInput: "file.go", ToolOutput: "edited"},
		{Kind: chatSystem, Text: "System initialized"},
		{Kind: chatError, Text: "something went wrong"},
		{Kind: chatVerify, Result: true, Text: "All tests pass"},
		{Kind: chatVerify, Result: false, Text: "Tests failed"},
	}

	result := ExportChatToString(history)

	if !strings.Contains(result, "# SIN-Code Chat Export") {
		t.Errorf("export should contain title header")
	}
	if !strings.Contains(result, "## User") {
		t.Errorf("export should contain User section")
	}
	if !strings.Contains(result, "Hello, can you help me?") {
		t.Errorf("export should contain user message text")
	}
	if !strings.Contains(result, "## Assistant") {
		t.Errorf("export should contain Assistant section")
	}
	if !strings.Contains(result, "## Tool: sin_edit") {
		t.Errorf("export should contain Tool section with tool name")
	}
	if !strings.Contains(result, "**Input:**") {
		t.Errorf("export should contain tool input label")
	}
	if !strings.Contains(result, "**Output:**") {
		t.Errorf("export should contain tool output label")
	}
	if !strings.Contains(result, "## System") {
		t.Errorf("export should contain System section")
	}
	if !strings.Contains(result, "## ⚠ Error") {
		t.Errorf("export should contain Error section")
	}
	if !strings.Contains(result, "## ✓ Verified") {
		t.Errorf("export should contain Verified section")
	}
	if !strings.Contains(result, "## ✗ Verification Failed") {
		t.Errorf("export should contain Verification Failed section")
	}
}

func TestExportChatToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "export.md")

	history := []ChatMessage{
		{Kind: chatUser, Text: "test message"},
		{Kind: chatAssistant, Text: "test response"},
	}

	if err := ExportChat(history, path); err != nil {
		t.Fatalf("ExportChat failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read export file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "test message") {
		t.Errorf("export file should contain user message")
	}
	if !strings.Contains(content, "test response") {
		t.Errorf("export file should contain assistant response")
	}
}

func TestDefaultExportPath(t *testing.T) {
	path := DefaultExportPath()

	if !strings.HasSuffix(path, ".md") {
		t.Errorf("default export path should end with .md, got %q", path)
	}
	if !strings.Contains(path, "sin-code-chat-") {
		t.Errorf("default export path should contain 'sin-code-chat-', got %q", path)
	}
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		t.Errorf("default export path should be in a real directory, got %q", dir)
	}
}

func TestExportChat_Empty(t *testing.T) {
	result := ExportChatToString(nil)

	if !strings.Contains(result, "# SIN-Code Chat Export") {
		t.Errorf("empty export should still contain title header")
	}
	if !strings.Contains(result, "**Messages:** 0") {
		t.Errorf("empty export should show 0 messages")
	}
}
