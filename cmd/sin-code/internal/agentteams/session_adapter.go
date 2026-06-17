// SPDX-License-Identifier: MIT
// Purpose: Cross-harness session normalization (issue #331). Adapts
// sessions from different agent harnesses (SIN-Code, Claude Code,
// opencode) into a single NormalizedSession format so the agent-team
// layer can work with sessions regardless of their origin.
//
// M7 invariant: SessionAdapter is stateless and safe for concurrent
// use.
package agentteams

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// NormalizedMessage is a single message in a NormalizedSession,
// abstracted across harness formats.
type NormalizedMessage struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp,omitempty"`
	ToolCalls []string  `json:"tool_calls,omitempty"`
}

// NormalizedSession is the harness-agnostic session representation.
type NormalizedSession struct {
	ID       string             `json:"id"`
	Harness  string             `json:"harness"`
	Title    string             `json:"title,omitempty"`
	Created  time.Time          `json:"created,omitempty"`
	Messages []NormalizedMessage `json:"messages"`
}

// SessionAdapter normalizes sessions from different harnesses into
// the NormalizedSession format. It is stateless and safe for
// concurrent use (M7).
type SessionAdapter struct{}

// NewSessionAdapter creates a new SessionAdapter.
func NewSessionAdapter() *SessionAdapter {
	return &SessionAdapter{}
}

// NormalizeSinCode converts a native SIN-Code session into a
// NormalizedSession.
func (a *SessionAdapter) NormalizeSinCode(sess *session.Session) (*NormalizedSession, error) {
	if sess == nil {
		return nil, fmt.Errorf("agentteams: nil session")
	}
	history := sess.History()
	msgs := make([]NormalizedMessage, 0, len(history))
	for _, m := range history {
		nm := NormalizedMessage{
			Role:    m.Role,
			Content: m.Content,
		}
		if m.ToolCalls != nil {
			var calls []string
			if err := json.Unmarshal(m.ToolCalls, &calls); err == nil {
				nm.ToolCalls = calls
			}
		}
		msgs = append(msgs, nm)
	}
	return &NormalizedSession{
		ID:       sess.ID,
		Harness:  "sin-code",
		Title:    "",
		Messages: msgs,
	}, nil
}

// NormalizeClaudeCode parses Claude Code session data in JSONL format
// (one JSON object per line) and returns a NormalizedSession.
// Claude Code sessions are arrays of JSON objects with "role" and
// "content" fields.
func (a *SessionAdapter) NormalizeClaudeCode(data []byte) (*NormalizedSession, error) {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	msgs := make([]NormalizedMessage, 0, len(lines))
	var sessionID string
	var created time.Time

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			return nil, fmt.Errorf("agentteams: claude parse line %d: %w", i+1, err)
		}
		if id, ok := raw["session_id"]; ok {
			var s string
			if json.Unmarshal(id, &s) == nil {
				sessionID = s
			}
		}
		if ts, ok := raw["timestamp"]; ok {
			var t time.Time
			if json.Unmarshal(ts, &t) == nil {
				if created.IsZero() || t.Before(created) {
					created = t
				}
			}
		}
		role := extractString(raw, "role")
		content := extractString(raw, "content")
		if role == "" && content == "" {
			continue
		}
		nm := NormalizedMessage{
			Role:      role,
			Content:   content,
			Timestamp: extractTime(raw, "timestamp"),
		}
		msgs = append(msgs, nm)
	}

	if sessionID == "" {
		sessionID = "claude-imported"
	}
	return &NormalizedSession{
		ID:       sessionID,
		Harness:  "claude-code",
		Created:  created,
		Messages: msgs,
	}, nil
}

// NormalizeOpenCode parses opencode session data (JSON format with
// "messages" array) and returns a NormalizedSession.
func (a *SessionAdapter) NormalizeOpenCode(data []byte) (*NormalizedSession, error) {
	var doc struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Harness  string `json:"harness"`
		Messages []struct {
			Role      string          `json:"role"`
			Content   string          `json:"content"`
			ToolCalls json.RawMessage `json:"tool_calls,omitempty"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("agentteams: opencode parse: %w", err)
	}
	msgs := make([]NormalizedMessage, 0, len(doc.Messages))
	for _, m := range doc.Messages {
		nm := NormalizedMessage{
			Role:    m.Role,
			Content: m.Content,
		}
		if m.ToolCalls != nil {
			var calls []string
			if json.Unmarshal(m.ToolCalls, &calls) == nil {
				nm.ToolCalls = calls
			}
		}
		msgs = append(msgs, nm)
	}
	harness := doc.Harness
	if harness == "" {
		harness = "opencode"
	}
	id := doc.ID
	if id == "" {
		id = "opencode-imported"
	}
	return &NormalizedSession{
		ID:       id,
		Harness:  harness,
		Title:    doc.Title,
		Messages: msgs,
	}, nil
}

// DetectHarness inspects raw session data and returns the likely
// harness name: "claude-code", "opencode", or "unknown".
func (a *SessionAdapter) DetectHarness(data []byte) string {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return "unknown"
	}

	// JSONL (multi-line JSON objects) → Claude Code
	if strings.Contains(trimmed, "\n") {
		firstLine := strings.TrimSpace(strings.SplitN(trimmed, "\n", 2)[0])
		if strings.HasPrefix(firstLine, "{") {
			var raw map[string]json.RawMessage
			if json.Unmarshal([]byte(firstLine), &raw) == nil {
				if _, ok := raw["session_id"]; ok {
					return "claude-code"
				}
				if _, ok := raw["role"]; ok {
					return "claude-code"
				}
			}
		}
	}

	// Single JSON object with "messages" array → opencode
	var doc map[string]json.RawMessage
	if json.Unmarshal(data, &doc) == nil {
		if _, ok := doc["messages"]; ok {
			return "opencode"
		}
	}

	return "unknown"
}

// --- helpers ------------------------------------------------------------

func extractString(raw map[string]json.RawMessage, key string) string {
	v, ok := raw[key]
	if !ok {
		return ""
	}
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s
	}
	// content might be a structured object; try to extract text
	var obj map[string]json.RawMessage
	if json.Unmarshal(v, &obj) == nil {
		if text, ok := obj["text"]; ok {
			json.Unmarshal(text, &s)
			return s
		}
	}
	return ""
}

func extractTime(raw map[string]json.RawMessage, key string) time.Time {
	v, ok := raw[key]
	if !ok {
		return time.Time{}
	}
	var t time.Time
	json.Unmarshal(v, &t)
	return t
}
