// SPDX-License-Identifier: MIT
// Package mcpcompress shrinks MCP tool descriptions on the wire to save
// input tokens for every turn of every session that loads the tool manifest.
//
// This is the Go-native analog of caveman's `caveman-shrink` MCP middleware
// (https://github.com/JuliusBrussee/caveman). SIN-Code is itself the MCP
// server in `serve`, so the compression runs inline — there is no proxy
// hop.
//
// # Rule format
//
// Each Rule is a deterministic, byte-stable description transformer. The
// five canonical rules are tagged against the ponytail tag set:
//
//	delete   — DeleteHedges:        drops pleasantries/hedges ("safely",
//	                                 "carefully", "thoroughly", ...).
//	stdlib   — StdlibPatterns:      drops redundant stdlib references.
//	native   — DropTrimEncouragement: drops "Always prefer over native X"
//	                                 tail clauses (M6 compliance text).
//	yagni    — YagniPatterns:       drops speculative / reserved / future.
//	shrink   — ShrinkExamples:      drops parenthetical "(e.g. ...)"
//	                                 examples and redundant trailing
//	                                 em-dash preambles.
//
// # Byte-stability contract
//
// For every (input, Pipeline) pair, Apply produces a byte-exact output
// that survives:
//
//   - the lifecycle of the binary (no time-based generation),
//   - the host CPU (no FP math on bytes — ratio calc is the only FP),
//   - any number of repeated calls (idempotent by construction).
//
// This is the prerequisite for the system-prompt hash metric
// (issue #2): identical `(tool_spec, ruleset)` ⇒ identical manifest bytes.
//
// # Wiring
//
// `cmd/sin-code/internal/serve.go` reads `--compress-tools` and feeds the
// pipeline into `registerAllMCPTools` so the registered `mcp.Tool` carries
// the compressed description. The 44+ tool names are untouched (they are
// public API per AGENTS.md §10). Only the `description` byte field shrinks.
package mcpcompress

import (
	"regexp"
	"sort"
	"strings"
)

// Tag is a ponytail-style categorical tag for a Rule. Five tags.
//
// Canonical set: delete | stdlib | native | yagni | shrink.
type Tag string

// Canonical ponytail tag set.
const (
	TagDelete Tag = "delete"
	TagStdlib Tag = "stdlib"
	TagNative Tag = "native"
	TagYagni  Tag = "yagni"
	TagShrink Tag = "shrink"
)

// DefaultTags is the canonical ponytail tag list in declaration order.
//
// Two ordering invariants matter:
//
//  1. The slice is the public `--compress-tags` default — reordering
//     is a silent API drift. Do NOT sort at construction time.
//  2. Pipeline application (see Selected) iterates All() in declaration
//     order; Selected filters, never reorders.
//
// Adding a tag is non-breaking; renaming or removing one is a major bump.
var DefaultTags = []Tag{TagDelete, TagStdlib, TagNative, TagYagni, TagShrink}

// Rule is a deterministic, byte-stable description transformer.
//
// All implementations MUST be:
//
//   - Deterministic: same input + rule set ⇒ same byte output, every run.
//   - Pure: no side effects, no reads of package-level mutable state.
//   - Idempotent: Apply(Apply(s)) == Apply(s).
//
// Rule names form a stable surface for debugging. Renames are breaking.
type Rule interface {
	Name() string
	Tag() Tag
	Apply(s string) string
}

// Pipeline is an ordered, immutable list of Rules.
//
// Rules run in declaration order. The Pipeline itself is a value type so
// callers can pass/copy without aliasing.
type Pipeline []Rule

// Apply runs every Rule in the Pipeline in declaration order.
// Byte-stable per (input, Pipeline).
func (p Pipeline) Apply(s string) string {
	for _, r := range p {
		s = r.Apply(s)
	}
	return s
}

// Names returns the Rule names in declaration order. Stable surface
// for telemetry + JSON manifests.
func (p Pipeline) Names() []string {
	out := make([]string, len(p))
	for i, r := range p {
		out[i] = r.Name()
	}
	return out
}

// Tags returns the Rule tags in declaration order. Stable surface.
func (p Pipeline) Tags() []Tag {
	out := make([]Tag, len(p))
	for i, r := range p {
		out[i] = r.Tag()
	}
	return out
}

// Spec carries the parts of an mcp.Tool that the compressor touches.
// The 44+ tool Names are public API (AGENTS.md §10) and are NEVER
// modified by the compressor.
type Spec struct {
	Name        string
	Description string
}

// Stats captures the byte budget before/after for one tool description.
// Always computed from len() of the original vs compressed bytes;
// Ratio is the only FP field.
type Stats struct {
	Name       string
	Original   int     // bytes, len(Description)
	Compressed int     // bytes, len(result)
	BytesSaved int     // bytes, Original - Compressed (clamped at 0)
	Ratio      float64 // (Original - Compressed) / Original in [0,1]
}

// CompressSpec applies the pipeline to one tool description and
// returns the compressed form plus Stats.
//
// Byte-stable per (Name, Description, Pipeline). Idempotent:
// CompressSpec(name, CompressSpec(name, s, p).Result, p) ==
// CompressSpec(name, s, p).
func CompressSpec(spec Spec, p Pipeline) (Spec, Stats) {
	out := p.Apply(spec.Description)
	out = Normalize(out)
	outName := spec.Name // Rule never touches Name.
	return Spec{Name: outName, Description: out}, Stats{
		Name:       spec.Name,
		Original:   len(spec.Description),
		Compressed: len(out),
		BytesSaved: bytesSaved(spec.Description, out),
		Ratio:      ratio(spec.Description, out),
	}
}

