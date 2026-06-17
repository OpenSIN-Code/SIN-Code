// SPDX-License-Identifier: MIT
// Purpose: Data-driven chart generation (bar, line, pie, area).
// Pure Go, no CGO — uses go-chart/v2 for PNG/SVG rendering.
package imagegraph

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/wcharczuk/go-chart/v2"
)

type ChartSpec struct {
	Title      string   `json:"title"`
	XLabel     string   `json:"x_label"`
	YLabel     string   `json:"y_label"`
	Type       string   `json:"type"`
	Categories []string `json:"categories"`
	Series     []Series `json:"series"`
	Items      []Item   `json:"items"`
	Width      int      `json:"width"`
	Height     int      `json:"height"`
}

type Series struct {
	Name   string    `json:"name"`
	Values []float64 `json:"values"`
}

type Item struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
}

var palette = []chart.Style{
	{FillColor: chart.ColorBlue, StrokeColor: chart.ColorBlue, StrokeWidth: 2},
	{FillColor: chart.ColorOrange, StrokeColor: chart.ColorOrange, StrokeWidth: 2},
	{FillColor: chart.ColorGreen, StrokeColor: chart.ColorGreen, StrokeWidth: 2},
	{FillColor: chart.ColorRed, StrokeColor: chart.ColorRed, StrokeWidth: 2},
	{FillColor: chart.ColorCyan, StrokeColor: chart.ColorCyan, StrokeWidth: 2},
	{FillColor: chart.ColorYellow, StrokeColor: chart.ColorYellow, StrokeWidth: 2},
	{FillColor: chart.ColorAlternateBlue, StrokeColor: chart.ColorAlternateBlue, StrokeWidth: 2},
	{FillColor: chart.ColorAlternateGreen, StrokeColor: chart.ColorAlternateGreen, StrokeWidth: 2},
}

func Render(spec ChartSpec, outputPath string) error {
	if spec.Width == 0 {
		spec.Width = 1280
	}
	if spec.Height == 0 {
		spec.Height = 720
	}

	switch strings.ToLower(spec.Type) {
	case "bar":
		return renderBar(spec, outputPath)
	case "line":
		return renderLine(spec, outputPath)
	case "pie":
		return renderPie(spec, outputPath)
	case "area":
		return renderArea(spec, outputPath)
	default:
		return fmt.Errorf("unsupported chart type: %s (use bar, line, pie, or area)", spec.Type)
	}
}

func ParseSpec(inputPath string) (ChartSpec, error) {
	var data []byte
	var err error

	if inputPath == "-" || inputPath == "" {
		stat, _ := os.Stdin.Stat()
		if stat.Size() == 0 {
			return ChartSpec{}, fmt.Errorf("no input data (use --data <file>, --json '<json>', or pipe via stdin)")
		}
		data, err = readAll(os.Stdin)
	} else {
		data, err = os.ReadFile(inputPath)
	}
	if err != nil {
		return ChartSpec{}, fmt.Errorf("read input: %w", err)
	}

	var spec ChartSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return ChartSpec{}, fmt.Errorf("parse JSON: %w", err)
	}
	return spec, nil
}

func renderBar(spec ChartSpec, outputPath string) error {
	var bars []chart.Value
	for _, s := range spec.Series {
		for i, v := range s.Values {
			label := s.Name
			if len(spec.Categories) > i && spec.Categories[i] != "" {
				label = spec.Categories[i]
			}
			bars = append(bars, chart.Value{Label: label, Value: v})
		}
	}
	if len(bars) == 0 {
		for _, item := range spec.Items {
			bars = append(bars, chart.Value{Label: item.Label, Value: item.Value})
		}
	}

	graph := chart.BarChart{
		Title:  spec.Title,
		Width:  spec.Width,
		Height: spec.Height,
		Bars:   bars,
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer f.Close()

	if isSVG(outputPath) {
		return graph.Render(chart.SVG, f)
	}
	return graph.Render(chart.PNG, f)
}

func renderLine(spec ChartSpec, outputPath string) error {
	var series []chart.Series
	for i, s := range spec.Series {
		style := palette[i%len(palette)]
		series = append(series, chart.ContinuousSeries{
			Name:    s.Name,
			XValues: makeXValues(len(s.Values)),
			YValues: s.Values,
			Style:   style,
		})
	}

	graph := chart.Chart{
		Title:  spec.Title,
		Width:  spec.Width,
		Height: spec.Height,
		Series: series,
	}

	return writeChart(graph, outputPath)
}

func renderArea(spec ChartSpec, outputPath string) error {
	var series []chart.Series
	for i, s := range spec.Series {
		style := palette[i%len(palette)]
		style.FillColor = style.StrokeColor
		series = append(series, chart.ContinuousSeries{
			Name:    s.Name,
			XValues: makeXValues(len(s.Values)),
			YValues: s.Values,
			Style:   style,
		})
	}

	graph := chart.Chart{
		Title:  spec.Title,
		Width:  spec.Width,
		Height: spec.Height,
		Series: series,
	}

	return writeChart(graph, outputPath)
}

func renderPie(spec ChartSpec, outputPath string) error {
	var values []chart.Value
	for _, item := range spec.Items {
		values = append(values, chart.Value{Label: item.Label, Value: item.Value})
	}

	graph := chart.PieChart{
		Title:  spec.Title,
		Width:  spec.Width,
		Height: spec.Height,
		Values: values,
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer f.Close()

	if isSVG(outputPath) {
		return graph.Render(chart.SVG, f)
	}
	return graph.Render(chart.PNG, f)
}

func writeChart(graph chart.Chart, outputPath string) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer f.Close()

	if isSVG(outputPath) {
		return graph.Render(chart.SVG, f)
	}
	return graph.Render(chart.PNG, f)
}

func isSVG(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".svg")
}

func makeXValues(n int) []float64 {
	x := make([]float64, n)
	for i := range x {
		x[i] = float64(i)
	}
	return x
}

func readAll(f *os.File) ([]byte, error) {
	var buf [4096]byte
	var result []byte
	for {
		n, err := f.Read(buf[:])
		if n > 0 {
			result = append(result, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	return result, nil
}
