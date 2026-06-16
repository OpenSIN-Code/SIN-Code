// SPDX-License-Identifier: MIT
// Purpose: Rule-based summary builder over the semantic session ledger,
// augmented with token-usage aggregations from internal/usage (issue
// #168). Converts a stream of ledger entries into a human-readable,
// evidence-backed session summary. No LLM call is required; heuristics
// guarantee determinism and keep M2 (no external deps) intact.
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
	TokensUsed   int     // sum of total_tokens across recorded LLM calls
	InputTokens  int     // sum of prompt tokens
	OutputTokens int     // sum of completion tokens
	CostUSD      float64 // aggregated from per-1k pricing
	TokenCount   int     // number of distinct LLM calls (sessions)
	HasUsage     bool    // true once any LLM usage was recorded
}

// Build reads ledger entries for a session and produces a Summary.
// TokenUsage is optional — pass nil if the usage store is unavailable.
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

// TokenSource produces an aggregate from any token-usage store
// (typically internal/usage.Aggregate). Implementations report one row
// per call — used by BuildWithTokens to fill the TokensUsed fields.
type TokenSource interface {
	SessionTokens(ctx context.Context, sessionID string) (input, output, total int, events int, cost float64, err error)
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

// BuildWithTokens is Build + token aggregation. TokenSrc may be nil;
// a nil source gracefully leaves token fields at zero (no error).
func BuildWithTokens(ctx context.Context, store *ledger.Store, sessionID string, src TokenSource) (*Summary, error) {
	s, err := Build(ctx, store, sessionID)
	if err != nil {
		return nil, err
	}
	if src == nil {
		return s, nil
	}
	in, out, total, n, cost, err := src.SessionTokens(ctx, sessionID)
	if err != nil {
		return s, nil // best-effort: token lookup failure does not break summary
	}
	s.InputTokens = in
	s.OutputTokens = out
	s.TokensUsed = total
	s.TokenCount = n
	s.CostUSD = cost
	s.HasUsage = total > 0
	return s, nil
}

// Format renders a Summary as markdown. Appends a one-line token counter
// (issue #168: matches the /caveman-stats one-liner gambit) so every
// summary surfaces the burn.
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
	// Token one-liner. Only render if usage was recorded — never show a
	// fake number (caveman lesson: absent until first /caveman-stats run).
	if s.HasUsage {
		fmt.Fprintf(&b, "\nTokens: %s (in %s / out %s)\n",
			humanInt(s.TokensUsed), humanInt(s.InputTokens), humanInt(s.OutputTokens))
		if s.CostUSD > 0 {
			fmt.Fprintf(&b, "Estimated cost: $%.4f\n", s.CostUSD)
		}
	}
	return b.String()
}

// OneLineToken returns the compact badge used by the TUI statusline and
// `sin-code tokens --share`. Returns empty when no usage is recorded.
func OneLineToken(s *Summary) string {
	if !s.HasUsage {
		return ""
	}
	if s.CostUSD > 0 {
		return fmt.Sprintf("⛏ %s · $%.2f", humanInt(s.TokensUsed), s.CostUSD)
	}
	return fmt.Sprintf("⛏ %s", humanInt(s.TokensUsed))
}

// Evidence returns a short evidence string for Oracle-style verification.
// Appends the token one-liner if available.
func Evidence(s *Summary) string {
	status := "UNVERIFIED"
	if s.Verified {
		status = "VERIFIED"
	}
	line := fmt.Sprintf("%s | %s | %d tool-call turns | %s", status, s.Verification, s.Turns, s.OneLiner)
	if s.HasUsage {
		line += fmt.Sprintf(" | tokens=%s cost=$%.4f", humanInt(s.TokensUsed), s.CostUSD)
	}
	return line
}

// humanInt renders a number with k/M suffix for the badge / share line.
// 1234 → "1.2k", 2_500_000 → "2.5M". Compact so it fits a statusline.
func humanInt(n int) string {
	if n == 0 {
		return "0"
	}
	sign := ""
	abs := n
	if abs < 0 {
		sign = "-"
		abs = -abs
	}
	switch {
	case abs >= 1_000_000:
		return fmt.Sprintf("%s%.1fM", sign, float64(abs)/1_000_000.0)
	case abs >= 1_000:
		return fmt.Sprintf("%s%.1fk", sign, float64(abs)/1_000.0)
	default:
		return fmt.Sprintf("%s%d", sign, abs)
	}
}
