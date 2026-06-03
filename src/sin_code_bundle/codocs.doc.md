# codocs.py

CoDocs — Co-located Docs Standard validator. Each code file may declare
a companion `.doc.md` file via a first-line reference comment; this
module finds and verifies those references. Also performs a SOTA
inline-doc compliance check (Purpose line or module docstring).

## Dependencies

- stdlib: `re`, `dataclasses`, `pathlib`

## Touched by

- `cli.py` — exposed as `sin codocs check`, `sin codocs list`,
  `sin codocs check-inline`, `sin codocs install-skill`
- `install.sh` — runs `sin codocs check-inline` as part of the bundle's
  own CI

## What it does

1. **`scan(root)`** — walks the tree (excluding `DEFAULT_EXCLUDE` dirs and
   non-code files) and returns every `DocReference` it finds.
2. **`find_broken(root)`** — `scan()` filtered to references whose target
   `.doc.md` does not exist.
3. **`check_inline_docs(root)`** — flags files missing a Purpose comment
   or module docstring in the first `_HEAD_LINES` (5) lines.
4. **`_check_inline_docs_json(root)`** — JSON form for `sin codocs
   check-inline --json`.

## Important config

- `DEFAULT_EXCLUDE` — directories never scanned (`.git`, `__pycache__`, …)
- `CODE_SUFFIXES` — file extensions considered "code" (Python, TS, Rust, …)
- `CODE_FILENAMES` — extension-less code files (`Makefile`, `Dockerfile`)
- `_HEAD_LINES = 5` — lines inspected for a `Docs:` reference; tolerates
  shebang / license header
- `_DOCS_RE` — single regex that handles all comment leaders
  (`#`, `//`, `/*`, `*`, `--`, `;`)

## Usage

```python
from sin_code_bundle.codocs import find_broken, check_inline_docs

broken = find_broken("/path/to/repo")
for ref in broken:
    print(f"{ref.source} → {ref.doc} (missing)")

issues = check_inline_docs("/path/to/repo")
for issue in issues:
    print(f"{issue.path}: {issue.kind} — {issue.detail}")
```

## Known caveats

- Only the *first* matching `Docs:` reference per file is returned;
  multiple references in one file are not supported.
- `_extract_reference` only inspects the first `_HEAD_LINES`; a `Docs:`
  comment deeper in the file is silently ignored.
- `check_inline_docs` is conservative: it flags any code file without a
  recognized header in the first 5 lines, even if the rest of the file
  is heavily commented.
