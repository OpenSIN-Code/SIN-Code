// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: when a second <type>-related handler is needed, merge into a shared file
package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
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

func renderThinkingIndicator(spinner Spinner, styles Styles) string {
	anim := NewThinkingAnimation()
	anim.SetFrame(spinner.Frame() % len(thinkingFrames))
	return anim.Render(styles)
}

func renderMarkdownWithCodeBlocks(text string, highlighter *SyntaxHighlighter, styles Styles, width int) string {
	if text == "" {
		return ""
	}

	lines := strings.Split(text, "\n")
	var b strings.Builder
	var textBuf strings.Builder
	inBlock := false
	var blockLang string
	var blockBuf strings.Builder

	flushText := func() {
		if textBuf.Len() == 0 {
			return
		}
		rendered := renderMarkdownSimple(textBuf.String(), styles, width)
		rendered = LinkifyText(rendered, styles)
		b.WriteString(strings.TrimRight(rendered, "\n"))
		textBuf.Reset()
	}

	flushCode := func() {
		code := strings.TrimSuffix(blockBuf.String(), "\n")
		code = strings.TrimPrefix(code, "\n")
		if code != "" {
			rendered := renderCodeBlock(code, blockLang, highlighter, styles, width, false)
			b.WriteString(rendered)
		}
		blockBuf.Reset()
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if !inBlock {
				flushText()
				if b.Len() > 0 {
					b.WriteString("\n\n")
				}
				inBlock = true
				blockLang = strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
			} else {
				flushCode()
				b.WriteString("\n")
				inBlock = false
				blockLang = ""
			}
		} else if inBlock {
			blockBuf.WriteString(line)
			blockBuf.WriteString("\n")
		} else {
			textBuf.WriteString(line)
			textBuf.WriteString("\n")
		}
	}

	if inBlock {
		flushCode()
	} else {
		flushText()
	}

	return b.String()
}

func renderMarkdownSimple(text string, styles Styles, width int) string {
	if text == "" {
		return ""
	}
	if !hasMarkdownSyntax(text) {
		return strings.TrimRight(text, "\n")
	}
	r := getCachedRenderer(width)
	if r == nil {
		return strings.TrimRight(text, "\n")
	}
	rendered, err := r.Render(text)
	if err != nil {
		return strings.TrimRight(text, "\n")
	}
	return strings.TrimSpace(rendered)
}

func renderChatMessageCompact(msg ChatMessage, md *markdownRenderer, styles Styles, width int, focused bool, spinner Spinner) string {
	highlighter := NewSyntaxHighlighter(styles.Theme)
	isStreaming := msg.Kind == chatThinking
	return renderChatMessageV2(msg, highlighter, styles, width, focused, spinner, isStreaming)
}

func renderVerificationCompact(status, message string, styles Styles) string {
	switch status {
	case "pass":
		return styles.StatusOK.Render("✓ " + message)
	case "fail":
		return styles.StatusErr.Render("✗ " + message)
	default:
		return styles.StatusWarn.Render("⏳ " + message)
	}
}

func (m *Model) chatViewHelp() string {
	if m.ChatInput == nil {
		return "Ctrl+S submit · /attach <path> · /clear"
	}
	return fmt.Sprintf("Ctrl+S submit · /attach <path> · /clear · %d attachments", len(m.ChatInput.Attachments()))
}

func (m *Model) renderChatFooter(styles Styles, width int) string {
	if width < 20 {
		width = 20
	}

	var b strings.Builder

	tokens := m.Footer.Tokens
	tokensPct := m.Footer.TokensPct
	cost := m.Footer.Cost
	agent := m.Footer.AgentName()

	left := styles.FooterKey.Render(" " + agent + " ")
	mid := styles.Muted.Render(fmt.Sprintf("tokens %d (%.0f%%)", tokens, tokensPct*100))
	right := styles.FooterVal.Render(cost)

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap > lipgloss.Width(mid) {
		mid += strings.Repeat(" ", gap-lipgloss.Width(mid))
	}

	b.WriteString(styles.Footer.Render(left + mid + right))
	return b.String()
}

