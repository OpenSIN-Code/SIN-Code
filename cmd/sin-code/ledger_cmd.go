// SPDX-License-Identifier: MIT
// Code extracted from commands.go — Ledger section.

package main

// sin-debt: shrink, upgrade: when a second ledger-related command is added, merge into a shared file

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hub"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
)

// ============================================================================
// Ledger command (sin-code ledger)
// ============================================================================

// NewLedgerCmd builds the `ledger` cobra subcommand.
func NewLedgerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ledger",
		Short: "Query the semantic session ledger",
		Long: `sin-code ledger reads the append-only session ledger that records
prompts, tool calls, verification results, and completions. Use it to audit
what the agent did in a session or to list recent sessions.`,
	}
	cmd.AddCommand(newLedgerListCmd())
	cmd.AddCommand(newLedgerShowCmd())
	cmd.AddCommand(newLedgerToolsCmd())
	return cmd
}

func ledgerStore() (*ledger.Store, error) {
	path := ledger.DefaultPath()
	if env := os.Getenv("SIN_CODE_LEDGER"); env != "" {
		path = env
	}
	return ledger.Open(path)
}

func newLedgerListCmd() *cobra.Command {
	var limit int
	c := &cobra.Command{
		Use:   "list",
		Short: "List recent sessions with ledger entries",
		RunE: func(_ *cobra.Command, _ []string) error {
			store, err := ledgerStore()
			if err != nil {
				return err
			}
			defer store.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			sessions, err := store.Sessions(ctx, limit)
			if err != nil {
				return err
			}
			if len(sessions) == 0 {
				fmt.Println("No sessions recorded.")
				return nil
			}
			for _, sid := range sessions {
				fmt.Println(sid)
			}
			return nil
		},
	}
	c.Flags().IntVarP(&limit, "limit", "n", 50, "Max sessions to show")
	return c
}

func newLedgerShowCmd() *cobra.Command {
	var limit int
	c := &cobra.Command{
		Use:   "show <session-id>",
		Short: "Show ledger entries for a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			store, err := ledgerStore()
			if err != nil {
				return err
			}
			defer store.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			entries, err := store.List(ctx, args[0], limit)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Println("No ledger entries for this session.")
				return nil
			}
			for _, e := range entries {
				fmt.Printf("%s  %-16s  %s\n", e.CreatedAt.Format(time.RFC3339), e.Type, e.Summary)
			}
			return nil
		},
	}
	c.Flags().IntVarP(&limit, "limit", "n", 100, "Max entries to show")
	return c
}

