// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: when a second <type>-related handler is needed, merge into a shared file
package tui

import (
	"fmt"
	"strings"
)

// Merged from chat_view.go (issue #426).

func (m *Model) renderChat(styles Styles, width, height int) string {
	if width < 10 {
		width = 10
	}
	if height < 6 {
		height = 6
	}

	textHeight := 3
	if m.ChatInput != nil {
		raw := m.ChatInput.RawValue()
		lines := strings.Count(raw, "\n") + 1
		if lines > 3 {
			textHeight = min(lines+2, 10)
		}
	}
	chatHeight := height - textHeight - 3
	if chatHeight < 3 {
		chatHeight = 3
	}

	modelName := m.Footer.ModelName
	if modelName == "" {
		modelName = m.Footer.AgentName()
	}
	m.SessionInfo.Update(shortSessionID(m.Tabs.Active().Name), modelName, countUserTurns(m.ChatHistory), m.VerifyPanel.State == VerifyPassed)
	m.TokenBar.Update(m.Footer.Tokens, parseCostStr(m.Footer.Cost), modelName)

	if len(m.ChatHistory) == 0 {
		inputEmpty := true
		if m.ChatInput != nil {
			inputEmpty = m.ChatInput.RawValue() == ""
		}

		var viewportContent string
		if inputEmpty {
			info := WelcomeInfo{
				ModelName:  modelName,
				Session:    "new",
				Workspace:  m.Workspace,
				VerifyMode: m.AgentConfig.VerifyMode,
			}
			viewportContent = RenderWelcome(styles, info, width, chatHeight)
		}

		m.ChatViewport.SetWidth(width)
		m.ChatViewport.SetHeight(chatHeight)
		m.ChatViewport.SetContent(viewportContent)
		m.ChatViewport.GotoTop()

		var b strings.Builder
		b.WriteString(m.SessionInfo.Render(styles, width))
		b.WriteString("\n")
		b.WriteString(m.ChatViewport.View())
		b.WriteString("\n")
		b.WriteString(styles.Muted.Render(strings.Repeat("─", width)))
		b.WriteString("\n")
		b.WriteString(m.TokenBar.Render(styles, width))
		b.WriteString("\n")
		if m.ContextMeter != nil {
			m.ContextMeter.SetUsage(m.Footer.Tokens, m.Footer.EstimatedTokens)
			m.ContextMeter.Width = min(30, width/3)
			meterLine := m.ContextMeter.Render()
			if meterLine != "" {
				b.WriteString(meterLine)
				b.WriteString("\n")
			}
		}
		if m.ChatInput != nil {
			m.ChatInput.SetSize(width, textHeight)
			b.WriteString(m.ChatInput.View())
		}
		return b.String()
	}

	highlighter := NewSyntaxHighlighter(styles.Theme)

	var content strings.Builder
	isStreaming := m.Footer.Streaming
	compact := m.CompactMode != nil && m.CompactMode.Active()

	// Virtualized rendering: only render the last N messages that fit in the viewport
	maxVisibleMsgs := chatHeight / 4
	if maxVisibleMsgs < 5 {
		maxVisibleMsgs = 5
	}
	startIdx := 0
	if len(m.ChatHistory) > maxVisibleMsgs {
		startIdx = len(m.ChatHistory) - maxVisibleMsgs
	}
	if m.ScrollToMatchIdx >= 0 && m.ScrollToMatchIdx < startIdx {
		startIdx = m.ScrollToMatchIdx
		if startIdx < 0 {
			startIdx = 0
		}
	}
	if startIdx > 0 {
		content.WriteString(styles.Muted.Render(fmt.Sprintf("⋯ %d earlier messages (scroll up to view)", startIdx)))
		content.WriteString("\n")
	}

	for i := startIdx; i < len(m.ChatHistory); i++ {
		msg := m.ChatHistory[i]
		if compact {
			content.WriteString(renderCompactMessage(msg, styles, width, i == m.ChatFocusIdx, m.Spinner))
			continue
		}
		isLast := i == len(m.ChatHistory)-1
		msgStreaming := isLast && isStreaming && (msg.Kind == chatAssistant || msg.Kind == chatThinking)

		// Bug 3: Don't mutate msg.Text — use a copy for rendering only
		renderMsg := msg
		if msgStreaming && msg.Kind == chatAssistant && m.TypewriterBuf != nil {
			visible := m.TypewriterBuf.Visible()
			if visible != "" && len(visible) < len(msg.Text) {
				renderMsg.Text = visible
			}
		}

		block := ChatBlock{
			Role:      chatKindString(renderMsg.Kind),
			Timestamp: renderMsg.Timestamp,
			Collapsed: false,
			Width:     width,
		}
		if renderMsg.Kind == chatVerify {
			if strings.Contains(renderMsg.Detail, "PASS") {
				block.VerifyResult = "pass"
			} else if strings.Contains(renderMsg.Detail, "FAIL") {
				block.VerifyResult = "fail"
			}
		}
		if renderMsg.Kind == chatTool {
			block.ToolCalls = 1
		}
		content.WriteString(RenderBlockHeader(block, styles, width))
		content.WriteString("\n")

		// Bug 4: Use RenderCache for non-streaming messages
		if !msgStreaming && m.RenderCache != nil {
			cacheContent := fmt.Sprintf("%d|%s|%s|%s|%s|%s|%v|%v",
				renderMsg.Kind, renderMsg.Text, renderMsg.Tool,
				renderMsg.ToolInput, renderMsg.ToolOutput, renderMsg.Detail,
				renderMsg.Result, i == m.ChatFocusIdx)
			cacheKey := renderCacheKey(i, cacheContent, width, styles.Theme.Name)
			if cached, ok := m.RenderCache.Get(cacheKey); ok {
				content.WriteString(cached)
			} else {
				rendered := renderChatMessageV2(renderMsg, highlighter, styles, width, i == m.ChatFocusIdx, m.Spinner, msgStreaming)
				m.RenderCache.Set(cacheKey, rendered)
				content.WriteString(rendered)
			}
		} else {
			rendered := renderChatMessageV2(renderMsg, highlighter, styles, width, i == m.ChatFocusIdx, m.Spinner, msgStreaming)
			content.WriteString(rendered)
		}

		if i < len(m.ChatHistory)-1 {
			content.WriteString("\n")
		}
	}

	m.ChatViewport.SetWidth(width)
	m.ChatViewport.SetHeight(chatHeight)
	m.ChatViewport.SetContent(content.String())
	if !m.userScrolledUp && !m.ChatViewport.AtBottom() {
		m.ChatViewport.GotoBottom()
	}
	if m.ScrollToMatchIdx >= 0 && m.ScrollToMatchIdx < len(m.ChatHistory) {
		offset := (m.ScrollToMatchIdx - startIdx) * 4
		if offset < 0 {
			offset = 0
		}
		m.ChatViewport.SetYOffset(offset)
		m.ScrollToMatchIdx = -1
	}

	var b strings.Builder
	b.WriteString(m.SessionInfo.Render(styles, width))
	b.WriteString("\n")
	b.WriteString(m.ChatViewport.View())

	if m.Mode == ModeSearch {
		b.WriteString("\n")
		if m.ChatSearch != nil {
			b.WriteString(m.ChatSearch.RenderBar(styles))
		} else {
			b.WriteString(styles.AccentText.Render("/" + m.SearchInput.View()))
		}
	}

	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", width)))
	b.WriteString("\n")
	b.WriteString(m.TokenBar.Render(styles, width))
	b.WriteString("\n")
	if m.ContextMeter != nil {
		m.ContextMeter.SetUsage(m.Footer.Tokens, m.Footer.EstimatedTokens)
		m.ContextMeter.Width = min(30, width/3)
		meterLine := m.ContextMeter.Render()
		if meterLine != "" {
			b.WriteString(meterLine)
			b.WriteString("\n")
		}
	}
	if m.ChatInput != nil {
		m.ChatInput.SetSize(width, textHeight)
		b.WriteString(m.ChatInput.View())
	}

	return b.String()
}

