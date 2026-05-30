# CoDocs — Co-located Docs Standard

CoDocs keeps documentation next to the code it describes. Every meaningful code
file gets a `.doc.md` companion in the same directory, referenced from the first
line of the source file.

> Merged into the bundle from the former
> `SIN-Hermes-Bundles/SIN-Code-CoDocs-Bundle` repository.

## The standard

| Code file        | Companion          | Reference (first line)   |
|------------------|--------------------|--------------------------|
| `router.py`      | `router.doc.md`    | `# Docs: router.doc.md`  |
| `api/types.ts`   | `api/types.doc.md` | `// Docs: types.doc.md`  |
| `Makefile`       | `Makefile.doc.md`  | `# Docs: Makefile.doc.md`|

A `.doc.md` should capture **why** and **how it connects** — purpose in one
sentence, which files touch it, important config values/limits, and the
rationale behind non-obvious decisions. It should **not** restate
implementation details or git history.

### Exceptions (no `.doc.md` needed)

- `docs/` for architecture docs, ADRs, setup guides
- `README.md` for the project overview
- Pure config files without logic (`.gitignore`, `.prettierrc`, ...)

## CLI

The bundle ships a robust validator that replaces the original fragile
`grep | sed` shell snippet:

```bash
sin codocs check [ROOT]        # exit code 1 if any reference is broken
sin codocs check --json        # machine-readable output
sin codocs list [ROOT]         # list every reference + whether it resolves
sin codocs install-skill       # install the agent skill (Hermes / OpenCode)
```

### CI usage

```yaml
- name: Validate CoDocs references
  run: sin codocs check .
```

## MCP

When running `sin serve`, CoDocs is exposed as the `codocs_check` MCP tool,
returning the list of broken references as JSON.

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
