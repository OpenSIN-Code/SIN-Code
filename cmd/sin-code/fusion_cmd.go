// SPDX-License-Identifier: MIT
// Purpose: `sin-code fusion` — SIN Fusion v1 status/config subcommand (issue #290).
// Read-only: shows tournament configuration, provider pool, and env var overrides.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/config"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/fusion"
)

func NewFusionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fusion",
		Short: "SIN Fusion v1 verify-tournament status and config",
		Long: `sin-code fusion shows the configuration and provider pool for the
SIN Fusion v1 verify-tournament (issue #290). When the verify-gate (M3) fails,
fusion fans out to N Fireworks models in parallel; first PoC-pass wins.
Oracle mode (issue #344) runs all candidates and uses an LLM judge.

All subcommands are read-only — no side effects, no API calls.`,
	}
	cmd.AddCommand(newFusionStatusCmd())
	cmd.AddCommand(newFusionConfigCmd())
	cmd.AddCommand(newFusionProvidersCmd())
	return cmd
}

func newFusionStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show fusion enabled/disabled status and gate mode",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, _ := config.LoadMergedConfig()
			fmt.Println("SIN Fusion v1 — Status")
			fmt.Println(strings.Repeat("─", 40))
			fmt.Printf("  Enabled:          %v\n", cfg.FusionEnabled)
			fmt.Printf("  Oracle mode:      %v\n", cfg.FusionOracleMode)
			fmt.Printf("  Difficulty gate:  %v\n", cfg.FusionDifficultyGate)
			fmt.Printf("  Max cost (USD):   %.2f\n", cfg.FusionMaxCostUSD)
			fmt.Printf("  Min quorum:       %d\n", cfg.FusionMinQuorum)
			fmt.Printf("  Per-provider TO:  %ds\n", cfg.FusionPerProviderTimeoutS)
			provStr := ""
			if len(cfg.FusionProviders) > 0 {
				provStr = strings.Join(cfg.FusionProviders, ",")
			}
			providers := fusion.LoadFireworksPool(nil, provStr)
			fmt.Printf("  Providers loaded: %d\n", len(providers))
			if evalModel := os.Getenv("SIN_EVALUATOR_MODEL"); evalModel != "" {
				fmt.Printf("  Evaluator model:  %s (SIN_EVALUATOR_MODEL)\n", evalModel)
			} else {
				fmt.Println("  Evaluator model:  (worker model fallback)")
			}
			return nil
		},
	}
}

func newFusionConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show full fusion configuration including env var overrides",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, _ := config.LoadMergedConfig()
			fmt.Println("SIN Fusion v1 — Configuration")
			fmt.Println(strings.Repeat("─", 40))
			fmt.Println("  Config keys (sin-code.toml):")
			fmt.Printf("    fusion.enabled:                %v\n", cfg.FusionEnabled)
			fmt.Printf("    fusion.oracle_mode:            %v\n", cfg.FusionOracleMode)
			fmt.Printf("    fusion.difficulty_gate:        %v\n", cfg.FusionDifficultyGate)
			fmt.Printf("    fusion.max_cost_usd:           %.2f\n", cfg.FusionMaxCostUSD)
			fmt.Printf("    fusion.min_quorum:             %d\n", cfg.FusionMinQuorum)
			fmt.Printf("    fusion.per_provider_timeout_s: %d\n", cfg.FusionPerProviderTimeoutS)
			if len(cfg.FusionProviders) > 0 {
				fmt.Printf("    fusion.providers:              %s\n", strings.Join(cfg.FusionProviders, ", "))
			} else {
				fmt.Println("    fusion.providers:              (default 6-model pool)")
			}
			fmt.Println()
			fmt.Println("  Environment overrides:")
			printEnvVar("SIN_EVALUATOR_MODEL", "")
			printEnvVar("SIN_EVALUATOR_BASE_URL", "")
			printEnvVar("SIN_EVALUATOR_API_KEY", "(masked)")
			return nil
		},
	}
}

func newFusionProvidersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "providers",
		Short: "List the Fireworks pool providers (model, base URL, max tokens)",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, _ := config.LoadMergedConfig()
			provStr := ""
			if len(cfg.FusionProviders) > 0 {
				provStr = strings.Join(cfg.FusionProviders, ",")
			}
			providers := fusion.LoadFireworksPool(nil, provStr)
			fmt.Println("SIN Fusion v1 — Provider Pool")
			fmt.Println(strings.Repeat("─", 40))
			if len(providers) == 0 {
				fmt.Println("  No providers loaded (check fusion.providers config or FIREWORKS_API_KEY)")
				return nil
			}
			fmt.Printf("  %-30s %-40s %s\n", "MODEL", "BASE URL", "MAX TOKENS")
			fmt.Printf("  %s %s %s\n", strings.Repeat("─", 30), strings.Repeat("─", 40), strings.Repeat("─", 10))
			for _, p := range providers {
				fmt.Printf("  %-30s %-40s %d\n", p.Model, p.BaseURL, p.MaxTokens)
			}
			fmt.Printf("\n  Total: %d providers\n", len(providers))
			return nil
		},
	}
}

func printEnvVar(key, mask string) {
	val := os.Getenv(key)
	if val == "" {
		fmt.Printf("    %s: (not set)\n", key)
	} else if mask != "" {
		fmt.Printf("    %s: %s\n", key, mask)
	} else {
		fmt.Printf("    %s: %s\n", key, val)
	}
}
