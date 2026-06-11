// SPDX-License-Identifier: MIT
// Purpose: Macros — pre-compiled fused chains exposed to the agent as
// single tool calls. sin_change fuses contract-guard + transactional
// edit + fast-verify; sin_refactor adds impact prediction and a
// mutation probe behind the green verdict.
package orchestrator

import (
	"context"
	"fmt"
	"strings"
)

type EditRequest struct {
	TaskID            string
	Edits             map[string][]byte
	DeclaredConfidence float64
}

type MacroResult struct {
	Applied       bool
	Verdict       *Verdict
	Trace         *Trace
	Decision      Decision
	ResumeContext string
}

type Macros struct {
	Workdir  string
	Contract *Contract
	Targeted *TargetedVerifier
	Probe    func(testCmd []string) *MutationProbe
	Calib    *Calibrator
	Policy   MergePolicy
	Agent    string
	Class    TaskClass
}

func (m *Macros) SinChange(ctx context.Context, req EditRequest) (*MacroResult, error) {
	res := &MacroResult{}

	if m.Contract != nil {
		var violations []Violation
		for path, content := range req.Edits {
			added := strings.Split(string(content), "\n")
			violations = append(violations, m.Contract.CheckEdit(path, added)...)
		}
		if len(violations) > 0 {
			var b strings.Builder
			b.WriteString("contract violations (nothing was written):\n")
			for _, v := range violations {
				b.WriteString("- " + v.String() + "\n")
			}
			res.ResumeContext = b.String()
			res.Decision = DecisionBlock
			return res, nil
		}
	}

	txn := BeginTxn(m.Workdir)
	defer func() { _ = txn.Rollback() }()

	for path, content := range req.Edits {
		if err := txn.WriteFile(path, content); err != nil {
			res.ResumeContext = fmt.Sprintf("apply failed at %s: %v (all edits rolled back)", path, err)
			return res, nil
		}
	}

	if m.Targeted == nil {
		return nil, fmt.Errorf("sin_change: TargetedVerifier not wired")
	}
	touched := txn.Touched()
	verdict := m.Targeted.Inner.Verify(ctx, req.TaskID, m.Agent+"@sin_change",
		m.Targeted.FastChecks(touched))
	res.Verdict = verdict

	if !verdict.Passed {
		res.ResumeContext = verdict.Diagnosis() +
			"\nAll edits were rolled back. Repo is in pre-call state. " +
			"Fix the failures above and call sin_change again."
		res.Decision = DecisionBlock
		return res, nil
	}

	if err := txn.Commit(); err != nil {
		return nil, fmt.Errorf("sin_change commit: %w", err)
	}
	res.Applied = true

	calibrated := req.DeclaredConfidence
	if m.Calib != nil {
		if c, err := m.Calib.Calibrate(ctx, m.Agent, req.DeclaredConfidence); err == nil {
			calibrated = c
		}
	}
	res.Decision = m.Policy.Decide(true, calibrated)
	return res, nil
}

func (m *Macros) SinRefactor(ctx context.Context, req EditRequest) (*MacroResult, error) {
	paths := make([]string, 0, len(req.Edits))
	for p := range req.Edits {
		paths = append(paths, p)
	}
	var imp *Impact
	if m.Targeted != nil && m.Targeted.Graph != nil {
		imp = m.Targeted.Graph.Predict(paths)
		if imp.Radius > 0.5 && m.Policy.AutoMergeThreshold < 0.95 {
			m.Policy.AutoMergeThreshold = 0.95
		}
	}

	res, err := m.SinChange(ctx, req)
	if err != nil || res == nil || !res.Applied {
		if res != nil {
			if imp != nil {
				res.ResumeContext = imp.RiskBrief() + "\n" + res.ResumeContext
			}
		}
		return res, err
	}

	if imp == nil || m.Targeted == nil {
		return res, nil
	}

	fast := m.Targeted.FastChecks(paths)
	var testCmd []string
	for _, c := range fast {
		if c.Kind == CheckTest {
			testCmd = c.Cmd
			break
		}
	}
	if len(testCmd) > 0 && m.Probe != nil {
		probeRes, perr := m.Probe(testCmd).Run(ctx, nil)
		if perr == nil && probeRes != nil && probeRes.ObservabilityScore < 0.6 {
			res.Decision = DecisionGreenReview
			res.ResumeContext = probeRes.Diagnosis()
		}
	}
	return res, nil
}
