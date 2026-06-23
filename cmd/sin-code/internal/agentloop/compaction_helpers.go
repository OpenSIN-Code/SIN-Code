// SPDX-License-Identifier: MIT
package agentloop
import (
	"fmt"
	"strings"
)
func toLowerTrim(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
func errUnknownMode(s string) error { return fmt.Errorf("agentloop: unknown compaction mode %q", s) }
func errUnknownTrigger(s string) error { return fmt.Errorf("agentloop: unknown compaction trigger %q", s) }

// containsEvidence reports whether s carries any of the canonical
// verification-evidence markers the agent loop preserves against
// compaction (mandate M3). Used by identifyEvidence.
func containsEvidence(s string) bool {
	if s == "" {
		return false
	}
	for _, marker := range []string{
		"VERIFICATION PASSED",
		"VERIFICATION FAILED",
		"NOT DONE",
		"Open acceptance criteria",
	} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

// ShouldCompactTokens is the tokens-trigger half of ShouldCompact —
// the original CompactionTriggerTurns path is msg-count based, this
// is the tokens-based sibling. tokens is the estimated token usage
// of the message list, ctxWin is the effective context window,
// threshold is the fraction (0..1) at which compaction fires. Issue
// #278 first PR — kept here so the agent loop compiles while the
// full compaction rewrite matures on the context-compaction-modes
// branch.
func ShouldCompactTokens(tokens, ctxWin int, threshold float64) bool {
	if threshold <= 0 || ctxWin <= 0 {
		return false
	}
	if tokens <= 0 {
		return false
	}
	return float64(tokens) >= threshold*float64(ctxWin)
}
