// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type AccessibilityMode struct {
	mu            sync.RWMutex
	highContrast  bool
	screenReader  bool
	keyboardOnly  bool
	largeText     bool
	reducedMotion bool
}

func NewAccessibilityMode() *AccessibilityMode {
	return &AccessibilityMode{}
}

func (a *AccessibilityMode) HighContrast() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.highContrast
}

func (a *AccessibilityMode) SetHighContrast(b bool) {
	a.mu.Lock()
	a.highContrast = b
	a.mu.Unlock()
}

func (a *AccessibilityMode) ScreenReader() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.screenReader
}

func (a *AccessibilityMode) SetScreenReader(b bool) {
	a.mu.Lock()
	a.screenReader = b
	a.mu.Unlock()
}

func (a *AccessibilityMode) KeyboardOnlyMode() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.keyboardOnly
}

func (a *AccessibilityMode) SetKeyboardOnlyMode(b bool) {
	a.mu.Lock()
	a.keyboardOnly = b
	a.mu.Unlock()
}

func (a *AccessibilityMode) LargeText() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.largeText
}

func (a *AccessibilityMode) SetLargeText(b bool) {
	a.mu.Lock()
	a.largeText = b
	a.mu.Unlock()
}

func (a *AccessibilityMode) ReducedMotion() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.reducedMotion
}

func (a *AccessibilityMode) SetReducedMotion(b bool) {
	a.mu.Lock()
	a.reducedMotion = b
	a.mu.Unlock()
}

func (a *AccessibilityMode) ApplyToConfig(cfg map[string]bool) {
	if cfg == nil {
		return
	}
	if v, ok := cfg["high_contrast"]; ok {
		a.SetHighContrast(v)
	}
	if v, ok := cfg["screen_reader"]; ok {
		a.SetScreenReader(v)
	}
	if v, ok := cfg["reduced_motion"]; ok {
		a.SetReducedMotion(v)
	}
	if v, ok := cfg["large_text"]; ok {
		a.SetLargeText(v)
	}
}

func (a *AccessibilityMode) Describe(view ViewKind) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("Current view: %s", view.String()))
	switch view {
	case ViewChat:
		parts = append(parts, "Chat view. Type a message and press Ctrl+S to send. Use up and down arrows to scroll through history.")
	case ViewTools:
		parts = append(parts, "Tools view. Use up and down arrows to navigate the tool list. Press R to run a selected tool.")
	case ViewSessions:
		parts = append(parts, "Sessions view. Use up and down arrows to navigate sessions. Press plus to add, minus to close.")
	case ViewTodos:
		parts = append(parts, "Todos view. Use up and down arrows to navigate todo items.")
	case ViewDAG:
		parts = append(parts, "DAG view. Use up and down arrows to navigate tasks.")
	case ViewLSP:
		parts = append(parts, "LSP diagnostics view. Shows language server protocol diagnostics.")
	case ViewKanban:
		parts = append(parts, "Kanban board view. Use up and down arrows to navigate cards. Use left and right arrows to move cards between columns.")
	default:
		parts = append(parts, "Use Tab to switch views. Press question mark for help.")
	}
	if a.highContrast {
		parts = append(parts, "High contrast mode is enabled.")
	}
	if a.reducedMotion {
		parts = append(parts, "Reduced motion mode is enabled. Animations are disabled.")
	}
	if a.largeText {
		parts = append(parts, "Large text mode is enabled.")
	}
	return strings.Join(parts, " ")
}

