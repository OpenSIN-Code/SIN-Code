# Template: Prompt Snippet

Docs: ../SKILL.md

## User wants to generate a chart

```markdown
You are generating a SOTA chart using sin-code image-graph.

Chart type: {bar|line|pie|area}
Data source: {JSON file path or inline JSON}
Output path: {output.html}

Follow the workflow from tasks/workflow.md:
1. Prepare JSON spec matching the chart type format
2. Run: sin-code image-graph --type {type} --data {file} --output {output}
3. Verify HTML opens in browser with correct data
4. Check PNG output if headless Chrome is available

JSON format reference:
- bar/line/area: {title, y_label, type, categories, series: [{name, values}]}
- pie: {title, type, items: [{label, value}]}
```
