// SPDX-License-Identifier: MIT
// Purpose: SOTA chart generation with Apache ECharts (direct JSON, no wrapper).
// Outputs interactive HTML (opens in browser) + optional PNG via headless Chrome.
//
// go-echarts wrapper was removed because it cannot express:
//   - LinearGradient objects on bar/area fills
//   - shadowBlur glow effects
//   - emphasis.focus = 'series' (dim others on hover)
//   - borderRadius on bars
//   - staggered animationDelay functions
//   - axisPointer type 'shadow' / 'cross'
//
// Direct JSON generation gives 100% ECharts feature access.
package imagegraph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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

type jsFunc string

type gradient struct {
	Type       string      `json:"type"`
	X          int         `json:"x"`
	Y          int         `json:"y"`
	X2         int         `json:"x2"`
	Y2         int         `json:"y2"`
	ColorStops []colorStop `json:"colorStops"`
}

type colorStop struct {
	Offset float64 `json:"offset"`
	Color  string  `json:"color"`
}

var palette = [][3]string{
	{"#818CF8", "#6366F1", "99,102,241"}, // Indigo
	{"#F472B6", "#EC4899", "236,72,153"}, // Pink
	{"#34D399", "#10B981", "16,185,129"}, // Emerald
	{"#FBBF24", "#F59E0B", "245,158,11"}, // Amber
	{"#60A5FA", "#3B82F6", "59,130,246"}, // Blue
	{"#F87171", "#EF4444", "239,68,68"},  // Red
	{"#A78BFA", "#8B5CF6", "139,92,246"}, // Purple
	{"#22D3EE", "#06B6D4", "6,182,212"},  // Cyan
}

func vGradient(bright, dark string) gradient {
	return gradient{
		Type: "linear", X: 0, Y: 0, X2: 0, Y2: 1,
		ColorStops: []colorStop{
			{Offset: 0, Color: bright},
			{Offset: 1, Color: dark},
		},
	}
}

func hGradient(bright, dark string) gradient {
	return gradient{
		Type: "linear", X: 0, Y: 0, X2: 1, Y2: 0,
		ColorStops: []colorStop{
			{Offset: 0, Color: dark},
			{Offset: 1, Color: bright},
		},
	}
}

func areaGradient(rgb string) gradient {
	return gradient{
		Type: "linear", X: 0, Y: 0, X2: 0, Y2: 1,
		ColorStops: []colorStop{
			{Offset: 0, Color: fmt.Sprintf("rgba(%s,0.35)", rgb)},
			{Offset: 1, Color: fmt.Sprintf("rgba(%s,0.02)", rgb)},
		},
	}
}

const bgColor = "#0B1120"
const cardBg = "#1E293B"
const borderClr = "#334155"
const gridClr = "#1E293B"
const textPrimary = "#F8FAFC"
const textSecondary = "#94A3B8"
const textMuted = "#64748B"
const axisLabelClr = "#CBD5E1"

