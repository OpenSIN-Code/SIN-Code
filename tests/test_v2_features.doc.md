# test_v2_features.py

**Purpose:** Tests for SIN-Code v2 features (VFS, Hashline, Memory, AST).

**Docs:** test_v2_features.doc.md

## What it tests

| Group | Tests | Covers |
|-------|-------|--------|
| VFS | 4 | URI scheme registration, invalid/unknown URIs, caching |
| Hashline | 5 | Anchor finding, patch creation/apply, staleness detection, atomic file writes |
| Memory | 8 | retain/recall/forget, tag filtering, stats, reflect, Honcho graceful degradation |
| AST | 2 | Lazy import without tree-sitter, error messages |

**Total: 19 tests, 100% pass rate.**

## Dependencies
- `pytest` (test runner)
- `sin_code_bundle.vfs`, `sin_code_bundle.hashline`, `sin_code_bundle.memory`, `sin_code_bundle.ast_edit` (SUT)
- `tmp_path` fixture (pytest built-in)

## Running

```bash
PYTHONPATH=src python3 -m pytest tests/test_v2_features.py -v
# or
PYTHONPATH=src python3 -m pytest tests/ -q  # full bundle suite
```

## Test design notes

- **No external deps required** — all tests use SQLite (in `tmp_path`) + lazy imports
- **Honcho tests are mocked** — `localhost:1` ensures connection failure, not actual server
- **AST tests verify lazy import** — confirms tree-sitter is optional, not required
- **Tmp dir isolation** — each test gets fresh `tmp_path` to avoid cross-contamination

## Touched by
SIN-Code maintainers only.