func (m *Model) updateChatMetrics(tokens int, cost float64, duration time.Duration) {
	m.Footer.Tokens = tokens
	m.Footer.Cost = fmt.Sprintf("$%.2f", cost)
	m.Footer.Duration = duration
	m.Footer.TokensPct = float64(tokens) / 128000.0
	if m.Footer.TokensPct > 1.0 {
		m.Footer.TokensPct = 1.0
	}

	if m.Footer.TokensPct >= 0.8 && !m.Footer.Compacted {
		m.autoCompactContext()
	}
}

func (m *Model) autoCompactContext() {
	if len(m.ChatHistory) <= 50 {
		return
	}

	summary := fmt.Sprintf("[context compacted: %d messages removed to free up space]", len(m.ChatHistory)-20)
	m.ChatHistory = append([]ChatMessage{{Kind: chatSystem, Text: summary}}, m.ChatHistory[len(m.ChatHistory)-20:]...)
	m.Footer.Compacted = true

	m.SetBanner(&NotificationItem{
		ID:      "auto-compact",
		Title:   "Context Compacted",
		Message: fmt.Sprintf("Reduced from %d to 20 messages to stay within token limit", len(m.ChatHistory)),
		Type:    "info",
	})

	m.AppendHistory(ViewChat.String(), "auto-compact", summary, true)
}

func (m *Model) manualCompactContext() {
	if len(m.ChatHistory) <= 10 {
		m.appendChat(ChatMessage{Kind: chatSystem, Text: "Not enough messages to compact (need >10)"})
		return
	}
	before := len(m.ChatHistory)
	keep := 8
	summary := fmt.Sprintf("[manual compaction: %d messages removed to free up space]", before-keep)
	m.ChatHistory = append([]ChatMessage{{Kind: chatSystem, Text: summary}}, m.ChatHistory[before-keep:]...)
	m.Footer.Compacted = true

	m.SetBanner(&NotificationItem{
		ID:      "manual-compact",
		Title:   "Context Compacted",
		Message: fmt.Sprintf("Reduced from %d to %d messages", before, keep+1),
		Type:    "info",
	})

	m.AppendHistory(ViewChat.String(), "manual-compact", summary, true)
}

func (m *Model) setStreaming(streaming bool) {
	m.Footer.Streaming = streaming
	if streaming && m.TypewriterBuf != nil {
		m.TypewriterBuf.Reset()
	}
	if m.ChatInput != nil {
		m.ChatInput.SetDisabled(streaming)
	}
}

// IsStreaming reports whether the model is currently receiving a streaming
// response or waiting for the agent loop to produce output.
func (m *Model) IsStreaming() bool {
	return m.Footer.Streaming
}

func (m *Model) updateSessionPreview() {
	if len(m.ChatHistory) == 0 {
		return
	}

	for i := len(m.ChatHistory) - 1; i >= 0; i-- {
		msg := m.ChatHistory[i]
		if msg.Kind == chatAssistant {
			preview := msg.Text
			if len(preview) > 60 {
				preview = preview[:57] + "..."
			}
			m.Tabs.UpdatePreview(m.Tabs.ActiveIdx, preview)
			return
		}
	}
}

func renderToolCall(name, args string, styles Styles) string {
	var b strings.Builder
	b.WriteString(styles.AccentText.Render("⚡ "))
	b.WriteString(styles.Bold.Render(name))
	if args != "" {
		b.WriteString(styles.Muted.Render(" " + args))
	}
	return b.String()
}

