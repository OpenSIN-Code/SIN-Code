# markitdown.py

MarkItDown bridge — invokes the upstream `markitdown` (CLI / library) and
`markitdown-mcp` (MCP server) packages and wires the MCP server into
OpenCode / Codex / Hermes. Never vendors or copies MarkItDown source;
the bundle stays MIT-licensed.

## Dependencies

- stdlib: `json`, `shutil`, `subprocess`, `dataclasses`
- external: `markitdown` CLI and/or `markitdown-mcp` (preferred via `uvx`)

## Touched by

- `cli.py` — `sin markitdown doctor|setup|convert`
- `install.sh` — `sin markitdown setup` is invoked during bundle install

## What it does

1. **`detect_env()`** — locates `uvx`, `markitdown-mcp`, and `markitdown`
   on `PATH`; does not mutate.
2. **`mcp_command()`** — returns the right launch dict (`uvx
   markitdown-mcp` if `uvx` is present, else `markitdown-mcp` directly).
3. **`convert(path, timeout=300)`** — shells out to the `markitdown` CLI
   to convert a document to Markdown. Raises `MarkItDownError` on
   timeout / non-zero exit.
4. **`doctor()`** — aggregate health report.
5. **`setup_agents(agents)`** — wires the MarkItDown MCP server into
   the three supported coders. Idempotent: existing entries are
   replaced.

## Important config

- `MARKITDOWN_MCP_PACKAGE = "markitdown-mcp"` — pinned to upstream
- `MARKITDOWN_CLI = "markitdown"` — invoked directly
- `AGENTS = ("opencode", "codex", "hermes")` — the three supported coders
- `timeout = 300` — default per-conversion timeout

## Usage

```python
from sin_code_bundle import markitdown

# Health
print(markitdown.doctor())

# Convert a PDF to Markdown
md = markitdown.convert("docs/spec.pdf")

# Wire into every supported agent
print(markitdown.setup_agents())
```

## Known caveats

- `markitdown` runs as a subprocess with the calling process's full
  file-IO rights. Never pass untrusted input directly — see
  `markitdown.convert_local()` upstream for a sandboxed variant.
- `_wire_*` wirers overwrite existing `markitdown` MCP entries on the
  same key; they do not merge per-field changes.
- The CLI's `markitdown[all]` extra is recommended for OCR/image
  support; the bundle does not enforce it.