// CompressAll applies the pipeline to every Spec in declaration order
// and returns the Stats slice in the same order. Byte-stable per
// (specs, Pipeline).
func CompressAll(specs []Spec, p Pipeline) []Stats {
	out := make([]Stats, len(specs))
	for i, s := range specs {
		_, out[i] = CompressSpec(s, p)
	}
	return out
}

// Normalize is the post-pipeline whitespace + punctuation normaliser.
// Run exactly once after the pipeline — never twice — to keep
// byte-stability across any future rule renames.
//
// Operations (in this exact order):
//
//  1. collapseWs: runs of " ", ", ,", "; ;", ": :" → single form.
//  2. stripOrphanedDelimiters: a leading/trailing ",", ";", ":".
//
// We do NOT append a trailing period to ensure the wire bytes
// NEVER grow above the source description. The "never increases"
// invariant is part of the public contract.
//
// This function is idempotent on already-normalized input.
func Normalize(s string) string {
	s = collapseWs(s)
	s = stripOrphanedDelimiters(s)
	return s
}

func collapseWs(s string) string {
	// Comma/space clusters after a drop rule leave ",," or ",  " debris.
	s = strings.ReplaceAll(s, ", ,", ",")
	s = strings.ReplaceAll(s, "; ;", ";")
	s = strings.ReplaceAll(s, ": :", ":")
	s = strings.ReplaceAll(s, "  ", " ")
	// Two passes: the first pass may leave "  " behind (e.g. "a   b" → "a b "
	// → "a b"). Loop is bounded by an absolute upper bound so we cannot
	// spin.
	for i := 0; i < 8; i++ {
		if !strings.Contains(s, "  ") {
			break
		}
		s = strings.ReplaceAll(s, "  ", " ")
	}
	s = strings.ReplaceAll(s, " .", ".")
	s = strings.ReplaceAll(s, " ,", ",")
	s = strings.ReplaceAll(s, " ;", ";")
	s = strings.ReplaceAll(s, " :", ":")
	return s
}

func stripOrphanedDelimiters(s string) string {
	s = strings.TrimSpace(s)
	for len(s) > 0 {
		last := s[len(s)-1]
		if last == ',' || last == ';' || last == ':' {
			s = s[:len(s)-1]
			s = strings.TrimRight(s, " ")
			continue
		}
		break
	}
	return s
}

// bytesSaved returns Original-Compressed clamped at 0. Pure integer math.
func bytesSaved(orig, comp string) int {
	diff := len(orig) - len(comp)
	if diff < 0 {
		return 0
	}
	return diff
}

// ratio returns (Original-Compressed)/Original in [0,1]. The only FP
// computation in the package; deterministic per (orig, comp) — no time,
// no random, no locale.
func ratio(orig, comp string) float64 {
	if len(orig) == 0 {
		return 0
	}
	saved := len(orig) - len(comp)
	if saved <= 0 {
		return 0
	}
	return float64(saved) / float64(len(orig))
}

// Selected returns a Pipeline composed of every Rule from All() whose
// tag appears in the requested tag set, preserving declaration order.
//
// The tag set is matched exactly against Rule.Tag(). There is no
// family matching — passing [TagDelete] yields only the single Rule
// tagged TagDelete. If you want all the delete-family Rules (today
// just DeleteHedges), pass their Tag values explicitly.
//
// Unknown tags in `want` are silently dropped. Caller-side validation
// (see ValidateTags) is preferred at config boundaries.
//
// Byte-stable per (want, Rule declarations). Adding a new Rule
// with new Tag does not affect the output of an existing
// (want, declarations) pair until that Rule is requested.
func Selected(want []Tag) Pipeline {
	if len(want) == 0 {
		return All()
	}
	idx := make(map[Tag]bool, len(want))
	for _, t := range want {
		idx[t] = true
	}
	var out Pipeline
	for _, r := range All() {
		if idx[r.Tag()] {
			out = append(out, r)
		}
	}
	return out
}

// All returns the default Rule set in canonical declaration order.
// This is the source of truth for tag↔rule pairing and order.
func All() Pipeline {
	return Pipeline{
		RuleDeleteHedges{},
		RuleStdlibPatterns{},
		RuleDropTrimEncouragement{},
		RuleYagniPatterns{},
		RuleShrinkExamples{},
	}
}

// ValidateTags returns the sorted-unique subset of input tags that
// the package recognises. Unknown tags are dropped. The returned
// slice is sorted alphabetically (TagDelete, TagNative, TagShrink,
// TagStdlib, TagYagni) so config-round-trip json output is
// deterministic across runs.
//
// Always returns a non-nil slice.
func ValidateTags(in []string) []Tag {
	known := map[Tag]struct{}{
		TagDelete: {},
		TagStdlib: {},
		TagNative: {},
		TagYagni:  {},
		TagShrink: {},
	}
	out := make([]Tag, 0, len(known))
	for _, raw := range in {
		t := Tag(raw)
		if _, ok := known[t]; ok {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// MustCompile is a tiny wrapper around regexp.MustCompile that exists
// so the package owns its compile-time panic surface and tests can
// share the compiled regex set.
func MustCompile(pat string) *regexp.Regexp {
	return regexp.MustCompile(pat)
}
