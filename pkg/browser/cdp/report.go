// SPDX-License-Identifier: MIT
// Purpose: aggregated Report for the agent loop.
//
// Report is the single structured object the agent consumes after a recording
// session. It is a *view* over the JSONL ground truth — the JSONL file remains
// the canonical source of truth and is never modified by this layer.
//
// The pipeline is:
//
//	rec.Events() → Analyze → Findings
//	             → Correlate → Chains
//	             → Suggest → FixSuggests
//	             → BuildReport → Report
//	             → Report.WriteJSON → report.json (agent persists for diffing)
package cdp

import (
	"encoding/json"
	"os"
	"time"
)

// Report is the top-level object the agent consumes after a navigation or
// interaction session. All fields are deterministic given the same event slice.
type Report struct {
	// GeneratedAt is the wall-clock time BuildReport was called (RFC3339Nano).
	GeneratedAt string `json:"generated_at"`

	// EventCount is the total number of CDP events captured in the session.
	EventCount int `json:"event_count"`

	// Findings are the grouped, classified problems (errors, warnings, info).
	// Sorted by severity then count — the most critical items come first.
	Findings []*Finding `json:"findings"`

	// Chains are root-cause correlation chains that link a failing network
	// request to the downstream exceptions and console errors it caused.
	Chains []*Chain `json:"chains"`

	// Suggestions are deterministic, rule-based fix hints the agent can route
	// to the appropriate auto-fix handler via FixClass.
	Suggestions []*FixSuggest `json:"suggestions"`

	// Summary is a quick health snapshot for agent decision branching.
	Summary ReportSummary `json:"summary"`
}

// ReportSummary contains the counters the agent reads first before deciding
// whether to dig into Findings, route Suggestions, or declare success.
type ReportSummary struct {
	Errors   int  `json:"errors"`
	Warnings int  `json:"warnings"`
	HasFatal bool `json:"has_fatal"` // true when any error-severity finding exists
}

// BuildReport runs the full deterministic analysis pipeline over events and
// returns a Report ready for the agent loop.
//
//	report := cdp.BuildReport(rec.Events(), 25)
//	if report.Summary.HasFatal { ... }
func BuildReport(events []*Event, window uint64) *Report {
	findings := Analyze(events)
	chains := Correlate(events, window)
	suggestions := Suggest(findings, chains)

	var errs, warns int
	for _, f := range findings {
		switch f.Severity {
		case SevError:
			errs += f.Count
		case SevWarn:
			warns += f.Count
		}
	}

	return &Report{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		EventCount:  len(events),
		Findings:    findings,
		Chains:      chains,
		Suggestions: suggestions,
		Summary: ReportSummary{
			Errors:   errs,
			Warnings: warns,
			HasFatal: errs > 0,
		},
	}
}

// WriteJSON persists the Report as indented JSON next to the JSONL log.
// The file can be re-read later for DiffReports without a re-run.
func (r *Report) WriteJSON(path string) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
