// SPDX-License-Identifier: MIT
// Purpose: rule set + deterministic byte-stable renderer. The output
// of Render is the system-prompt block an activated rule produces;
// downstream wiring prepends it ahead of the model's existing context.
// Docs: rules.doc.md
package autoactivate

import (
	"sort"
	"strings"
)

// Rule is a single named rule block. The body is the literal text that
// will be prepended to the system prompt when the rule is active.
type Rule struct {
	Name      string // e.g. "terse", "skill-code-create"
	Body      string // free-form text, mixed Markdown OK
	Trigger   string // optional natural-language phrase that auto-activates
	NoTrigger bool   // when true, natural-language triggers are ignored
}

// RuleSet is a deduplicated, sorted collection of rules keyed by name.
// A nil RuleSet behaves like an empty one (Render returns "").
type RuleSet map[string]Rule

// Add inserts r. If a rule with the same name already exists, the
// caller-supplied r overwrites it.
func (s RuleSet) Add(r Rule) {
	if s == nil {
		return
	}
	if r.Name == "" {
		return
	}
	s[r.Name] = r
}

// Remove drops r.Name. Missing names are silent no-ops.
func (s RuleSet) Remove(name string) {
	if s == nil {
		return
	}
	delete(s, name)
}

// Has reports whether the named rule is present.
func (s RuleSet) Has(name string) bool {
	if s == nil {
		return false
	}
	_, ok := s[name]
	return ok
}

// Len returns the number of rules.
func (s RuleSet) Len() int { return len(s) }

// Names returns the rule names in sorted, lexicographic order. The
// order is part of the public contract — Render iterates Names()
// exactly, so any caller can rely on a stable byte sequence.
func (s RuleSet) Names() []string {
	if s == nil {
		return nil
	}
	out := make([]string, 0, len(s))
	for n := range s {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Clone returns a defensive copy so callers may mutate without
// aliasing the activator's session state (mandate M7 isolation).
func (s RuleSet) Clone() RuleSet {
	if s == nil {
		return nil
	}
	out := make(RuleSet, len(s))
	for k, v := range s {
		out[k] = v
	}
	return out
}

// Render returns a deterministic byte sequence for the rule set. The
// output is identical for any same-content RuleSet regardless of
// insertion order — the input map is iterated in sorted-by-name order
// and each rule's body is trimmed of trailing whitespace so the
// concatenation boundary is stable.
//
// The header is "## Active rules\n" so the block is greppable in
// transcripts. The output is safe to prepend to a system prompt
// verbatim; downstream tools (style, hook runner) must not mutate it.
func (s RuleSet) Render() string {
	if len(s) == 0 {
		return ""
	}
	names := s.Names()
	var b strings.Builder
	b.WriteString("## Active rules\n")
	for i, n := range names {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("### ")
		b.WriteString(n)
		b.WriteByte('\n')
		body := strings.TrimRight(s[n].Body, " \t\n")
		if body != "" {
			b.WriteString(body)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// Equal reports whether two RuleSets have the same name+body+trigger
// tuples. Used in tests to confirm that Activate/Deactivate does not
// accidentally re-order the iteration order.
func (s RuleSet) Equal(other RuleSet) bool {
	if len(s) != len(other) {
		return false
	}
	for n, r := range s {
		o, ok := other[n]
		if !ok {
			return false
		}
		if r.Name != o.Name || r.Body != o.Body || r.Trigger != o.Trigger || r.NoTrigger != o.NoTrigger {
			return false
		}
	}
	return true
}
