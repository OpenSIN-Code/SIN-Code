// SPDX-License-Identifier: MIT
// Purpose: caveman-style output contract for orchestrator sub-agents.
//
// Inspired by JuliusBrussee/caveman's `cavecrew-*` sub-agents, the four
// orchestrator sub-agents (Critic, Adversary, Governor, Cartographer) MUST
// emit a slice of `Finding` structs — never free-form prose. Each Finding
// renders to ONE byte-stable line of the form
//
//	<path>:<line> — <symbol> — <tag> — <hint> [# c=<confidence>]
//
// and a Verifier enforces the contract in two passes:
//  1. Structural: every Finding has Path + Line (file-level uses Line=0).
//  2. Lexical: zero hedging words ("you might", "perhaps", …) and the hint
//     ends with no softener ("maybe", "or so").
//
// The byte-stability is a hard prerequisite for downstream consumers:
// the ledger (issue #168) hashes the rendered bytes; the orchestrator's
// repair-loop re-ingests them and any whitespace drift would re-bill 1k+
// tokens of "looks-similar" prose.
//
// The 5 tags parallel `JuliusBrussee/ponytail`'s ponytail-tag convention:
//
//	delete    → remove this code/path/symbol
//	simplify  → inline / collapse / reduce
//	rebuild   → rewrite from scratch
//	risk      → blast radius, regression source
//	verify    → needs human verification
//
// Once a Finding is in the slice we freeze it. The proactive form is the
// only legal form. Prose around the slice (intro, outro) is forbidden.
package orchestrator

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Tag is the closed caveman-tag enumeration. The Verifier rejects any
// other value — including the empty string. We use a string type so
// the rendered output is human-readable and the parser can be regex-only.
type Tag string

const (
	TagDelete   Tag = "delete"
	TagSimplify Tag = "simplify"
	TagRebuild  Tag = "rebuild"
	TagRisk     Tag = "risk"
	TagVerify   Tag = "verify"
)

// AllTags is the closed set. Ordering is canonical: keep alphabetical
// (delete, rebuild, risk, simplify, verify) so internal hash outputs
// don't drift between builds.
var AllTags = []Tag{TagDelete, TagRebuild, TagRisk, TagSimplify, TagVerify}

// IsValidTag returns true iff t is one of the five accepted tags.
func IsValidTag(t Tag) bool {
	for _, a := range AllTags {
		if a == t {
			return true
		}
	}
	return false
}

// Finding is one caveman one-liner. Sub-agents MUST produce slices of
// Finding — never free-form prose. The struct fields are deliberately
// flat: zero nested types, no pointers, no maps. This keeps the rendered
// output byte-stable and trivially hashable.
//
// Confidence is the only optional field. The renderer always emits
// " # c=<x>" with two-digit precision so byte-equality checks stay
// stable; 0.0 means "unspecified" and still renders.
type Finding struct {
	Tag        Tag
	Symbol     string
	Path       string
	Line       int
	Confidence float64
	Hint       string
}

// Render produces the canonical, byte-stable caveman one-liner.
//
//	<path>:<line> — <symbol> — <tag> — <hint> [# c=<conf>]
//
// File-level findings (Line == 0) drop the ":0" suffix. The em-dash is
// U+2014; the parser splits on " — " (three bytes). Confidence renders
// with two decimal places; 0.0 still emits " # c=0.00" so missing-value
// and zero-value findings produce the same bytes.
//
// Render does NOT allocate (or even consume) trailing whitespace.
// Hint is trimmed on both sides so finding-write tools never inject a
// phantom newline that would break round-tripping.
func (f Finding) Render() string {
	loc := f.Path
	if f.Line > 0 {
		loc = fmt.Sprintf("%s:%d", f.Path, f.Line)
	}
	sym := strings.TrimSpace(f.Symbol)
	if sym == "" {
		sym = "-"
	}
	hint := strings.TrimSpace(f.Hint)
	return fmt.Sprintf("%s — %s — %s — %s # c=%.2f", loc, sym, string(f.Tag), hint, f.Confidence)
}

// findingLineRegex captures the canonical line shape.
//
// Group 1 = locator (`<path>:<line>` OR `<path>` for file-level).
// Group 2 = symbol (anything non-em-dash).
// Group 3 = tag (lowercase ASCII).
// Group 4 = hint.
// Group 5 = optional numeric confidence.
//
// The regex is anchored end-to-end; partial matches are rejected.
var findingLineRegex = regexp.MustCompile(
	`^([^—]+?) — ([^—]+?) — ([a-z]+) — (.+?) # c=(\d+\.\d{2})$`,
)

