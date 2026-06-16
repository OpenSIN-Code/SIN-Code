// SPDX-License-Identifier: MIT
// Purpose: render the verbosity / compression ruleset that is appended to
// the agent's system prompt. Inspired by JuliusBrussee/caveman (https://
// github.com/JuliusBrussee/caveman/blob/main/skills/caveman/SKILL.md) —
// prompt-level terseness that preserves every byte of technical
// substance. The package is dependency-free (mandate M2), owns no
// mutable state (mandate M7 safe under `go test -race`), and emits
// byte-stable output per (mode, skillBody) pair so the system-prompt
// hash metric (issue #2) can lock it down with a golden test.
// Docs: style.doc.md
package style

import "strings"

// Mode is the agent output style. Empty rulesets indicate "no
// instruction" — the agent should fall back to its native voice.
type Mode string

// Canonical mode values. ParseMode accepts any string but normalizes
// to one of these before lookup.
const (
	ModeDefault Mode = "default"
	ModeVerbose Mode = "verbose"
	ModeNormal  Mode = "normal"
	ModeTerse   Mode = "terse"
	ModeUltra   Mode = "ultra"
)

// AllModes returns every valid mode (sorted by ascending compression).
// Used by config validation and the `sin-code config list` surface.
func AllModes() []Mode {
	return []Mode{ModeDefault, ModeVerbose, ModeNormal, ModeTerse, ModeUltra}
}

// ParseMode normalizes a user-supplied string (case-insensitive,
// whitespace-trimmed) to a canonical Mode. Unknown values parse as
// ModeDefault so misconfiguration fails closed to the safe behavior.
func ParseMode(s string) Mode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case string(ModeDefault), "":
		return ModeDefault
	case string(ModeVerbose):
		return ModeVerbose
	case string(ModeNormal):
		return ModeNormal
	case string(ModeTerse):
		return ModeTerse
	case string(ModeUltra):
		return ModeUltra
	default:
		return ModeDefault
	}
}

// Valid reports whether m is one of the canonical mode values. Useful
// in config validation — we keep it separate from ParseMode so a
// caller can distinguish "explicit default" from "invalid".
func (m Mode) Valid() bool {
	switch m {
	case ModeDefault, ModeVerbose, ModeNormal, ModeTerse, ModeUltra:
		return true
	}
	return false
}

// EmitsBlock reports whether the mode contributes a non-empty ruleset
// to the system prompt. ModeDefault and ModeVerbose are no-ops.
func (m Mode) EmitsBlock() bool {
	switch m {
	case ModeNormal, ModeTerse, ModeUltra:
		return true
	}
	return false
}

// String returns the canonical name of the mode.
func (m Mode) String() string {
	if m == "" {
		return string(ModeDefault)
	}
	return string(m)
}

// ─── Rulesets ──────────────────────────────────────────────────────────────
//
// Every ruleset is a `const` so the output is byte-stable across builds
// for a given (mode, skillBody). The auto-clarity clause is duplicated
// deliberately so each ruleset is self-contained — no inheritance, no
// concatenation surprises. Satisfies mandate M3: terse output is never
// an excuse to skip the careful prose around a destructive operation.

const header = "# Output style\n"

// modeNormal: today's behavior minus pleasantries and tool-call
// narration. Safe default for users on `default`.
const rulesNormal = `# Output style (normal)
- No pleasantries, no "Sure!" / "I'd be happy to help" / "Great question" openers.
- No tool-call narration ("First, I'll use sin_read to...") — only the result.
- Group related work under a heading. End code blocks with the file path and line range when known.
- Code blocks, URLs, file paths, error strings, commit-type keywords, exact line numbers, and ` + "`func`/`var`/`const` names " + `are byte-preserved.

# Auto-clarity
When the next action is destructive, security-relevant, or order-sensitive (schema drops, force-push, token rotation, lock ordering, multi-step migrations), drop to normal prose for that section, label the section, then resume normal prose after.
`

