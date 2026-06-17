// SPDX-License-Identifier: MIT
// Purpose: `sin-code image-graph` — generate data-driven charts as PNG/SVG.
// Supports bar, line, pie, scatter, and area charts from JSON input.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/imagegraph"
)

func NewImageGraphCmd() *cobra.Command {
	var (
		chartType   string
		dataFile    string
		outputFile  string
		title       string
		xLabel      string
		yLabel      string
		width       int
		height      int
		inlineJSON  string
	)

	cmd := &cobra.Command{
		Use:   "image-graph",
		Short: "Generate data-driven charts (bar, line, pie, scatter, area) as PNG/SVG",
		Long: `sin-code image-graph — deterministic chart generation from JSON data.

No AI, no credits. Pure Go rendering with go-chart/v2.

Chart types:
  bar      — Bar chart (multiple series, grouped)
  line     — Line chart (trends over time)
  pie      — Pie chart (proportions)
  scatter  — Scatter plot (correlations)
  area     — Area chart (cumulative trends)

Input: JSON file (--data) or inline JSON (--json) or stdin.

JSON format (bar/line/area/scatter):
  {
    "title": "Benchmark Results",
    "x_label": "Model",
    "y_label": "Tokens/sec",
    "type": "bar",
    "categories": ["Task1", "Task2", "Task3"],
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
  sin-code image-graph --type bar --data bench.json --output chart.png
  sin-code image-graph --type pie --json '{"items":[...]}' --output pie.svg
  echo '{"series":[...]}' | sin-code image-graph --type line --output line.png`,
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
			if xLabel != "" {
				spec.XLabel = xLabel
			}
			if yLabel != "" {
				spec.YLabel = yLabel
			}
			if width > 0 {
				spec.Width = width
			}
			if height > 0 {
				spec.Height = height
			}

			if spec.Type == "" {
				return fmt.Errorf("chart type required (--type bar|line|pie|scatter|area)")
			}
			if outputFile == "" {
				outputFile = "chart.png"
			}

			if err := imagegraph.Render(spec, outputFile); err != nil {
				return err
			}

			abs, _ := absPath(outputFile)
			fmt.Fprintf(os.Stdout, "✅ Chart generated: %s (%s, %dx%d)\n",
				abs, spec.Type, spec.Width, spec.Height)
			return nil
		},
	}

	cmd.Flags().StringVarP(&chartType, "type", "t", "", "chart type: bar, line, pie, scatter, area")
	cmd.Flags().StringVarP(&dataFile, "data", "d", "", "JSON data file (use - for stdin)")
	cmd.Flags().StringVarP(&outputFile, "output", "o", "", "output file (.png or .svg)")
	cmd.Flags().StringVar(&title, "title", "", "chart title")
	cmd.Flags().StringVar(&xLabel, "xlabel", "", "X axis label")
	cmd.Flags().StringVar(&yLabel, "ylabel", "", "Y axis label")
	cmd.Flags().IntVar(&width, "width", 1280, "chart width in pixels")
	cmd.Flags().IntVar(&height, "height", 720, "chart height in pixels")
	cmd.Flags().StringVarP(&inlineJSON, "json", "j", "", "inline JSON spec (alternative to --data)")

	return cmd
}

func absPath(p string) (string, error) {
	if p == "" {
		return "", nil
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p, nil
	}
	return abs, nil
}
