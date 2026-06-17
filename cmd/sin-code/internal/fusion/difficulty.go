// SPDX-License-Identifier: MIT
// Purpose: Difficulty gate for SIN Fusion v1 (issue #290).
//
// Not every verify-fail needs a full N-provider tournament. The difficulty
// gate classifies the failure using the verify report and decides whether
// to fan out (low-confidence structural failures) or retry with the same
// model (high-confidence single-miss).
package fusion

import (
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

// ShouldTournament decides whether a verify-fail warrants a tournament
// fan-out or should fall back to the legacy single-model retry. Returns
// true when the failure looks structural (compile, test, build break) and
// false for stylistic or edge-case misses.
//
// The heuristic is intentionally simple and conservative: we look for
// keywords in the verify report that indicate a hard structural failure.
// Unknown modes default to true (better to try a tournament than not).
func ShouldTournament(vr verify.Result) bool {
	if vr.Passed {
		return false
	}
	report := strings.ToLower(vr.Report)

	stylisticIndicators := []string{
		"style", "format", "naming", "convention",
		"documentation", "comment", "readability",
		"cosmetic", "whitespace", "indentation",
	}

	for _, ind := range stylisticIndicators {
		if strings.Contains(report, ind) {
			return false
		}
	}

	structuralIndicators := []string{
		"compile", "build", "syntax error", "parse error",
		"test fail", "tests failed", "test: fail",
		"undefined", "unresolved", "cannot find",
		"type error", "type mismatch",
		"missing", "not found", "no such file",
		"panic", "segfault", "nil pointer",
	}

	for _, ind := range structuralIndicators {
		if strings.Contains(report, ind) {
			return true
		}
	}

	return true
}
