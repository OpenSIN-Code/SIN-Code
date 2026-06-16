// SPDX-License-Identifier: MIT
// Purpose: spec check — runs every `verify:` command in a *.spec.md
// file and reports pass/fail. Bounded by spec.check.timeout. Aggregates
// per-criterion results into a summary that the CI gate consumes.
// Docs: docs/SPEC-LAYER.md
package spec

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// DefaultCheckTimeout is the per-criterion timeout if not configured.
const DefaultCheckTimeout = 60 * time.Second

// Policy controls how spec-drift failures affect exit codes (issue #157).
//   - PolicyOff:   never block, exit 0 even with must-failures
//   - PolicyWarn:  print warnings, exit 0 (advisory mode)
//   - PolicyError: block on must-failures, exit 1 (CI gate mode)
type Policy string

const (
	PolicyOff   Policy = "off"
	PolicyWarn  Policy = "warn"
	PolicyError Policy = "error"
)

// ParsePolicy normalises a raw policy string. Unknown values
// default to PolicyError (fail-closed; the verify gate is sacred).
func ParsePolicy(s string) Policy {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "off":
		return PolicyOff
	case "warn", "warning":
		return PolicyWarn
	case "error", "strict", "":
		return PolicyError
	default:
		return PolicyError
	}
}

// CheckResult is the outcome of running a single criterion's verify
// command. The ID matches the Criterion.ID from the parsed spec.
type CheckResult struct {
	ID       string        `json:"id"`
	Text     string        `json:"text"`
	Command  string        `json:"command,omitempty"`
	Passed   bool          `json:"passed"`
	Skipped  bool          `json:"skipped,omitempty"`
	ExitCode int           `json:"exit_code,omitempty"`
	Duration time.Duration `json:"duration_ns"`
	Output   string        `json:"output,omitempty"` // truncated stdout+stderr
	Priority Priority      `json:"priority,omitempty"`
}

// CheckReport aggregates per-criterion results into a summary.
type CheckReport struct {
	SpecPath string        `json:"spec_path"`
	Title    string        `json:"title"`
	Results  []CheckResult `json:"results"`
	Passed   int           `json:"passed"`
	Failed   int           `json:"failed"`
	Skipped  int           `json:"skipped"`
	Total    int           `json:"total"`
	Duration time.Duration `json:"duration_ns"`
}

// HasFailures reports whether any must-priority criterion failed. The
// CI gate uses this to decide whether the PR blocks.
func (r *CheckReport) HasFailures() bool {
	for _, res := range r.Results {
		if !res.Passed && !res.Skipped && res.Priority == Must {
			return true
		}
	}
	return false
}

// ShouldBlock returns true if the report should cause the calling
// CLI to exit non-zero, given the active policy. PolicyOff and
// PolicyWarn never block; PolicyError blocks on must-failures.
func (r *CheckReport) ShouldBlock(p Policy) bool {
	switch p {
	case PolicyOff, PolicyWarn:
		return false
	default:
		return r.HasFailures()
	}
}

// Check runs every criterion's verify: command in s. The per-command
// timeout is enforced via context.WithTimeout. Output is truncated to
// 4KB per criterion to keep the report small.
func (s *Spec) Check(ctx context.Context, timeout time.Duration) (*CheckReport, error) {
	if timeout <= 0 {
		timeout = DefaultCheckTimeout
	}
	rep := &CheckReport{SpecPath: s.Path, Title: s.Title, Total: len(s.Criteria)}
	start := time.Now()

	// Build a priority lookup so a criterion's result can carry it.
	prioByID := map[string]Priority{}
	for _, r := range s.Requirements {
		prioByID[r.ID] = r.Priority
	}
	// Criterions inherit from the most recent requirement with the
	// same prefix family if no explicit mapping exists; default must.
	// For simplicity we default each criterion to must and let the
	// spec author override via the [must]/[should]/[may] prefix on
	// the criterion's text. (The existing parseCriterion does not
	// extract that today; this is a known limitation. We default
	// to must so the CI gate is conservative.)
	for i := range s.Criteria {
		// Future: parse [must]/[should]/[may] from criterion.Text
		// and propagate here. For now, assume must.
		_ = i
	}

	for _, c := range s.Criteria {
		res := CheckResult{
			ID:       c.ID,
			Text:     c.Text,
			Priority: Must, // conservative until the parser learns priorities
		}
		if strings.TrimSpace(c.Verify) == "" {
			res.Skipped = true
			rep.Skipped++
			rep.Results = append(rep.Results, res)
			continue
		}
		res.Command = c.Verify
		cctx, cancel := context.WithTimeout(ctx, timeout)
		t0 := time.Now()
		out, err := runVerify(cctx, c.Verify)
		res.Duration = time.Since(t0)
		cancel()

		res.Output = truncateOutput(out, 4096)
		if err == nil {
			res.Passed = true
			rep.Passed++
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			res.ExitCode = exitErr.ExitCode()
			rep.Failed++
		} else {
			// ctx.Err() (timeout) or exec.LookPath error: treat as
			// failure with exit code -1 so the CI gate catches it.
			res.ExitCode = -1
			res.Output = truncateOutput(err.Error()+"\n"+out, 4096)
			rep.Failed++
		}
		rep.Results = append(rep.Results, res)
	}

	rep.Duration = time.Since(start)
	return rep, nil
}

// runVerify executes cmd via `sh -c`, captures combined stdout+stderr,
// returns the output and a *exec.ExitError on non-zero exit. A context
// timeout surfaces as a plain error (we map it to exit -1 in Check).
func runVerify(ctx context.Context, cmd string) (string, error) {
	c := exec.CommandContext(ctx, "sh", "-c", cmd)
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf
	err := c.Run()
	return buf.String(), err
}

func truncateOutput(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n... (truncated, " + fmt.Sprintf("%d", len(s)-n) + " more bytes)"
}