func renderChatMessageV2(msg ChatMessage, highlighter *SyntaxHighlighter, styles Styles, width int, focused bool, spinner Spinner, streaming bool) string {
	var b strings.Builder

	focusPrefix := ""
	if focused {
		focusPrefix = "▸ "
	}

	switch msg.Kind {
	case chatUser:
		b.WriteString(renderUserBubble(msg, styles, width))
		b.WriteString("\n")

	case chatAssistant:
		b.WriteString(renderAssistantBubble(msg, highlighter, styles, width, streaming, spinner))
		b.WriteString("\n")

	case chatTool:
		b.WriteString(renderToolCard(msg, styles, width, focused))
		if isFileModifyingTool(msg.Tool) {
			diffText := extractDiffFromOutput(msg.ToolOutput)
			if diffText == "" {
				diffText = extractDiffFromOutput(msg.Detail)
			}
			if diffText != "" {
				renderer := NewDiffRenderer(styles)
				compact := renderer.RenderCompact(diffText, styles, width-4)
				if compact != "" {
					b.WriteString(compact)
					b.WriteString("\n")
				}
			}
		}
		b.WriteString("\n")

	case chatVerify:
		status := "pending"
		if strings.Contains(msg.Detail, "PASS") {
			status = "pass"
		} else if strings.Contains(msg.Detail, "FAIL") {
			status = "fail"
		}
		b.WriteString(renderVerificationCompact(status, msg.Detail, styles))
		b.WriteString("\n")

	case chatAsk:
		b.WriteString(styles.StatusWarn.Render(focusPrefix + "🔒 " + msg.Detail))
		b.WriteString("\n")

	case chatDone:
		detail := msg.Detail
		if len(detail) > 80 {
			detail = detail[:77] + "..."
		}
		b.WriteString(styles.StatusOK.Render(focusPrefix + "✓ " + detail))
		b.WriteString("\n")

	case chatError:
		b.WriteString(renderErrorBubble(msg, styles, width))

	case chatThinking:
		anim := NewThinkingAnimation()
		anim.SetFrame(spinner.Frame() % len(thinkingFrames))
		anim.SetStart(msg.Timestamp)
		b.WriteString(anim.Render(styles))
		b.WriteString("\n")

	case chatSystem:
		b.WriteString(renderSystemBubble(msg, styles, width))

	case chatAgent:
		b.WriteString(styles.Muted.Render(focusPrefix + "⟳ " + msg.Text))
		b.WriteString("\n")
	}

	return b.String()
}

func renderChatMessageCompact(msg ChatMessage, md *markdownRenderer, styles Styles, width int, focused bool, spinner Spinner) string {
	highlighter := NewSyntaxHighlighter(styles.Theme)
	isStreaming := msg.Kind == chatThinking
	return renderChatMessageV2(msg, highlighter, styles, width, focused, spinner, isStreaming)
}
