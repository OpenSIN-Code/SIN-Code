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

// DifficultyInput bundles the signals the difficulty gate consumes when
// deciding whether a verify-fail warrants a tournament fan-out. It augments
// the raw verify report with orchestrator confidence (from the critic /
// governor calibrated-confidence layer), the attempt count for the failing
// task, and an explicit error-type classification.
//
// ConfidenceScore is 0..1 where 0 means "unknown / not provided" and is
// treated conservatively (low-confidence → tournament). ErrorType is one
// of "compile", "test", "runtime", "stylistic" (case-insensitive) or
// empty; an empty ErrorType defers to the confidence signals and finally
// to the text heuristic on VerifyReport.
type DifficultyInput struct {
	VerifyReport    string
	AttemptCount    int
	ConfidenceScore float64
	ErrorType       string
}

// ShouldTournament decides whether a verify-fail warrants a tournament
// fan-out or should fall back to the legacy single-model retry. Returns
// true when the failure looks structural (compile, test, build break) and
// false for stylistic or edge-case misses.
//
// This is the text-only legacy entry point; callers with orchestrator
// confidence should prefer ShouldTournamentWithConfidence. The heuristic
// is intentionally simple and conservative: we look for keywords in the
// verify report that indicate a hard structural failure. Unknown modes
// default to true (better to try a tournament than not).
func ShouldTournament(vr verify.Result) bool {
	if vr.Passed {
		return false
	}
	return shouldTournamentByText(vr.Report)
}

// ShouldTournamentWithConfidence applies the confidence-aware difficulty
// gate. The orchestrator confidence score (critic / governor) is honoured
// before the legacy text heuristic: low confidence or repeated failures
// escalate to a tournament, while high-confidence stylistic misses stay
// on the legacy single-model retry path. When no explicit signal is
// decisive the decision falls back to the text-based heuristic on
// VerifyReport.
//
// Decision order:
//  1. ConfidenceScore > 0.7 && ErrorType == "stylistic" → legacy retry.
//  2. ConfidenceScore < 0.3 → tournament (structural / unknown).
//  3. AttemptCount > 3 → tournament (repeated failures).
//  4. ErrorType "compile" or "runtime" → tournament (structural).
//  5. ErrorType "test" → low confidence (< 0.5) tournament, else retry.
//  6. Default → text-based heuristic on VerifyReport.
func ShouldTournamentWithConfidence(input DifficultyInput) bool {
	et := strings.ToLower(strings.TrimSpace(input.ErrorType))
	conf := input.ConfidenceScore

	if conf > 0.7 && et == "stylistic" {
		return false
	}
	if conf < 0.3 {
		return true
	}
	if input.AttemptCount > 3 {
		return true
	}
	if et == "compile" || et == "runtime" {
		return true
	}
	if et == "test" {
		return conf < 0.5
	}
	return shouldTournamentByText(input.VerifyReport)
}

// shouldTournamentByText is the keyword-based heuristic shared by the
// legacy and confidence-aware entry points. Returns true for structural
// indicators and false for stylistic ones; unknown reports default to
// true (better to fan out than silently retry a broken model).
func shouldTournamentByText(report string) bool {
	r := strings.ToLower(report)

	stylisticIndicators := []string{
		"style", "format", "naming", "convention",
		"documentation", "comment", "readability",
		"cosmetic", "whitespace", "indentation",
	}

	for _, ind := range stylisticIndicators {
		if strings.Contains(r, ind) {
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
		if strings.Contains(r, ind) {
			return true
		}
	}

	return true
}
