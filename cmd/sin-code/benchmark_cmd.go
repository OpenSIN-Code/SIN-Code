// SPDX-License-Identifier: MIT
// Purpose: sin-code benchmark — run eval golden datasets and produce a
// scoring report. Thin wrapper around the existing internal/dataset +
// internal/eval infrastructure (issue #75). Supports text, JSON, and
// markdown output formats for CI integration.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/dataset"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/style"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

// NewBenchmarkCmd returns the `sin-code benchmark` cobra command.
func NewBenchmarkCmd() *cobra.Command {
	var (
		model       string
		minPassRate float64
		timeout     time.Duration
		format      string
		dryRun      bool
	)

	cmd := &cobra.Command{
		Use:   "benchmark [datasets...]",
		Short: "Run eval golden datasets and produce a scoring report",
		Long: `sin-code benchmark runs one or more eval golden datasets and
produces a benchmark report with per-dataset metrics.

If no dataset paths are given, all *.json files in the evals/ directory
are discovered and run.

Examples:

  sin-code benchmark evals/critical.json evals/test-generation.json
  sin-code benchmark --format json --min-pass-rate 0.9
  sin-code benchmark --dry-run
  sin-code benchmark --model glm-5p2 --timeout 10m`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			paths, err := resolveDatasetPaths(args)
			if err != nil {
				return err
			}
			if len(paths) == 0 {
				return fmt.Errorf("benchmark: no datasets found (pass paths or create evals/*.json)")
			}

			sort.Strings(paths)

			if dryRun {
				return dryRunDatasets(paths)
			}

			useModel := model != ""
			report, err := runBenchmark(ctx, paths, useModel, timeout)
			if err != nil {
				return err
			}
			report.MinPassRate = minPassRate

			switch format {
			case "json":
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(report); err != nil {
					return fmt.Errorf("benchmark: write json: %w", err)
				}
			case "markdown":
				fmt.Print(formatBenchmarkMarkdown(report))
			default:
				fmt.Print(formatText(report))
			}

			return checkThreshold(report, minPassRate)
		},
	}

	cmd.Flags().StringVar(&model, "model", "", "Model to use for eval (requires LLM config). Empty = offline stub.")
	cmd.Flags().Float64Var(&minPassRate, "min-pass-rate", 0.8, "Minimum pass rate (0.0-1.0). Exit 1 if any dataset falls below.")
	cmd.Flags().DurationVar(&timeout, "timeout", 300*time.Second, "Timeout per test case")
	cmd.Flags().StringVar(&format, "format", "text", "Output format: text, json, or markdown")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate datasets without running them")

	return cmd
}

// resolveDatasetPaths returns explicit paths if args is non-empty,
// otherwise discovers all *.json files under evals/.
func resolveDatasetPaths(args []string) ([]string, error) {
	if len(args) > 0 {
		out := make([]string, 0, len(args))
		for _, a := range args {
			abs, err := filepath.Abs(a)
			if err != nil {
				return nil, fmt.Errorf("benchmark: resolve %q: %w", a, err)
			}
			if _, err := os.Stat(abs); err != nil {
				return nil, fmt.Errorf("benchmark: stat %q: %w", a, err)
			}
			out = append(out, abs)
		}
		return out, nil
	}
	files, err := dataset.ListDatasets("evals")
	if err != nil {
		return nil, fmt.Errorf("benchmark: list evals: %w", err)
	}
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, filepath.Join("evals", f))
	}
	return out, nil
}

// dryRunDatasets loads and validates each dataset without running it.
func dryRunDatasets(paths []string) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Dataset\tCases\tStatus")
	hadErr := false
	for _, p := range paths {
		ds, err := dataset.LoadDataset(p)
		if err != nil {
			fmt.Fprintf(w, "%s\t-\tERROR: %v\n", filepath.Base(p), err)
			hadErr = true
			continue
		}
		skipped := countModelRequired(ds)
		fmt.Fprintf(w, "%s\t%d\tOK", filepath.Base(p), len(ds.TestCases))
		if skipped > 0 {
			fmt.Fprintf(w, " (%d model-required cases will be skipped)", skipped)
		}
		fmt.Fprintln(w)
	}
	w.Flush()
	if hadErr {
		return fmt.Errorf("benchmark: one or more datasets failed validation")
	}
	return nil
}

// countModelRequired returns the number of test cases whose scorer
// requires a real model (ScorerConfig.RequiresModel == true).
func countModelRequired(ds *dataset.Dataset) int {
	n := 0
	for i := range ds.TestCases {
		if ds.TestCases[i].Scorer.RequiresModel {
			n++
		}
	}
	return n
}

// BenchmarkReport is the structured output for `sin-code benchmark --format json`.
type BenchmarkReport struct {
	Datasets     []DatasetResult `json:"datasets"`
	MinPassRate  float64         `json:"min_pass_rate"`
	StartedAt    time.Time       `json:"started_at"`
	FinishedAt   time.Time       `json:"finished_at"`
	OverallRate  float64         `json:"overall_pass_rate"`
	TotalCases   int             `json:"total_cases"`
	TotalPassed  int             `json:"total_passed"`
	TotalSkipped int             `json:"total_skipped"`
}

