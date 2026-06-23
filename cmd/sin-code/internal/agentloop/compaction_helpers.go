// SPDX-License-Identifier: MIT
package agentloop
import ("fmt"; "strings")
func toLowerTrim(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
func errUnknownMode(s string) error { return fmt.Errorf("agentloop: unknown compaction mode %q", s) }
func errUnknownTrigger(s string) error { return fmt.Errorf("agentloop: unknown compaction trigger %q", s) }

// containsEvidence is the deterministic marker detector the
// mode-based compaction path uses to decide which messages MUST stay
// (mandate M3 — verification evidence is sacred across compaction).
// Returns true when content carries any of the canonical evidence
// markers: "VERIFICATION PASSED", "VERIFICATION FAILED",
// "NOT DONE", or "Open acceptance criteria".
func containsEvidence(content string) bool {
	if content == "" {
		return false
	}
	switch {
	case strings.Contains(content, "VERIFICATION PASSED"):
		return true
	case strings.Contains(content, "VERIFICATION FAILED"):
		return true
	case strings.Contains(content, "NOT DONE"):
		return true
	case strings.Contains(content, "Open acceptance criteria"):
		return true
	}
	return false
}
