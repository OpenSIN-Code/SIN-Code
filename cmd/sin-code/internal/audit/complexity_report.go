// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when audit is refactored
// Purpose: result aggregation, sin-debt approval, formatting, and tag validation.
package audit

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

func approvedBySinDebt(f Finding, debtRE *regexp.Regexp) (bool, string) {
	// #nosec G304 — f.Path is a file from the user-supplied audit root.
	data, err := os.ReadFile(f.Path)
	if err != nil {
		return false, ""
	}
	lines := strings.Split(string(data), "\n")
	if f.Line < 1 || f.Line > len(lines) {
		return false, ""
	}
	start := f.Line - 10
	if start < 0 {
		start = 0
	}
	for i := start; i < f.Line && i < len(lines); i++ {
		m := debtRE.FindStringSubmatch(lines[i])
		if m != nil {
			return true, strings.TrimSpace(m[1])
		}
	}
	if f.Line == 1 {
		for i := 0; i < len(lines); i++ {
			m := debtRE.FindStringSubmatch(lines[i])
			if m != nil {
				return true, strings.TrimSpace(m[1])
			}
		}
	}
	return false, ""
}

func aggregate(findings []Finding, maxNet int) *Result {
	r := &Result{Findings: findings}
	for _, f := range findings {
		if f.Approved {
			continue
		}
		r.NetLines += f.LineCount
		if f.Tag == TagStdlib || f.Tag == TagNative {
			r.DepsRemovable++
		}
	}
	if len(findings) == 0 {
		r.Status = "Lean already. Ship."
		r.NetLines = 0
		r.DepsRemovable = 0
		return r
	}
	if maxNet > 0 && r.NetLines > maxNet {
		r.Status = fmt.Sprintf("net: -%d lines, -%d deps possible. (exceeds threshold %d)", r.NetLines, r.DepsRemovable, maxNet)
		return r
	}
	r.Status = fmt.Sprintf("net: -%d lines, -%d deps possible.", r.NetLines, r.DepsRemovable)
	return r
}

// FormatFinding returns the one-line ponytail format.
func FormatFinding(f Finding) string {
	approved := ""
	if f.Approved {
		approved = fmt.Sprintf(" (approved: sin-debt marker %s)", f.Approver)
	}
	return fmt.Sprintf("%s: %s. %s. [%s:%d]%s", f.Tag, f.Problem, f.Replacement, f.Path, f.Line, approved)
}

// FormatResult returns the full text report.
func FormatResult(r *Result, format string) string {
	if format == "json" {
		return ""
	}
	var sb strings.Builder
	for _, f := range r.Findings {
		sb.WriteString(FormatFinding(f))
		sb.WriteString("\n")
	}
	sb.WriteString(r.Status)
	sb.WriteString("\n")
	return sb.String()
}

// ValidateTags returns an error if any tag is unknown.
func ValidateTags(tags []string) error {
	known := map[string]bool{TagDelete: true, TagStdlib: true, TagNative: true, TagYagni: true, TagShrink: true}
	for _, t := range tags {
		if !known[strings.ToLower(strings.TrimSpace(t))] {
			return fmt.Errorf("unknown tag: %s", t)
		}
	}
	return nil
}
