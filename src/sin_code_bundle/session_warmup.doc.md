# session_warmup.py

One-call session context primer. Call this ONCE at the start of every
agent session before any code edits.

## What it does

`sin_session_warmup` is the first call an agent should make when entering
a repository. It assembles 5 independent signals in one shot:

1. **Git state** — current branch, clean/dirty, number of uncommitted changes
2. **CoDocs coverage** — broken `.doc.md` references in the working tree
3. **ceo-audit grade** — runs the QUICK profile (~30s) and returns the grade
4. **Top risks** — heuristic scan for the 5 largest Python files (cheap
   proxy for "where could go wrong")
5. **Last commit age** — human-readable string ("3 days ago")

Then it composes a single ``session_recommendation`` string so the agent
can decide "ready" vs "fix first" in one read.

## Files that touch this

- `mcp_server.py` — exposes `sin_session_warmup` MCP tool
- `~/.config/opencode/agents/SIN-Zeus.md` — agent rule "call sin_session_warmup
  first thing on every new repo"
- `sin_programming_workflow.py` — `action=session_warmup` is a thin wrapper
  around this module

## Inputs

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `repo_path` | str | `"."` | Path to the repo (passed through by the MCP wrapper) |

## Outputs

```json
{
  "branch": "main",
  "git_state": "dirty",
  "git_changes_count": 3,
  "last_commit_age": "2 hours ago",
  "codocs_coverage": {"ok": true, "broken": 0, "checked": "auto"},
  "ceo_audit_grade": "B",
  "ceo_audit_path": "~/ceo-audits/repo-2026-06-05/report.md",
  "top_risks": [
    {"path": "src/sin_code_bundle/mcp_server.py", "lines": 850},
    ...
  ],
  "session_recommendation": "READY — proceed with coding",
  "signals": { "git": {...}, "codocs": {...}, "ceo_audit": {...}, ... },
  "timestamp": "2026-06-05T..."
}
```

## Recommendation logic

| Condition | Verdict |
|-----------|---------|
| ceo-audit grade = F | `BLOCK — ceo-audit grade F. Fix critical issues first.` |
| ceo-audit grade = D OR (broken docs AND top risks present) | `FIX — improve docs/quality before coding` |
| Working tree dirty | `STASH or COMMIT first — working tree dirty` |
| Otherwise | `READY — proceed with coding` |

## Known caveats

- **ceo-audit ceiling is 3 minutes.** A failing or absent `sin` CLI returns
  `ceo_audit_grade: null` — the session_recommendation will then fall
  through to the dirty-tree check or to `READY`. Install the full bundle
  (`pip install sin-code-bundle[ceo-audit]`) for proper coverage.
- **Top-risks heuristic is intentionally cheap.** It's just "5 largest
  Python files". For real architectural-debt signals, run ceo-audit
  with the FULL profile or call `sin_session_warmup` after the agent
  has already done its own code archaeology.
- **No network calls.** Everything is local — safe to call on air-gapped
  machines or in CI containers without internet.
