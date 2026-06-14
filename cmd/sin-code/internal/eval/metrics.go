// SPDX-License-Identifier: MIT
// Purpose: aggregate pass/fail metrics across an eval suite (issue #75)
// and write the JSON envelope consumed by the CLI (`eval run --json`)
// and the n8n-CI step. Pure stdlib; zero side effects.
//
// Docs: metrics.doc.md
package eval

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/dataset"
)

// Summary is the per-batch aggregate the CLI surfaces and the CI
// step evaluates. Fields are flat so the n8n jq filter stays a
// one-liner: `.summary.pass_rate >= .summary.min_required`.
type Summary struct {
	Total       int      `json:"total"`
	Passed      int      `json:"passed"`
	Failed      int      `json:"failed"`
	PassRate    float64  `json:"pass_rate"`
	MinRequired float64  `json:"min_required"`
	MeanJudge   float64  `json:"mean_judge_score"`
	MeanTurns   float64  `json:"mean_turns"`
	MeanDurMS   float64  `json:"mean_duration_ms"`
	Timeouts    int      `json:"timeouts"`
	Failures    []string `json:"failures,omitempty"`
}

// Report is the full JSON envelope: top-level metadata + per-case
// results + rolled-up Summary.
type Report struct {
	Dataset  string              `json:"dataset"`
	Version  string              `json:"version"`
	Profile  string              `json:"profile"`
	MinRate  float64             `json:"min_pass_rate"`
	Started  time.Time           `json:"started_at"`
	Finished time.Time           `json:"finished_at"`
	Summary  Summary             `json:"summary"`
	Results  []dataset.RunResult `json:"results"`
}

// Summarise crosses the RunResult list once and returns the Summary
// view. Idempotent on empty input (returns zero-value Summary).
func Summarise(rs []dataset.RunResult, minRate float64) Summary {
	s := Summary{Total: len(rs), MinRequired: minRate, Failures: []string{}}
	if len(rs) == 0 {
		s.PassRate = 1.0
		return s
	}
	var (
		turns, durNs int64
		jSum         float64
		jCount       int
	)
	for _, r := range rs {
		if r.Success {
			s.Passed++
		} else {
			s.Failed++
			if label := r.TestCaseID; label != "" {
				if r.Error != "" {
					s.Failures = append(s.Failures, label+": "+r.Error)
				} else {
					s.Failures = append(s.Failures, label)
				}
			}
		}
		if r.JudgeScore > 0 {
			jSum += r.JudgeScore
			jCount++
		}
		turns += int64(r.Turns)
		durNs += int64(r.Duration)
		if r.TimedOut {
			s.Timeouts++
		}
	}
	s.PassRate = float64(s.Passed) / float64(s.Total)
	s.MeanTurns = float64(turns) / float64(s.Total)
	if durNs > 0 {
		s.MeanDurMS = float64(durNs) / float64(s.Total) / float64(time.Millisecond)
	}
	if jCount > 0 {
		s.MeanJudge = jSum / float64(jCount)
	}
	sort.Strings(s.Failures)
	return s
}

// NewReport builds the full envelope around a finished run. The
// Started/Finished timestamps let CI detect partial runs (Finished
// zero → crashed before flushing).
func NewReport(ds *dataset.Dataset, profile string, minRate float64, rs []dataset.RunResult, start, end time.Time) *Report {
	return &Report{
		Dataset:  ds.Name,
		Version:  ds.Version,
		Profile:  profile,
		MinRate:  minRate,
		Started:  start,
		Finished: end,
		Summary:  Summarise(rs, minRate),
		Results:  rs,
	}
}

// WriteJSON encodes the Report to w with a stable indent so the CI
// job's jq filter is robust against drift, and uses
// json.NewEncoder so a write error is observable.
func WriteJSON(w io.Writer, r *Report) error {
	if r == nil {
		return errors.New("eval: nil report")
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// BelowMinRate is the sentinel error returned when a report fails
// the CI threshold. The CLI prints this as well-formed JSON so a
// wrapper script can grep for it.
type BelowMinRate struct {
	PassRate float64
	Minimum  float64
}

func (e *BelowMinRate) Error() string {
	return fmt.Sprintf("eval: pass rate %.2f%% below minimum %.2f%%", e.PassRate*100, e.Minimum*100)
}

// PassRateFloor enforces the threshold and returns *BelowMinRate
// when the suite should fail the CI gate.
func PassRateFloor(s Summary) error {
	if s.Total == 0 {
		return nil
	}
	if s.PassRate+1e-9 < s.MinRequired {
		return &BelowMinRate{PassRate: s.PassRate, Minimum: s.MinRequired}
	}
	return nil
}

// FormatHuman prints a CLI-friendly summary on stdout. Kept in this
// package so the JSON (WriteJSON) and human pipelines share the
// same source-of-truth summary struct.
func FormatHuman(s Summary) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Total: %d | Passed: %d | Failed: %d\n", s.Total, s.Passed, s.Failed)
	fmt.Fprintf(&sb, "Pass Rate: %.2f%% (min: %.2f%%)\n", s.PassRate*100, s.MinRequired*100)
	if s.Timeouts > 0 {
		fmt.Fprintf(&sb, "Timeouts: %d\n", s.Timeouts)
	}
	if s.MeanJudge > 0 {
		fmt.Fprintf(&sb, "Mean judge score: %.2f\n", s.MeanJudge)
	}
	if len(s.Failures) > 0 {
		fmt.Fprintf(&sb, "Failed cases:\n")
		for _, f := range s.Failures {
			fmt.Fprintf(&sb, "  - %s\n", f)
		}
	}
	return sb.String()
}

// RoundPassRate normalizes a pass-rate to 4 decimals so output
// without JSON is stable across repeated runs of an identical suite.
func RoundPassRate(p float64) float64 {
	return float64(int64(p*10000+0.5)) / 10000.0
}
