# Frameworks: Standards & Constraints

Docs: ../SKILL.md

## Technology Stack

- `sin-code image-graph` Go binary (stdlib only, no external Go deps)
- Apache ECharts 5.5.0 via CDN (no Go wrapper library)
- Headless Chrome for PNG screenshots

## Standards

- Dark theme: #0B1120 background, Inter font
- SOTA color palette: Indigo, Pink, Emerald, Amber, Blue, Red, Purple, Cyan
- Gradient fills on bars and lines
- Glow shadows (shadowBlur 12-24)
- Rounded bar corners (borderRadius [8,8,0,0])
- Staggered animations (elasticOut/cubicOut, per-item delay)
- Interactive HTML output (hover tooltips, save-as-image, toolbox)

## Constraints

- Same JSON input = same output (deterministic, byte-stable)
- No AI, no credits, no external API calls
- JSON input must match the chart type format (bar/line/area vs pie)
- PNG requires headless Chrome on PATH
