// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// WelcomeInfo carries the dynamic data shown on the welcome screen.
type WelcomeInfo struct {
	ModelName  string
	Session    string
	Workspace  string
	VerifyMode string
}

// RenderWelcome renders the initial welcome banner shown when the TUI
// starts with no chat history. The banner is centered within the given
// width and height.
func RenderWelcome(styles Styles, info WelcomeInfo, width, height int) string {
	if width < 10 {
		width = 10
	}
	if height < 6 {
		height = 6
	}

	modelName := info.ModelName
	if modelName == "" {
		modelName = "unknown"
	}
	session := info.Session
	if session == "" {
		session = "new"
	}
	workspace := info.Workspace
	if workspace == "" {
		workspace = "."
	}
	verifyMode := info.VerifyMode
	if verifyMode == "" {
		verifyMode = "poc"
	}

	maxWorkspace := width - 18
	if maxWorkspace < 10 {
		maxWorkspace = 10
	}
	if len(workspace) > maxWorkspace {
		workspace = "…" + workspace[len(workspace)-maxWorkspace+1:]
	}

	// Banner box — shrink for narrow terminals.
	bannerTitle := "S I N - C o d e"
	bannerSub := "verification-first coding agent"
	bannerInner := len(bannerTitle)
	if len(bannerSub) > bannerInner {
		bannerInner = len(bannerSub)
	}
	bannerInner += 4 // padding inside box
	if bannerInner > width-8 {
		bannerInner = width - 8
	}
	if bannerInner < 10 {
		bannerInner = 10
	}

	topBorder := "╔" + strings.Repeat("═", bannerInner) + "╗"
	midLine1 := "║" + centerString(bannerTitle, bannerInner) + "║"
	midLine2 := "║" + centerString(bannerSub, bannerInner) + "║"
	botBorder := "╚" + strings.Repeat("═", bannerInner) + "╝"
	banner := topBorder + "\n" + midLine1 + "\n" + midLine2 + "\n" + botBorder

	// Info lines.
	infoLines := fmt.Sprintf("Model:    %s\nSession:  %s\nWorkspace: %s",
		modelName, session, workspace)

	// Quick start hints.
	quickStart := "Quick start:\n" +
		"  Type a message and press Enter to chat\n" +
		"  /help for all commands\n" +
		"  /agent to spawn a sub-agent\n" +
		"  Ctrl+P for command palette\n" +
		"  Ctrl+T for tool tree"

	// Verification gate status.
	var modeLabel string
	switch strings.ToLower(verifyMode) {
	case "poc":
		modeLabel = "PoC"
	case "oracle":
		modeLabel = "Oracle"
	case "off":
		modeLabel = "Off"
	default:
		modeLabel = verifyMode
		if len(modeLabel) > 0 {
			modeLabel = strings.ToUpper(modeLabel[:1]) + modeLabel[1:]
		} else {
			modeLabel = "PoC"
		}
	}
	verifyStatus := fmt.Sprintf("Verification Gate: ACTIVE (%s mode)", modeLabel)

	// Assemble the content block.
	var content strings.Builder
	content.WriteString(styles.AccentText.Render(banner))
	content.WriteString("\n\n")
	content.WriteString(styles.Muted.Render(infoLines))
	content.WriteString("\n\n")
	content.WriteString(styles.Muted.Render(quickStart))
	content.WriteString("\n\n")
	content.WriteString(styles.StatusOK.Render(verifyStatus))

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Top, content.String())
}

// centerString pads s with spaces on both sides so it is centered within w.
func centerString(s string, w int) string {
	if len(s) >= w {
		return s
	}
	total := w - len(s)
	left := total / 2
	right := total - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}
