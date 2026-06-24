// SPDX-License-Identifier: MIT
// Purpose: `sin-code hub` — tool catalog subcommand. Lists, searches, and
// prints detailed information about every sin-code command and relevant MCP surface.
// Also hosts `sin-code image-graph` — SOTA chart generation (bar, line, pie, area)
// using Apache ECharts (via go-echarts) for professional rendering.
// Docs: hub_cmd.doc.md
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hub"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/imagegraph"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/telemetry"
)

// NewHubCmd builds the `hub` cobra subcommand.
func NewHubCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hub",
		Short: "Tool catalog and landing page for sin-code",
		Long: `sin-code hub is a read-only catalog of all 36+ subcommands and
relevant MCP skill surfaces. Use it to discover commands, search by keyword,
or show detailed info for a specific tool. The catalog is static and mirrors
the command surface; dynamic MCP servers are listed via 'sin-code mcp status'.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Println("SIN-Code Tool Catalog")
			fmt.Println(strings.Repeat("═", 60))
			fmt.Print(hub.FormatCategories(hub.DefaultCatalog()))
			return nil
		},
	}

	cmd.AddCommand(newHubListCmd())
	cmd.AddCommand(newHubSearchCmd())
	cmd.AddCommand(newHubInfoCmd())
	return cmd
}

func newHubListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Flat list of all tools",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Print(hub.FormatList(hub.AllTools()))
			return nil
		},
	}
}

func newHubSearchCmd() *cobra.Command {
	var unused bool
	cmd := &cobra.Command{
		Use:   "search [keyword]",
		Short: "Search tools by name, short, or description",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if unused {
				return runHubUnused(c)
			}
			if len(args) == 0 {
				return fmt.Errorf("search requires a keyword")
			}
			res := hub.Search(args[0])
			if len(res) == 0 {
				fmt.Println("No tools matched.")
				return nil
			}
			fmt.Printf("Matched %d tool(s):\n\n", len(res))
			fmt.Print(hub.FormatList(res))
			return nil
		},
	}
	cmd.Flags().BoolVar(&unused, "unused", false, "list hub tools never used according to telemetry")
	return cmd
}

func runHubUnused(c *cobra.Command) error {
	provider, err := telemetry.DefaultProvider()
	if err != nil {
		return err
	}
	used, err := provider.UsedTools(c.Context())
	if err != nil {
		return err
	}
	var unused []hub.Tool
	for _, t := range hub.AllTools() {
		name := t.Namespace
		if name == "" {
			name = t.Name
		}
		if used[name] == 0 && used[t.Name] == 0 {
			unused = append(unused, t)
		}
	}
	fmt.Printf("%d unused hub tool(s):\n\n", len(unused))
	fmt.Print(hub.FormatList(unused))
	return nil
}

func newHubInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <tool>",
		Short: "Show detailed info for a single tool",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := strings.ToLower(args[0])
			for _, t := range hub.AllTools() {
				if strings.EqualFold(t.Name, name) {
					fmt.Print(hub.FormatDetail(t))
					return nil
				}
			}
			return fmt.Errorf("unknown tool: %q (run 'sin-code hub list')", args[0])
		},
	}
}

func NewImageGraphCmd() *cobra.Command {
	var (
		chartType  string
		dataFile   string
		outputFile string
		title      string
		subtitle   string
		xLabel     string
		yLabel     string
		width      string
		height     string
		inlineJSON string
	)

	cmd := &cobra.Command{
		Use:   "image-graph",
		Short: "Generate SOTA charts (bar, line, pie, area) as HTML + PNG",
		Long: `sin-code image-graph — professional chart generation with Apache ECharts.

Modern dark theme, interactive tooltips, smooth animations.
No AI, no credits. Pure Go + ECharts JS.

Chart types:
  bar      — Bar chart (rounded corners, multi-series)
  line     — Line chart (smooth curves, gradient area)
  pie      — Donut chart (proportions, percentages)
  area     — Area chart (cumulative trends, filled gradients)

Input: JSON file (--data) or inline JSON (--json) or stdin.
Output: HTML (opens in browser) + PNG (via headless Chrome if available).

JSON format (bar/line/area):
  {
    "title": "Benchmark Results",
    "subtitle": "Higher is better",
    "y_label": "Tokens/sec",
    "type": "bar",
    "categories": ["Model A", "Model B", "Model C"],
    "series": [
      {"name": "GPT-5", "values": [150, 200, 175]},
      {"name": "Claude", "values": [120, 180, 160]}
    ]
  }

JSON format (pie):
  {
    "title": "Market Share",
    "type": "pie",
    "items": [
      {"label": "OpenAI", "value": 45},
      {"label": "Anthropic", "value": 30},
      {"label": "Google", "value": 25}
    ]
  }

