// SPDX-License-Identifier: MIT
// The five canonical ponytail rules for the MCP-tool-description
// compressor. See compressor.go for the byte-stability contract and
// tagging scheme.
//
// Rule order in this file matches All() in compressor.go — do NOT
// reorder. Pipeline composition is order-sensitive (the post-pipeline
// Normalise pass hides whitespace differences but cannot rescue a
// Rule that depends on tokens deleted by an earlier Rule).
package mcpcompress

// ----------------------------------------------------------------------------
// Rule 1 — DeleteHedges (tag: delete)
//
// Drops standalone pleasantries / hedge adverbs that pad verb phrases
// without adding information to tool selection.
//
// Examples (input → output):
//   "Execute shell commands safely with secret redaction, timeout, and error analysis"
//     → "Execute shell commands with secret redaction, timeout, and error analysis"
//
//   "Carefully validates generated bundles"
//     → "validates generated bundles"
//
//   "Robustly indexes the codebase"  (verb-bearing adverb)
//     → "indexes the codebase"
//
// Standalone-token semantics: "safety" or "carefully-named" never
// match (compound words are not hedges). The regex is anchored on
// \b … \b. Whitespace cleanup runs in normalize().
//
// Byte-stable: compiles once, runs in O(n) over the description.
// ----------------------------------------------------------------------------

type RuleDeleteHedges struct{}

// Name returns the stable Rule identifier. Stable surface — do NOT rename.
func (RuleDeleteHedges) Name() string { return "DeleteHedges" }

// Tag returns the ponytail tag for this Rule.
func (RuleDeleteHedges) Tag() Tag { return TagDelete }

// Apply drops hedge adverbs. Idempotent.
func (RuleDeleteHedges) Apply(s string) string {
	s = hedgeAdverbPattern.ReplaceAllString(s, "")
	return s
}

// hedgeAdverbPattern matches standalone -ly hedge adverbs. Case-insensitive
// because descriptions capitalise the first word ("Carefully handles …",
// "Safely executes …"). The word-boundary anchors protect compound
// words like "safety-scanner" or "carefully-named".
var hedgeAdverbPattern = MustCompile(
	`(?i)\b(safely|carefully|thoroughly|reliably|robustly|elegantly|gracefully|seamlessly|effortlessly|smoothly|meticulously)\b`,
)

// ----------------------------------------------------------------------------
// Rule 2 — StdlibPatterns (tag: stdlib)
//
// Drops redundant references to platform / stdlib libraries when
// those references are parenthetical decorations rather than load-
// bearing facts.
//
// Examples (input → output):
//   "(via stdlib)"                       → ""
//   "Go stdlib-based parser"             → "Go-based parser"
//   "Python stdlib helper"               → "Python helper"
//
// "stdlib" as a noun in a tool's primary value proposition (the
// rare case) is NOT touched — this Rule only targets parenthetical
// debris. See coveredRegex for the exact surface.
//
// Byte-stable.
// ----------------------------------------------------------------------------

type RuleStdlibPatterns struct{}

// Name returns the stable Rule identifier.
func (RuleStdlibPatterns) Name() string { return "StdlibPatterns" }

// Tag returns the ponytail tag for this Rule.
func (RuleStdlibPatterns) Tag() Tag { return TagStdlib }

// Apply removes redundant stdlib parenthesisations. Idempotent.
func (RuleStdlibPatterns) Apply(s string) string {
	s = stdlibParenthetical.ReplaceAllString(s, "")
	s = stdlibLanguageAdjective.ReplaceAllString(s, "$1")
	return s
}

var stdlibParenthetical = MustCompile(`\(\s*via\s+stdlib\s*\)`)
var stdlibLanguageAdjective = MustCompile(`(Go|Python|Rust|Java|JavaScript|TypeScript)\s+stdlib\b`)

// ----------------------------------------------------------------------------
// Rule 3 — DropTrimEncouragement (tag: native)
//
// Drops M6-style "Always prefer over native X" tail clauses. SIN-Code's
// M6 mandate (AGENTS.md §3) is internal to the agent loop, not the
// model-facing manifest; surfacing it as advice to the model wastes
// bytes and biases tool choice.
//
// Examples (input → output):
//   "Surgical file edit, three addressing modes. Always prefer over native read."
//     → "Surgical file edit, three addressing modes."
//
//   "X foo. Prefer sin_X over native force."
//     → "X foo."
//
//   "Read files safely. Use sin_read when possible."  (leading-encouragement)
//     → "Read files safely."
//
// Byte-stable: anchored patterns, no look-ahead.
// ----------------------------------------------------------------------------

type RuleDropTrimEncouragement struct{}

// Name returns the stable Rule identifier.
func (RuleDropTrimEncouragement) Name() string { return "DropTrimEncouragement" }