func Render(spec ChartSpec, outputPath string) error {
	if spec.Width == "" {
		spec.Width = "1280px"
	}
	if spec.Height == "" {
		spec.Height = "720px"
	}

	switch strings.ToLower(spec.Type) {
	case "bar":
		return renderBar(spec, outputPath)
	case "line":
		return renderLine(spec, outputPath, false)
	case "pie":
		return renderPie(spec, outputPath)
	case "area":
		return renderLine(spec, outputPath, true)
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
		data, err = io.ReadAll(os.Stdin)
	} else {
		// #nosec G304 -- inputPath is the explicit --data file selected by the
		// local operator; no implicit or remote path expansion occurs here.
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

func baseOption(spec ChartSpec) map[string]interface{} {
	opt := map[string]interface{}{
		"backgroundColor": bgColor,
		"title": map[string]interface{}{
			"text":    spec.Title,
			"subtext": spec.Subtitle,
			"left":    "center",
			"top":     "4%",
			"textStyle": map[string]interface{}{
				"color":      textPrimary,
				"fontSize":   26,
				"fontWeight": "bold",
				"fontFamily": "Inter, -apple-system, sans-serif",
			},
			"subtextStyle": map[string]interface{}{
				"color":      textSecondary,
				"fontSize":   14,
				"fontFamily": "Inter, sans-serif",
			},
		},
		"tooltip": map[string]interface{}{
			"backgroundColor": cardBg,
			"borderColor":     borderClr,
			"borderWidth":     1,
			"padding":         12,
			"textStyle": map[string]interface{}{
				"color":      textPrimary,
				"fontFamily": "Inter, sans-serif",
				"fontSize":   13,
			},
		},
		"legend": map[string]interface{}{
			"show":       true,
			"top":        "13%",
			"right":      "5%",
			"icon":       "circle",
			"itemWidth":  10,
			"itemHeight": 10,
			"itemGap":    20,
			"textStyle": map[string]interface{}{
				"color":      axisLabelClr,
				"fontSize":   13,
				"fontFamily": "Inter, sans-serif",
			},
		},
		"toolbox": map[string]interface{}{
			"show":  true,
			"right": "3%",
			"top":   "5%",
			"feature": map[string]interface{}{
				"saveAsImage": map[string]interface{}{
					"name":            spec.Title,
					"backgroundColor": bgColor,
					"pixelRatio":      2,
				},
				"restore":  map[string]interface{}{},
				"dataView": map[string]interface{}{"readOnly": true},
			},
			"iconStyle": map[string]interface{}{
				"borderColor": textMuted,
			},
		},
		"animation":         true,
		"animationEasing":   "cubicOut",
		"animationDuration": 1200,
	}

	return opt
}

func axisOpts(spec ChartSpec) (map[string]interface{}, map[string]interface{}) {
	x := map[string]interface{}{
		"type": "category",
		"data": makeCategories(spec),
		"name": spec.XLabel,
		"nameTextStyle": map[string]interface{}{
			"color":    textSecondary,
			"fontSize": 12,
		},
		"axisLine": map[string]interface{}{
			"lineStyle": map[string]interface{}{"color": borderClr},
		},
		"axisTick": map[string]interface{}{"show": false},
		"axisLabel": map[string]interface{}{
			"color":      axisLabelClr,
			"fontSize":   12,
			"fontFamily": "Inter, sans-serif",
			"margin":     14,
		},
		"splitLine": map[string]interface{}{"show": false},
	}

	y := map[string]interface{}{
		"type": "value",
		"name": spec.YLabel,
		"nameTextStyle": map[string]interface{}{
			"color":    textSecondary,
			"fontSize": 12,
		},
		"axisLine": map[string]interface{}{"show": false},
		"axisTick": map[string]interface{}{"show": false},
		"axisLabel": map[string]interface{}{
			"color":      axisLabelClr,
			"fontSize":   12,
			"fontFamily": "Inter, sans-serif",
		},
		"splitLine": map[string]interface{}{
			"show": true,
			"lineStyle": map[string]interface{}{
				"color":      gridClr,
				"type":       "dashed",
				"dashOffset": 5,
			},
		},
	}

	return x, y
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
	opt := baseOption(spec)
	opt["tooltip"].(map[string]interface{})["trigger"] = "axis"
	opt["tooltip"].(map[string]interface{})["axisPointer"] = map[string]interface{}{
		"type": "shadow",
		"shadowStyle": map[string]interface{}{
			"color": "rgba(30,41,59,0.5)",
		},
	}

	x, y := axisOpts(spec)
	opt["xAxis"] = x
	opt["yAxis"] = y
	opt["grid"] = map[string]interface{}{
		"top":          "22%",
		"bottom":       "12%",
		"left":         "8%",
		"right":        "5%",
		"containLabel": true,
	}

	seriesList := make([]map[string]interface{}, len(spec.Series))
	for i, s := range spec.Series {
		p := palette[i%len(palette)]
		data := make([]map[string]interface{}, len(s.Values))
		for j, v := range s.Values {
			data[j] = map[string]interface{}{
				"value": v,
				"itemStyle": map[string]interface{}{
					"color":         vGradient(p[0], p[1]),
					"borderRadius":  []int{8, 8, 0, 0},
					"shadowBlur":    12,
					"shadowColor":   fmt.Sprintf("rgba(%s,0.25)", p[2]),
					"shadowOffsetY": 4,
				},
			}
		}

		seriesList[i] = map[string]interface{}{
			"name":     s.Name,
			"type":     "bar",
			"data":     data,
			"barWidth": "45%",
			"barGap":   "15%",
			"emphasis": map[string]interface{}{
				"focus": "series",
				"itemStyle": map[string]interface{}{
					"shadowBlur":  24,
					"shadowColor": fmt.Sprintf("rgba(%s,0.5)", p[2]),
				},
			},
			"label": map[string]interface{}{
				"show":       true,
				"position":   "top",
				"color":      textSecondary,
				"fontSize":   11,
				"fontFamily": "Inter, sans-serif",
			},
			"animationDelay":    fmt.Sprintf("@JS@function(idx){return idx*80+%d;}@JS@", i*200),
			"animationDuration": 800,
			"animationEasing":   "elasticOut",
		}
	}
	opt["series"] = seriesList

	return writeHTML(opt, spec, outputPath)
}

func renderLine(spec ChartSpec, outputPath string, area bool) error {
	opt := baseOption(spec)
	opt["tooltip"].(map[string]interface{})["trigger"] = "axis"
	opt["tooltip"].(map[string]interface{})["axisPointer"] = map[string]interface{}{
		"type": "cross",
		"crossStyle": map[string]interface{}{
			"color": borderClr,
			"width": 1,
			"type":  "dashed",
		},
		"label": map[string]interface{}{
			"backgroundColor": cardBg,
		},
	}

	x, y := axisOpts(spec)
	opt["xAxis"] = x
	opt["yAxis"] = y
	opt["grid"] = map[string]interface{}{
		"top":          "22%",
		"bottom":       "12%",
		"left":         "8%",
		"right":        "5%",
		"containLabel": true,
	}

	areaOpacity := 0.0
	if area {
		areaOpacity = 0.35
	}

	seriesList := make([]map[string]interface{}, len(spec.Series))
	for i, s := range spec.Series {
		p := palette[i%len(palette)]
		data := make([]float64, len(s.Values))
		copy(data, s.Values)

		itemStyle := map[string]interface{}{
			"color":       vGradient(p[0], p[1]),
			"shadowBlur":  8,
			"shadowColor": fmt.Sprintf("rgba(%s,0.3)", p[2]),
		}

		seriesEntry := map[string]interface{}{
			"name":           s.Name,
			"type":           "line",
			"data":           data,
			"smooth":         true,
			"smoothMonotone": "x",
			"showSymbol":     false,
			"symbol":         "circle",
			"symbolSize":     8,
			"lineStyle": map[string]interface{}{
				"width":       3,
				"color":       hGradient(p[0], p[1]),
				"shadowBlur":  6,
				"shadowColor": fmt.Sprintf("rgba(%s,0.3)", p[2]),
			},
			"itemStyle": itemStyle,
			"emphasis": map[string]interface{}{
				"focus": "series",
				"scale": 1.5,
			},
			"animationDuration": 1500,
			"animationEasing":   "cubicOut",
			"animationDelay":    fmt.Sprintf("@JS@function(idx){return idx*60+%d;}@JS@", i*300),
		}

		if area {
			seriesEntry["areaStyle"] = map[string]interface{}{
				"color":       areaGradient(p[2]),
				"opacity":     areaOpacity,
				"shadowBlur":  20,
				"shadowColor": fmt.Sprintf("rgba(%s,0.15)", p[2]),
			}
		} else {
			seriesEntry["areaStyle"] = map[string]interface{}{
				"color":   areaGradient(p[2]),
				"opacity": 0,
			}
		}

		seriesList[i] = seriesEntry
	}
	opt["series"] = seriesList

	return writeHTML(opt, spec, outputPath)
}

func renderPie(spec ChartSpec, outputPath string) error {
	opt := baseOption(spec)
	opt["tooltip"].(map[string]interface{})["trigger"] = "item"
	opt["tooltip"].(map[string]interface{})["formatter"] = "{b}: {c} ({d}%)"

	opt["legend"] = map[string]interface{}{
		"show":       true,
		"bottom":     "5%",
		"orient":     "horizontal",
		"icon":       "circle",
		"itemWidth":  10,
		"itemHeight": 10,
		"itemGap":    20,
		"textStyle": map[string]interface{}{
			"color":      axisLabelClr,
			"fontSize":   13,
			"fontFamily": "Inter, sans-serif",
		},
	}

	data := make([]map[string]interface{}, len(spec.Items))
	for i, item := range spec.Items {
		p := palette[i%len(palette)]
		data[i] = map[string]interface{}{
			"name":  item.Label,
			"value": item.Value,
			"itemStyle": map[string]interface{}{
				"color":        vGradient(p[0], p[1]),
				"borderRadius": 6,
				"borderColor":  bgColor,
				"borderWidth":  4,
				"shadowBlur":   20,
				"shadowColor":  "rgba(0,0,0,0.3)",
			},
		}
	}

	opt["series"] = []map[string]interface{}{
		{
			"type":     "pie",
			"radius":   []string{"42%", "70%"},
			"center":   []string{"50%", "52%"},
			"data":     data,
			"roseType": "radius",
			"label": map[string]interface{}{
				"show":       true,
				"formatter":  "{b}\n{d}%",
				"color":      axisLabelClr,
				"fontSize":   13,
				"fontFamily": "Inter, sans-serif",
				"lineHeight": 18,
			},
			"labelLine": map[string]interface{}{
				"show":    true,
				"length":  15,
				"length2": 20,
				"smooth":  true,
				"lineStyle": map[string]interface{}{
					"color": borderClr,
					"width": 1,
				},
			},
			"emphasis": map[string]interface{}{
				"scaleSize": 10,
				"itemStyle": map[string]interface{}{
					"shadowBlur":  40,
					"shadowColor": "rgba(0,0,0,0.5)",
				},
				"label": map[string]interface{}{
					"show":       true,
					"fontSize":   15,
					"fontWeight": "bold",
					"color":      textPrimary,
				},
			},
			"animationDuration": 1200,
			"animationEasing":   "cubicOut",
			"animationDelay":    "@JS@function(idx){return idx*100;}@JS@",
		},
	}

	return writeHTML(opt, spec, outputPath)
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<script src="https://cdn.jsdelivr.net/npm/echarts@5.5.0/dist/echarts.min.js"></script>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{background:%s;display:flex;align-items:center;justify-content:center;min-height:100vh;font-family:Inter,-apple-system,sans-serif}
#chart{width:%s;height:%s}
</style>
</head>
<body>
<div id="chart"></div>
<script>
var chart=echarts.init(document.getElementById('chart'),null,{renderer:'canvas'});
var option=%s;
option.animation=false;
chart.setOption(option);
window.addEventListener('resize',function(){chart.resize()});
window.addEventListener('load',function(){setTimeout(function(){document.title='CHART_READY'},500)});
</script>
</body>
</html>`

func writeHTML(opt map[string]interface{}, spec ChartSpec, outputPath string) error {
	if !strings.HasSuffix(outputPath, ".html") {
		ext := filepath.Ext(outputPath)
		if ext != "" {
			outputPath = strings.TrimSuffix(outputPath, ext)
		}
		outputPath += ".html"
	}

	jsonBytes, err := json.MarshalIndent(opt, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal option: %w", err)
	}

	jsonStr := string(jsonBytes)
	jsonStr = strings.ReplaceAll(jsonStr, `"@JS@`, "")
	jsonStr = strings.ReplaceAll(jsonStr, `@JS@"`, "")

	w := spec.Width
	if w == "" {
		w = "1280px"
	}
	h := spec.Height
	if h == "" {
		h = "720px"
	}

	title := spec.Title
	if title == "" {
		title = "Chart"
	}

	html := fmt.Sprintf(htmlTemplate, title, bgColor, w, h, jsonStr)

	// #nosec G304 -- outputPath is the explicit local chart destination chosen
	// by the operator; Render does not derive it from network content.
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(html); err != nil {
		return fmt.Errorf("write html: %w", err)
	}

	abs, _ := filepath.Abs(outputPath)
	pngPath := strings.TrimSuffix(abs, ".html") + ".png"

	if chromeScreenshot(abs, pngPath) {
		fmt.Fprintf(os.Stdout, "Chart generated: %s (HTML: %s)\n", pngPath, abs)
	} else {
		fmt.Fprintf(os.Stdout, "Chart generated: %s\n", abs)
	}
	if err := browserOpen(abs); err != nil {
		fmt.Fprintf(os.Stderr, "warning: open chart in browser: %v\n", err)
	}
	return nil
}

var (
	chromeScreenshot = tryChromeScreenshot
	browserOpen      = openBrowser
)

func tryChromeScreenshot(htmlPath, pngPath string) bool {
	chrome := findChrome()
	if chrome == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// #nosec G204 -- chrome is selected exclusively from findChrome's fixed
	// absolute allowlist and every option is a separately bounded argv value.
	cmd := exec.CommandContext(ctx, chrome,
		"--headless=new",
		"--disable-gpu",
		"--no-sandbox",
		"--screenshot="+pngPath,
		"--window-size=1280,720",
		"--default-background-color=00000000",
		"--virtual-time-budget=8000",
		"--hide-scrollbars",
		"--run-all-compositor-stages-before-draw",
		"--disable-software-rasterizer",
		"--enable-unsafe-swiftshader",
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

func openBrowser(path string) error {
	// #nosec G204 -- `open` is a fixed local OS binary and path is a single argv
	// value, never shell-interpreted.
	return exec.Command("open", path).Start()
}
