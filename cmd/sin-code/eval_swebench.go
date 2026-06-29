// SPDX-License-Identifier: MIT
// Purpose: `sin-code eval swe-bench` and `eval swebench` commands —
// SWE-bench dataset conversion, scoring, and full harness execution
// (issue #363).
// Extracted from eval_cmd.go for single-responsibility file layout.
// sin-debt: shrink, upgrade: consolidate when eval is refactored
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/eval"
	swebench "github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/swebench"
)

// ── eval swe-bench (issue #363) ──────────────────────────────────────

func newEvalSWEBenchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "swe-bench",
		Short: "SWE-bench dataset conversion and scoring (issue #363)",
		Long: `sin-code eval swe-bench converts SWE-bench instances to the
Golden Dataset format and scores verification output against expected
test results.

    sin-code eval swe-bench convert --input swe-bench.json [--output eval.json] [--limit N]
    sin-code eval swe-bench score   --input swe-bench.json --verify-output out.txt [--json]`,
	}
	cmd.AddCommand(newEvalSWEBenchConvertCmd(), newEvalSWEBenchScoreCmd())
	return cmd
}

func newEvalSWEBenchConvertCmd() *cobra.Command {
	var (
		inputPath  string
		outputPath string
		limit      int
	)
	cmd := &cobra.Command{
		Use:   "convert",
		Short: "Convert a SWE-bench JSON dataset to Golden Dataset format",
		RunE: func(cmd *cobra.Command, args []string) error {
			ds, err := swebench.LoadDataset(inputPath)
			if err != nil {
				return fmt.Errorf("swe-bench convert: load: %w", err)
			}
			if limit > 0 && limit < len(ds.Instances) {
				ds.Instances = ds.Instances[:limit]
			}
			cases := swebench.ConvertDataset(ds)
			if outputPath != "" {
				if err := swebench.WriteEvalDataset(cases, outputPath); err != nil {
					return fmt.Errorf("swe-bench convert: write: %w", err)
				}
				fmt.Fprintf(os.Stderr, "converted %d instances → %s\n", len(cases), outputPath)
				return nil
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(map[string]any{"name": "swe-bench", "version": "1.0", "test_cases": cases})
		},
	}
	cmd.Flags().StringVar(&inputPath, "input", "", "Path to SWE-bench JSON file (required)")
	cmd.Flags().StringVar(&outputPath, "output", "", "Output Golden Dataset JSON path (default: stdout)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Limit to first N instances (0 = all)")
	_ = cmd.MarkFlagRequired("input")
	return cmd
}

func newEvalSWEBenchScoreCmd() *cobra.Command {
	var (
		inputPath    string
		verifyOutput string
		jsonOut      bool
	)
	cmd := &cobra.Command{
		Use:   "score",
		Short: "Score SWE-bench verification output against expected tests",
		RunE: func(cmd *cobra.Command, args []string) error {
			ds, err := swebench.LoadDataset(inputPath)
			if err != nil {
				return fmt.Errorf("swe-bench score: load: %w", err)
			}
			verifyData, err := os.ReadFile(verifyOutput)
			if err != nil {
				return fmt.Errorf("swe-bench score: read verify output: %w", err)
			}
			verifyStr := string(verifyData)
			results := make([]swebench.ScorerResult, 0, len(ds.Instances))
			for i := range ds.Instances {
				results = append(results, swebench.ScoreInstance(&ds.Instances[i], verifyStr))
			}
			summary := swebench.SummarizeResults(results)
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(summary)
			}
			fmt.Printf("SWE-bench Score: %d/%d resolved (%.1f%%), mean score %.3f\n",
				summary.Resolved, summary.Total, summary.ResolveRate*100, summary.MeanScore)
			return nil
		},
	}
	cmd.Flags().StringVar(&inputPath, "input", "", "Path to SWE-bench JSON file (required)")
	cmd.Flags().StringVar(&verifyOutput, "verify-output", "", "Path to verification output file (required)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output summary as JSON")
	_ = cmd.MarkFlagRequired("input")
	_ = cmd.MarkFlagRequired("verify-output")
	return cmd
}

// ── eval swebench (issue #363) ──────────────────────────────────────

func newEvalSwebenchCmd() *cobra.Command {
	var (
		datasetPath string
		outputPath  string
		workspace   string
		maxTurns    int
		timeout     time.Duration
		sinCodeBin  string
		dryRun      bool
	)

	cmd := &cobra.Command{
		Use:   "swebench",
		Short: "Run SWE-bench evaluation harness (issue #363)",
		Long: `Run sin-code against a SWE-bench dataset and evaluate the results.

SWE-bench measures an agent's ability to fix real GitHub issues. This harness:
  - Loads SWE-bench JSON instances
  - Runs sin-code against each issue
  - Applies the predicted patch
  - Evaluates with test_patch
  - Records pass/fail

Examples:

  sin-code eval swebench --dataset swebench.json --output results.json
  sin-code eval swebench --dataset swebench.json --dry-run
  sin-code eval swebench --dataset swebench.json --workspace /tmp/swe --max-turns 200`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := eval.SweConfig{
				DatasetPath: datasetPath,
				OutputPath:  outputPath,
				Workspace:   workspace,
				MaxTurns:    maxTurns,
				Timeout:     timeout,
				SinCodeBin:  sinCodeBin,
				DryRun:      dryRun,
			}
			report, err := eval.RunSweBench(cmd.Context(), cfg)
			if err != nil {
				return fmt.Errorf("swebench: %w", err)
			}
			eval.SwePrintSummary(os.Stdout, report)
			return nil
		},
	}

	cmd.Flags().StringVarP(&datasetPath, "dataset", "d", "", "Path to SWE-bench JSON dataset (required)")
	cmd.Flags().StringVarP(&outputPath, "output", "o", "swebench-results.json", "Output results JSON path")
	cmd.Flags().StringVarP(&workspace, "workspace", "w", "", "Workspace directory for repo clones")
	cmd.Flags().IntVar(&maxTurns, "max-turns", 100, "Max agent turns per instance")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "Timeout per instance")
	cmd.Flags().StringVar(&sinCodeBin, "sin-code-bin", "", "Path to sin-code binary (default: auto-detect)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate dataset without running agents")

	_ = cmd.MarkFlagRequired("dataset")
	return cmd
}
