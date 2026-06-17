package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTUIScreenshots(t *testing.T) {
	screenshotDir := filepath.Join(os.TempDir(), "sin-code-tui-screenshots")
	if err := os.MkdirAll(screenshotDir, 0o755); err != nil {
		t.Fatalf("failed to create screenshot dir: %v", err)
	}

	m := NewModel()
	m.Width = 120
	m.Height = 40
	m.ApplyTheme()

	// 1. Tools View (default)
	t.Run("ToolsView", func(t *testing.T) {
		m.ViewKind = ViewTools
		m.Mode = ModeNormal
		view := m.View().Content
		saveScreenshot(t, screenshotDir, "01_tools_view.txt", view)
	})

	// 2. Sessions View
	t.Run("SessionsView", func(t *testing.T) {
		m.ViewKind = ViewSessions
		m.Tabs.Sessions = []Session{
			{Name: "Debug Session", Active: true, Preview: "Last: Fixed auth bug", LastActive: time.Now()},
			{Name: "Feature Work", Dirty: true, Preview: "Last: Added new endpoint", LastActive: time.Now().Add(-1 * time.Hour)},
			{Name: "Code Review", Preview: "Last: Reviewed PR #123", LastActive: time.Now().Add(-2 * time.Hour)},
		}
		view := m.View().Content
		saveScreenshot(t, screenshotDir, "02_sessions_view.txt", view)
	})

	// 3. Chat View (empty)
	t.Run("ChatView_Empty", func(t *testing.T) {
		m.ViewKind = ViewChat
		m.ChatHistory = []ChatMessage{}
		view := m.View().Content
		saveScreenshot(t, screenshotDir, "03_chat_view_empty.txt", view)
	})

	// 4. Chat View (with messages)
	t.Run("ChatView_WithMessages", func(t *testing.T) {
		m.ViewKind = ViewChat
		m.ChatHistory = []ChatMessage{
			{Kind: chatUser, Text: "Fix the authentication bug in the login endpoint"},
			{Kind: chatAssistant, Text: "I'll help you fix the authentication bug. Let me analyze the login endpoint code.\n\n## Analysis\n\nThe issue is in the token validation logic:\n\n```go\nif token == \"\" {\n    return nil, errors.New(\"missing token\")\n}\n```\n\nThis doesn't handle expired tokens properly."},
			{Kind: chatTool, Tool: "sin_edit", Detail: "auth/login.go:42-48"},
			{Kind: chatVerify, Detail: "pass — all tests green"},
			{Kind: chatAssistant, Text: "✅ Fixed! The authentication bug has been resolved. All tests pass."},
		}
		m.Footer.Tokens = 15234
		m.Footer.TokensPct = 0.12
		m.Footer.Cost = "$0.045"
		m.Footer.Duration = 12 * time.Second
		view := m.View().Content
		saveScreenshot(t, screenshotDir, "04_chat_view_messages.txt", view)
	})

	// 5. Chat View (streaming)
	t.Run("ChatView_Streaming", func(t *testing.T) {
		m.ViewKind = ViewChat
		m.ChatHistory = []ChatMessage{
			{Kind: chatUser, Text: "Refactor the database layer"},
			{Kind: chatThinking},
		}
		m.Footer.Streaming = true
		view := m.View().Content
		m.Footer.Streaming = false
		saveScreenshot(t, screenshotDir, "05_chat_view_streaming.txt", view)
	})

	// 6. Config View
	t.Run("ConfigView", func(t *testing.T) {
		m.ViewKind = ViewConfig
		m.ConfigSel = 2
		view := m.View().Content
		saveScreenshot(t, screenshotDir, "06_config_view.txt", view)
	})

	// 7. History View
	t.Run("HistoryView", func(t *testing.T) {
		m.ViewKind = ViewHistory
		m.History = []HistoryEntry{
			{Time: time.Now(), View: "Chat", Action: "submit", Detail: "Fix auth bug", Success: true},
			{Time: time.Now().Add(-1 * time.Minute), View: "Chat", Action: "agent-event", Detail: "tool(sin_edit)", Success: true},
			{Time: time.Now().Add(-2 * time.Minute), View: "Chat", Action: "verify", Detail: "pass", Success: true},
			{Time: time.Now().Add(-3 * time.Minute), View: "Tools", Action: "run", Detail: "discover", Success: true},
		}
		view := m.View().Content
		saveScreenshot(t, screenshotDir, "07_history_view.txt", view)
	})

	// 8. Todos View
	t.Run("TodosView", func(t *testing.T) {
		m.ViewKind = ViewTodos
		m.TodoItems = []TodoRow{
			{ID: "1", Title: "Fix authentication bug", Priority: "P0", Status: "open", Type: "bug"},
			{ID: "2", Title: "Add unit tests", Priority: "P1", Status: "open", Type: "task"},
			{ID: "3", Title: "Update documentation", Priority: "P2", Status: "done", Type: "task"},
		}
		m.Sidebar.TodoOpen = 2
		m.Sidebar.TodoBlocked = 1
		view := m.View().Content
		saveScreenshot(t, screenshotDir, "08_todos_view.txt", view)
	})

	// 9. Session Switcher Modal
	t.Run("SessionSwitcher", func(t *testing.T) {
		m.ViewKind = ViewChat
		m.Tabs.Sessions = []Session{
			{Name: "Debug Session", Preview: "Fixed auth bug", LastActive: time.Now()},
			{Name: "Feature Work", Preview: "Added endpoint", LastActive: time.Now().Add(-1 * time.Hour)},
			{Name: "Code Review", Preview: "Reviewed PR #123", LastActive: time.Now().Add(-2 * time.Hour)},
			{Name: "Refactoring", Preview: "Cleaned up utils", LastActive: time.Now().Add(-3 * time.Hour)},
		}
		m.OpenSessionSwitcher()
		m.SessionSwitcher.Query = "debug"
		view := m.View().Content
		m.CloseSessionSwitcher()
		saveScreenshot(t, screenshotDir, "09_session_switcher.txt", view)
	})

	// 10. Model Selector Modal
	t.Run("ModelSelector", func(t *testing.T) {
		m.ViewKind = ViewChat
		m.OpenModelSelector()
		m.ModelSelector.CurrentID = "qwen3-coder-plus"
		m.ModelSelector.Sel = 0
		view := m.View().Content
		m.CloseModelSelector()
		saveScreenshot(t, screenshotDir, "10_model_selector.txt", view)
	})

	// 11. Permission Dialog Modal
	t.Run("PermissionDialog", func(t *testing.T) {
		m.ViewKind = ViewChat
		diff := `--- a/auth/login.go
+++ b/auth/login.go
@@ -42,7 +42,10 @@
 func validateToken(token string) error {
-    if token == "" {
-        return errors.New("missing token")
-    }
+    if token == "" {
+        return errors.New("missing token")
+    }
+    if isExpired(token) {
+        return errors.New("token expired")
+    }
     return nil
 }`
		m.OpenPermissionDialog("sin_edit", "Modify auth/login.go to add token expiration check", diff)
		view := m.View().Content
		m.ClosePermissionDialog()
		saveScreenshot(t, screenshotDir, "11_permission_dialog.txt", view)
	})

	// 12. Command Palette
	t.Run("CommandPalette", func(t *testing.T) {
		m.ViewKind = ViewTools
		m.OpenPalette()
		m.Palette.Query = "view"
		m.filterPalette(m.Palette.Query)
		view := m.View().Content
		m.ClosePalette()
		saveScreenshot(t, screenshotDir, "12_command_palette.txt", view)
	})

	// 13. Footer with all metrics
	t.Run("Footer_Metrics", func(t *testing.T) {
		m.ViewKind = ViewChat
		m.Footer.Tokens = 45678
		m.Footer.TokensPct = 0.36
		m.Footer.Cost = "$0.137"
		m.Footer.Duration = 2*time.Minute + 15*time.Second
		m.Footer.Streaming = false
		m.ChatHistory = []ChatMessage{{Kind: chatUser, Text: "Test message"}}
		view := m.View().Content
		saveScreenshot(t, screenshotDir, "13_footer_metrics.txt", view)
	})

	// 14. EFM View
	t.Run("EFMView", func(t *testing.T) {
		m.ViewKind = ViewEFM
		m.EFMStks = []EFMStack{
			{Name: "dev-stack", Status: "running", URL: "http://localhost:8080", CreatedAt: time.Now().Add(-2 * time.Hour), TTL: 3600},
			{Name: "test-env", Status: "stopped", URL: "", CreatedAt: time.Now().Add(-1 * time.Hour), TTL: 1800},
		}
		view := m.View().Content
		saveScreenshot(t, screenshotDir, "14_efm_view.txt", view)
	})

	t.Logf("✅ All screenshots saved to: %s", screenshotDir)
	t.Logf("📸 Generated 14 screenshots covering all TUI features")
}

func saveScreenshot(t *testing.T, dir, filename, content string) {
	t.Helper()
	path := filepath.Join(dir, filename)
	
	// Clean ANSI escape codes for readability
	cleanContent := cleanANSI(content)
	
	if err := os.WriteFile(path, []byte(cleanContent), 0o644); err != nil {
		t.Errorf("failed to save screenshot %s: %v", filename, err)
		return
	}
	t.Logf("📸 Saved: %s", filename)
}

func cleanANSI(s string) string {
	// Remove ANSI escape sequences
	var result strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEscape = false
			}
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}
