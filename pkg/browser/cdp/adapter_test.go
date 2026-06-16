package cdp

import (
	"context"
	"testing"
)

// fakeExec resolves a fix on the first apply; fakeRerun then returns a clean report.
type fakeExec struct {
	applied int
	reverts int
}

func (f *fakeExec) Apply(_ context.Context, _ *FixSuggest) (bool, error) {
	f.applied++
	return true, nil
}

func (f *fakeExec) Revert(_ context.Context, _ *FixSuggest) error {
	f.reverts++
	return nil
}

type fakeRerun struct{ after *Report }

func (f *fakeRerun) Rerun(_ context.Context) (*Report, error) { return f.after, nil }

func TestAutoFixConverges(t *testing.T) {
	initial := &Report{
		Findings:    []*Finding{{Signature: "exc:boom", Severity: SevError, Count: 1}},
		Suggestions: []*FixSuggest{{Signature: "exc:boom", Severity: SevError, FixClass: "js.reference", Confidence: "high"}},
		Summary:     ReportSummary{Errors: 1, HasFatal: true},
	}
	clean := &Report{Findings: []*Finding{}, Summary: ReportSummary{Errors: 0, HasFatal: false}}

	exec := &fakeExec{}
	res, err := RunAutoFix(context.Background(), initial, exec, &fakeRerun{after: clean}, DefaultLoopConfig())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Converged {
		t.Error("expected convergence after the only error was resolved")
	}
	if exec.applied != 1 {
		t.Errorf("expected 1 apply, got %d", exec.applied)
	}
	if res.Resolved != 1 {
		t.Errorf("expected 1 resolved, got %d", res.Resolved)
	}
}

func TestAutoFixRevertsOnRegression(t *testing.T) {
	initial := &Report{
		Findings:    []*Finding{{Signature: "exc:a", Severity: SevError, Count: 1}},
		Suggestions: []*FixSuggest{{Signature: "exc:a", Severity: SevError, FixClass: "js.reference", Confidence: "high"}},
		Summary:     ReportSummary{Errors: 1, HasFatal: true},
	}
	// Re-run introduces a NEW error and keeps the old one -> regression.
	worse := &Report{
		Findings: []*Finding{
			{Signature: "exc:a", Severity: SevError, Count: 1},
			{Signature: "exc:b", Severity: SevError, Count: 1},
		},
		Summary: ReportSummary{Errors: 2, HasFatal: true},
	}
	exec := &fakeExec{}
	cfg := DefaultLoopConfig()
	cfg.MaxAttempts = 1
	res, _ := RunAutoFix(context.Background(), initial, exec, &fakeRerun{after: worse}, cfg)
	if exec.reverts != 1 {
		t.Errorf("expected 1 revert on regression, got %d", exec.reverts)
	}
	if res.Attempts[0].Improved {
		t.Error("attempt should not be marked improved on regression")
	}
}