func renderVerification(status, message string, styles Styles) string {
	var b strings.Builder
	switch status {
	case "pass":
		b.WriteString(styles.StatusOK.Render("✅ "))
		b.WriteString(styles.StatusOK.Render(message))
	case "fail":
		b.WriteString(styles.StatusErr.Render("❌ "))
		b.WriteString(styles.StatusErr.Render(message))
	default:
		b.WriteString(styles.StatusWarn.Render("⏳ "))
		b.WriteString(styles.StatusWarn.Render(message))
	}
	return b.String()
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func renderError(err string, styles Styles) string {
	return styles.StatusErr.Render("❌ " + err)
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func renderErrorExpanded(msg ChatMessage, styles Styles, width int) string {
	return renderErrorBubble(msg, styles, width)
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

func renderToolOutput(output string, styles Styles, width int) string {
	if output == "" {
		return ""
	}
	if width < 10 {
		width = 10
	}

	lines := strings.Split(output, "\n")
	const maxLines = 50
	if len(lines) > maxLines {
		output = strings.Join(lines[:maxLines], "\n")
		output += fmt.Sprintf("\n⋯ %d more lines (use /tools to see full output)", len(lines)-maxLines)
	}

	highlighter := NewSyntaxHighlighter(styles.Theme)

	if looksLikeGoCode(output) {
		return renderCodeBlock(output, "go", highlighter, styles, width, false) + "\n"
	}

	panelStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(c(styles.Theme.Accent)).
		Padding(0, 1).
		Width(width - 2)

	return panelStyle.Render(styles.Content.Render(output))
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func renderSpinner(text string, styles Styles) string {
	return styles.AccentText.Render("⏳ " + text)
}

type chatBubbleStyles struct {
	user      lipgloss.Style
	assistant lipgloss.Style
	tool      lipgloss.Style
	system    lipgloss.Style
	error     lipgloss.Style
	success   lipgloss.Style
	warning   lipgloss.Style
	timestamp lipgloss.Style
}

func newChatBubbleStyles(styles Styles) chatBubbleStyles {
	t := styles.Theme
	return chatBubbleStyles{
		user: lipgloss.NewStyle().
			Foreground(c(t.Background)).
			Background(c(t.Accent)).
			Bold(true).
			Padding(0, 2).
			Border(lipgloss.RoundedBorder()),

		assistant: lipgloss.NewStyle().
			Foreground(c(t.Text)).
			PaddingLeft(2).
			PaddingRight(2),

		tool: lipgloss.NewStyle().
			Foreground(c(t.AccentDim)).
			PaddingLeft(2).
			PaddingRight(2),

		system: lipgloss.NewStyle().
			Foreground(c(t.TextDim)).
			Italic(true).
			Padding(0, 2),

		error: lipgloss.NewStyle().
			Foreground(c(t.Error)).
			BorderLeft(true).
			BorderLeftForeground(c(t.Error)).
			Background(lipgloss.Color(t.Background)).
			Padding(0, 1).
			Bold(true),

		success: lipgloss.NewStyle().
			Foreground(c(t.Success)).
			Bold(true).
			Padding(0, 2),

		warning: lipgloss.NewStyle().
			Foreground(c(t.Warn)).
			Bold(true).
			Padding(0, 2),

		timestamp: lipgloss.NewStyle().
			Foreground(c(t.TextDim)).
			Faint(true),
	}
}

func renderChatBubble(role, content string, styles chatBubbleStyles, width int) string {
	var b strings.Builder

	switch role {
	case "user":
		label := styles.user.Render(" You ")
		ts := styles.timestamp.Render(time.Now().Format("15:04:05"))
		b.WriteString(label)
		b.WriteString("  ")
		b.WriteString(ts)
		b.WriteString("\n")
		wrapped := wrapText(content, width-6)
		b.WriteString(wrapped)
	case "assistant":
		label := styles.assistant.Render("Assistant")
		ts := styles.timestamp.Render(time.Now().Format("15:04:05"))
		b.WriteString(label)
		b.WriteString("  ")
		b.WriteString(ts)
		b.WriteString("\n")
		wrapped := wrapText(content, width-6)
		b.WriteString(wrapped)
	case "tool":
		b.WriteString(styles.tool.Render("⚡ " + content))
	case "system":
		b.WriteString(styles.system.Render("⚠ " + content))
	case "error":
		b.WriteString(styles.error.Render("❌ " + content))
	case "success":
		b.WriteString(styles.success.Render("✅ " + content))
	case "warning":
		b.WriteString(styles.warning.Render("⚠ " + content))
	default:
		b.WriteString(content)
	}

	return b.String()
}

func wrapText(text string, width int) string {
	if width <= 0 {
		width = 80
	}

	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		if len(paragraph) <= width {
			lines = append(lines, paragraph)
			continue
		}

		words := strings.Fields(paragraph)
		var currentLine strings.Builder
		for _, word := range words {
			if currentLine.Len()+len(word)+1 > width {
				if currentLine.Len() > 0 {
					lines = append(lines, currentLine.String())
					currentLine.Reset()
				}
			}
			if currentLine.Len() > 0 {
				currentLine.WriteString(" ")
			}
			currentLine.WriteString(word)
		}
		if currentLine.Len() > 0 {
			lines = append(lines, currentLine.String())
		}
	}

	return strings.Join(lines, "\n")
}

func indentText(text string, indent int) string {
	prefix := strings.Repeat(" ", indent)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func renderChatDivider(styles Styles, width int) string {
	return styles.Muted.Render(strings.Repeat("─", width))
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func renderChatHeader(title string, styles Styles) string {
	return styles.ContentHdr.Render("💬 " + title)
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func renderChatStatus(status string, styles Styles) string {
	return styles.Muted.Render(status)
}

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

func renderUserBubble(msg ChatMessage, styles Styles, width int) string {
	var b strings.Builder

	labelStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Background)).
		Background(c(styles.Theme.Accent)).
		Bold(true).
		Padding(0, 1)

	tsStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.TextDim)).
		Faint(true)

	label := labelStyle.Render("You")
	ts := tsStyle.Render(formatTimestamp(msg.Timestamp))

	prefixWidth := lipgloss.Width(label + "  " + ts)
	_ = prefixWidth
	bodyWidth := width - 6
	if bodyWidth < 10 {
		bodyWidth = 10
	}
	wrapped := wrapText(msg.Text, bodyWidth)

	bodyStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Text)).
		Background(lipgloss.Color(styles.Theme.Background)).
		Padding(0, 1).
		Width(bodyWidth)

	body := bodyStyle.Render(wrapped)

	rightAligned := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Right)

	content := label + "  " + ts + "\n" + body
	b.WriteString(rightAligned.Render(content))
	b.WriteString("\n")
	return b.String()
}

