// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
	"unicode/utf8"
)

type SessionInfo struct {
	mu        sync.RWMutex
	sessionID string
	model     string
	turns     int
	verified  bool
}

func NewSessionInfo() *SessionInfo {
	return &SessionInfo{}
}

func (s *SessionInfo) Update(sessionID, model string, turns int, verified bool) {
	s.mu.Lock()
	s.sessionID = sessionID
	s.model = model
	s.turns = turns
	s.verified = verified
	s.mu.Unlock()
}

func (s *SessionInfo) Render(styles Styles, width int) string {
	if width < 10 {
		width = 10
	}
	s.mu.RLock()
	sessionID := s.sessionID
	model := s.model
	turns := s.turns
	verified := s.verified
	s.mu.RUnlock()

	verifiedStr := "✓"
	verifiedStyle := styles.StatusOK
	if !verified {
		verifiedStr = "✗"
		verifiedStyle = styles.StatusErr
	}

	prefix := fmt.Sprintf("session: %s · turns: %d · verified: ", sessionID, turns)
	modelLabel := " · model: "

	prefixW := utf8.RuneCountInString(prefix)
	labelW := utf8.RuneCountInString(modelLabel)
	verW := utf8.RuneCountInString(verifiedStr)
	avail := width - prefixW - verW - labelW

	if avail < 4 {
		plain := fmt.Sprintf("session: %s · turns: %d · verified: %s · model: %s",
			sessionID, turns, verifiedStr, model)
		return styles.Muted.Render(truncateRunes(plain, width))
	}

	modelShown := model
	if utf8.RuneCountInString(modelShown) > avail {
		modelShown = truncateRunes(modelShown, avail)
	}

	var b strings.Builder
	b.WriteString(styles.Muted.Render(prefix))
	b.WriteString(verifiedStyle.Render(verifiedStr))
	b.WriteString(styles.Muted.Render(modelLabel + modelShown))
	return b.String()
}

func (s *SessionInfo) SessionID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessionID
}

func (s *SessionInfo) Model() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.model
}

func (s *SessionInfo) Turns() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.turns
}

func (s *SessionInfo) Verified() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.verified
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 3 {
		return string(r[:max])
	}
	return string(r[:max-3]) + "..."
}

func shortSessionID(name string) string {
	h := fnv.New32a()
	h.Write([]byte(name))
	return fmt.Sprintf("%06x", h.Sum32())
}