// Tag returns the ponytail tag for this Rule.
func (RuleDropTrimEncouragement) Tag() Tag { return TagNative }

// Apply removes encourage-tail clauses. Idempotent.
func (RuleDropTrimEncouragement) Apply(s string) string {
	s = preferOverNativeTail.ReplaceAllString(s, "")
	s = preferSinOverNativeTail.ReplaceAllString(s, "")
	s = leadingUseSinEncouragement.ReplaceAllString(s, "")
	return s
}

// preferOverNativeTail matches ". Always prefer over native X" where X
// is 1–20 lowercase letters OR an existing sin_* identifier.
var preferOverNativeTail = MustCompile(
	`\.?\s*Always prefer over native\s+(?:[a-z]{1,20}|sin_[a-z_]+)\.?\s*$`,
)

// preferSinOverNativeTail matches ". Prefer sin_X over native Y" forms.
var preferSinOverNativeTail = MustCompile(
	`\.?\s*Prefer\s+sin_[a-z_]+\s+over\s+native\s+[a-z]{1,20}\.?\s*$`,
)

// leadingUseSinEncouragement matches a leading "Use sin_X …" instruction
// when it is followed by a period and at least one extra clause in the
// description. We do not consume the contribution of the sentence, just
// the encouragement preamble.
var leadingUseSinEncouragement = MustCompile(
	`^\s*Use\s+sin_[a-z_]+\s+when\s+possible[.,]\s+`,
)

// ----------------------------------------------------------------------------
// Rule 4 — YagniPatterns (tag: yagni)
//
// Drops speculative / reserved / placeholder mentions. Bloat that
// promises future capability without delivering it.
//
// Examples (input → output):
//   "Memory DB statistics (total, links, embeddings), embedder status"
//     → unchanged
//
//   "List notifications (may be deprecated in the future)"
//     → "List notifications"
//
//   "Reserved for the orchestrator rework (TBD)"
//     → "Reserved for the orchestrator rework"
//
//   "Manage todos atomically (experimental)"
//     → "Manage todos atomically"
//
// Byte-stable: anchored paren clusters and word-boundary matches.
// ----------------------------------------------------------------------------

type RuleYagniPatterns struct{}

// Name returns the stable Rule identifier.
func (RuleYagniPatterns) Name() string { return "YagniPatterns" }

// Tag returns the ponytail tag for this Rule.
func (RuleYagniPatterns) Tag() Tag { return TagYagni }

// Apply strips yagni debris.
func (RuleYagniPatterns) Apply(s string) string {
	s = yagniParenthetical.ReplaceAllString(s, "")
	s = yagniPhrase.ReplaceAllString(s, "")
	return s
}

var yagniParenthetical = MustCompile(
	`\(\s*(?:experimental|TBD|TBA|reserved|may be deprecated in the future|may be deprecated|potentially)\s*\)`,
)

var yagniPhrase = MustCompile(
	`\b(TBD|TBA)\b`,
)

// ----------------------------------------------------------------------------
// Rule 5 — ShrinkExamples (tag: shrink)
//
// Drops parenthetical examples "(e.g. **/*.py)", "(for CLI subcommands)",
// "(such as …)" that the schema (required/typed fields, enum constraints)
// already documents. Tool descriptions are short on purpose; a parenthetical
// "e.g." is usually redundant.
//
// Examples (input → output):
//   "Add a todo (v2 bbolt store, hash ID, supports priority/type/tags/project/assignee)"
//     → "Add a todo"
//
//   "Discover files (e.g. **/*.py) with relevance scoring"
//     → "Discover files with relevance scoring"
//
//   "Search code (such as regex patterns) across the repo"
//     → "Search code across the repo"
//
// We do NOT touch em-dash preambles (— <verb phrase>) — they are
// legitimate clarifications, not redundant examples. Examples:
//
//   "Ephemeral Full-Stack Mocking — spin up disposable test environments"
//     → unchanged
//
// Byte-stable.
// ----------------------------------------------------------------------------

type RuleShrinkExamples struct{}

// Name returns the stable Rule identifier.
func (RuleShrinkExamples) Name() string { return "ShrinkExamples" }

// Tag returns the ponytail tag for this Rule.
func (RuleShrinkExamples) Tag() Tag { return TagShrink }

// Apply strips redundant examples.
func (RuleShrinkExamples) Apply(s string) string {
	s = egsParenthetical.ReplaceAllString(s, "")
	return s
}

// egsParenthetical drops "(e.g. …)" / "(such as …)" parentheticals entirely
// when there is non-empty content. Single-line tool descriptions are
// short enough that any parenthetical example is redundant with the
// schema's typed fields and enum constraints.
var egsParenthetical = MustCompile(`\(\s*(?:e\.g\.|such as)\s+[^)]+\)`)