Examples:
  sin-code image-graph --type bar --data bench.json --output chart.html
  sin-code image-graph --type pie --json '{"items":[...]}' --output pie
  echo '{"series":[...]}' | sin-code image-graph --type line --output trend`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var spec imagegraph.ChartSpec

			if inlineJSON != "" {
				if err := json.Unmarshal([]byte(inlineJSON), &spec); err != nil {
					return fmt.Errorf("parse --json: %w", err)
				}
			} else {
				var err error
				spec, err = imagegraph.ParseSpec(dataFile)
				if err != nil {
					return err
				}
			}

			if chartType != "" {
				spec.Type = chartType
			}
			if title != "" {
				spec.Title = title
			}
			if subtitle != "" {
				spec.Subtitle = subtitle
			}
			if xLabel != "" {
				spec.XLabel = xLabel
			}
			if yLabel != "" {
				spec.YLabel = yLabel
			}
			if width != "" {
				spec.Width = width
			}
			if height != "" {
				spec.Height = height
			}

			if spec.Type == "" {
				return fmt.Errorf("chart type required (--type bar|line|pie|area)")
			}
			if outputFile == "" {
				outputFile = "chart.html"
			}

			if err := imagegraph.Render(spec, outputFile); err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&chartType, "type", "t", "", "chart type: bar, line, pie, area")
	cmd.Flags().StringVarP(&dataFile, "data", "d", "", "JSON data file (use - for stdin)")
	cmd.Flags().StringVarP(&outputFile, "output", "o", "", "output file (.html, .png auto-detected)")
	cmd.Flags().StringVar(&title, "title", "", "chart title")
	cmd.Flags().StringVar(&subtitle, "subtitle", "", "chart subtitle")
	cmd.Flags().StringVar(&xLabel, "xlabel", "", "X axis label")
	cmd.Flags().StringVar(&yLabel, "ylabel", "", "Y axis label")
	cmd.Flags().StringVar(&width, "width", "1200px", "chart width (e.g. 1200px, 100%)")
	cmd.Flags().StringVar(&height, "height", "720px", "chart height (e.g. 720px, 100%)")
	cmd.Flags().StringVarP(&inlineJSON, "json", "j", "", "inline JSON spec (alternative to --data)")

	return cmd
}

// ============================================================================
// tool-search — debug tool relevance ranking (issue #364)
// ============================================================================

func NewToolSearchCmd() *cobra.Command {
	var (
		k       int
		jsonOut bool
		keyword bool
		timeout time.Duration
	)

	cmd := &cobra.Command{
		Use:   "tool-search <query>",
		Short: "Debug tool relevance ranking",
		Long: `Search the effective tool catalog (built-in + reachable MCP servers) and
print the top-k relevant tools. By default the offline semantic index is used;
use --keyword to compare the legacy keyword index.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.Join(args, " ")
			ws, err := mcpHookVars.getwd()
			if err != nil {
				return err
			}

			mgr := mcpHookVars.newManager(mcpHookVars.loadConfigs(ws))
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			_ = mgr.ConnectAll(ctx)
			defer mgr.Close()

			specs := toolSearchSpecs(mgr.Tools())
			var results []mcpclient.ToolSpec
			if keyword {
				loader := mcpclient.NewLazyToolLoader(specs)
				results = loader.Search(query, k)
			} else {
				idx := mcpclient.NewSemanticIndex(specs)
				results = idx.Search(query, k)
			}

			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(results)
			}
			if len(results) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no matching tools")
				return nil
			}
			for _, s := range results {
				fmt.Fprintf(cmd.OutOrStdout(), "%s - %s\n", s.Name, s.Description)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&k, "k", 10, "maximum number of results")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	cmd.Flags().BoolVar(&keyword, "keyword", false, "use keyword retrieval instead of semantic")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Second, "MCP connection timeout")
	return cmd
}

// toolSearchSpecs merges built-in specs with the external tools reported by the
// MCP manager into the LazyToolLoader's internal ToolSpec representation.
func toolSearchSpecs(external []mcpclient.Tool) []mcpclient.ToolSpec {
	specs := make([]mcpclient.ToolSpec, 0, len(builtinSpecs())+len(external))
	for _, s := range builtinSpecs() {
		specs = append(specs, mcpclient.ToolSpec{
			Name:        s.Name,
			Description: s.Description,
			InputSchema: s.InputSchema,
		})
	}
	for _, t := range external {
		desc := t.Description
		if desc == "" {
			desc = fmt.Sprintf("External MCP tool %s on server %s", t.Name, t.Server)
		}
		schema := t.InputSchema
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		specs = append(specs, mcpclient.ToolSpec{
			Name:        t.Qualified,
			Description: desc,
			InputSchema: schema,
		})
	}
	return specs
}
