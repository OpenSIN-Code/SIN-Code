# lsp_bootstrap.py

Detects the languages used in a repo and reports which LSP servers are
installed and which are missing. Used by `sin doctor` to give users an
exact install command for accurate impact analysis.

## Dependencies

- stdlib: `shutil`, `collections.Counter`, `pathlib`

## Touched by

- `cli.py` — `sin doctor` calls `server_status()` and prints it
- `lsp_backend.py` (loose coupling) — uses the same `_LANG_BY_EXT`
  mapping as a separate constant; both must stay in sync

## What it does

1. **`detect_languages(root)`** — rglobs the tree, counts files per
   language (via extension map), returns the counter most_common() first.
2. **`server_status(root)`** — for every detected language, returns a
   row with: language, file count, server binary name, installed bool,
   and an `install_hint` (the exact command to run).

## Important config

- `SERVERS` — language → (binary, install hint) catalog. If a language
  has no entry, `install_hint` defaults to `"no LSP integration yet"`.
- `_EXT_LANG` — extension → language map; covers Python, TS/JS, Go,
  Rust, Java.
- `_IGNORE` — dirs never counted (`.git`, `node_modules`, `.venv`,
  `__pycache__`, `.sin`).

## Usage

```python
from pathlib import Path
from sin_code_bundle.lsp_bootstrap import server_status

for row in server_status(Path(".")):
    status = "✓" if row["installed"] else "✗"
    print(f"  {status}  {row['language']:<10}  {row['files']:>4} files  → {row['install_hint']}")
```

## Known caveats

- `shutil.which()` searches `PATH` only. If the user installs a server
  in a non-standard location, it will be reported as "not installed".
- Detection is purely extension-based; mixed-language repos (Python
  with embedded JS templates) will count both.
- The `install_hint` is plain text, **not a copyable command**; do not
  pipe it directly into a shell.
