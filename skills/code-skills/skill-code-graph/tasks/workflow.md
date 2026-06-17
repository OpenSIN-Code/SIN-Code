# Tasks: Workflow

Docs: ../SKILL.md

## Pre-flight

- [ ] Identify the data source (JSON file, API endpoint, inline JSON)
- [ ] Determine chart type: bar (comparison), line (trend), pie (proportion), area (volume trend)
- [ ] Prepare the JSON spec matching the chart type format

## Execution

- [ ] Task 1: Prepare JSON spec
  - Acceptance: Valid JSON with title, categories/series/items, type field
  - Verify: JSON parses without error
- [ ] Task 2: Run `sin-code image-graph`
  - Acceptance: HTML file generated at output path
  - Verify: File exists and opens in browser
- [ ] Task 3: Verify PNG output (if headless Chrome available)
  - Acceptance: PNG file generated alongside HTML
  - Verify: File exists and is a valid image

## Post-flight

- [ ] Open HTML in browser to verify interactivity (tooltips, hover, save-as-image)
- [ ] Verify chart renders with correct data, labels, and colors
- [ ] Clean up temporary JSON files if any