// DatasetResult holds per-dataset metrics.
type DatasetResult struct {
	Path         string  `json:"path"`
	Name         string  `json:"name"`
	Version      string  `json:"version"`
	Cases        int     `json:"cases"`
	Passed       int     `json:"passed"`
	Failed       int     `json:"failed"`
	Skipped      int     `json:"skipped"`
	PassRate     float64 `json:"pass_rate"`
	MedianLatMS  int64   `json:"median_latency_ms"`
	MedianTokens int64   `json:"median_tokens"`
	MedianLOC    int64   `json:"median_loc"`
	Error        string  `json:"error,omitempty"`
}

// runBenchmark executes each dataset through the eval runner and
// collects metrics. When useModel is false, the offline stub is used
// (same as `eval run` without --use-model).
func runBenchmark(ctx context.Context, paths []string, useModel bool, perCaseTimeout time.Duration) (*BenchmarkReport, error) {
	report := &BenchmarkReport{
		StartedAt: time.Now().UTC(),
	}
	for _, p := range paths {
		result := runOneDataset(ctx, p, useModel, perCaseTimeout)
		report.Datasets = append(report.Datasets, result)
		report.TotalCases += result.Cases
		report.TotalPassed += result.Passed
		report.TotalSkipped += result.Skipped
	}
	report.FinishedAt = time.Now().UTC()
	if report.TotalCases > 0 {
		report.OverallRate = float64(report.TotalPassed) / float64(report.TotalCases)
	} else {
		report.OverallRate = 1.0
	}
	return report, nil
}

// runOneDataset loads, runs, and scores a single dataset file.
func runOneDataset(ctx context.Context, path string, useModel bool, perCaseTimeout time.Duration) DatasetResult {
	ds, err := dataset.LoadDataset(path)
	if err != nil {
		return DatasetResult{Path: path, Error: err.Error()}
	}

	store, err := session.Open(filepath.Join(os.TempDir(), "sin-code-benchmark-sessions.db"))
	if err != nil {
		return DatasetResult{Path: path, Name: ds.Name, Version: ds.Version, Error: fmt.Sprintf("open session store: %v", err)}
	}
	defer store.Close()

	gate := verify.NewGate("off", nil, nil)

	loop := &agentloop.Loop{
		Gate:         gate,
		Workspace:    workspaceRoot(path),
		MaxTurns:     80,
		SystemPrompt: style.RenderSystemPrompt("default"),
		Hooks:        &hooks.Engine{},
	}
	if useModel {
		completion, merr := buildEvalCompletion()
		if merr != nil {
			return DatasetResult{Path: path, Name: ds.Name, Version: ds.Version, Error: fmt.Sprintf("model config: %v", merr)}
		}
		loop.Completion = completion
	} else {
		loop.RunOverride = stubRunOverride
	}

	runner, err := dataset.NewRunner(dataset.RunnerConfig{
		HeadlessMode:   true,
		VerifyMode:     "off",
		TimeoutPerCase: perCaseTimeout,
		MaxConcurrency: 1,
		UseModel:       useModel,
	}, loop, store)
	if err != nil {
		return DatasetResult{Path: path, Name: ds.Name, Version: ds.Version, Error: fmt.Sprintf("new runner: %v", err)}
	}

	results, err := runner.RunDataset(ctx, ds)
	if err != nil {
		return DatasetResult{Path: path, Name: ds.Name, Version: ds.Version, Error: fmt.Sprintf("run dataset: %v", err)}
	}

	return summariseDatasetResult(path, ds, results)
}

// summariseDatasetResult converts RunResults into a DatasetResult with
// computed medians. Cases whose scorer requires a model but the runner
// is in stub mode are counted as "skipped" rather than failed.
func summariseDatasetResult(path string, ds *dataset.Dataset, results []dataset.RunResult) DatasetResult {
	dr := DatasetResult{
		Path:    path,
		Name:    ds.Name,
		Version: ds.Version,
		Cases:   len(ds.TestCases),
	}

	var latencies []int64
	var tokens []int64
	var locs []int64

	for i := range ds.TestCases {
		if i < len(results) {
			r := results[i]
			if r.Error == "" && !r.Success && r.Turns == 0 && r.FinalOutput == "" {
				dr.Skipped++
				continue
			}
			if r.Success {
				dr.Passed++
			} else {
				dr.Failed++
			}
			latencies = append(latencies, r.Duration.Milliseconds())
			tokens = append(tokens, approxTokens(r.FinalOutput))
			locs = append(locs, countLines(r.FinalOutput))
		}
	}

	if dr.Cases > 0 {
		run := dr.Passed + dr.Failed
		if run > 0 {
			dr.PassRate = float64(dr.Passed) / float64(run)
		} else {
			dr.PassRate = 1.0
		}
	}

	dr.MedianLatMS = medianInt64(latencies)
	dr.MedianTokens = medianInt64(tokens)
	dr.MedianLOC = medianInt64(locs)

	return dr
}

