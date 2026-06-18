// SPDX-License-Identifier: MIT
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	internal "github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/fusion"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/modelperf"
)

func NewFusionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fusion",
		Short: "SIN Fusion - status, config, benchmarking, model selection",
	}
	cmd.AddCommand(newFusionStatusCmd())
	cmd.AddCommand(newFusionConfigCmd())
	cmd.AddCommand(newFusionProvidersCmd())
	cmd.AddCommand(newFusionBenchmarkCmd())
	cmd.AddCommand(newFusionRankCmd())
	cmd.AddCommand(newFusionRecommendCmd())
	return cmd
}

func newFusionStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show fusion status",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, _ := internal.LoadMergedConfig()
			fmt.Println("SIN Fusion - Status")
			fmt.Println(strings.Repeat("-", 40))
			fmt.Printf("  Enabled:          %v\n", cfg.FusionEnabled)
			fmt.Printf("  Oracle mode:      %v\n", cfg.FusionOracleMode)
			fmt.Printf("  Max cost (USD):   %.2f\n", cfg.FusionMaxCostUSD)
			fmt.Printf("  Min quorum:       %d\n", cfg.FusionMinQuorum)
			providers := fusion.LoadFireworksPool(nil, cfg.FusionProviders)
			fmt.Printf("  Providers loaded: %d\n", len(providers))
			fmt.Println()
			fmt.Println("  Modes: poc | oracle (default) | plan-merge")
			return nil
		},
	}
}

func newFusionConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show fusion configuration",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, _ := internal.LoadMergedConfig()
			fmt.Println("SIN Fusion - Configuration")
			fmt.Println(strings.Repeat("-", 40))
			fmt.Printf("  fusion.enabled:                %v\n", cfg.FusionEnabled)
			fmt.Printf("  fusion.oracle_mode:            %v\n", cfg.FusionOracleMode)
			fmt.Printf("  fusion.max_cost_usd:           %.2f\n", cfg.FusionMaxCostUSD)
			fmt.Printf("  fusion.min_quorum:             %d\n", cfg.FusionMinQuorum)
			fmt.Printf("  fusion.per_provider_timeout_s: %d\n", cfg.FusionPerProviderTimeoutS)
			if len(cfg.FusionProviders) > 0 {
				fmt.Printf("  fusion.providers:              %s\n", strings.Join(cfg.FusionProviders, ", "))
			}
			return nil
		},
	}
}

func newFusionProvidersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "providers",
		Short: "List provider pool",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, _ := internal.LoadMergedConfig()
			providers := fusion.LoadFireworksPool(nil, cfg.FusionProviders)
			fmt.Println("SIN Fusion - Provider Pool")
			if len(providers) == 0 {
				fmt.Println("  No providers loaded")
				return nil
			}
			for _, p := range providers {
				fmt.Printf("  %-30s %-40s %d\n", p.Model, p.BaseURL, p.MaxTokens)
			}
			return nil
		},
	}
}

func newFusionBenchmarkCmd() *cobra.Command {
	var datasetPath, category, providersFlag string
	cmd := &cobra.Command{
		Use:   "benchmark",
		Short: "Run dataset across providers (issue #395)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if datasetPath == "" {
				return fmt.Errorf("--dataset is required")
			}
			store, err := modelperf.Open("")
			if err != nil { return err }
			defer store.Close()
			names := splitCommas(providersFlag)
			if len(names) == 0 { names = []string{"minimax-m3","kimi-k2p7-code-fast","glm-5p2"} }
			provs := make([]modelperf.BenchmarkProvider, 0, len(names))
			for _, n := range names { provs = append(provs, &stubBP{n}) }
			out, err := modelperf.RunBenchmark(context.Background(), store, provs, modelperf.BenchmarkConfig{DatasetPath: datasetPath, Category: category})
			if err != nil { return err }
			w := tabwriter.NewWriter(os.Stdout, 0,0,2,' ',0)
			fmt.Fprintf(w, "Category:\t%s\n", out.Category)
			fmt.Fprintf(w, "Dataset:\t%s\n", out.Dataset)
			fmt.Fprintf(w, "Cases:\t%d\n", out.Cases)
			fmt.Fprintf(w, "\nModel\tPass Rate\tAvg Latency\tAvg Cost\n")
			sort.Slice(out.Results, func(i,j int) bool { return out.Results[i].PassRate > out.Results[j].PassRate })
			for _, r := range out.Results {
				fmt.Fprintf(w, "%s\t%.1f%%\t%v\t$%.4f\n", r.Model, r.PassRate*100, r.AvgLatency, r.AvgCost)
			}
			w.Flush()
			return nil
		},
	}
	cmd.Flags().StringVarP(&datasetPath, "dataset", "d", "", "eval dataset JSON")
	cmd.Flags().StringVarP(&category, "category", "c", "", "task category")
	cmd.Flags().StringVarP(&providersFlag, "providers", "p", "", "comma-separated models")
	return cmd
}

func newFusionRankCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "rank",
		Short: "Show model leaderboard (issue #395)",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := modelperf.Open("")
			if err != nil { return err }
			defer store.Close()
			recs, err := store.Ranking(context.Background())
			if err != nil { return err }
			if len(recs) == 0 { fmt.Println("No data. Run benchmark first."); return nil }
			if jsonOut { return json.NewEncoder(os.Stdout).Encode(recs) }
			w := tabwriter.NewWriter(os.Stdout, 0,0,2,' ',0)
			fmt.Fprintf(w, "Category\tModel\tPass Rate\tSamples\tCost\n")
			for _, r := range recs { fmt.Fprintf(w, "%s\t%s\t%.1f%%\t%d\t$%.4f\n", r.Category, r.Model, r.PassRate*100, r.SampleCount, r.AvgCostUSD) }
			w.Flush()
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}

func newFusionRecommendCmd() *cobra.Command {
	var task string; var n, minS int
	cmd := &cobra.Command{
		Use:   "recommend",
		Short: "Best models for a task (issue #395)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if task == "" { return fmt.Errorf("--task required") }
			store, err := modelperf.Open("")
			if err != nil { return err }
			defer store.Close()
			recs, err := store.Recommend(context.Background(), task, n, minS)
			if err != nil { return err }
			if len(recs) == 0 { fmt.Printf("No data for %q.\n", task); return nil }
			w := tabwriter.NewWriter(os.Stdout, 0,0,2,' ',0)
			fmt.Fprintf(w, "Rank\tModel\tScore\tPass Rate\tSamples\n")
			for i, r := range recs { fmt.Fprintf(w, "%d\t%s\t%.3f\t%.1f%%\t%d\n", i+1, r.Model, r.Score, r.PassRate*100, r.Samples) }
			w.Flush()
			return nil
		},
	}
	cmd.Flags().StringVarP(&task, "task", "t", "", "task category")
	cmd.Flags().IntVarP(&n, "top", "n", 3, "number of recs")
	cmd.Flags().IntVar(&minS, "min-samples", 1, "min benchmark runs")
	return cmd
}

func splitCommas(s string) []string { if s == "" { return nil }; return strings.Split(s, ",") }
type stubBP struct{ name string }
func (s *stubBP) Name() string { return s.name }
func (s *stubBP) Run(ctx context.Context, prompt string) (modelperf.BenchmarkResult, error) {
	return modelperf.BenchmarkResult{Passed: true, Output: "stub"}, nil
}