// modeTerse: caveman-`full` analog. Fragments OK. Drop articles.
// One word when one word is enough.
const rulesTerse = `# Output style (terse)
- Drop articles, conjunctions, hedging, filler phrases.
- Fragments OK when one fragment suffices. Code/URLs/paths/error strings byte-preserved.
- No tool-call narration. Show the result, not the journey.
- Lists preferred over prose for >2 items.

# Auto-clarity
When the next action is destructive, security-relevant, or order-sensitive (schema drops, force-push, token rotation, lock ordering, multi-step migrations), drop to normal prose for that section, label the section, then resume terse prose after.
`

// modeUltra: caveman-`ultra` analog. Tightest valid compression.
// Use `→` for causal chains. Abbreviate only prose words.
const rulesUltra = `# Output style (ultra)
- Tightest valid compression. One word when one word is enough.
- Fragments. Drop articles, auxiliaries, hedging entirely. Code/URLs/paths/error strings byte-preserved.
- Use '→' for causal chains (read → parse → write). Use '<-' for assignment direction in verbose mode.
- Abbreviate only prose words (config→cfg, directory→dir OK; func RenderSystemBlock not OK).
- No narrating, no hedging, no greeting. Start answer with the answer.

# Auto-clarity
When the next action is destructive, security-relevant, or order-sensitive (schema drops, force-push, token rotation, lock ordering, multi-step migrations), drop to normal prose for that section, label the section, then resume ultra prose after.
`

// rulesFor returns the ruleset text for a non-default mode. Default
// and Verbose callers must short-circuit on EmitsBlock before calling.
func rulesFor(m Mode) string {
	switch m {
	case ModeNormal:
		return rulesNormal
	case ModeTerse:
		return rulesTerse
	case ModeUltra:
		return rulesUltra
	}
	return ""
}

// ─── Public renderers ─────────────────────────────────────────────────────

// RenderRules returns the rendered ruleset for mode, optionally
// followed by an injected skill body (separated by a blank line).
//
//   - default / verbose / unknown → returns skillBody unchanged.
//   - normal / terse / ultra       → returns header + ruleset (+ skillBody).
//
// The output is byte-stable for a given (mode, skillBody) pair.
func RenderRules(m Mode, skillBody string) string {
	if !m.EmitsBlock() {
		return skillBody
	}
	var b strings.Builder
	b.Grow(len(header) + len(rulesFor(m)) + len(skillBody) + 2)
	b.WriteString(header)
	b.WriteString(rulesFor(m))
	if strings.TrimSpace(skillBody) != "" {
		// Original skillBody — never trim its trailing newlines, the
		// caller may rely on them (Markdown linters do).
		b.WriteString("\n\n")
		b.WriteString(skillBody)
	}
	return b.String()
}

// RenderSystemBlock is the convenience entry point used by the
// agent loop and config getters. Returns "" when level is empty,
// default, verbose, or unknown. Matches the
// instinct.RenderSystemBlock convention: a non-empty string means
// "inject this verbatim into the system prompt".
func RenderSystemBlock(level string) string {
	return RenderRules(ParseMode(level), "")
}

// AppendVerbosity is the composition primitive. It returns
//
//	existing verbatim when mode is default/verbose — never alters
//	the caller's content (instinct block, skill body, etc.) — and
//	returns "existing\n\n<ruleset>" otherwise.
func AppendVerbosity(existing string, mode Mode) string {
	if !mode.EmitsBlock() {
		return existing
	}
	rules := strings.TrimPrefix(rulesFor(mode), header)
	return existing + "\n\n" + header + rules
}

// ─── Functional options ───────────────────────────────────────────────────

// SystemPromptOption mutates a system-prompt builder. The verbosity
// option appends the ruleset after any prior block was written.
type SystemPromptOption func(*strings.Builder)

// WithVerbosity returns a SystemPromptOption that appends the ruleset
// for mode to the prompt under construction. Mode default/verbose
// produces a no-op option.
func WithVerbosity(m Mode) SystemPromptOption {
	return func(b *strings.Builder) {
		if !m.EmitsBlock() {
			return
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(header)
		b.WriteString(rulesFor(m))
	}
}