// approxTokens estimates token count from output length (~4 chars/token).
func approxTokens(s string) int64 {
	if len(s) == 0 {
		return 0
	}
	return int64(len(s) / 4)
}

// countLines returns the number of newline-separated lines in s.
func countLines(s string) int64 {
	if s == "" {
		return 0
	}
	return int64(strings.Count(s, "\n") + 1)
}

// medianInt64 returns the median of a sorted copy of xs.
func medianInt64(xs []int64) int64 {
	if len(xs) == 0 {
		return 0
	}
	c := append([]int64(nil), xs...)
	sort.Slice(c, func(i, j int) bool { return c[i] < c[j] })
	return c[len(c)/2]
}

// checkThreshold returns an error if any dataset's pass rate is below
// the minimum. The error is non-nil so cobra exits with code 1.
func checkThreshold(report *BenchmarkReport, min float64) error {
	var failed []string
	for _, d := range report.Datasets {
		if d.Error != "" {
			failed = append(failed, fmt.Sprintf("%s: error — %s", filepath.Base(d.Path), d.Error))
			continue
		}
		if d.Cases > 0 && d.PassRate+1e-9 < min {
			failed = append(failed, fmt.Sprintf("%s: pass rate %.1f%% < %.1f%%", filepath.Base(d.Path), d.PassRate*100, min*100))
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("benchmark: %d dataset(s) below threshold:\n  %s", len(failed), strings.Join(failed, "\n  "))
	}
	return nil
}

// formatText renders the report as a human-readable table.
func formatText(report *BenchmarkReport) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "SIN-Code Benchmark Report\n")
	fmt.Fprintf(&sb, "Started: %s\n", report.StartedAt.Format(time.RFC3339))
	fmt.Fprintf(&sb, "Overall Pass Rate: %.1f%% (%d/%d", report.OverallRate*100, report.TotalPassed, report.TotalCases)
	if report.TotalSkipped > 0 {
		fmt.Fprintf(&sb, ", %d skipped", report.TotalSkipped)
	}
	fmt.Fprintf(&sb, ")\n\n")

	w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Dataset\tCases\tPass\tFail\tSkip\tPass%\tMed Lat(ms)\tMed Tok\tMed LOC\tStatus")
	fmt.Fprintln(w, "-------\t-----\t----\t----\t----\t-----\t-----------\t-------\t-------\t------")
	for _, d := range report.Datasets {
		status := "OK"
		if d.Error != "" {
			status = "ERROR"
		} else if d.PassRate+1e-9 < report.MinPassRate {
			status = "BELOW"
		}
		name := d.Name
		if name == "" {
			name = filepath.Base(d.Path)
		}
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\t%.1f%%\t%d\t%d\t%d\t%s\n",
			name, d.Cases, d.Passed, d.Failed, d.Skipped,
			d.PassRate*100, d.MedianLatMS, d.MedianTokens, d.MedianLOC, status)
	}
	w.Flush()
	return sb.String()
}

// formatBenchmarkMarkdown renders the report as a markdown table for CI integration.
func formatBenchmarkMarkdown(report *BenchmarkReport) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## SIN-Code Benchmark Report\n\n")
	fmt.Fprintf(&sb, "**Overall Pass Rate:** %.1f%% (%d/%d", report.OverallRate*100, report.TotalPassed, report.TotalCases)
	if report.TotalSkipped > 0 {
		fmt.Fprintf(&sb, ", %d skipped", report.TotalSkipped)
	}
	fmt.Fprintf(&sb, ")  \n")
	fmt.Fprintf(&sb, "**Min Pass Rate:** %.1f%%  \n", report.MinPassRate*100)
	fmt.Fprintf(&sb, "**Started:** %s  \n\n", report.StartedAt.Format(time.RFC3339))

	fmt.Fprintf(&sb, "| Dataset | Cases | Pass | Fail | Skip | Pass%% | Med Latency (ms) | Med Tokens | Med LOC | Status |\n")
	fmt.Fprintf(&sb, "|---------|-------|------|------|------|-------|-----------------|------------|---------|--------|\n")
	for _, d := range report.Datasets {
		status := "OK"
		if d.Error != "" {
			status = "ERROR"
		} else if d.PassRate+1e-9 < report.MinPassRate {
			status = "BELOW"
		}
		name := d.Name
		if name == "" {
			name = filepath.Base(d.Path)
		}
		fmt.Fprintf(&sb, "| %s | %d | %d | %d | %d | %.1f%% | %d | %d | %d | %s |\n",
			name, d.Cases, d.Passed, d.Failed, d.Skipped,
			d.PassRate*100, d.MedianLatMS, d.MedianTokens, d.MedianLOC, status)
	}
	return sb.String()
}
