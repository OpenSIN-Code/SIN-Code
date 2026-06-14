// SPDX-License-Identifier: MIT
// Purpose: eval command - Run evaluation suite against golden datasets
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/dataset"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/eval"
)

var evalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Run evaluation suite against golden datasets",
	Long: `Run evaluation suite against golden datasets using LLM-as-a-Judge.
	
The eval command executes predefined test cases from golden datasets and evaluates
agent behavior automatically, providing metrics and regression protection.`,
	RunE: runEval,
}

var (
	evalDatasetPath   string
	evalOutputPath    string
	evalHeadlessMode  bool
	evalTimeoutPerCase int
)

func init() {
	evalCmd.Flags().StringVar(&evalDatasetPath, "dataset", "evals/critical.json", 
		"Path to the golden dataset JSON file")
	evalCmd.Flags().StringVar(&evalOutputPath, "output", "evals/results.json", 
		"Path to save evaluation results")
	evalCmd.Flags().BoolVar(&evalHeadlessMode, "headless", false, 
		"Run in headless mode (no interactive prompts)")
	evalCmd.Flags().IntVar(&evalTimeoutPerCase, "timeout", 300, 
		"Timeout per test case in seconds")
	
	rootCmd.AddCommand(evalCmd)
}

func runEval(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Load dataset
	fmt.Printf("Loading dataset from: %s\n", evalDatasetPath)
	ds, err := dataset.LoadDataset(evalDatasetPath)
	if err != nil {
		return fmt.Errorf("failed to load dataset: %w", err)
	}

	fmt.Printf("Loaded dataset: %s (v%s)\n", ds.Name, ds.Version)
	fmt.Printf("Description: %s\n", ds.Description)
	fmt.Printf("Test cases: %d\n\n", len(ds.TestCases))

	// Create runner
	config := dataset.RunnerConfig{
		HeadlessMode:   evalHeadlessMode,
		TimeoutPerCase: time.Duration(evalTimeoutPerCase) * time.Second,
	}
	runner := dataset.NewRunner(config)

	// Run evaluation
	results, err := runner.Run(ctx, ds)
	if err != nil {
		return fmt.Errorf("evaluation failed: %w", err)
	}

	// Save results
	outputDir := filepath.Dir(evalOutputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	if err := runner.SaveResults(evalOutputPath); err != nil {
		return fmt.Errorf("failed to save results: %w", err)
	}

	fmt.Printf("\nResults saved to: %s\n", evalOutputPath)

	// Calculate and display metrics
	metricsResults := make([]interface{}, len(results))
	for i, r := range results {
		metricsResults[i] = r
	}

	report := eval.CalculateMetrics(ds.Name, metricsResults)
	report.PrintSummary()

	// Save metrics report
	metricsPath := filepath.Join(outputDir, "metrics.json")
	if err := report.SaveReport(metricsPath); err != nil {
		return fmt.Errorf("failed to save metrics: %w", err)
	}

	fmt.Printf("Metrics saved to: %s\n", metricsPath)

	return nil
}
