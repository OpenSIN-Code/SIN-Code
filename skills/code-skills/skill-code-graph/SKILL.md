---
name: skill-code-graph
description: "SOTA chart generation via sin-code Go binary. Generates interactive HTML + PNG charts (bar, line, pie, area) with Apache ECharts — gradients, glow shadows, staggered animations, dark theme. Deterministic, free, no AI. Triggers on 'skill-code-graph', 'chart', 'graph', 'benchmark chart', 'comparison chart', 'plot data', 'visualize data', 'bar chart', 'pie chart', 'line chart', 'generate graph', or any task involving numerical data visualization."
license: MIT
compatibility:
  - opencode
  - sin-code
metadata:
  author: SIN-Rotator
  version: 2.0.0
  sources: "OpenSIN-Code/Infra-SIN-OpenCode-Stack/skills/skill-code-graph"
required_tools:
  - sin_image_graph
lifecycle: external
---

# skill-code-graph — SOTA Chart Generation

## Overview

Generates **SOTA-quality charts** (bar, line, pie, area) from JSON data using
`sin-code image-graph`. Direct Apache ECharts JSON generation — no wrapper
library, 100% feature access.

**Output:** Interactive HTML (opens in browser, hover tooltips, save-as-image)
+ optional PNG (via headless Chrome screenshot).

**No AI, no credits, no external Go dependencies.** Pure stdlib + ECharts CDN.

## SOTA Features

| Feature | Detail |
|---|---|
| **Gradient fills** | LinearGradient on bars (bright top → dark bottom), lines (left → right) |
| **Glow shadows** | shadowBlur 12-24 on bars, pie slices, lines |
| **Rounded bars** | borderRadius [8,8,0,0] — rounded top corners |
| **Hover emphasis** | `emphasis.focus='series'` dims other series on hover |
| **Staggered animation** | elasticOut/cubicOut with per-item delay (idx*80ms) |
| **Axis pointer** | Shadow (bar) / crosshair (line) tooltip guides |
| **Toolbox** | Save-as-image (2x pixelRatio), data view, restore |
| **Rose pie** | `roseType='radius'` donut with rounded slices + gap borders |
| **Dark theme** | #0B1120 background, Inter font, SOTA color palette |
| **Color palette** | Indigo, Pink, Emerald, Amber, Blue, Red, Purple, Cyan |

## When to Use

- **Benchmarks:** Compare model performance (tokens/sec, latency, accuracy)
- **Comparisons:** Side-by-side bar charts of metrics
- **Proportions:** Pie charts (pool status, market share, distribution)
- **Trends:** Line/area charts over time (latency, throughput, cost)
- **Any numerical data** that needs visual representation

## When NOT to Use

- Creative/conceptual visuals (logos, mockups) → use `sin-image-generation` skill (FLUX.2 Max)
- Architecture diagrams → use `sin-image-generation` skill
- Text-based tables → just print the data

## Tool: `sin-code image-graph`

```bash
sin-code image-graph --type <bar|line|pie|area> --data <file.json> --output <chart.html>
```

### Flags

| Flag | Default | Purpose |
|---|---|---|
| `--type, -t` | (required) | Chart type: bar, line, pie, area |
| `--data, -d` | (stdin) | JSON data file (use `-` for stdin) |
| `--output, -o` | chart.html | Output file (.html, PNG auto-generated alongside) |
| `--title` | (from JSON) | Chart title |
| `--subtitle` | (from JSON) | Chart subtitle |
| `--xlabel` | (from JSON) | X axis label |
| `--ylabel` | (from JSON) | Y axis label |
| `--width` | 1280px | Chart width |
| `--height` | 720px | Chart height |
| `--json, -j` | (none) | Inline JSON spec (alternative to --data) |

## JSON Input Format

### Bar / Line / Area Chart

```json
{
  "title": "Benchmark Results",
  "subtitle": "Tokens per second — higher is better",
  "y_label": "Tokens/sec",
  "type": "bar",
  "categories": ["GLM-5", "DeepSeek-V4", "Qwen3-Max", "Kimi-K2", "MiniMax-M3"],
  "series": [
    {"name": "Tokens/sec", "values": [150, 200, 175, 120, 190]}
  ]
}
```

Multi-series with categories:

