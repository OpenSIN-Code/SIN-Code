// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"testing"
)

func TestWelcomeScreen_Render(t *testing.T) {
	styles := NewStyles(Themes[0])
	info := WelcomeInfo{
		ModelName:  "glm-5p2 (Fireworks AI)",
		Session:    "new",
		Workspace:  "/Users/jeremy/dev/SIN-Code-Bundle",
		VerifyMode: "poc",
	}
	out := RenderWelcome(styles, info, 80, 30)

	if !strings.Contains(out, "S I N - C o d e") {
		t.Error("expected banner title in render output")
	}
	if !strings.Contains(out, "verification-first coding agent") {
		t.Error("expected banner subtitle in render output")
	}
	if !strings.Contains(out, "glm-5p2") {
		t.Error("expected model name in render output")
	}
	if !strings.Contains(out, "Workspace:") {
		t.Error("expected workspace label in render output")
	}
	if !strings.Contains(out, "Verification Gate:") {
		t.Error("expected verification gate status in render output")
	}
	if !strings.Contains(out, "PoC mode") {
		t.Error("expected PoC mode in render output")
	}
	if !strings.Contains(out, "/help") {
		t.Error("expected /help hint in render output")
	}
	if !strings.Contains(out, "Ctrl+P") {
		t.Error("expected Ctrl+P hint in render output")
	}
}

func TestWelcomeScreen_Defaults(t *testing.T) {
	styles := NewStyles(Themes[0])
	info := WelcomeInfo{} // all empty
	out := RenderWelcome(styles, info, 80, 30)

	if !strings.Contains(out, "unknown") {
		t.Error("expected 'unknown' model name when empty")
	}
	if !strings.Contains(out, "new") {
		t.Error("expected 'new' session when empty")
	}
	if !strings.Contains(out, "PoC mode") {
		t.Error("expected PoC mode when verify mode is empty")
	}
}

func TestWelcomeScreen_NarrowWidth(t *testing.T) {
	styles := NewStyles(Themes[0])
	info := WelcomeInfo{
		ModelName:  "glm-5p2",
		Session:    "new",
		Workspace:  "/Users/jeremy/dev/SIN-Code-Bundle",
		VerifyMode: "poc",
	}

	// Should not panic or produce empty output at narrow widths.
	out := RenderWelcome(styles, info, 20, 10)
	if out == "" {
		t.Error("expected non-empty render for narrow width")
	}
	if !strings.Contains(out, "S I N") {
		t.Error("expected banner even in narrow width")
	}

	// Extremely narrow.
	out = RenderWelcome(styles, info, 10, 6)
	if out == "" {
		t.Error("expected non-empty render for very narrow width")
	}
}

func TestWelcomeScreen_LongWorkspace(t *testing.T) {
	styles := NewStyles(Themes[0])
	longPath := "/Users/jeremy/dev/some/very/very/very/long/workspace/path/that/should/be/truncated"
	info := WelcomeInfo{
		ModelName:  "glm-5p2",
		Session:    "new",
		Workspace:  longPath,
		VerifyMode: "poc",
	}
	out := RenderWelcome(styles, info, 60, 20)
	if !strings.Contains(out, "…") {
		t.Error("expected workspace truncation with ellipsis")
	}
}
