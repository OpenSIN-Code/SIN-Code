# Purpose: Doc companion for checkpoint.py — what it does and when to use it.
# Docs: checkpoint.py

# `checkpoint.py` — Pre-refactor checkpoint orchestrator

## What it does

`Checkpointer` creates a **recoverable snapshot + state report** in a
single call. Use before any risky refactor to guarantee a clean rollback
path AND see what the working tree looks like right now.

## Sub-checks

| Section         | Source                          | Purpose                          |
|-----------------|---------------------------------|----------------------------------|
| `snapshot_id`   | `sin-honcho-rollback snapshot`  | Recoverable rollback point       |
| `docs_broken`   | `codocs.find_broken`            | Broken `.doc.md` references      |
| `git_clean`     | `git status --porcelain`        | Clean working tree?              |
| `usages_found`  | `scout` (or `grep` fallback)    | How many places reference `name` |
| `tests_status`  | `pytest --collect-only`         | Can the test suite be collected? |

## When to use

Before any risky edit (refactor, mass rename, dependency upgrade).
Replaces the manual chain:

```python
rollback_snapshot("before-x", description="...")
sin_bash("sin codocs check")
sin_bash("git status")
sin_search("X", search_type="usage")
sin_bash("pytest --collect-only")
```

with a single call:

```python
sin_checkpoint("before-auth-refactor",
               include="snapshot,docs,git,usages,tests",
               description="Migrate to new JWT lib")
# → {
#     "checkpoint_name": "before-auth-refactor",
#     "snapshot_id": "snap-2026-06-05-abc123",
#     "docs_broken": 0,
#     "git_clean": true,
#     "git_changes_count": 0,
#     "usages_found": 7,
#     "tests_status": "pass",
#     "tests_collected": "42 tests collected"
#   }
```

## Idempotency

Calling `sin_checkpoint("before-x")` twice with the same name is safe.
`sin-honcho-rollback` deduplicates on the name, so the second call returns
the **existing** snapshot id (no new snapshot row, no extra disk usage).

## Graceful degradation

Each sub-check is independent. A missing `sin-honcho-rollback` (no
snapshot) still produces a useful state report. A missing `scout` falls
back to `grep`. Missing `pytest` sets `tests_status = "skipped"`. **Never
crashes the MCP.**

## Caveats

- `usages_found` is a *file count* under the grep fallback, an
  *occurrence count* under scout. Read the field name carefully when
  comparing numbers across installations.
- `pytest --collect-only` is bounded by a 15-second timeout — large
  suites may be marked `"timeout"`.
- The snapshot database lives at `.sin/rollback.db` by default; pass a
  custom path via the `db_path` constructor argument for monorepos.
