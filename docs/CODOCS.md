# CoDocs — SOTA Code Documentation Standard

CoDocs is a **two-layer** documentation standard: every meaningful code file
gets both a `.doc.md` companion (overview) AND SOTA inline `#` comments
(detail) so agents never misunderstand what they're looking at.

> Merged into the bundle from the former
> `SIN-Hermes-Bundles/SIN-Code-CoDocs-Bundle` repository.

## The standard (Layer 1: .doc.md)

| Code file        | Companion          | Reference (first line)   |
|------------------|--------------------|--------------------------|
| `router.py`      | `router.doc.md`    | `# Docs: router.doc.md`  |
| `api/types.ts`   | `api/types.doc.md` | `// Docs: types.doc.md`  |
| `Makefile`       | `Makefile.doc.md`  | `# Docs: Makefile.doc.md`|

A `.doc.md` should capture **why** and **how it connects** — purpose in one
sentence, which files touch it, important config values/limits, and the
rationale behind non-obvious decisions. It should **not** restate
implementation details or git history.

## The standard (Layer 2: SOTA inline docs)

Every code file must also have professional inline `#`/`//` comments. This
is **not** "comment every line" — it is semantic context agents can't infer:

| Element | Rule |
|---------|------|
| **File header** | `# Purpose: <what this does in 1 line>` |
| **Public API** | Docstrings on every public function/class/method |
| **Non-obvious logic** | Why NOT the obvious approach |
| **Magic values** | Explain `MAX_RETRIES = 3  # upstream SLA` |
| **Section separators** | `# ── Auth ──────────────────────` for files > 100 lines |
| **Deprecation markers** | `# DEPRECATED(v2): use X instead` |

Full reference: see the packaged skill at `src/sin_code_bundle/data/codocs/SKILL.md`.

### Exceptions (no docs needed)

- `docs/` for architecture docs, ADRs, setup guides
- `README.md` for project overview
- Pure config files without logic (`.gitignore`, `.prettierrc`, ...)

## CLI

```bash
sin codocs check [ROOT]            # exit 1 if any .doc.md reference broken
sin codocs check --json            # machine-readable output
sin codocs check-inline [ROOT]     # check files for Purpose header
sin codocs check-inline --json     # machine-readable output
sin codocs list [ROOT]             # list every reference + resolve status
sin codocs install-skill           # install agent skill (Hermes / OpenCode)
```

### CI usage

```yaml
- name: Validate CoDocs + inline docs
  run: |
    sin codocs check .
    sin codocs check-inline .
```

## MCP

When running `sin serve`, CoDocs is exposed as MCP tools:
- `codocs_check` — broken .doc.md references
- `codocs_check_inline` — missing Purpose headers

## MarkItDown pipeline

Convert existing PDFs/Office docs into `.doc.md` companions:

```bash
for f in docs/*.pdf docs/*.docx docs/*.pptx; do
    markitdown "$f" -o "${f%.*}.doc.md"
done
```

See the packaged skill at `src/sin_code_bundle/data/codocs/SKILL.md` for the
full MarkItDown reference, and [`examples/codocs/`](../examples/codocs) for a
worked example.
