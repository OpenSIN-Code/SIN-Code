// SPDX-License-Identifier: MIT
// Purpose: Verified Execution — machine-checkable postconditions on agent output.
// Every agent action is gated by Check(s) returning a scored Verdict,
// not a binary "did not crash". Verdicts are replayable, weighted, and
// feed repair / selection loops.
package orchestrator

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

type CheckKind string

const (
	CheckBuild     CheckKind = "build"
	CheckTest      CheckKind = "test"
	CheckLint      CheckKind = "lint"
	CheckPredicate CheckKind = "predicate"
	CheckDiffScope CheckKind = "diff-scope"
)

var checkWeights = map[CheckKind]float64{
	CheckBuild:     1.0,
	CheckTest:      0.9,
	CheckDiffScope: 0.7,
	CheckPredicate: 0.5,
	CheckLint:      0.3,
}

type Check struct {
	Kind    CheckKind
	Name    string
	Cmd     []string
	Timeout time.Duration
	AllowedPaths []string
}

type CheckResult struct {
	Check    Check
	Passed   bool
	Output   string
	Duration time.Duration
}

type Verdict struct {
	TaskID    string
	Candidate string
	Results   []CheckResult
	Score     float64
	Passed    bool
	CreatedAt time.Time
}

func (v *Verdict) Diagnosis() string {
	var b strings.Builder
	hasFailed := false
	for _, r := range v.Results {
		if r.Passed {
			continue
		}
		hasFailed = true
		fmt.Fprintf(&b, "FAILED [%s] %s\n", r.Check.Kind, r.Check.Name)
		fmt.Fprintf(&b, "--- output (truncated) ---\n%s\n", truncate(r.Output, 2000))
	}
	if !hasFailed {
		return "all checks passed"
	}
	return fmt.Sprintf("VERDICT task=%s candidate=%s score=%.2f\n%s",
		v.TaskID, v.Candidate, v.Score, b.String())
}

type Verifier struct {
	Workdir       string
	MandatoryKinds []CheckKind
}

func NewVerifier(workdir string) *Verifier {
	return &Verifier{
		Workdir:        workdir,
		MandatoryKinds: []CheckKind{CheckBuild, CheckTest},
	}
}

func (vf *Verifier) Verify(ctx context.Context, taskID, candidate string, checks []Check) *Verdict {
	v := &Verdict{TaskID: taskID, Candidate: candidate, CreatedAt: timeNow()}
	mandatory := make(map[CheckKind]bool, len(vf.MandatoryKinds))
	for _, k := range vf.MandatoryKinds {
		mandatory[k] = true
	}

	var weightTotal, weightPassed float64
	allMandatoryPassed := true

	for _, c := range checks {
		res := vf.runCheck(ctx, c)
		v.Results = append(v.Results, res)

		w := checkWeights[c.Kind]
		if w == 0 {
			w = 0.5
		}
		weightTotal += w
		if res.Passed {
			weightPassed += w
		} else if mandatory[c.Kind] {
			allMandatoryPassed = false
		}
	}

	if weightTotal > 0 {
		v.Score = weightPassed / weightTotal
	}
	v.Passed = allMandatoryPassed
	return v
}

func (vf *Verifier) runCheck(ctx context.Context, c Check) CheckResult {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 3 * time.Minute
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := timeNow()
	if len(c.Cmd) == 0 {
		return CheckResult{Check: c, Passed: false, Output: "empty check command"}
	}
	cmd := exec.CommandContext(cctx, c.Cmd[0], c.Cmd[1:]...)
	cmd.Dir = vf.Workdir
	out, err := cmd.CombinedOutput()
	return CheckResult{
		Check:    c,
		Passed:   err == nil,
		Output:   string(out),
		Duration: timeNow().Sub(start),
	}
}

func DefaultGoChecks() []Check {
	return []Check{
		{Kind: CheckBuild, Name: "go build", Cmd: []string{"go", "build", "./..."}},
		{Kind: CheckTest, Name: "go test", Cmd: []string{"go", "test", "./...", "-count=1", "-timeout=120s"}},
		{Kind: CheckLint, Name: "go vet", Cmd: []string{"go", "vet", "./..."}},
	}
}

func BestVerdict(verdicts []*Verdict) *Verdict {
	if len(verdicts) == 0 {
		return nil
	}
	sorted := make([]*Verdict, len(verdicts))
	copy(sorted, verdicts)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.Passed != b.Passed {
			return a.Passed
		}
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		return a.CreatedAt.Before(b.CreatedAt)
	})
	return sorted[0]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n...[truncated]"
}