```json
{
  "title": "Multi-Task Benchmark",
  "y_label": "Score",
  "type": "bar",
  "categories": ["Coding", "Math", "Reasoning", "Writing"],
  "series": [
    {"name": "Model A", "values": [85, 92, 78, 88]},
    {"name": "Model B", "values": [90, 85, 82, 75]}
  ]
}
```

### Pie Chart

```json
{
  "title": "Pool Key Status",
  "subtitle": "484 total keys",
  "type": "pie",
  "items": [
    {"label": "Available", "value": 93},
    {"label": "Suspended", "value": 381},
    {"label": "Leased", "value": 10}
  ]
}
```

## Examples

### Example 1: Model Benchmark Bar Chart

```bash
echo '{"title":"Fireworks Model Benchmark","subtitle":"Tokens/sec — higher is better","y_label":"Tokens/sec","type":"bar","categories":["GLM-5","DeepSeek-V4","Qwen3-Max","Kimi-K2","MiniMax-M3"],"series":[{"name":"Tokens/sec","values":[150,200,175,120,190]}]}' | sin-code image-graph --type bar --output bench

# Opens interactive HTML in browser + generates bench.png
```

### Example 2: Pool Status Pie Chart

```bash
curl -s http://localhost:8100/api/v1/pool/stats | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(json.dumps({
    'title': 'Fireworks Pool Status',
    'subtitle': str(d['total']) + ' total keys',
    'type': 'pie',
    'items': [
        {'label': 'Available', 'value': d['available']},
        {'label': 'Suspended', 'value': d['suspended']},
        {'label': 'Leased', 'value': d['leased']},
    ]
}))
" | sin-code image-graph --type pie --output pool
```

### Example 3: Latency Line Chart

```bash
echo '{"title":"API Latency Over 6 Hours","subtitle":"Pool A vs Pool B — lower is better","y_label":"Latency (ms)","type":"line","categories":["12:00","13:00","14:00","15:00","16:00","17:00"],"series":[{"name":"Pool A","values":[50,45,40,42,38,35]},{"name":"Pool B","values":[80,70,65,60,55,50]}]}' | sin-code image-graph --type line --output latency
```

### Example 4: Area Chart (throughput)

```bash
echo '{"title":"Pool Throughput 24h","subtitle":"Requests per minute","y_label":"Req/min","type":"area","categories":["00h","04h","08h","12h","16h","20h"],"series":[{"name":"Pool A","values":[120,200,450,890,750,340]},{"name":"Pool B","values":[80,150,380,720,620,280]}]}' | sin-code image-graph --type area --output throughput
```

## Comparison: skill-code-graph vs sin-image-generation

| Feature | skill-code-graph (Go+ECharts) | sin-image-generation (AI) |
|---|---|---|
| **Accuracy** | Exact data, precise axes | AI hallucinates numbers |
| **Cost** | Free | $0.06-0.07/image (credits) |
| **Speed** | <1s | 10-30s |
| **Deterministic** | Same data = same output | Different every time |
| **Chart types** | bar, line, pie, area | Any (but inaccurate) |
| **Visual quality** | SOTA (gradients, glow, animations) | Creative but imprecise |
| **Interactive** | HTML (tooltips, hover, save) | Static image |
| **Use case** | Benchmarks, metrics, data | Logos, concepts, mockups |

## Decision Guide

```
User wants to visualize data?
├── Numerical data (benchmarks, metrics, comparisons)
│   → skill-code-graph (Go+ECharts, deterministic, free, SOTA)
└── Creative/conceptual (logo, mockup, architecture)
    → sin-image-generation (AI, FLUX.2 Max)
```

## Architecture

```
sin-code image-graph --type bar --data spec.json
    ↓
imagegraph.ParseSpec() ← JSON → ChartSpec
    ↓
imagegraph.Render(spec, output)
    ↓
Direct ECharts JSON option (no wrapper library)
    ↓
HTML template + ECharts 5.5.0 CDN
    ↓
Interactive HTML (browser) + PNG (headless Chrome)
```

## Files

- Go source: `SIN-Code/cmd/sin-code/image_graph_cmd.go`
- Chart logic: `SIN-Code/cmd/sin-code/internal/imagegraph/chart.go`
- Rendering: Apache ECharts 5.5.0 via CDN (no Go dependency)
- Output: HTML (interactive) + PNG (headless Chrome screenshot)
