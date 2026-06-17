# Template: Output Format

Docs: ../SKILL.md

## Chart Generation Report

```markdown
# Generated Chart: {title}

## Details
- Type: {bar|line|pie|area}
- Source: {data file or inline JSON}
- Output: {output.html} + {output.png}

## Files
- {output}.html — interactive chart (opens in browser)
- {output}.png — static screenshot (if headless Chrome available)

## Verification
- [x] HTML opens in browser
- [x] Data renders correctly
- [x] Colors and gradients applied
- [x] Hover tooltips functional
- [x] Save-as-image toolbox available
```
