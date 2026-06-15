// SPDX-License-Identifier: MIT
// Purpose: per-kind schema validation, including unsafe-unicode and
// duplicate detection. Port of ECC's CI validators.
// Docs: validate.doc.md
package assets

import (
	"fmt"
	"strings"
)

// Issue is a single validation finding.
type Issue struct {
	Path    string
	Level   string // "error" | "warn"
	Message string
}

// Validate enforces the per-kind schema, porting ECC's CI validators
// (validate-agents.js / validate-skills.js) into Go.
func Validate(a *Asset) []Issue {
	var issues []Issue
	add := func(level, msg string) { issues = append(issues, Issue{a.Path, level, msg}) }

	if strings.TrimSpace(a.Name) == "" {
		add("error", "missing required field: name")
	}
	if strings.TrimSpace(a.Description) == "" {
		add("error", "missing required field: description")
	}
	if len(a.Body) < 20 {
		add("warn", "body is suspiciously short (<20 chars)")
	}

	switch a.Kind {
	case KindAgent:
		if a.Model == "" {
			add("warn", "agent has no model hint")
		}
		if len(a.Tools) == 0 {
			add("warn", "agent declares no tools")
		}
	case KindCommand:
		// commands frequently use $ARGUMENTS / $1 placeholders
		if !strings.Contains(a.Body, "$") && a.Argument != "" {
			add("warn", "argument-hint set but body references no $ placeholder")
		}
	case KindSkill:
		if !strings.Contains(strings.ToLower(a.Body), "## ") {
			add("warn", "skill body has no markdown sections")
		}
	}

	// Unicode safety: reject bidi/zero-width.
	if hasUnsafeUnicode(a.Body) || hasUnsafeUnicode(a.Description) {
		add("error", "contains unsafe unicode (bidi/zero-width) control characters")
	}
	return issues
}

// ValidateAll runs validation across a set and returns a flat issue
// list. Detects duplicates by (kind, name).
func ValidateAll(list []*Asset) []Issue {
	var all []Issue
	names := map[string]string{}
	for _, a := range list {
		all = append(all, Validate(a)...)
		key := string(a.Kind) + "/" + a.Name
		if prev, ok := names[key]; ok {
			all = append(all, Issue{a.Path, "error", fmt.Sprintf("duplicate %s (also %s)", key, prev)})
		}
		names[key] = a.Path
	}
	return all
}

var unsafeRunes = []rune{
	'\u202A', '\u202B', '\u202C', '\u202D', '\u202E', // bidi overrides
	'\u200B', '\u200C', '\u200D', '\uFEFF', // zero-width / BOM
	'\u2066', '\u2067', '\u2068', '\u2069', // isolates
}

func hasUnsafeUnicode(s string) bool {
	for _, r := range s {
		for _, u := range unsafeRunes {
			if r == u {
				return true
			}
		}
	}
	return false
}

// contains is a small substring helper used by tests.
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
