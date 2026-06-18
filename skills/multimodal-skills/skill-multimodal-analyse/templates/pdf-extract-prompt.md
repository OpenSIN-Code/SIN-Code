---
name: pdf-extract-prompt
description: Reusable prompt template for a PDF extraction task. Pair with tasks/extract-pdf.md.
---

# Extract PDF: {{PATH}}

You are opening the PDF asset at `{{PATH}}`. Use the
`skill-multimodal-analyse` workflow to extract structure before
responding to the user.

## Required steps

1. Confirm the path ends in `.pdf`. If labelled differently, route
   through `analyse__data_detect` and check `%PDF-` magic bytes.
2. Call `analyse__pdf_parse` with:
   - `path = {{PATH}}`
   - `pages = {{PAGES_RANGE}}` (default: all)
3. Branch on the response:
   - No tables → summarise the first 2–3 pages; ask the user which
     page range to deep-dive.
   - Tables present → surface as JSON arrays; numeric reconciliation
     belongs in code, not in chat.
4. If the user wants a working markdown copy, persist with
   `sin_write path = {{MD_OUT}}` (`ask` policy — surface the path
   before writing).
5. If the user wants surgical edits, anchor against the persisted
   markdown file, never against a hallucinated line.

## User context

{{CONTEXT}}

## Output expectations

- Page count and detection time on the first line of the summary.
- Top-of-document 2-paragraph abstract.
- List of tables with a 1-line description each.
- If security-relevant content (credentials, internal hostnames,
  CVEs) is in the PDF, flag with `M4` notation and pair with
  `skill-code-audit`.
