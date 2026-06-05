# merge_safety.py

Pre-merge / pre-PR safety gate. Call this BEFORE every `git push` to
`main` and BEFORE every `gh pr create`.

## What it does

`sin_merge_safety` runs 4 independent checks in one call:

1. **CoDocs coverage** — broken `.doc.md` references (blocker)
2. **ceo-audit grade** — QUICK profile, cached 5 min per (profile, base, head)
   - Grade F → blocker
   - Grade D → warning
   - ceo-audit not runnable → warning (don't block)
3. **git diff stat + secret scan** — line count + cheap substring regex
   - >1000 lines changed → warning
   - Any `sk-`/`ghp_`/`AIza`/`AKIA`/etc. or `key=value` pattern → blocker
4. **Working tree state** — clean or dirty (warning if dirty)

Returns ``pass: bool`` plus human-readable ``blockers`` and ``warnings``
so the agent can decide in one read.

## Files that touch this

- `mcp_server.py` — exposes `sin_merge_safety` MCP tool
- `~/.config/opencode/hooks/pre-commit.sh` — optionally calls this before
  every commit (configured in opencode.json)
- `~/.config/opencode/agents/SIN-Zeus.md` — agent rule "use sin_merge_safety
  before every PR/merge"

## Inputs

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `base` | str | `"main"` | Base ref for the diff |
| `head` | str | `"HEAD"` | Head ref for the diff |
| `profile` | str | `"QUICK"` | ceo-audit profile (QUICK, RELEASE, FULL, SECURITY) |

## Outputs

```json
{
  "pass": false,
  "verdict": "FIX_FIRST",
  "blockers": [
    "Diff contains possible secrets (1 line(s)) — rotate keys and re-commit before merge"
  ],
  "warnings": [
    "Working tree is dirty (3 change(s)) — commit/stash before merge"
  ],
  "checks": {
    "codocs": {"ok": true, "broken": 0},
    "ceo_audit": {"ok": true, "grade": "B", "report_path": "...", "cache_hit": true},
    "diff": {"ok": true, "lines_changed": 142, "shortstat": " 3 files changed, ...", "secret_hits": ["line 42: contains 'sk-'"]},
    "working_tree": {"ok": true, "clean": false, "changes_count": 3}
  },
  "base": "main",
  "head": "HEAD",
  "profile": "QUICK",
  "timestamp": "2026-06-05T..."
}
```

## Verdict logic

| Condition | Verdict |
|-----------|---------|
| Any blocker present | `FIX_FIRST` (pass=false) |
| No blockers | `READY` (pass=true) |

## Caching

The ceo-audit result is cached in-process for 5 minutes per
``(profile, base, head)`` triple. Repeated calls during a single
pre-PR review are essentially free (just the diff + CoDocs + tree
check, which together run in <1s).

## Known caveats

- **Secret scanner is a coarse heuristic.** It will miss rotated or
  encoded secrets and produce false positives on legitimate key-looking
  strings. Always pair with `gitleaks` or a proper secret-scanner in CI.
- **5-minute cache is per-process.** A fresh MCP server (or a fresh
  call from a different agent) will re-run the audit. If you need
  cross-process caching, run ceo-audit separately and pass the result
  in via the cache.
- **Diff ceiling is 30s.** A 100k-line diff (e.g. a vendor rebase) will
  timeout the secret scan; the result will be ``diff.ok=false`` with
  ``error: "git diff timeout"`` and the merge will be allowed (the
  warning "ceo-audit could not run" path).