func renderAssistantBubble(msg ChatMessage, highlighter *SyntaxHighlighter, styles Styles, width int, streaming bool, spinner Spinner) string {
	var b strings.Builder

	labelStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Accent)).
		Bold(true)

	tsStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.TextDim)).
		Faint(true)

	label := labelStyle.Render("Assistant")
	ts := tsStyle.Render(formatTimestamp(msg.Timestamp))

	headerLine := label + "  " + ts

	bodyWidth := width - 6
	if bodyWidth < 10 {
		bodyWidth = 10
	}

	rendered := renderMarkdownWithCodeBlocks(msg.Text, highlighter, styles, bodyWidth)

	if strings.TrimSpace(rendered) == "" {
		if streaming {
			rendered = styles.Muted.Render("Thinking…")
		} else {
			return headerLine + "\n"
		}
	}

	if streaming {
		cursor := renderStreamingCursor(spinner, styles)
		rendered = strings.TrimRight(rendered, "\n") + cursor
	}

	bodyStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Text)).
		PaddingLeft(2).
		PaddingRight(2).
		Width(bodyWidth)

	body := bodyStyle.Render(rendered)

	b.WriteString(headerLine)
	b.WriteString("\n")
	b.WriteString(body)
	b.WriteString("\n")
	return b.String()
}

func renderSystemBubble(msg ChatMessage, styles Styles, width int) string {
	text := msg.Text
	if text == "" {
		text = msg.Detail
	}

	style := lipgloss.NewStyle().
		Foreground(c(styles.Theme.TextDim)).
		Italic(true).
		Align(lipgloss.Center).
		Width(width)

	return style.Render(text) + "\n"
}

