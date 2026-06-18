# Task: Extract from a PDF

Use this task when the user opens a PDF (spec, paper, invoice, slide
deck) and asks the agent to read, summarise, quote, or extract data.
The agent must never `sin_read` a PDF — binary content. It routes
through `analyse__pdf_parse`.

## Inputs

- `path` — absolute path to the PDF inside the workspace root
- `pages` — optional comma-separated range (e.g. `1-3,7`); default
  all pages

## Workflow

```
1. Verify extension is .pdf
2. Call analyse__pdf_parse(path=..., pages=...)
3. Branch on table presence
4. For code-adjacent content (spec, README inside PDF, changelog):
     sin_edit the extracted text into a working .md
5. For numeric / structured content (invoice, table):
     surface the table payload as-is, do not paraphrase
6. Return summary + first table as preview
```

## Step-by-step

### Step 1 — Verify extension

If the path does not end in `.pdf`, route through
`analyse__data_detect` per the decision tree. PDFs mis-labelled as
`.txt` or `.dat` still flow through `analyse__pdf_parse` after
detection confirms `%PDF-` magic bytes.

### Step 2 — Call the tool

```bash
sin-code mcp call analyse__pdf_parse \
  --path docs/spec.pdf \
  --pages 1-10 \
  --json
```

Expected response:

```json
{
  "ok": true,
  "path": "docs/spec.pdf",
  "pages": [
    {
      "n": 1,
      "text": "SIN-Code v3.22.0 Specification ...",
      "tables": []
    },
    {
      "n": 2,
      "text": "## Hook Events\n\nThe 24 hook events ...",
      "tables": []
    }
  ]
}
```

### Step 3 — Branch on tables

- No tables → return concatenated `text` per page; summarise the
  first 2–3 pages back to the user; let them pick a page range to
  deep-dive.
- Tables present → return the tables as JSON arrays, do not flatten
  to prose. Numeric reconciliation belongs in code, not in chat.

### Step 4 — Persist as Markdown

If the user wants a working markdown copy:

```bash
sin-code mcp call analyse__pdf_parse --path docs/spec.pdf --pages 1-10 > /tmp/pdf.json
jq -r '.pages[] | "## Page \(.n)\n\n\(.text)\n"' /tmp/pdf.pdf.json > /tmp/pdf.md
sin_write docs/spec.extracted.md /tmp/pdf.md
```

`sin_write` is `ask` policy. Surface the proposed write path before
running.

### Step 5 — Surgical edits

For the common case "the spec on page 7 says we use X but I want Y"
flow:

```
extract p7 via analyse__pdf_parse
diff against the current sin-code docs/*.md
sin_edit the specific block via anchor
```

## Verification

- [ ] `analyse__pdf_parse` returned `{ok: true, pages: [...]}`.
- [ ] If user wanted persistence, file written with `sin_write` and
  source PDF unchanged.
- [ ] Tables surfaced as JSON, not paraphrased.
- [ ] If user asked for surgical edits, edit anchored to a real block
  in the persisted file, not a hallucinated line.
