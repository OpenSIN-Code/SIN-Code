// SPDX-License-Identifier: MIT
// Purpose: `sin-code image-graph` — generate SOTA charts (bar, line, pie, area).
// Uses Apache ECharts (via go-echarts) for professional rendering.
// Output: interactive HTML (opens in browser) + PNG via headless Chrome.
package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/imagegraph"
)

func NewImageGraphCmd() *cobra.Command {
	var (
		chartType   string
		dataFile    string
		outputFile  string
		title       string
		subtitle    string
		xLabel      string
		yLabel      string
		width       string
		height      string
		inlineJSON  string
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
