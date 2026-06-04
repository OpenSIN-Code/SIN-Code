# test_v2_features.py

**Purpose:** Tests for SIN-Code v2 features (VFS, Hashline, AST).

**Docs:** test_v2_features.doc.md

## What it tests

| Group | Tests | Covers |
|-------|-------|--------|
| VFS | 4 | URI scheme registration, invalid/unknown URIs, caching |
| Hashline | 5 | Anchor finding, patch creation/apply, staleness detection, atomic file writes |
| AST | 2 | Lazy import without tree-sitter, error messages |

**Total: 11 tests, 100% pass rate.**

## Dependencies
- `pytest` (test runner)
- `sin_code_bundle.vfs`, `sin_code_bundle.hashline`, `sin_code_bundle.ast_edit` (SUT)
- `tmp_path` fixture (pytest built-in)

## Note on memory tests

Memory tests for the `sin-code-bundle` adapter live in `tests/test_memory.py`
and exercise the thin pass-through to `sin_brain.mcp_tools` with a fake
`sin_brain` injected into `sys.modules`. The previous in-bundle `SINMemory` /
`HonchoBackend` integration tests were removed because those classes were
moved to the external `sin-brain` package (commit af69464, BR-1, Issue #14).
The bundle no longer has an in-process SQLite store or a Honcho client.

## Running

```bash
PYTHONPATH=src python3 -m pytest tests/test_v2_features.py -v
# or
PYTHONPATH=src python3 -m pytest tests/ -q  # full bundle suite
```

## Test design notes

- **No external deps required** — all tests use lazy imports; missing
  optional packages degrade to `is_available() is False` instead of erroring.
- **AST tests verify lazy import** — confirms tree-sitter is optional, not required
- **Tmp dir isolation** — each test gets fresh `tmp_path` to avoid cross-contamination

## Touched by
SIN-Code maintainers only.
