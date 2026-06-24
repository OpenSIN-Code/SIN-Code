package cdp

import (
	"context"
	"fmt"
	"sort"
)

// FixExecutor is implemented by the SIN-Code agent. The adapter calls Apply for
// a suggestion and expects the agent to perform the concrete remediation
// (LLM code edit, config change, endpoint check, ...). Revert undoes the last
// applied fix when a regression is detected.
// sin-debt: yagni, upgrade: when a second FixExecutor implementation lands, remove this marker
type FixExecutor interface {
	// Apply attempts a fix for one suggestion. Return ok=false if this executor
	// does not handle the suggestion's FixClass (the loop then tries the next).
	Apply(ctx context.Context, s *FixSuggest) (ok bool, err error)
	// Revert rolls back the most recent successful Apply.
	Revert(ctx context.Context, s *FixSuggest) error
}

// Rerunner re-records the page after a fix and returns a fresh report, so the
// adapter can diff before/after. The agent owns browser/navigation lifecycle.
type Rerunner interface {
	Rerun(ctx context.Context) (*Report, error)
}

// LoopConfig tunes the auto-fix loop.
type LoopConfig struct {
	MaxAttempts   int    // hard cap on fix attempts (default 10)
	MinConfidence string // skip suggestions below this ("low"|"medium"|"high")
	OnlyFatal     bool   // only act when the report HasFatal
}

func DefaultLoopConfig() LoopConfig {
	return LoopConfig{MaxAttempts: 10, MinConfidence: "low", OnlyFatal: true}
}

// LoopResult is the audit trail of one auto-fix session.
type LoopResult struct {
	Attempts  []*Attempt `json:"attempts"`
	Resolved  int        `json:"resolved"`
	Remaining int        `json:"remaining"`
	Converged bool       `json:"converged"` // no fatal findings remain
}

type Attempt struct {
	Signature string `json:"signature"`
	FixClass  string `json:"fix_class"`
	Applied   bool   `json:"applied"`
	Improved  bool   `json:"improved"`
	Reverted  bool   `json:"reverted"`
	Error     string `json:"error,omitempty"`
}

// RunAutoFix drives the closed loop: pick the highest-priority suggestion,
// apply it, re-run, diff, accept or revert, repeat until convergence or cap.
func RunAutoFix(ctx context.Context, initial *Report, exec FixExecutor, rerun Rerunner, cfg LoopConfig) (*LoopResult, error) {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 10
	}
	res := &LoopResult{}
	current := initial

	for len(res.Attempts) < cfg.MaxAttempts {
		if cfg.OnlyFatal && !current.Summary.HasFatal {
			break // nothing critical left to fix
		}

		s := pickSuggestion(current, cfg.MinConfidence)
		if s == nil {
			break // no actionable suggestion remains
		}

		att := &Attempt{Signature: s.Signature, FixClass: s.FixClass}

		ok, err := exec.Apply(ctx, s)
		if err != nil {
			att.Error = err.Error()
			res.Attempts = append(res.Attempts, att)
			break
		}
		if !ok {
			// No executor handled this class; record and stop to avoid a busy loop.
			att.Error = "no executor for fix_class " + s.FixClass
			res.Attempts = append(res.Attempts, att)
			break
		}
		att.Applied = true

		after, err := rerun.Rerun(ctx)
		if err != nil {
			att.Error = fmt.Sprintf("rerun: %v", err)
			res.Attempts = append(res.Attempts, att)
			break
		}

		diff := DiffReports(current, after)
		att.Improved = diff.Improved

		if diff.Improved || len(diff.Resolved) > 0 && !introducedFatal(diff) {
			// Accept the fix and continue from the new state.
			res.Resolved += len(diff.Resolved)
			current = after
		} else {
			// Regression or no progress: roll back and move on.
			if rerr := exec.Revert(ctx, s); rerr != nil {
				att.Error = fmt.Sprintf("revert: %v", rerr)
			}
			att.Reverted = true
		}
		res.Attempts = append(res.Attempts, att)
	}

	res.Remaining = current.Summary.Errors
	res.Converged = !current.Summary.HasFatal
	return res, nil
}

// pickSuggestion selects the next suggestion to try: errors before warnings,
// higher confidence first, filtered by the minimum confidence threshold.
func pickSuggestion(r *Report, minConf string) *FixSuggest {
	var candidates []*FixSuggest
	min := confRank(minConf)
	for _, s := range r.Suggestions {
		if confRank(s.Confidence) >= min {
			candidates = append(candidates, s)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if severityRank(candidates[i].Severity) != severityRank(candidates[j].Severity) {
			return severityRank(candidates[i].Severity) < severityRank(candidates[j].Severity)
		}
		return confRank(candidates[i].Confidence) > confRank(candidates[j].Confidence)
	})
	return candidates[0]
}

func introducedFatal(d *Diff) bool {
	for _, f := range d.Introduced {
		if f.Severity == SevError {
			return true
		}
	}
	return false
}

// confRank maps confidence strings to a comparable order (high > medium > low).
func confRank(c string) int {
	switch c {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}
