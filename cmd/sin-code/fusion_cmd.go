// SPDX-License-Identifier: MIT
// Purpose: `sin-code fusion` — read-only SIN Fusion v1 tournament status
// and configuration viewer (issue #290). No side effects, no API calls.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/fusion"
)

// NewFusionCmd builds the `fusion` cobra subcommand.
// v3.22.0 — SIN Fusion v1 status/config (issue #290)
func NewFusionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fusion",
		Short: "SIN Fusion v1 tournament status and configuration",
		Long: `sin-code fusion is a read-only viewer for the SIN Fusion v1
verify-tournament system (issue #290). It shows whether fusion is enabled,
the current configuration, and the available Fireworks provider pool.
No side effects, no API calls.`,
	}
	cmd.AddCommand(newFusionStatusCmd())
	cmd.AddCommand(newFusionConfigCmd())
	cmd.AddCommand(newFusionProvidersCmd())
	return cmd
}

func newFusionStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether fusion is enabled and a config summary",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := internal.LoadMergedConfig()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			providers := fusion.LoadFireworksPool(nil, cfg.FusionProviders)
			gateMode := cfg.AgentVerifyMode
			tournMode := fusion.ModePoC
			if cfg.FusionOracleMode {
				tournMode = fusion.ModeOracle
			}
			wouldWire := cfg.FusionEnabled &&
				(gateMode == "poc" || gateMode == "oracle") &&
				len(providers) >= 2

			w := cmd.OutOrStdout()
			fmt.Fprintln(w, "SIN Fusion v1 — Tournament Status")
			fmt.Fprintln(w, strings.Repeat("=", 50))
			fmt.Fprintf(w, "Enabled:              %v\n", cfg.FusionEnabled)
			fmt.Fprintf(w, "Verify gate mode:     %s\n", gateMode)
			fmt.Fprintf(w, "Tournament mode:      %s\n", tournMode)
			fmt.Fprintf(w, "Difficulty gate:      %v\n", cfg.FusionDifficultyGate)
			fmt.Fprintf(w, "Oracle mode:          %v\n", cfg.FusionOracleMode)
			fmt.Fprintf(w, "Providers:            %d (min quorum: %d)\n", len(providers), cfg.FusionMinQuorum)
			fmt.Fprintf(w, "Max cost USD:         %.2f\n", cfg.FusionMaxCostUSD)
			fmt.Fprintf(w, "Per-provider timeout: %ds\n", cfg.FusionPerProviderTimeoutS)
			fmt.Fprintln(w)
			fmt.Fprintf(w, "Tournament would wire: ")
			if wouldWire {
				fmt.Fprintf(w, "yes (enabled, %s mode, %d providers >= 2 quorum)\n", tournMode, len(providers))
			} else {
				var reasons []string
				if !cfg.FusionEnabled {
					reasons = append(reasons, "fusion.enabled = false")
				}
				if gateMode != "poc" && gateMode != "oracle" {
					reasons = append(reasons, fmt.Sprintf("gate mode is %q (need poc or oracle)", gateMode))
				}
				if len(providers) < 2 {
					reasons = append(reasons, fmt.Sprintf("only %d provider(s) available (need >= 2)", len(providers)))
				}
				if len(reasons) == 0 {
					reasons = append(reasons, "unknown")
				}
				fmt.Fprintf(w, "no (%s)\n", strings.Join(reasons, "; "))
			}
			return nil
		},
	}
}

func newFusionConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show the full fusion configuration (config + env + defaults)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := internal.LoadMergedConfig()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintln(w, "SIN Fusion v1 — Configuration")
			fmt.Fprintln(w, strings.Repeat("=", 50))
			fmt.Fprintf(w, "%-30s = %v\n", "fusion.enabled", cfg.FusionEnabled)
			fmt.Fprintf(w, "%-30s = %s\n", "fusion.providers", strings.Join(cfg.FusionProviders, ","))
			fmt.Fprintf(w, "%-30s = %.2f\n", "fusion.max_cost_usd", cfg.FusionMaxCostUSD)
			fmt.Fprintf(w, "%-30s = %d\n", "fusion.min_quorum", cfg.FusionMinQuorum)
			fmt.Fprintf(w, "%-30s = %d\n", "fusion.per_provider_timeout_s", cfg.FusionPerProviderTimeoutS)
			fmt.Fprintf(w, "%-30s = %v\n", "fusion.difficulty_gate", cfg.FusionDifficultyGate)
			fmt.Fprintf(w, "%-30s = %v\n", "fusion.oracle_mode", cfg.FusionOracleMode)
			fmt.Fprintf(w, "%-30s = %s\n", "agent.verify_mode", cfg.AgentVerifyMode)
			fmt.Fprintln(w)
			fmt.Fprintln(w, "Environment overrides:")
			printEnvVar(w, "SIN_EVALUATOR_MODEL", os.Getenv("SIN_EVALUATOR_MODEL"))
			printEnvVar(w, "SIN_EVALUATOR_BASE_URL", os.Getenv("SIN_EVALUATOR_BASE_URL"))
			printEnvVar(w, "SIN_EVALUATOR_API_KEY", maskFusionKey(os.Getenv("SIN_EVALUATOR_API_KEY")))
			printEnvVar(w, "FIREWORKS_API_KEY", maskFusionKey(os.Getenv("FIREWORKS_API_KEY")))
			printEnvVar(w, "FIREWORKS_BASE_URL", os.Getenv("FIREWORKS_BASE_URL"))
			return nil
		},
	}
}

func newFusionProvidersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "providers",
		Short: "List the Fireworks pool providers (model, base URL, tokens, price)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := internal.LoadMergedConfig()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			providers := fusion.LoadFireworksPool(nil, cfg.FusionProviders)
			w := cmd.OutOrStdout()
			fmt.Fprintln(w, "SIN Fusion v1 — Fireworks Provider Pool")
			fmt.Fprintln(w, strings.Repeat("=", 80))
			if len(providers) == 0 {
				fmt.Fprintln(w, "No providers configured.")
				return nil
			}
			baseURL := providers[0].BaseURL
			fmt.Fprintf(w, "Base URL: %s\n", baseURL)
			fmt.Fprintln(w)
			fmt.Fprintf(w, "%-22s  %-48s  %8s  %6s  %7s  %8s  %9s\n",
				"Name", "Model Slug", "MaxTok", "Vision", "Think", "In$/1M", "Out$/1M")
			fmt.Fprintln(w, strings.Repeat("-", 118))
			for _, p := range providers {
				vision := "no"
				if p.Vision {
					vision = "yes"
				}
				thinking := "no"
				if p.Thinking {
					thinking = "yes"
				}
				fmt.Fprintf(w, "%-22s  %-48s  %8d  %6s  %7s  %8.2f  %9.2f\n",
					p.Name, p.Model, p.MaxTokens, vision, thinking, p.InputPer1M, p.OutputPer1M)
			}
			fmt.Fprintln(w)
			fmt.Fprintf(w, "Total: %d provider(s)\n", len(providers))
			return nil
		},
	}
}

func printEnvVar(w io.Writer, name, value string) {
	if value == "" {
		fmt.Fprintf(w, "  %-26s = (not set)\n", name)
		return
	}
	fmt.Fprintf(w, "  %-26s = %s\n", name, value)
}

func maskFusionKey(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + "..." + s[len(s)-4:]
}
