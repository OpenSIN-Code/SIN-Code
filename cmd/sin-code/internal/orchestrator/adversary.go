// SPDX-License-Identifier: MIT
// Purpose: Adversarial Reviewer — a second agent whose explicit mandate
// is to BREAK the change. Hypotheses become executable probes; a probe
// that runs red PROVES the attack landed. Surviving probes stay on
// disk as permanent regression tests (the change makes the suite
// stronger by failing review).
package orchestrator

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
)

type AttackKind string

const (
	AttackBoundary    AttackKind = "boundary"
	AttackConcurrency AttackKind = "concurrency"
	AttackResource    AttackKind = "resource"
	AttackContract    AttackKind = "contract"
	AttackInjection   AttackKind = "injection"
)

type Attack struct {
	Kind        AttackKind
	Hypothesis  string
	ProbeSource string
	Landed      bool
	Output      string
}

type AdversaryAgent interface {
	ProposeAttacks(ctx context.Context, diff, impactBrief string, maxAttacks int) ([]Attack, error)
}

// AdversaryResult is the bounded probe outcome. Findings carries the
// caveman-style one-liners derived from each Attack; the prose
// CounterexampleBrief below is rendered from the same Findings when
// possible and falls back to free-form only when the contract was
// not satisfied. ParseErrors is the per-line rejection trace.
type AdversaryResult struct {
	Attacks     []Attack
	Landed      int
	Cleared     bool
	Findings    []Finding
	ParseErrors []string
}

func (r *AdversaryResult) CounterexampleBrief() string {
	if r.Cleared {
		return fmt.Sprintf("adversarial review: %d attacks attempted, none landed — cleared", len(r.Attacks))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "adversarial review: %d/%d attacks LANDED. Reproducible counterexamples:\n",
		r.Landed, len(r.Attacks))
	for _, a := range r.Attacks {
		if !a.Landed {
			continue
		}
		fmt.Fprintf(&b, "\n=== [%s] %s ===\nFailing probe output:\n%s\n", a.Kind, a.Hypothesis, truncate(a.Output, 1500))
		fmt.Fprintf(&b, "Probe source (keep as regression test after fixing):\n%s\n", truncate(a.ProbeSource, 3000))
	}
	b.WriteString("\nFix the code so ALL probes pass, then keep the probes as permanent tests.\n")
	return b.String()
}

type Adversary struct {
	Agent        AdversaryAgent
	Workdir      string
	MaxAttacks   int
	ProbeTimeout time.Duration
	Hooks        *hooks.Engine
}

func NewAdversary(agent AdversaryAgent, workdir string) *Adversary {
	return &Adversary{Agent: agent, Workdir: workdir, MaxAttacks: 6, ProbeTimeout: 90 * time.Second}
}

func (adv *Adversary) Review(ctx context.Context, diff, impactBrief string) (*AdversaryResult, error) {
	attacks, err := adv.Agent.ProposeAttacks(ctx, diff, impactBrief, adv.MaxAttacks)
	if err != nil {
		return nil, fmt.Errorf("adversary propose: %w", err)
	}

	res := &AdversaryResult{}
	for i := range attacks {
		a := &attacks[i]
		landed, output, err := adv.executeProbe(ctx, a, i)
		if err != nil {
			a.Output = "probe error: " + err.Error()
			res.Attacks = append(res.Attacks, *a)
			continue
		}
		a.Landed = landed
		a.Output = output
		if landed {
			res.Landed++
		}
		res.Attacks = append(res.Attacks, *a)
	}
	res.Cleared = res.Landed == 0
	collectAdversaryFindings(res)
	if len(res.Attacks) > 0 && adv.Hooks != nil {
		adv.Hooks.Fire(ctx, hooks.Payload{
			Event: hooks.AdversaryFinding,
			Data: map[string]any{
				"landed": res.Landed,
				"total":  len(res.Attacks),
				"cleared": res.Cleared,
			},
		})
	}
	return res, nil
}

// collectAdversaryFindings derives caveman-style Findings from each
// Attack. The Attack struct already carries structured fields (Kind,
// Hypothesis) so the Finding is cheaply assembled. landed attacks
// are tagged `risk` (regression source), cleared attacks are tagged
// `verify` (probe passed, human review still wanted). ParseErrors
// is reserved for the prose layer (currently unused but kept for
// symmetry with the Critic and Governor result types).
func collectAdversaryFindings(res *AdversaryResult) {
	res.Findings = make([]Finding, 0, len(res.Attacks))
	for _, a := range res.Attacks {
		tag := TagVerify
		conf := 0.5
		if a.Landed {
			tag = TagRisk
			conf = 1.0
		}
		// Adversary Attacks have no natural Path:Line (they are
		// hypothesis-shaped, not locator-shaped). We emit a
		// file-level Finding so the orchestrator's verifiers can
		// still attribute — the Adversary produces the hypothesis,
		// not the location.
		res.Findings = append(res.Findings, Finding{
			Tag:        tag,
			Symbol:     string(a.Kind),
			Path:       "adversary://probe",
			Line:       0,
			Confidence: conf,
			Hint:       a.Hypothesis,
		})
	}
}

func (adv *Adversary) executeProbe(ctx context.Context, a *Attack, idx int) (landed bool, output string, err error) {
	if adv.Workdir == "" {
		return false, "", fmt.Errorf("adversary: empty workdir")
	}
	pkgDir, err := probePackageDir(a.ProbeSource, adv.Workdir)
	if err != nil {
		return false, "", err
	}
	probePath := filepath.Join(pkgDir, fmt.Sprintf("adversary_%d_test.go", idx))
	if err := os.WriteFile(probePath, []byte(a.ProbeSource), 0o600); err != nil {
		return false, "", err
	}

	cctx, cancel := context.WithTimeout(ctx, adv.ProbeTimeout)
	defer cancel()
	rel := relOrDot(adv.Workdir, pkgDir)
	out, runErr := adversaryExecCommand(cctx, adv.Workdir, rel, adv.ProbeTimeout)

	landed = runErr != nil
	if !landed {
		_ = os.Remove(probePath)
	}
	return landed, string(out), nil
}

// adversaryExecCommand runs the adversary probe test command. Tests override
// this hook to avoid spawning real subprocesses.
var adversaryExecCommand = func(ctx context.Context, workdir, rel string, timeout time.Duration) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "go", "test", "-run", "TestAdversary", "-count=1", "./"+rel)
	cmd.Dir = workdir
	return cmd.CombinedOutput()
}

func probePackageDir(src, workdir string) (string, error) {
	for _, line := range strings.Split(src, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "package ") {
			pkg := strings.TrimPrefix(t, "package ")
			pkg = strings.TrimSuffix(pkg, "_test")
			var found string
			_ = filepath.WalkDir(workdir, func(path string, d os.DirEntry, err error) error {
				if err != nil || found != "" || d.IsDir() {
					return err
				}
				if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
					content, rerr := os.ReadFile(path)
					if rerr == nil && strings.Contains(string(content), "package "+pkg+"\n") {
						found = filepath.Dir(path)
					}
				}
				return nil
			})
			if found != "" {
				return found, nil
			}
			return "", fmt.Errorf("adversary: package %q not found in worktree", pkg)
		}
	}
	return "", fmt.Errorf("adversary: probe has no package clause")
}

func relOrDot(base, target string) string {
	rel, err := filepath.Rel(base, target)
	if err != nil || rel == "." {
		return "."
	}
	return rel
}