func renderErrorBubble(msg ChatMessage, styles Styles, width int) string {
	errText := msg.Text
	if errText == "" && msg.Error != nil {
		errText = msg.Error.Error()
	}

	bodyWidth := width - 6
	if bodyWidth < 10 {
		bodyWidth = 10
	}

	category := "Error"
	hint := "Press Esc to interrupt, then retry"
	if strings.Contains(errText, "context deadline exceeded") || strings.Contains(errText, "timeout") {
		category = "Timeout"
		hint = "The request took too long. Try again or use a faster model."
	} else if strings.Contains(errText, "connection refused") || strings.Contains(errText, "no such host") {
		category = "Network Error"
		hint = "Check your internet connection and API endpoint."
	} else if strings.Contains(errText, "unauthorized") || strings.Contains(errText, "401") || strings.Contains(errText, "api key") {
		category = "Auth Error"
		hint = "Check your API key with: sin-code config get llm.api_key"
	} else if strings.Contains(errText, "rate limit") || strings.Contains(errText, "429") {
		category = "Rate Limited"
		hint = "Too many requests. Wait a moment and try again."
	} else if strings.Contains(errText, "permission denied") {
		category = "Permission Denied"
		hint = "This action was blocked by the permission engine. Use --yolo to allow."
	}

	catStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Error)).
		Bold(true)

	bodyStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Text)).
		BorderLeft(true).
		BorderLeftForeground(c(styles.Theme.Error)).
		Padding(0, 1).
		Width(bodyWidth)

	hintStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.TextDim)).
		Faint(true)

	if !msg.Expanded {
		short := truncateString(errText, bodyWidth-4)
		return catStyle.Render("❌ "+category+": ") + styles.StatusErr.Render(short) + "\n"
	}

	var b strings.Builder
	b.WriteString(catStyle.Render("❌ " + category))
	b.WriteString("\n")
	b.WriteString(bodyStyle.Render(errText))
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("  → " + hint))
	b.WriteString("\n")
	return b.String()
}

func renderToolCard(msg ChatMessage, styles Styles, width int, focused bool) string {
	focusPrefix := ""
	if focused {
		focusPrefix = "▸ "
	}

	bodyWidth := width - 6
	if bodyWidth < 10 {
		bodyWidth = 10
	}

	if msg.Expanded {
		var b strings.Builder

		iconStyle := lipgloss.NewStyle().
			Foreground(c(styles.Theme.Accent)).
			Bold(true)

		hdr := iconStyle.Render(focusPrefix + "⚡ " + msg.Tool)
		b.WriteString(hdr)
		b.WriteString("\n")

		if msg.ToolInput != "" {
			inputText := msg.ToolInput
			if lipgloss.Width(inputText) > bodyWidth-10 {
				inputText = truncateString(inputText, bodyWidth-13)
			}
			b.WriteString(styles.Muted.Render("  in: "))
			b.WriteString(styles.Muted.Render(inputText))
			b.WriteString("\n")
		}

		output := msg.ToolOutput
		if output == "" {
			output = msg.Detail
		}
		if output != "" {
			rendered := renderToolOutput(output, styles, bodyWidth)
			b.WriteString(rendered)
		}

		cardStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(c(styles.Theme.AccentDim)).
			Padding(0, 1).
			Width(bodyWidth)

		return cardStyle.Render(b.String()) + "\n"
	}

	var b strings.Builder
	if msg.Result {
		b.WriteString(styles.StatusOK.Render(focusPrefix + "✓ " + msg.Tool))
	} else {
		b.WriteString(styles.AccentText.Render(focusPrefix + "⚡ " + msg.Tool))
	}
	if msg.Detail != "" {
		detail := msg.Detail
		if len(detail) > 60 {
			detail = detail[:57] + "..."
		}
		b.WriteString(styles.Muted.Render(" → " + detail))
	}
	b.WriteString("\n")
	return b.String()
}

func renderStreamingCursor(spinner Spinner, styles Styles) string {
	visible := spinner.pulse%2 == 0
	if visible {
		return styles.AccentText.Render("▋")
	}
	return " "
}

func renderTypingDots(spinner Spinner, styles Styles) string {
	phase := spinner.frame % 3
	switch phase {
	case 0:
		return styles.Muted.Render("·  ")
	case 1:
		return styles.Muted.Render("·· ")
	default:
		return styles.Muted.Render("···")
	}
}
