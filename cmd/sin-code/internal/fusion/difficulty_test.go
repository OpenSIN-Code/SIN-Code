// SPDX-License-Identifier: MIT
// Purpose: Confidence-aware difficulty gate tests (issue #290). Covers the
// orchestrator-confidence signal path (ShouldTournamentWithConfidence) and
// the legacy text-only entry point (ShouldTournament). All tests must pass
// under `go test -race -count=1` (mandate M7).
package fusion

import (
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

func TestShouldTournamentWithConfidence_HighConfidenceStylistic_NoTournament(t *testing.T) {
	got := ShouldTournamentWithConfidence(DifficultyInput{
		VerifyReport:    "style: naming convention violation",
		AttemptCount:    1,
		ConfidenceScore: 0.8,
		ErrorType:       "stylistic",
	})
	if got {
		t.Error("expected false for high-confidence stylistic miss (legacy retry)")
	}
}

func TestShouldTournamentWithConfidence_LowConfidence_Tournament(t *testing.T) {
	got := ShouldTournamentWithConfidence(DifficultyInput{
		VerifyReport:    "ambiguous failure mode",
		AttemptCount:    1,
		ConfidenceScore: 0.2,
		ErrorType:       "",
	})
	if !got {
		t.Error("expected true for low confidence (structural, tournament)")
	}
}

func TestShouldTournamentWithConfidence_UnknownConfidence_Tournament(t *testing.T) {
	got := ShouldTournamentWithConfidence(DifficultyInput{
		VerifyReport:    "style: naming convention violation",
		AttemptCount:    1,
		ConfidenceScore: 0,
		ErrorType:       "",
	})
	if !got {
		t.Error("expected true for unknown confidence (0 = unknown, conservative tournament)")
	}
}

func TestShouldTournamentWithConfidence_HighAttemptCount_Tournament(t *testing.T) {
	got := ShouldTournamentWithConfidence(DifficultyInput{
		VerifyReport:    "unclear failure",
		AttemptCount:    4,
		ConfidenceScore: 0.5,
		ErrorType:       "",
	})
	if !got {
		t.Error("expected true for AttemptCount > 3 (repeated failures suggest structural)")
	}
}

func TestShouldTournamentWithConfidence_CompileError_TournamentRegardlessOfConfidence(t *testing.T) {
	tests := []float64{0.0, 0.5, 0.9}
	for _, conf := range tests {
		got := ShouldTournamentWithConfidence(DifficultyInput{
			VerifyReport:    "irrelevant",
			AttemptCount:    1,
			ConfidenceScore: conf,
			ErrorType:       "compile",
		})
		if !got {
			t.Errorf("expected true for compile error regardless of confidence, got false at conf=%.2f", conf)
		}
	}
}

func TestShouldTournamentWithConfidence_RuntimeError_Tournament(t *testing.T) {
	got := ShouldTournamentWithConfidence(DifficultyInput{
		VerifyReport:    "irrelevant",
		AttemptCount:    1,
		ConfidenceScore: 0.9,
		ErrorType:       "runtime",
	})
	if !got {
		t.Error("expected true for runtime error (structural)")
	}
}

func TestShouldTournamentWithConfidence_TestError_LowConfidence_Tournament(t *testing.T) {
	got := ShouldTournamentWithConfidence(DifficultyInput{
		VerifyReport:    "irrelevant",
		AttemptCount:    1,
		ConfidenceScore: 0.4,
		ErrorType:       "test",
	})
	if !got {
		t.Error("expected true for test error with low confidence (< 0.5)")
	}
}

func TestShouldTournamentWithConfidence_TestError_HighConfidence_NoTournament(t *testing.T) {
	got := ShouldTournamentWithConfidence(DifficultyInput{
		VerifyReport:    "irrelevant",
		AttemptCount:    1,
		ConfidenceScore: 0.6,
		ErrorType:       "test",
	})
	if got {
		t.Error("expected false for test error with high confidence (>= 0.5, legacy retry)")
	}
}

func TestShouldTournamentWithConfidence_DefaultFallsBackToTextHeuristic(t *testing.T) {
	tests := []struct {
		name   string
		report string
		want   bool
	}{
		{"structural report", "compile error: undefined variable x", true},
		{"stylistic report", "style: naming convention violation", false},
		{"empty report", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldTournamentWithConfidence(DifficultyInput{
				VerifyReport:    tt.report,
				AttemptCount:    1,
				ConfidenceScore: 0.5,
				ErrorType:       "",
			})
			if got != tt.want {
				t.Errorf("fallback text heuristic for %q = %v, want %v", tt.report, got, tt.want)
			}
		})
	}
}

func TestShouldTournamentWithConfidence_ErrorTypeCaseInsensitive(t *testing.T) {
	got := ShouldTournamentWithConfidence(DifficultyInput{
		VerifyReport:    "irrelevant",
		AttemptCount:    1,
		ConfidenceScore: 0.9,
		ErrorType:       "COMPILE",
	})
	if !got {
		t.Error("expected true for uppercase COMPILE error type (case-insensitive)")
	}
}

func TestShouldTournament_LegacyPassedResult(t *testing.T) {
	vr := verify.Result{Passed: true, Mode: verify.ModePoC, Report: "all good"}
	if ShouldTournament(vr) {
		t.Error("expected false for passed result via legacy entry point")
	}
}

func TestShouldTournament_LegacyStructuralFailure(t *testing.T) {
	vr := verify.Result{Passed: false, Mode: verify.ModePoC, Report: "compile error: undefined x"}
	if !ShouldTournament(vr) {
		t.Error("expected true for structural failure via legacy entry point")
	}
}
