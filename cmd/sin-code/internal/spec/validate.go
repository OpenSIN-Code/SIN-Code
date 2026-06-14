// SPDX-License-Identifier: MIT
// Purpose: structural validation of a parsed Spec (issue #122). This checks
// the spec is well-formed enough to drive an autonomous change — it does NOT
// run the acceptance commands (that is the agent/CI's job).
package spec

import (
	"fmt"
	"strings"
)

// Issue is a single validation finding.
type Issue struct {
	Severity string // "error" | "warning"
	Field    string // where the problem is
	Message  string
}

func (i Issue) String() string {
	return fmt.Sprintf("[%s] %s: %s", i.Severity, i.Field, i.Message)
}

// Result is the outcome of Validate.
type Result struct {
	Issues []Issue
}

// OK reports whether the spec has no error-severity issues (warnings allowed).
func (r Result) OK() bool {
	for _, i := range r.Issues {
		if i.Severity == "error" {
			return false
		}
	}
	return true
}

// Errors returns only the error-severity issues.
func (r Result) Errors() []Issue {
	var out []Issue
	for _, i := range r.Issues {
		if i.Severity == "error" {
			out = append(out, i)
		}
	}
	return out
}

// Validate checks a Spec for structural completeness.
//
// Rules:
//   - error:   missing Objective
//   - error:   no Requirements at all
//   - error:   no Acceptance Criteria at all (a spec must be checkable)
//   - error:   duplicate requirement / criterion ids
//   - warning: a "must" requirement with no acceptance criterion referencing
//     a verify command (best-effort: warns when NO criterion has a verify cmd)
//   - warning: empty requirement / criterion text
func Validate(s *Spec) Result {
	var res Result
	add := func(sev, field, msg string) {
		res.Issues = append(res.Issues, Issue{Severity: sev, Field: field, Message: msg})
	}

	if strings.TrimSpace(s.Objective) == "" {
		add("error", "objective", "spec has no # Objective section")
	}
	if len(s.Requirements) == 0 {
		add("error", "requirements", "spec has no # Requirements")
	}
	if len(s.Criteria) == 0 {
		add("error", "acceptance", "spec has no # Acceptance Criteria (it must be checkable)")
	}

	seen := map[string]string{}
	for _, r := range s.Requirements {
		if prev, dup := seen[r.ID]; dup {
			add("error", "requirements", fmt.Sprintf("duplicate id %q (already used by %q)", r.ID, prev))
		}
		seen[r.ID] = r.Text
		if strings.TrimSpace(r.Text) == "" {
			add("warning", "requirements", fmt.Sprintf("%s has empty text", r.ID))
		}
	}

	seenA := map[string]bool{}
	anyVerify := false
	for _, c := range s.Criteria {
		if seenA[c.ID] {
			add("error", "acceptance", fmt.Sprintf("duplicate id %q", c.ID))
		}
		seenA[c.ID] = true
		if strings.TrimSpace(c.Text) == "" {
			add("warning", "acceptance", fmt.Sprintf("%s has empty text", c.ID))
		}
		if strings.TrimSpace(c.Verify) != "" {
			anyVerify = true
		}
	}
	if len(s.Criteria) > 0 && !anyVerify {
		add("warning", "acceptance", "no criterion has a `verify:` command; spec cannot be auto-checked")
	}

	return res
}
