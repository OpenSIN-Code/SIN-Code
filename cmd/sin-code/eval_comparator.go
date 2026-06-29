// SPDX-License-Identifier: MIT
// Purpose: Eval four-arm comparator logic (issue #171) — arm parsing,
// comparator execution, golden-dataset-to-evalset conversion, and
// matrix rendering.
// Extracted from eval_cmd.go for single-responsibility file layout.
// sin-debt: shrink, upgrade: consolidate when eval is refactored
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/dataset"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/evalharness"
)

// ── eval comparator (issue #171) ────────────────────────────────────
//
// The comparator bypasses the agent loop entirely: each arm is a
// system-prompt rendering wired straight into a stub Subject, so
// the run stays offline and deterministic. This matches the
// caveman evals/README.md guarantee that "reading the snapshot
// requires no LLM, no API key, runs in CI".

// parseArms turns the --arm flag value into a []evalharness.Arm.
// Reserved tokens ("baseline", "terse", "lazy_skill") map to
// pinned arms; everything else is treated as a bundled skill name.
// userSkill is the stand-in for the "__user_skill__" arm reserved
// token.
func parseArms(value, userSkill string) ([]evalharness.Arm, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("arms flag is empty")
	}
	tokens := strings.Split(value, ",")
	out := make([]evalharness.Arm, 0, len(tokens))
	seen := map[string]bool{}
	for _, tok := range tokens {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if seen[tok] {
			continue
		}
		seen[tok] = true
		switch tok {
		case "baseline", "__baseline__":
			out = append(out, evalharness.NoSystemPromptArm())
		case "terse", "__terse__":
			out = append(out, evalharness.StandardTerseArm())
		case "lazy_skill", "__lazy_skill__":
			out = append(out, evalharness.LazySkillArm(func() (string, error) { return evalharness.ReadBundledSkillBody(evalharness.LazySkillName) }))
		case "user_skill", "__user_skill__":
			out = append(out, evalharness.SkillArm(userSkill, func() (string, error) {
				if userSkill == "" {
					return "", errors.New("__user_skill__ arm: --skill is empty")
				}
				return evalharness.ReadBundledSkillBody(userSkill)
			}))
		default:
			out = append(out, evalharness.SkillArm(tok, func() (string, error) {
				return evalharness.ReadBundledSkillBody(tok)
			}))
		}
	}
	if len(out) == 0 {
		return nil, errors.New("arms flag produced no arms")
	}
	return out, nil
}

// runArmComparator executes the four-arm comparator path. It does
// not produce the Golden-Dataset JSON envelope; instead it emits
// the matrix-shaped table whose columns mirror ponytail's
// benchmarks/README.md:34-58.
func runArmComparator(ctx context.Context, datasetPath, armsFlag, userSkill, modelPricing string, timeout time.Duration, jsonOutput bool) error {
	arms, err := parseArms(armsFlag, userSkill)
	if err != nil {
		return fmt.Errorf("eval run: --arm: %w", err)
	}
	for i := range arms {
		if arms[i].PricingName == "" {
			arms[i].PricingName = modelPricing
		}
	}
	evalSet, err := loadEvalSetFromGoldenDataset(datasetPath)
	if err != nil {
		return fmt.Errorf("eval run: load evalset: %w", err)
	}
	opts := evalharness.CompareOptions{}
	if timeout > 0 {
		opts.PerCaseTimeout = timeout
	}
	report, err := evalharness.Compare(ctx, evalSet, arms, opts)
	if err != nil {
		return fmt.Errorf("eval run: compare: %w", err)
	}
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	return printCompareMatrix(os.Stdout, report)
}

// loadEvalSetFromGoldenDataset reuses the dataset JSON parser so
// the comparator harness accepts the SAME files as `eval run`.
// We translate TestCase to EvalCase 1:1.
func loadEvalSetFromGoldenDataset(path string) (evalharness.EvalSet, error) {
	ds, err := dataset.LoadDataset(path)
	if err != nil {
		return evalharness.EvalSet{}, err
	}
	if ds == nil {
		return evalharness.EvalSet{}, errors.New("nil dataset")
	}
	out := evalharness.EvalSet{Name: ds.Name + " (via dataset)", Description: ds.Description}
	out.Cases = make([]evalharness.EvalCase, 0, len(ds.TestCases))
	for _, tc := range ds.TestCases {
		meta := map[string]string{}
		for k, v := range tc.Metadata {
			meta[k] = v
		}
		// Surface the canonical "expected.keywords" as the EvalCase
		// Expected string so ContainsAll scorer works transparently.
		expected := strings.Join(tc.Expected.OutputContains, "\n")
		if expected == "" {
			expected = strings.Join(tc.Expected.ContainsKeywords, "\n")
		}
		ec := evalharness.EvalCase{
			ID:       tc.ID,
			Prompt:   tc.Prompt,
			Expected: expected,
			Tags:     tc.Tags,
			Meta:     meta,
		}
		out.Cases = append(out.Cases, ec)
	}
	return out, nil
}

// printCompareMatrix renders the report as a ponytail-shaped table:
//
//	| arm        | pass_rate | med_LOC | med_MS | med_USD | med_tokens | med_score |
//	|------------|-----------|---------|--------|---------|------------|-----------|
//	| __baseline__|  1.00     |    0    |    0   | 0.000.. |        288 |     1.00  |
//
// Output goes to w (typically os.Stdout).
func printCompareMatrix(w io.Writer, rep evalharness.CompareReport) error {
	if w == nil {
		return errors.New("matrix writer is nil")
	}
	fmt.Fprintln(w, "| arm            | pass_rate | med_LOC | med_latency_ms | med_usd     | med_tokens | med_score |")
	fmt.Fprintln(w, "|----------------|-----------|---------|----------------|-------------|------------|-----------|")
	// Output rows in declaration order of arms (stable, byte-stable).
	for _, arm := range rep.Arms {
		tot, ok := rep.TotalsByArm[arm.ID]
		if !ok {
			continue
		}
		fmt.Fprintf(w, "| %-14s | %9.2f | %7d | %14d | %11.6f | %10d | %9.2f |\n",
			arm.ID,
			tot.PassRate(),
			medianIntLocal(tot.LOC),
			medianIntLocal(tot.LatencyMS),
			medianFloatLocal(tot.USD),
			medianIntLocal(tot.Tokens),
			medianFloatLocal(tot.Scores),
		)
	}
	fmt.Fprintf(w, "\n(honest delta = user-skill row - terse row)\n")
	if len(rep.Warnings) > 0 {
		for _, msg := range rep.Warnings {
			fmt.Fprintf(w, "warn: %s\n", msg)
		}
	}
	return nil
}

// medianIntLocal / medianFloatLocal are tiny local copies of
// medianInt/medianFloat — they live in snapshot.go but are
// unexported; we replicate them here so the comparator CLI
// doesn't need to widen the package API.
func medianIntLocal(xs []int) int {
	if len(xs) == 0 {
		return 0
	}
	c := append([]int(nil), xs...)
	for i := 1; i < len(c); i++ {
		for j := i; j > 0 && c[j-1] > c[j]; j-- {
			c[j-1], c[j] = c[j], c[j-1]
		}
	}
	return c[len(c)/2]
}

func medianFloatLocal(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	c := append([]float64(nil), xs...)
	for i := 1; i < len(c); i++ {
		for j := i; j > 0 && c[j-1] > c[j]; j-- {
			c[j-1], c[j] = c[j], c[j-1]
		}
	}
	return c[len(c)/2]
}