// ParseFinding parses ONE caveman line. Returns an error for any
// structural violation (unknown tag, missing em-dash, trailing
// confidence missing). Whitespace around the line is trimmed.
//
// This is intentionally strict: a single bad character (mismatched
// em-dash, wrong number of decimals on the confidence) costs the
// whole parse. The Verifier never silently passes.
func ParseFinding(s string) (Finding, error) {
	line := strings.TrimSpace(s)
	m := findingLineRegex.FindStringSubmatch(line)
	if m == nil {
		return Finding{}, fmt.Errorf("not a caveman line (missing em-dashes or bad tag): %q", s)
	}
	loc, sym, tag, hint, confStr := strings.TrimSpace(m[1]), strings.TrimSpace(m[2]), m[3], strings.TrimSpace(m[4]), m[5]
	if !IsValidTag(Tag(tag)) {
		return Finding{}, fmt.Errorf("unknown tag %q (allowed: delete, simplify, rebuild, risk, verify)", tag)
	}
	path, lineNo := loc, 0
	if idx := strings.LastIndex(loc, ":"); idx >= 0 {
		if n, err := strconv.Atoi(loc[idx+1:]); err == nil {
			path = loc[:idx]
			lineNo = n
		}
	}
	conf, err := strconv.ParseFloat(confStr, 64)
	if err != nil {
		return Finding{}, fmt.Errorf("confidence must be numeric (e.g. 0.85): %w", err)
	}
	return Finding{
		Tag:        Tag(tag),
		Symbol:     sym,
		Path:       path,
		Line:       lineNo,
		Confidence: conf,
		Hint:       hint,
	}, nil
}

// ParseFindings parses a multi-line sub-agent output. Blank lines are
// skipped silently. Each non-blank line MUST parse — partial success
// counts as failure and the caller (the parent Verifier) decides what
// to do.
//
// `errs` carries per-line diagnostics so the caller can re-inject them
// as retry feedback (similar to verify.fail: bad lines are mirrored
// back at the sub-agent verbatim with the line numbers).
func ParseFindings(s string) (fs []Finding, errs []string, err error) {
	allLines := strings.Split(s, "\n")
	for i, raw := range allLines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		f, perr := ParseFinding(line)
		if perr != nil {
			errs = append(errs, fmt.Sprintf("line %d: %s", i+1, perr.Error()))
			continue
		}
		fs = append(fs, f)
	}
	if len(fs) == 0 && len(errs) > 0 {
		return fs, errs, fmt.Errorf("no valid findings in %d-line input", len(errs))
	}
	return fs, errs, nil
}

// hedgingPhrases is the closed set of phrases a sub-agent MUST NOT emit.
// They are the canonical "AI assistant" softeners — when you see one in
// a Finding's Hint, the sub-agent has re-entered chat-mode and the
// orchestrator should reject the entire batch.
//
// Lower-cased before match. All tokens are matched as substrings
// (no word-boundary) so "you might want" and "perhaps the function"
// are both caught, regardless of Model capitalization.
var hedgingPhrases = []string{
	"you might",
	"perhaps",
	"could consider",
	"maybe",
	"i think",
	"i would",
	"sort of",
	"kind of",
	"tends to",
	"should probably",
	"i'd suggest",
	"we should",
}

// FindHedging returns the list of hedging phrases found in s, lower-cased.
// Empty slice means "clean". A non-empty slice blocks the Finding from
// being admitted to the orchestrator's re-ingestion stream.
func FindHedging(s string) []string {
	low := strings.ToLower(s)
	var found []string
	for _, p := range hedgingPhrases {
		if strings.Contains(low, p) {
			found = append(found, p)
		}
	}
	return found
}

// VerifyHint runs the lexical half of the contract: zero hedging, no
// trailing softener, length under 240 chars (one-line contract — multi-
// line hints are forbidden). Returns a per-Finding error wrapped in a
// slice so the parser can attribute violations precisely.
func VerifyHint(f Finding) error {
	if bad := FindHedging(f.Hint); len(bad) > 0 {
		return fmt.Errorf("hint at %s:%d contains hedging phrase %q", f.Path, f.Line, bad[0])
	}
	if len(f.Hint) > 240 {
		return fmt.Errorf("hint at %s:%d exceeds 240 chars (%d) — multi-line is forbidden", f.Path, f.Line, len(f.Hint))
	}
	if strings.HasSuffix(f.Hint, ".") || strings.HasSuffix(f.Hint, "!") {
		return fmt.Errorf("hint at %s:%d ends in %q — imperative style forbids it", f.Path, f.Line, f.Hint[len(f.Hint)-1:])
	}
	return nil
}

// VerifyFindings runs the FULL contract — structural + lexical — over a
// slice of Findings. Returns a per-Finding error wrapped in a slice.
// Empty result means "all findings accepted".
//
// Required invariants:
//
//  1. Path != ""             (locator MUST be present)
//  2. Tag ∈ AllTags         (closed enumeration)
//  3. zero hedging in Hint  (FindHedging(Hint) == ∅)
//  4. Hint length < 240     (one-liner rule)
//  5. Hint ends WITHOUT punctuation  (imperative style)
func VerifyFindings(fs []Finding) (errs []string) {
	for _, f := range fs {
		if f.Path == "" {
			errs = append(errs, fmt.Sprintf("finding for symbol %q has empty Path", f.Symbol))
			continue
		}
		if !IsValidTag(f.Tag) {
			errs = append(errs, fmt.Sprintf("finding %s:%d has unknown tag %q", f.Path, f.Line, f.Tag))
			continue
		}
		if perr := VerifyHint(f); perr != nil {
			errs = append(errs, perr.Error())
			continue
		}
	}
	return errs
}

// FindingsToBytes renders a slice to a multi-line, newline-terminated
// string. Trailing newline is preserved so callers can write directly to
// stderr without an extra fmt.Fprintf. Empty input yields the empty
// string — the sub-agent "had nothing to say" — and that is intentional,
// NOT an error.
func FindingsToBytes(fs []Finding) string {
	if len(fs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, f := range fs {
		b.WriteString(f.Render())
		b.WriteByte('\n')
	}
	return b.String()
}