func (a *AccessibilityMode) DescribeMessage(msg ChatMessage) string {
	var role string
	switch msg.Kind {
	case chatUser:
		role = "User message"
	case chatAssistant:
		role = "Assistant message"
	case chatTool:
		role = "Tool call"
	case chatVerify:
		role = "Verification"
	case chatAsk:
		role = "Permission request"
	case chatDone:
		role = "Task complete"
	case chatError:
		role = "Error"
	case chatThinking:
		role = "Thinking"
	case chatSystem:
		role = "System message"
	case chatAgent:
		role = "Agent message"
	default:
		role = "Message"
	}
	var parts []string
	parts = append(parts, role)
	if msg.Text != "" {
		parts = append(parts, msg.Text)
	}
	if msg.Tool != "" {
		parts = append(parts, fmt.Sprintf("Tool: %s", msg.Tool))
	}
	if msg.Detail != "" {
		parts = append(parts, msg.Detail)
	}
	if msg.ToolInput != "" {
		parts = append(parts, fmt.Sprintf("Input: %s", msg.ToolInput))
	}
	if msg.ToolOutput != "" {
		output := msg.ToolOutput
		if len(output) > 200 {
			output = output[:200] + "..."
		}
		parts = append(parts, fmt.Sprintf("Output: %s", output))
	}
	if msg.Result {
		parts = append(parts, "Result: success")
	}
	if msg.Error != nil {
		parts = append(parts, fmt.Sprintf("Error: %s", msg.Error.Error()))
	}
	if !msg.Timestamp.IsZero() {
		parts = append(parts, fmt.Sprintf("Time: %s", msg.Timestamp.Format("15:04:05")))
	}
	return strings.Join(parts, ". ")
}

func (a *AccessibilityMode) AllKeyboardShortcuts() []HintPair {
	return DefaultHints(ViewChat)
}

func (a *AccessibilityMode) AllKeyboardShortcutsForView(view ViewKind) []HintPair {
	hints := DefaultHints(view)
	hints = append(hints, HintPair{Key: "?", Label: "help"})
	hints = append(hints, HintPair{Key: "ctrl+p", Label: "palette"})
	hints = append(hints, HintPair{Key: "ctrl+b", Label: "sidebar"})
	hints = append(hints, HintPair{Key: "q", Label: "quit"})
	return hints
}

func (a *AccessibilityMode) StatusText(status string) string {
	switch status {
	case "running":
		return "[RUNNING] "
	case "passed":
		return "[PASSED] "
	case "failed":
		return "[FAILED] "
	case "idle":
		return "[IDLE] "
	case "pending":
		return "[PENDING] "
	default:
		return "[" + strings.ToUpper(status) + "] "
	}
}

func (a *AccessibilityMode) SpinnerText(elapsed time.Duration) string {
	if a.reducedMotion {
		return fmt.Sprintf("thinking... (%ds)", int(elapsed.Seconds()))
	}
	return ""
}

func (a *AccessibilityMode) ShouldAnimate() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return !a.reducedMotion
}

func (a *AccessibilityMode) ShouldShowMouseCursor() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return !a.keyboardOnly
}

func (a *AccessibilityMode) BoldAll() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.largeText
}

func (a *AccessibilityMode) ExtraPadding() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.largeText {
		return 2
	}
	return 0
}

func HighContrastTheme() Theme {
	return Theme{
		Name:       "HighContrast",
		Accent:     "#FFFF00",
		AccentDim:  "#FFD700",
		Text:       "#FFFFFF",
		TextDim:    "#CCCCCC",
		Background: "#000000",
		Border:     "#FFFF00",
		Success:    "#00FF00",
		Warn:       "#FFFF00",
		Error:      "#FF0000",
	}
}

func (a *AccessibilityMode) ApplyTheme(base Theme) Theme {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.highContrast {
		return HighContrastTheme()
	}
	return base
}

func (a *AccessibilityMode) ApplyToStyles(base Styles) Styles {
	a.mu.RLock()
	hc := a.highContrast
	lt := a.largeText
	a.mu.RUnlock()
	if hc {
		return NewStyles(HighContrastTheme())
	}
	if lt {
		s := base
		s.Content = s.Content.Bold(true)
		s.Muted = s.Muted.Bold(true)
		s.Footer = s.Footer.Bold(true)
		s.FooterKey = s.FooterKey.Bold(true)
		s.AccentText = s.AccentText.Bold(true)
		return s
	}
	return base
}
