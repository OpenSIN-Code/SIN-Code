// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
)

type SearchResult struct {
	MessageIdx int
	Text       string
	MatchPos   int
	MatchLen   int
	Context    string
}

type ChatSearch struct {
	mu        sync.Mutex
	results   []SearchResult
	cursor    int
	query     string
	history   []ChatMessage
	maxResult int
}

func NewChatSearch() *ChatSearch {
	return &ChatSearch{
		maxResult: 100,
	}
}

func (s *ChatSearch) Search(history []ChatMessage, query string) []SearchResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.query = query
	s.results = nil
	s.cursor = 0
	s.history = history

	if query == "" || len(history) == 0 {
		return nil
	}

	q := strings.ToLower(query)
	for i, msg := range history {
		text := msg.Text
		if text == "" {
			text = msg.Detail
		}
		if text == "" {
			text = msg.Tool
		}
		if text == "" {
			continue
		}

		lowerText := strings.ToLower(text)
		searchStart := 0
		for {
			pos := strings.Index(lowerText[searchStart:], q)
			if pos < 0 {
				break
			}
			actualPos := searchStart + pos
			ctx := extractContext(text, actualPos, len(q))
			s.results = append(s.results, SearchResult{
				MessageIdx: i,
				Text:       text,
				MatchPos:   actualPos,
				MatchLen:   len(q),
				Context:    ctx,
			})
			searchStart = actualPos + len(q)
			if len(s.results) >= s.maxResult {
				break
			}
		}
	}

	if len(s.results) > 0 {
		out := make([]SearchResult, len(s.results))
		copy(out, s.results)
		return out
	}
	return nil
}

func extractContext(text string, matchPos, matchLen int) string {
	const radius = 30
	start := matchPos - radius
	if start < 0 {
		start = 0
	}
	end := matchPos + matchLen + radius
	if end > len(text) {
		end = len(text)
	}
	ctx := text[start:end]
	prefix := ""
	suffix := ""
	if start > 0 {
		prefix = "…"
	}
	if end < len(text) {
		suffix = "…"
	}
	return prefix + ctx + suffix
}

func (s *ChatSearch) Results() []SearchResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SearchResult, len(s.results))
	copy(out, s.results)
	return out
}

func (s *ChatSearch) Query() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.query
}

func (s *ChatSearch) Next() *SearchResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.results) == 0 {
		return nil
	}
	s.cursor++
	if s.cursor >= len(s.results) {
		s.cursor = 0
	}
	r := s.results[s.cursor]
	return &r
}

func (s *ChatSearch) Prev() *SearchResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.results) == 0 {
		return nil
	}
	s.cursor--
	if s.cursor < 0 {
		s.cursor = len(s.results) - 1
	}
	r := s.results[s.cursor]
	return &r
}

func (s *ChatSearch) CurrentResult() *SearchResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.results) == 0 || s.cursor < 0 || s.cursor >= len(s.results) {
		return nil
	}
	r := s.results[s.cursor]
	return &r
}

func (s *ChatSearch) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results = nil
	s.cursor = 0
	s.query = ""
	s.history = nil
}

func (s *ChatSearch) Render(styles Styles, width, height int) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if width < 20 {
		width = 20
	}
	if height < 6 {
		height = 6
	}

	innerWidth := width - 6
	if innerWidth < 14 {
		innerWidth = 14
	}

	var b strings.Builder

	b.WriteString(styles.AccentText.Render("/search: "))
	if s.query != "" {
		b.WriteString(styles.Bold.Render(s.query))
	} else {
		b.WriteString(styles.Muted.Render("(type to search)"))
	}
	if len(s.results) > 0 {
		b.WriteString(styles.Muted.Render(" [" + itoa(s.cursor+1) + "/" + itoa(len(s.results)) + "]"))
	}
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  " + strings.Repeat("─", max(innerWidth, 10))))
	b.WriteString("\n")

	resultsToShow := height - 6
	if resultsToShow < 3 {
		resultsToShow = 3
	}
	if resultsToShow > len(s.results) {
		resultsToShow = len(s.results)
	}

	if len(s.results) == 0 {
		if s.query != "" {
			b.WriteString(styles.Muted.Render("  (no matches found)"))
		} else {
			b.WriteString(styles.Muted.Render("  (type a search query)"))
		}
		b.WriteString("\n")
	} else {
		for i := 0; i < resultsToShow; i++ {
			r := s.results[i]
			role := searchResultRole(r.MessageIdx, s.history)
			line := formatSearchResultLine(r, role, s.query, styles, innerWidth)
			if i == s.cursor {
				selStyle := lipgloss.NewStyle().
					Foreground(lipgloss.Color(styles.Theme.Background)).
					Background(lipgloss.Color(styles.Theme.Accent)).
					Bold(true)
				b.WriteString(selStyle.Render(padRight("  "+stripANSI(line), innerWidth+2)))
			} else {
				b.WriteString(styles.PopupItem.Render(padRight("  "+line, innerWidth+2)))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  ↑/↓ navigate · Enter jump · Esc close"))
	b.WriteString("\n")

	popupStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(styles.Theme.Accent)).
		Foreground(lipgloss.Color(styles.Theme.Text)).
		Background(lipgloss.Color(styles.Theme.Background)).
		Padding(0, 1)
	return popupStyle.Render(b.String())
}

func searchResultRole(idx int, history []ChatMessage) string {
	if history != nil && idx >= 0 && idx < len(history) {
		switch history[idx].Kind {
		case chatUser:
			return "You"
		case chatAssistant:
			return "Assistant"
		case chatTool:
			return "Tool"
		case chatSystem:
			return "System"
		case chatError:
			return "Error"
		case chatVerify:
			return "Verify"
		case chatDone:
			return "Done"
		case chatAgent:
			return "Agent"
		default:
			return "Msg"
		}
	}
	return "Msg"
}

func formatSearchResultLine(r SearchResult, role, query string, styles Styles, width int) string {
	available := width - len(role) - 4
	if available < 8 {
		available = 8
	}

	ctx := r.Context
	if len(ctx) > available {
		ctx = truncateString(ctx, available)
	}

	highlighted := highlightMatchText(ctx, query, styles)
	return role + ": " + highlighted
}

func highlightMatchText(ctx, query string, styles Styles) string {
	if query == "" {
		return ctx
	}
	warnStyle := styles.StatusWarn
	lowerCtx := strings.ToLower(ctx)
	lowerQuery := strings.ToLower(query)

	var b strings.Builder
	i := 0
	for i < len(ctx) {
		idx := strings.Index(lowerCtx[i:], lowerQuery)
		if idx < 0 {
			b.WriteString(ctx[i:])
			break
		}
		if idx > 0 {
			b.WriteString(ctx[i : i+idx])
		}
		end := i + idx + len(query)
		if end > len(ctx) {
			end = len(ctx)
		}
		b.WriteString(warnStyle.Render(ctx[i+idx : end]))
		i = end
	}
	return b.String()
}

func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}



func (s *ChatSearch) RenderBar(styles Styles) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b strings.Builder
	b.WriteString(styles.AccentText.Render("/search: "))
	if s.query != "" {
		b.WriteString(styles.Bold.Render(s.query))
	} else {
		b.WriteString(styles.Muted.Render("(type to search)"))
	}
	if len(s.results) > 0 {
		b.WriteString(styles.Muted.Render(" [" + itoa(s.cursor+1) + "/" + itoa(len(s.results)) + "]"))
	}
	return b.String()
}
