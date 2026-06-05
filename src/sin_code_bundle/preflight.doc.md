# Purpose: Doc companion for preflight.py — what it does and when to use it.
# Docs: preflight.py

# `preflight.py` — Pre-flight safety gate

## What it does

`PreflightChecker` runs **four independent safety checks** in a single call
and returns a unified verdict:

1. **Policy** — same `SINInterceptor` engine as `sin_check_architecture`.
2. **Docs** — `codocs.find_broken` (broken `.doc.md` references).
3. **Git** — `git status --porcelain` (clean working tree?).
4. **Tests** — `pytest --collect-only` (collection succeeds?).

It then derives an `estimated_risk` (`low` / `medium` / `high`) and a final
`allowed` boolean (block when `high`).

## When to use

Run **before** any state-changing call: `sin_write`, `sin_edit`, `sin_bash`,
`sin_ast_edit`. Replaces the manual chain:

```python
sin_check_architecture(tool_name, tool_input)   # policy
sin_bash("sin codocs check")                     # docs
sin_bash("git status")                           # git
sin_bash("pytest --collect-only")                # tests
```

with a single call:

```python
sin_preflight("sin_write", {"path": "auth/handler.py", "content": "..."})
# → {
#     "tool_name": "sin_write",
#     "allowed": true,
#     "policy_ok": true,
#     "docs_ok": true,
#     "git_clean": false,
#     "tests_status": "pass",
#     "estimated_risk": "medium",
#     "violations": [],
#     "details": {"git_changes_count": 3, "tests_collected": "42 tests collected"}
#   }
```

## Graceful degradation

Every check is wrapped in `try/except`. If a check fails (e.g. git not
installed, pytest missing, codocs module absent), the failure is recorded in
`details` and the call still returns. **Never crashes the MCP.**

## Risk scoring

| Signals failing | `estimated_risk` | `allowed` |
|-----------------|------------------|-----------|
| 0 | `low` | `true` |
| 1-2 | `medium` | `true` |
| 3+ | `high` | `false` |

A "signal" is one of: `policy_ok=false`, `docs_ok=false`, `git_clean=false`,
`tests_status=fail`, `len(violations) > 0`.

## Caveats

- `pytest --collect-only` may take a few seconds on large projects
  (15-second timeout enforced).
- Git check is skipped silently outside git repositories.
- The risk score is a heuristic — a `medium` verdict is **not** a
  recommendation to skip the change; it just means the agent should
  pay attention.