func newLedgerToolsCmd() *cobra.Command {
	var (
		heatmapFlag  bool
		coverageFlag bool
		unusedFlag   bool
		familyFlag   bool
		jsonFlag     bool
		sinceStr     string
		untilStr     string
	)
	c := &cobra.Command{
		Use:   "tools",
		Short: "Tool usage heatmap, coverage, and unused-tool report",
		Long: `sin-code ledger tools reads the tool_usage table populated by the
agent loop and reports per-tool counts, coverage against the known tool set,
and never-used tools. Run it after the agent has executed sessions to see
which tools are hot and which are gaps.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			store, err := ledgerStore()
			if err != nil {
				return err
			}
			defer store.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			var since, until time.Time
			if sinceStr != "" {
				since, err = time.Parse(time.RFC3339, sinceStr)
				if err != nil {
					return fmt.Errorf("invalid --since: %w", err)
				}
			}
			if untilStr != "" {
				until, err = time.Parse(time.RFC3339, untilStr)
				if err != nil {
					return fmt.Errorf("invalid --until: %w", err)
				}
			}

			known := knownToolNames()

			// Default to heatmap when no specific report is requested.
			if !coverageFlag && !unusedFlag && !familyFlag {
				heatmapFlag = true
			}

			if coverageFlag {
				res, err := store.ToolCoverage(ctx, known)
				if err != nil {
					return err
				}
				if jsonFlag {
					enc := json.NewEncoder(cmd.OutOrStdout())
					enc.SetIndent("", "  ")
					return enc.Encode(res)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Coverage: %.1f%% (%d/%d tools used)\n",
					res.Coverage*100, len(res.Used), res.Total)
				return nil
			}

			if unusedFlag {
				unused, err := store.UnusedTools(ctx, known)
				if err != nil {
					return err
				}
				if jsonFlag {
					enc := json.NewEncoder(cmd.OutOrStdout())
					enc.SetIndent("", "  ")
					return enc.Encode(map[string]any{"unused": unused, "total_known": len(known)})
				}
				if len(unused) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "All known tools have been used.")
					return nil
				}
				for _, name := range unused {
					fmt.Fprintln(cmd.OutOrStdout(), name)
				}
				return nil
			}

			if familyFlag {
				counts, err := store.FamilyUsageCounts(ctx, since, until)
				if err != nil {
					return err
				}
				if jsonFlag {
					enc := json.NewEncoder(cmd.OutOrStdout())
					enc.SetIndent("", "  ")
					return enc.Encode(counts)
				}
				return formatFamilyHeatmap(cmd.OutOrStdout(), counts)
			}

			counts, err := store.ToolUsageCounts(ctx, since, until)
			if err != nil {
				return err
			}
			if jsonFlag {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(counts)
			}
			return formatToolHeatmap(cmd.OutOrStdout(), counts)
		},
	}
	c.Flags().BoolVar(&heatmapFlag, "heatmap", true, "Show per-tool usage heatmap")
	c.Flags().BoolVar(&coverageFlag, "coverage", false, "Show coverage score")
	c.Flags().BoolVar(&unusedFlag, "unused", false, "List never-used tools")
	c.Flags().BoolVar(&familyFlag, "family", false, "Show family-level heatmap")
	c.Flags().BoolVar(&jsonFlag, "json", false, "Emit JSON for CI")
	c.Flags().StringVar(&sinceStr, "since", "", "RFC3339 start of window")
	c.Flags().StringVar(&untilStr, "until", "", "RFC3339 end of window")
	return c
}

func formatToolHeatmap(w io.Writer, counts []ledger.UsageCount) error {
	if len(counts) == 0 {
		fmt.Fprintln(w, "No tool usage recorded.")
		return nil
	}
	maxName := 0
	for _, c := range counts {
		if len(c.ToolName) > maxName {
			maxName = len(c.ToolName)
		}
	}
	fmt.Fprintf(w, "%-*s  %-10s  %6s  %s\n", maxName, "tool", "family", "total", "ok/error/denied")
	for _, c := range counts {
		fmt.Fprintf(w, "%-*s  %-10s  %6d  %d/%d/%d\n",
			maxName, c.ToolName, c.Family, c.Total,
			c.ByOutcome[ledger.OutcomeOK],
			c.ByOutcome[ledger.OutcomeError],
			c.ByOutcome[ledger.OutcomeDenied])
	}
	return nil
}

func formatFamilyHeatmap(w io.Writer, counts []ledger.FamilyCount) error {
	if len(counts) == 0 {
		fmt.Fprintln(w, "No tool usage recorded.")
		return nil
	}
	maxName := 0
	for _, c := range counts {
		if len(c.Family) > maxName {
			maxName = len(c.Family)
		}
	}
	fmt.Fprintf(w, "%-*s  %6s  %s\n", maxName, "family", "total", "ok/error/denied")
	for _, c := range counts {
		fmt.Fprintf(w, "%-*s  %6d  %d/%d/%d\n",
			maxName, c.Family, c.Total,
			c.ByOutcome[ledger.OutcomeOK],
			c.ByOutcome[ledger.OutcomeError],
			c.ByOutcome[ledger.OutcomeDenied])
	}
	return nil
}

func knownToolNames() []string {
	seen := map[string]bool{}
	for _, t := range builtinSpecs() {
		seen[t.Name] = true
	}
	for _, t := range hub.AllTools() {
		seen[t.Name] = true
	}
	for _, name := range []string{
		// Representative external MCP tools from the default ecosystem registry.
		"websearch__search",
		"browser__navigate",
		"browser__findings",
		"browser__snapshot",
		"simone__search",
		"simone__symbol",
		"scheduler__schedule_job",
		"scheduler__schedule_list",
		"goalmode__goal_start",
		"goalmode__goal_list",
		"grillme__grill_start",
		"marketplace__marketplace_search",
		"codocs__doc_start",
		"contextbridge__sin_context",
		"honcho__sin_memory_add",
		"frontend__design_component_create",
		"mcpbuilder__mcp_scaffold",
		"symfonylens__symfony_analyze_routes",
	} {
		seen[name] = true
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	return out
}
