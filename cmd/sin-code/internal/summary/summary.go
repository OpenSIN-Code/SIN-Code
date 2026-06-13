// SPDX-License-Identifier: MIT
// Purpose: Rule-based summary builder over the semantic session ledger.
// Converts a stream of ledger entries into a human-readable, evidence-backed
// session summary. No LLM call is required; heuristics guarantee
// determinism and keep M2 (no external deps) intact.
// Docs: summary.doc.md
package summary

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
)

// Summary is a condensed view of a session.
type Summary struct {
	SessionID    string
	Turns        int
	Verified     bool
	Verification string
	ToolsUsed    []string
	UserPrompts  []string
	OneLiner     string
	CreatedAt    time.Time
}

// Build reads ledger entries for a session and produces a Summary.
func Build(ctx context.Context, store *ledger.Store, sessionID string) (*Summary, error) {
	entries, err := store.List(ctx, sessionID, 10000)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no ledger entries for session %s", sessionID)
	}
	return buildFromEntries(entries)
}

func buildFromEntries(entries []ledger.Entry) (*Summary, error) {
	s := &Summary{
		SessionID: entries[0].SessionID,
		CreatedAt: entries[0].CreatedAt,
	}
	toolSet := make(map[string]bool)
	for _, e := range entries {
		if e.CreatedAt.Before(s.CreatedAt) {
			s.CreatedAt = e.CreatedAt
		}
		switch e.Type {
		case ledger.TypeUserPrompt:
			if c, ok := e.Data["content"].(string); ok && c != "" {
				s.UserPrompts = append(s.UserPrompts, c)
			}
		case ledger.TypeToolCall:
			s.Turns++
			if name, ok := e.Data["tool"].(string); ok {
				toolSet[name] = true
			}
		case ledger.TypeVerifyPass:
			s.Verified = true
			if mode, ok := e.Data["mode"].(string); ok {
				s.Verification = mode
			}
		case ledger.TypeVerifyFail:
			if mode, ok := e.Data["mode"].(string); ok {
				s.Verification = mode + " (failed)"
			}
		case ledger.TypeTaskComplete:
			if text, ok := e.Data["summary"].(string); ok && text != "" {
				s.OneLiner = text
			}
		}
	}
	for name := range toolSet {
		s.ToolsUsed = append(s.ToolsUsed, name)
	}
	if len(s.UserPrompts) > 0 {
		first := s.UserPrompts[0]
		if len(first) > 80 {
			first = first[:80] + "…"
		}
		if s.OneLiner == "" {
			s.OneLiner = first
		}
	}
	if s.Verification == "" {
		if s.Verified {
			s.Verification = "unknown mode"
		} else {
			s.Verification = "not verified"
		}
	}
	return s, nil
}

// Format renders a Summary as markdown.
func Format(s *Summary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Session Summary: %s\n\n", s.SessionID)
	fmt.Fprintf(&b, "- **Created:** %s\n", s.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "- **Turns:** %d\n", s.Turns)
	fmt.Fprintf(&b, "- **Verified:** %v (%s)\n", s.Verified, s.Verification)
	fmt.Fprintf(&b, "- **One-liner:** %s\n\n", s.OneLiner)
	if len(s.ToolsUsed) > 0 {
		fmt.Fprintf(&b, "## Tools Used\n")
		for _, t := range s.ToolsUsed {
			fmt.Fprintf(&b, "- %s\n", t)
		}
		b.WriteString("\n")
	}
	if len(s.UserPrompts) > 0 {
		fmt.Fprintf(&b, "## Prompts\n")
		for _, p := range s.UserPrompts {
			fmt.Fprintf(&b, "- %s\n", p)
		}
	}
	return b.String()
}

// Evidence returns a short evidence string for Oracle-style verification.
func Evidence(s *Summary) string {
	status := "UNVERIFIED"
	if s.Verified {
		status = "VERIFIED"
	}
	return fmt.Sprintf("%s | %s | %d tool-call turns | %s", status, s.Verification, s.Turns, s.OneLiner)
}
