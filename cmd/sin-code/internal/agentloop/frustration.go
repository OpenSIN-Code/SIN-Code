// SPDX-License-Identifier: MIT
// Purpose: frustration detection and adaptive UX (issue #271). Detects user
// frustration from message content (keywords, repetition, caps lock, rapid
// retries) and produces a system-prompt suffix that shifts the agent's tone
// toward conciliatory, precise, and careful responses.
//
// The detector is race-safe: all state mutations go through a mutex (M7).
package agentloop

import (
	"strings"
	"sync"
	"time"
)

type FrustrationLevel int

const (
	FrustrationNone FrustrationLevel = iota
	FrustrationMild
	FrustrationHigh
)

func (l FrustrationLevel) String() string {
	switch l {
	case FrustrationNone:
		return "none"
	case FrustrationMild:
		return "mild"
	case FrustrationHigh:
		return "high"
	default:
		return "unknown"
	}
}

var (
	normalKeywords = []string{
		"why", "stop", "no", "wrong", "again", "doesn't work",
		"seriously", "not working", "broken", "still broken",
		"not helpful", "this is bad",
	}
	strongKeywords = []string{
		"wtf", "ugh", "damn", "stupid", "hate", "useless",
		"are you kidding", "what the hell",
	}
)

const (
	frustrationWindow     = 60 * time.Second
	rapidRetryWindow      = 30 * time.Second
	rapidRetryThreshold   = 3
	repetitionThreshold   = 3
	capsLockMinAlpha      = 5
	capsLockRatio         = 0.7
	mildThreshold  int    = 1
	highThreshold  int    = 3
)

type trackedMessage struct {
	normalized string
	timestamp  time.Time
}

type FrustrationDetector struct {
	mu       sync.Mutex
	messages []trackedMessage
	counts   map[string]int
	level    FrustrationLevel
	score    int
}

func NewFrustrationDetector() *FrustrationDetector {
	return &FrustrationDetector{
		counts: make(map[string]int),
	}
}

func (d *FrustrationDetector) Track(message string, timestamp time.Time) FrustrationLevel {
	if d == nil {
		return FrustrationNone
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	norm := normalizeMessage(message)
	d.messages = append(d.messages, trackedMessage{
		normalized: norm,
		timestamp:  timestamp,
	})
	d.counts[norm]++

	cutoff := timestamp.Add(-frustrationWindow)
	d.expireLocked(cutoff)

	d.score = d.computeScoreLocked(message, norm, timestamp)
	d.level = scoreToLevel(d.score)
	return d.level
}

func (d *FrustrationDetector) Level() FrustrationLevel {
	if d == nil {
		return FrustrationNone
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.level
}

func (d *FrustrationDetector) Reset() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.messages = nil
	d.counts = make(map[string]int)
	d.level = FrustrationNone
	d.score = 0
}

func (d *FrustrationDetector) IsFrustrated() bool {
	return d.Level() >= FrustrationMild
}

func (d *FrustrationDetector) SystemPromptSuffix() string {
	switch d.Level() {
	case FrustrationMild:
		return "\n\nLet me be more precise and direct in my response."
	case FrustrationHigh:
		return "\n\nThe user seems frustrated. Be extra careful, explain clearly, and double-check your work."
	default:
		return ""
	}
}

func (d *FrustrationDetector) Metadata() map[string]any {
	level := d.Level()
	return map[string]any{
		"frustration_level": level.String(),
		"frustrated":        level >= FrustrationMild,
	}
}

func (d *FrustrationDetector) Score() int {
	if d == nil {
		return 0
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.score
}

func (d *FrustrationDetector) computeScoreLocked(message, norm string, now time.Time) int {
	score := 0

	lower := strings.ToLower(message)
	for _, kw := range normalKeywords {
		if strings.Contains(lower, kw) {
			score++
			break
		}
	}
	for _, kw := range strongKeywords {
		if strings.Contains(lower, kw) {
			score += 2
			break
		}
	}

	if d.counts[norm] >= repetitionThreshold {
		score += 2
	}

	if isCapsLock(message) {
		score++
	}

	if d.countRecentLocked(now, rapidRetryWindow) >= rapidRetryThreshold {
		score++
	}

	return score
}

func (d *FrustrationDetector) countRecentLocked(now time.Time, window time.Duration) int {
	cutoff := now.Add(-window)
	count := 0
	for _, m := range d.messages {
		if !m.timestamp.Before(cutoff) {
			count++
		}
	}
	return count
}

func (d *FrustrationDetector) expireLocked(cutoff time.Time) {
	kept := d.messages[:0]
	for _, m := range d.messages {
		if m.timestamp.Before(cutoff) {
			d.counts[m.normalized]--
			if d.counts[m.normalized] <= 0 {
				delete(d.counts, m.normalized)
			}
		} else {
			kept = append(kept, m)
		}
	}
	d.messages = kept
}

func scoreToLevel(score int) FrustrationLevel {
	if score >= highThreshold {
		return FrustrationHigh
	}
	if score >= mildThreshold {
		return FrustrationMild
	}
	return FrustrationNone
}

func normalizeMessage(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return s
}

func isCapsLock(s string) bool {
	letters := 0
	upper := 0
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			letters++
		} else if r >= 'A' && r <= 'Z' {
			letters++
			upper++
		}
	}
	if letters < capsLockMinAlpha {
		return false
	}
	return float64(upper)/float64(letters) >= capsLockRatio
}
