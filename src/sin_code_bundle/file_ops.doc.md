# file_ops.py — File Operations Core

## What does this file do?

Single source of truth for the 5 core file-ops (`sin_read`, `sin_write`,
`sin_edit`, `sin_bash`, `sin_search`) that replace opencode's native
read/write/edit/bash/search. Both the MCP server (`mcp_server.py`) and
the standalone CLI shims (`cli/sin_*.py`) call into this module — there
is exactly ONE implementation of each operation.

## Which other files import / touch it?

| Importer | Why |
|----------|-----|
| `mcp_server.py` | `@mcp.tool()` definitions call the functions directly |
| `cli/sin_read.py` | `sin-read` console script |
| `cli/sin_write.py` | `sin-write` console script |
| `cli/sin_edit.py` | `sin-edit` console script |
| `cli/sin_bash.py` | `sin-bash` console script |
| `cli/sin_search.py` | `sin-search` console script |

## Important config values & limits

- `sin_read.max_chars` default 50000 — files larger than this are truncated
  to head + tail (`max_chars // 2` each).
- `sin_bash.timeout` default 60s, max 600s — Python wraps the `execute`
  Go tool with a `timeout + 10s` buffer.
- `sin_search` Python-regex fallback has a hard ceiling of 200 results
  to prevent context flooding.
- `sin_write` only pre-validates `.py` files via `compile()`. Other
  languages (`.ts`, `.js`, `.go`) require the AST-edit tool.

## Why certain decisions were made

- **Why ONE module, not five?** — DRY. Two callers (MCP + CLI) need
  identical behavior. Inlining the logic in each CLI would create
  drift within days.
- **Why atomic rename instead of `os.replace`?** — `Path.replace()` is
  atomic on POSIX and Windows. A crash mid-write leaves the old file
  untouched, not a partial blob.
- **Why auto-fallback to raw shell in `sin_bash`?** — Keeps the tool
  usable on bare Python venvs that don't have the `execute` Go binary
  installed. We mark `redacted: false` so callers know secrets are
  not being masked.

## Usage examples

```python
from sin_code_bundle.file_ops import sin_read, sin_write, sin_edit, sin_bash, sin_search

# Read with size safety
print(sin_read("/path/to/file.py", summarize=True))

# Atomic write with backup
print(sin_write("/tmp/x.py", "print('hi')"))

# Hashline-anchored edit
print(sin_edit("/tmp/x.py", "print('hi')", "print('hello')", intent="rename"))

# Safe shell exec (60s default timeout)
print(sin_bash("ls -la"))

# Semantic search
print(sin_search("def main", path="/path/to/repo", search_type="regex"))
```

## Known caveats or footguns

- `sin_read` on a directory returns a JSON listing of children (sorted
  by relative path), not an error. Pass a file path for content.
- `sin_edit` requires the `old_content` to be unique in the file
  (after hashline normalization). If it matches multiple locations,
  the patcher will refuse and return `anchor not found`.
- `sin_bash` with no `execute` binary returns `redacted: false` — the
  caller is responsible for not leaking secrets to the raw shell.
