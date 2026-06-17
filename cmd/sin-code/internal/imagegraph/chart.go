// SPDX-License-Identifier: MIT
// Purpose: SOTA chart generation with Apache ECharts (via go-echarts).
// Outputs interactive HTML (opens in browser) + optional PNG via headless Chrome.
package imagegraph

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
)

type ChartSpec struct {
	Title      string   `json:"title"`
	Subtitle   string   `json:"subtitle"`
	XLabel     string   `json:"x_label"`
	YLabel     string   `json:"y_label"`
	Type       string   `json:"type"`
	Categories []string `json:"categories"`
	Series     []Series `json:"series"`
	Items      []Item   `json:"items"`
	Width      string   `json:"width"`
	Height     string   `json:"height"`
	Theme      string   `json:"theme"`
}

type Series struct {
	Name   string    `json:"name"`
	Values []float64 `json:"values"`
}

type Item struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
}

var sotaColors = []string{
	"#6366F1", "#EC4899", "#10B981", "#F59E0B",
	"#3B82F6", "#EF4444", "#8B5CF6", "#06B6D4",
}

func Render(spec ChartSpec, outputPath string) error {
	if spec.Width == "" {
		spec.Width = "1200px"
	}
	if spec.Height == "" {
		spec.Height = "720px"
	}
	if spec.Theme == "" {
		spec.Theme = "dark"
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
			return ChartSpec{}, fmt.Errorf("no input data (use --data <file>, --json, or stdin)")
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

func titleOpts(spec ChartSpec) opts.Title {
	return opts.Title{
		Title:    spec.Title,
		Subtitle: spec.Subtitle,
		Left:     "center",
		Top:      "5%",
		TitleStyle: &opts.TextStyle{
			FontSize:   24,
			Color:      "#F8FAFC",
			FontFamily: "Inter, -apple-system, sans-serif",
		},
		SubtitleStyle: &opts.TextStyle{
			FontSize:   14,
			Color:      "#94A3B8",
			FontFamily: "Inter, sans-serif",
		},
	}
}

func initOpts(spec ChartSpec) opts.Initialization {
	return opts.Initialization{
		Theme:           spec.Theme,
		Width:           spec.Width,
		Height:          spec.Height,
		BackgroundColor: "#0F172A",
	}
}

func tooltipAxis() opts.Tooltip {
	return opts.Tooltip{
		Show:           opts.Bool(true),
		Trigger:        "axis",
		BackgroundColor: "#1E293B",
		BorderColor:    "#334155",
	}
}

func tooltipItem() opts.Tooltip {
	return opts.Tooltip{
		Show:           opts.Bool(true),
		Trigger:        "item",
		Formatter:      "{b}: {c} ({d}%)",
		BackgroundColor: "#1E293B",
		BorderColor:    "#334155",
	}
}

func legendTop() opts.Legend {
	return opts.Legend{
		Show:   opts.Bool(true),
		Top:    "12%",
		Right:  "5%",
		Orient: "horizontal",
		TextStyle: &opts.TextStyle{
			Color:      "#CBD5E1",
			FontSize:   13,
			FontFamily: "Inter, sans-serif",
		},
	}
}

func legendBottom() opts.Legend {
	return opts.Legend{
		Show:   opts.Bool(true),
		Bottom: "5%",
		Orient: "horizontal",
		TextStyle: &opts.TextStyle{
			Color:    "#CBD5E1",
			FontSize: 13,
		},
	}
}

func xAxisOpts(spec ChartSpec) opts.XAxis {
	return opts.XAxis{
		Name: spec.XLabel,
		AxisLabel: &opts.AxisLabel{
			Color:      "#CBD5E1",
			FontSize:   12,
			FontFamily: "Inter, sans-serif",
		},
		AxisLine: &opts.AxisLine{
			LineStyle: &opts.LineStyle{Color: "#334155"},
		},
		SplitLine: &opts.SplitLine{Show: opts.Bool(false)},
	}
}

func yAxisOpts(spec ChartSpec) opts.YAxis {
	return opts.YAxis{
		Name: spec.YLabel,
		AxisLabel: &opts.AxisLabel{
			Color:      "#CBD5E1",
			FontSize:   12,
			FontFamily: "Inter, sans-serif",
		},
		AxisLine: &opts.AxisLine{
			LineStyle: &opts.LineStyle{Color: "#334155"},
		},
		SplitLine: &opts.SplitLine{
			Show: opts.Bool(true),
			LineStyle: &opts.LineStyle{Color: "#1E293B", Type: "dashed"},
		},
	}
}

func makeCategories(spec ChartSpec) []string {
	if len(spec.Categories) > 0 {
		return spec.Categories
	}
	maxLen := 0
	for _, s := range spec.Series {
		if len(s.Values) > maxLen {
			maxLen = len(s.Values)
		}
	}
	cats := make([]string, maxLen)
	for i := range cats {
		cats[i] = fmt.Sprintf("%d", i+1)
	}
	return cats
}

func renderBar(spec ChartSpec, outputPath string) error {
	bar := charts.NewBar()
	bar.SetGlobalOptions(
		charts.WithTitleOpts(titleOpts(spec)),
		charts.WithInitializationOpts(initOpts(spec)),
		charts.WithTooltipOpts(tooltipAxis()),
		charts.WithLegendOpts(legendTop()),
		charts.WithXAxisOpts(xAxisOpts(spec)),
		charts.WithYAxisOpts(yAxisOpts(spec)),
	)

	bar.SetXAxis(makeCategories(spec))

	for i, s := range spec.Series {
		data := make([]opts.BarData, len(s.Values))
		for j, v := range s.Values {
			data[j] = opts.BarData{
				Value:     v,
				ItemStyle: &opts.ItemStyle{Color: sotaColors[i%len(sotaColors)]},
			}
		}
		bar.AddSeries(s.Name, data).
			SetSeriesOptions(
				charts.WithBarChartOpts(opts.BarChart{
					BarWidth: "40%",
				}),
				charts.WithLabelOpts(opts.Label{
					Show:     opts.Bool(true),
					Position: "top",
					Color:    "#94A3B8",
					FontSize: 11,
				}),
			)
	}

	return writeHTML(bar, outputPath)
}

func renderLine(spec ChartSpec, outputPath string) error {
	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithTitleOpts(titleOpts(spec)),
		charts.WithInitializationOpts(initOpts(spec)),
		charts.WithTooltipOpts(tooltipAxis()),
		charts.WithLegendOpts(legendTop()),
		charts.WithXAxisOpts(xAxisOpts(spec)),
		charts.WithYAxisOpts(yAxisOpts(spec)),
	)

	line.SetXAxis(makeCategories(spec))

	for i, s := range spec.Series {
		data := make([]opts.LineData, len(s.Values))
		for j, v := range s.Values {
			data[j] = opts.LineData{Value: v}
		}
		line.AddSeries(s.Name, data).
			SetSeriesOptions(
				charts.WithLineChartOpts(opts.LineChart{
					Smooth: opts.Bool(true),
				}),
				charts.WithLineStyleOpts(opts.LineStyle{
					Width: 3,
					Color: sotaColors[i%len(sotaColors)],
				}),
				charts.WithAreaStyleOpts(opts.AreaStyle{
					Color:   sotaColors[i%len(sotaColors)],
					Opacity: opts.Float(0.1),
				}),
			)
	}

	return writeHTML(line, outputPath)
}

func renderArea(spec ChartSpec, outputPath string) error {
	area := charts.NewLine()
	area.SetGlobalOptions(
		charts.WithTitleOpts(titleOpts(spec)),
		charts.WithInitializationOpts(initOpts(spec)),
		charts.WithTooltipOpts(tooltipAxis()),
		charts.WithLegendOpts(legendTop()),
		charts.WithXAxisOpts(xAxisOpts(spec)),
		charts.WithYAxisOpts(yAxisOpts(spec)),
	)

	area.SetXAxis(makeCategories(spec))

	for i, s := range spec.Series {
		data := make([]opts.LineData, len(s.Values))
		for j, v := range s.Values {
			data[j] = opts.LineData{Value: v}
		}
		area.AddSeries(s.Name, data).
			SetSeriesOptions(
				charts.WithLineChartOpts(opts.LineChart{
					Smooth: opts.Bool(true),
				}),
				charts.WithLineStyleOpts(opts.LineStyle{
					Width: 2.5,
					Color: sotaColors[i%len(sotaColors)],
				}),
				charts.WithAreaStyleOpts(opts.AreaStyle{
					Color:   sotaColors[i%len(sotaColors)],
					Opacity: opts.Float(0.35),
				}),
			)
	}

	return writeHTML(area, outputPath)
}

func renderPie(spec ChartSpec, outputPath string) error {
	pie := charts.NewPie()
	pie.SetGlobalOptions(
		charts.WithTitleOpts(titleOpts(spec)),
		charts.WithInitializationOpts(initOpts(spec)),
		charts.WithTooltipOpts(tooltipItem()),
		charts.WithLegendOpts(legendBottom()),
	)

	data := make([]opts.PieData, len(spec.Items))
	for i, item := range spec.Items {
		data[i] = opts.PieData{
			Name:      item.Label,
			Value:     item.Value,
			ItemStyle: &opts.ItemStyle{Color: sotaColors[i%len(sotaColors)]},
		}
	}

	pie.AddSeries("data", data).
		SetSeriesOptions(
			charts.WithPieChartOpts(opts.PieChart{
				Radius: []string{"40%", "70%"},
				Center: []string{"50%", "55%"},
			}),
			charts.WithLabelOpts(opts.Label{
				Show:      opts.Bool(true),
				Formatter: "{b}\n{d}%",
				Color:     "#CBD5E1",
				FontSize:  13,
			}),
			charts.WithItemStyleOpts(opts.ItemStyle{
				BorderColor: "#0F172A",
				BorderWidth: 3,
			}),
		)

	return writeHTML(pie, outputPath)
}

func writeHTML(chart interface{ Render(io.Writer) error }, outputPath string) error {
	if !strings.HasSuffix(outputPath, ".html") {
		ext := filepath.Ext(outputPath)
		if ext != "" {
			outputPath = strings.TrimSuffix(outputPath, ext)
		}
		outputPath += ".html"
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer f.Close()

	if err := chart.Render(f); err != nil {
		return fmt.Errorf("render chart: %w", err)
	}

	abs, _ := filepath.Abs(outputPath)

	pngPath := strings.TrimSuffix(abs, ".html") + ".png"
	if tryChromeScreenshot(abs, pngPath) {
		fmt.Fprintf(os.Stdout, "✅ Chart generated: %s (HTML: %s)\n", pngPath, abs)
		openBrowser(abs)
		return nil
	}

	fmt.Fprintf(os.Stdout, "✅ Chart generated: %s\n", abs)
	openBrowser(abs)
	return nil
}

func tryChromeScreenshot(htmlPath, pngPath string) bool {
	chrome := findChrome()
	if chrome == "" {
		return false
	}
	cmd := exec.Command(chrome,
		"--headless",
		"--disable-gpu",
		"--no-sandbox",
		"--screenshot="+pngPath,
		"--window-size=1280,720",
		"--default-background-color=00000000",
		"file://"+htmlPath,
	)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

func findChrome() string {
	candidates := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/usr/bin/google-chrome",
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func openBrowser(path string) {
	exec.Command("open", path).Start()
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
