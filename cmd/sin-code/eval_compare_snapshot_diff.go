// SPDX-License-Identifier: MIT
// Purpose: `sin-code eval compare`, `eval snapshot`, `eval diff`
// commands and the buildScorer helper.
// Extracted from eval_cmd.go for single-responsibility file layout.
// sin-debt: shrink, upgrade: consolidate when eval is refactored
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/evalharness"
)

// ── eval compare / snapshot / diff ──────────────────────────────────

// newEvalCompareCmd is the shortcut for "run all four arms and
// print the matrix". Same wiring as --arm baseline,terse,lazy_skill,<user>.
func newEvalCompareCmd() *cobra.Command {
	var (
		userSkill    string
		modelPricing string
		timeout      time.Duration
	)
	cmd := &cobra.Command{
		Use:   "compare",
		Short: "Run all four arms (baseline/terse/lazy_skill/<user>) on a dataset",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			dsPath, _ := cmd.Flags().GetString("dataset")
			if dsPath == "" {
				return errors.New("eval compare: --dataset is required")
			}
			armsFlag := "baseline,terse,lazy_skill," + userSkill
			return runArmComparator(ctx, dsPath, armsFlag, userSkill, modelPricing, timeout, false)
		},
	}
	cmd.Flags().String("dataset", "", "Path to Golden Dataset JSON file (required)")
	cmd.Flags().StringVar(&userSkill, "skill", "skill-code-create", "User-skill arm name")
	cmd.Flags().StringVar(&modelPricing, "model-pricing", "stub", "Price-book entry (issue #171)")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "Per-case timeout")
	_ = cmd.MarkFlagRequired("dataset")
	return cmd
}

// newEvalSnapshotCmd writes a snapshot (one row per arm) to disk
// so CI can diff the resulting JSON against the committed baseline.
func newEvalSnapshotCmd() *cobra.Command {
	var (
		userSkill    string
		modelPricing string
		outPath      string
		timeout      time.Duration
	)
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Write a one-row-per-arm snapshot file (issue #171)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			dsPath, _ := cmd.Flags().GetString("dataset")
			if dsPath == "" {
				return errors.New("eval snapshot: --dataset is required")
			}
			if outPath == "" {
				return errors.New("eval snapshot: --out is required")
			}
			arms, err := parseArms("baseline,terse,lazy_skill,"+userSkill, userSkill)
			if err != nil {
				return fmt.Errorf("eval snapshot: %w", err)
			}
			for i := range arms {
				if arms[i].PricingName == "" {
					arms[i].PricingName = modelPricing
				}
			}
			es, err := loadEvalSetFromGoldenDataset(dsPath)
			if err != nil {
				return fmt.Errorf("eval snapshot: %w", err)
			}
			opts := evalharness.CompareOptions{PerCaseTimeout: timeout}
			rep, err := evalharness.Compare(ctx, es, arms, opts)
			if err != nil {
				return fmt.Errorf("eval snapshot: compare: %w", err)
			}
			hdr := evalharness.SnapshotHeader{
				SetName:       filepath.Base(dsPath),
				SinCodeVer:    "v3.18.0",
				SchemaVersion: evalharness.SnapshotSchemaVersion,
			}
			if err := evalharness.WriteSnapshotFile(outPath, rep, hdr); err != nil {
				return fmt.Errorf("eval snapshot: write: %w", err)
			}
			fmt.Fprintf(os.Stderr, "snapshot written: %s (arms=%d cases=%d)\n", outPath, len(rep.Arms), len(rep.PerCase))
			return nil
		},
	}
	cmd.Flags().String("dataset", "", "Path to Golden Dataset JSON file (required)")
	cmd.Flags().StringVar(&outPath, "out", "", "Output snapshot file path (required)")
	cmd.Flags().StringVar(&userSkill, "skill", "skill-code-create", "User-skill arm name")
	cmd.Flags().StringVar(&modelPricing, "model-pricing", "stub", "Price-book entry (issue #171)")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "Per-case timeout")
	_ = cmd.MarkFlagRequired("dataset")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}

// newEvalDiffCmd produces a row-by-row delta between two snapshot
// files. Used in CI to deep-diff PRs against the committed
// baseline snapshot (caveman evals/README.md §3).
func newEvalDiffCmd() *cobra.Command {
	var snapA, snapB string
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Diff two snapshot files (issue #171)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if snapA == "" || snapB == "" {
				return errors.New("eval diff: --snapshot and --snapshot-b are both required")
			}
			A, err := evalharness.LoadSnapshotFile(snapA)
			if err != nil {
				return fmt.Errorf("eval diff: load %s: %w", snapA, err)
			}
			B, err := evalharness.LoadSnapshotFile(snapB)
			if err != nil {
				return fmt.Errorf("eval diff: load %s: %w", snapB, err)
			}
			deltas, err := evalharness.DiffSnapshots(A, B)
			if err != nil {
				return fmt.Errorf("eval diff: %w", err)
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(map[string]any{
				"snapshot_a": A.Header.SetName,
				"snapshot_b": B.Header.SetName,
				"deltas":     deltas,
			})
		},
	}
	cmd.Flags().StringVar(&snapA, "snapshot", "", "Path to snapshot A (must compare against)")
	cmd.Flags().StringVar(&snapB, "snapshot-b", "", "Path to snapshot B (the candidate)")
	_ = cmd.MarkFlagRequired("snapshot")
	_ = cmd.MarkFlagRequired("snapshot-b")
	return cmd
}

// buildScorer constructs an evalharness.Scorer from the CLI flags.
// Supported types: compile_and_run, exact, contains.
func buildScorer(typ, lang, selfCheck string, skipTest bool, binary string) (evalharness.Scorer, error) {
	switch typ {
	case "compile_and_run":
		if lang == "" {
			return nil, errors.New("--language is required for compile_and_run scorer")
		}
		if !evalharness.IsCompileAndRunLanguage(lang) {
			return nil, fmt.Errorf("unsupported language %q", lang)
		}
		return evalharness.CompileAndRun{
			Language:  lang,
			SelfCheck: selfCheck,
			SkipTest:  skipTest,
			Binary:    binary,
		}, nil
	case "exact":
		return evalharness.ExactMatch{}, nil
	case "contains":
		return evalharness.ContainsAll{}, nil
	default:
		return nil, fmt.Errorf("unsupported scorer %q", typ)
	}
}
