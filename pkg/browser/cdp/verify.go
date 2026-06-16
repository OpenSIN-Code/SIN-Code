// SPDX-License-Identifier: MIT
// Purpose: Fix-Verify loop — before/after report diffing for the agent.
//
// After applying a fix the agent creates a fresh Recorder, re-navigates, and
// calls BuildReport again. DiffReports compares the two reports keyed by
// Finding.Signature and tells the agent whether to accept the fix, revert it,
// or try the next suggestion.
//
// Usage:
//
//	baseline := cdp.BuildReport(rec.Events(), 25)
//
//	// ... agent applies a fix (code edit / config / endpoint) ...
//
//	rec2, _ := cdp.NewRecorder(cdp.DefaultConfig("evidence.after.jsonl"))
//	// ... set up ctx2, rec2.Attach + EnableDomains + InstallVitals, navigate ...
//	after := cdp.BuildReport(rec2.Events(), 25)
//
//	diff := cdp.DiffReports(baseline, after)
//	switch {
//	case diff.Improved:
//	    agent.AcceptFix()
//	case len(diff.Introduced) > 0:
//	    agent.RevertFix()       // regression introduced
//	default:
//	    agent.TryNextSuggestion() // fix had no effect
//	}
package cdp

// Diff is the result of comparing a before-fix and after-fix Report. The agent
// uses it to decide whether to accept a fix, revert it, or try the next one.
type Diff struct {
	// Resolved contains findings that were present before and gone after the fix.
	Resolved []*Finding `json:"resolved"`

	// Introduced contains new findings that appeared after the fix (regressions).
	Introduced []*Finding `json:"introduced"`

	// Persisted contains findings that exist in both reports.
	Persisted []*Finding `json:"persisted"`

	// Improved is true when the net error count decreased AND no new
	// error-severity findings were introduced.
	Improved bool `json:"improved"`

	// BeforeErr and AfterErr are the total error event counts from each report.
	BeforeErr int `json:"before_errors"`
	AfterErr  int `json:"after_errors"`
}

// DiffReports computes the before/after delta keyed by Finding.Signature.
// It is deterministic: given the same two reports it always returns the same Diff.
func DiffReports(before, after *Report) *Diff {
	beforeBySig := map[string]*Finding{}
	for _, f := range before.Findings {
		beforeBySig[f.Signature] = f
	}
	afterBySig := map[string]*Finding{}
	for _, f := range after.Findings {
		afterBySig[f.Signature] = f
	}

	d := &Diff{BeforeErr: before.Summary.Errors, AfterErr: after.Summary.Errors}

	for sig, f := range beforeBySig {
		if _, ok := afterBySig[sig]; !ok {
			d.Resolved = append(d.Resolved, f)
		} else {
			d.Persisted = append(d.Persisted, afterBySig[sig])
		}
	}
	for sig, f := range afterBySig {
		if _, ok := beforeBySig[sig]; !ok {
			d.Introduced = append(d.Introduced, f)
		}
	}

	// A fix "improved" things only if total errors dropped AND no new
	// error-severity findings were introduced.
	regressed := false
	for _, f := range d.Introduced {
		if f.Severity == SevError {
			regressed = true
			break
		}
	}
	d.Improved = d.AfterErr < d.BeforeErr && !regressed
	return d
}
