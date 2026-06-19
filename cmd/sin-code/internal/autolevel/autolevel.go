// SPDX-License-Identifier: MIT
// Purpose: deterministic prompt-intent → permission-mode classifier.
// Given a user prompt, returns the recommended `permission.Mode` so the
// operator doesn't have to type `--mode=plan` explicitly.
//
// Mirrors Claude Code's `/permissions` auto-classifier (Anthropic
// v2.1, 2026-01-22) — but replaces Anthropic's LLM-side classification
// with deterministic, byte-stable regex matching. Mirrors mandate M3:
// no silent LLM-driven behaviour shifts without an operator-visible
// classification reason.
//
// M2-friendly: zero new deps. Pure Go + stdlib regexp.
package autolevel

import (
	"regexp"
	"sort"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
)

// ModeReason bundles the recommended Mode and a short English
// rationale (suitable for the chat toggle's stderr/JSON output).
type ModeReason struct {
	Mode   permission.Mode
	Reason string // human-readable, e.g. "build/compile verb detected"
}

// Classify returns the recommended mode for `prompt`. The second
// return is "no match" (ModeReason.Mode == "default", Reason == "no
// classifier signal") when no rule scored high.
//
// The classifier is byte-stable: same prompt bytes map to same
// result regardless of run, goroutine, or platform. Determinism is
// mandated by `Classify` so downstream `byte-stable` tests can
// pin the rule weights.
func Classify(prompt string) ModeReason {
	p := strings.ToLower(prompt)
	scored := []scorerMatch{}
	for _, r := range rules {
		if hit, idx := matchFirst(p, r); hit >= 0 {
			scored = append(scored, scorerMatch{r, hit, idx})
		}
	}
	if len(scored) == 0 {
		return ModeReason{Mode: permission.ModeDefault, Reason: "no classifier signal"}
	}
	sort.SliceStable(scored, func(i, j int) bool {
		// Higher weight wins; tiebreak by earliest hit.
		if scored[i].rule.Weight != scored[j].rule.Weight {
			return scored[i].rule.Weight > scored[j].rule.Weight
		}
		return scored[i].hitIdx < scored[j].hitIdx
	})
	winner := scored[0].rule
	return ModeReason{Mode: winner.Mode, Reason: winner.Reason}
}

// rule is the typed form of one classifier entry.
type rule struct {
	Name    string           // stable identifier (used in tests/golden)
	Mode    permission.Mode  // the mode the rule emits
	Reason  string           // a short human-readable explanation
	Weight  int              // higher wins; default 5 if unspecified
	Phrases []string         // literal substring triggers (case-insensitive)
	Regex   []*regexp.Regexp // regex triggers
}

func matchFirst(prompt string, r rule) (int, int) {
	bestIdx := -1
	bestRun := -1
	for _, phrase := range r.Phrases {
		if idx := strings.Index(prompt, phrase); idx >= 0 {
			if bestIdx < 0 || idx < bestIdx {
				bestIdx = idx
			}
			bestRun++
		}
	}
	for _, re := range r.Regex {
		if loc := re.FindStringIndex(prompt); loc != nil {
			if bestIdx < 0 || loc[0] < bestIdx {
				bestIdx = loc[0]
			}
			bestRun++
		}
	}
	return bestIdx, bestRun
}

type scorerMatch struct {
	rule   rule
	hitIdx int
	score  int
}

// rules defines the classifier's decision matrix. Sorted only by
// declaration order; the comparator in `Classify` picks the winner.
//
// Heuristic rationale (we keep the WHY next to the WHAT so future
// maintainers can audit the matrix without re-deriving intent):
//   - plan: questions, exploration, no execution verbs present
//   - acceptEdits: the user is asking the model to write code, add
//     files, format, refactor — all read-after-write local edits
//   - bypass: explicit "rm -rf", or "delete /", etc.
//   - default: fallback when no signal is strong
var rules = []rule{
	{
		Name:   "explicit_plan",
		Mode:   permission.ModePlan,
		Reason: "explicit plan / read-only verb",
		Weight: 12,
		Phrases: []string{
			"plan this", "outline", "review", "explain", "describe",
			"what does", "what is", "shows me", "show me", "dry-run",
			"dry run", "no changes", "don't change", "do not change",
			"without running", "without writing",
		},
	},
	{
		Name:   "explicit_accept",
		Mode:   permission.ModeAcceptEdits,
		Reason: "build/edit verb detected",
		Weight: 8,
		Phrases: []string{
			"write", "edit ", "create file", "add ", "implement",
			"refactor", "rename", "format ", "fix the", "make it",
			"update", "patch", "rewrite", "tidy up", "cleanup",
		},
		Regex: []*regexp.Regexp{
			regexp.MustCompile(`(?i)\bmake\s+\w+`),
			regexp.MustCompile(`(?i)\badd\s+(a|an|the)?\s*\w+\b`),
			regexp.MustCompile(`(?i)\bfile\s+(should|needs?)\s+to\s+`),
		},
	},
	{
		Name:   "explicit_bypass",
		Mode:   permission.ModeBypass,
		Reason: "destructive verb + explicit user OK",
		Weight: 50,
		Phrases: []string{
			"force push", "rm -rf", "delete everything", "reset --hard",
			"drop table", "wipe", "destroy",
		},
	},
	{
		Name:   "explicit_test_accept",
		Mode:   permission.ModeAcceptEdits,
		Reason: "test-only instructions",
		Weight: 6,
		Phrases: []string{
			"add tests", "add test", "write tests", "write test",
			"verify", "self-check", "make sure it",
		},
	},
	{
		Name:   "explicit_question_only",
		Mode:   permission.ModePlan,
		Reason: "ending with ?",
		Weight: 3,
		Regex: []*regexp.Regexp{
			regexp.MustCompile(`\?\s*$`),
		},
	},
}
