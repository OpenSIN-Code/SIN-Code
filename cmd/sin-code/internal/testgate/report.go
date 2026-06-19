// SPDX-License-Identifier: MIT
// Purpose: structured report types for the quality gate pipeline.
// Docs: runner.doc.md
package testgate

import "time"

// StepReport is the outcome of one pipeline step.
type StepReport struct {
	Name     string        `json:"name"`
	Status   string        `json:"status"`
	Output   string        `json:"output,omitempty"`
	Error    string        `json:"error,omitempty"`
	Skipped  bool          `json:"skipped,omitempty"`
	Duration time.Duration `json:"duration_ms"`
	Coverage string        `json:"coverage,omitempty"`
}

// Report is the final quality gate result.
type Report struct {
	Status    string        `json:"status"`
	Steps     []StepReport  `json:"steps"`
	Coverage  string        `json:"coverage,omitempty"`
	Duration  time.Duration `json:"duration_ms"`
	Threshold float64       `json:"threshold"`
}

// Passed returns true only when every step passed.
func (r *Report) Passed() bool {
	if r.Status != "" {
		return r.Status == "PASS"
	}
	for _, s := range r.Steps {
		if s.Status != "PASS" && !s.Skipped {
			return false
		}
	}
	return true
}

// CoveragePercent parses the coverage string and returns a percentage.
// Returns 0 when the string is empty or unparseable.
func (r *Report) CoveragePercent() float64 {
	return parseCoveragePercent(r.Coverage)
}
