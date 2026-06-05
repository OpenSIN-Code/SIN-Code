# programming_workflow.py

One-call orchestration of the common programming workflows. The agent
picks an action, the tool fans out to the right combination of underlying
`sin_*` tools, and returns a single structured verdict.

## What it does

`sin_programming_workflow` exposes 6 actions behind a single MCP tool:

| Action | Underlying calls | Verdict options |
|--------|------------------|-----------------|
| `pre_write` | sin_read + sin_preflight | READY / FIX_FIRST |
| `write` | sin_preflight + sin_write | PROCEED / BLOCK |
| `post_write` | sin_preflight + codocs_check + pytest --collect-only | READY / FIX_FIRST |
| `pre_commit` | sin_checkpoint + git_status + codocs_check + ceo-audit (cached) | READY_TO_COMMIT / FIX_FIRST |
| `refactor` | sin_checkpoint + gitnexus_impact + gitnexus_detect_changes | PROCEED / REVIEW / FIX_FIRST |
| `session_warmup` | sin_session_warmup | (full snapshot) |

Each action returns:

```json
{
  "action": "pre_commit",
  "verdict": "READY_TO_COMMIT",
  "suggested_message": "feat: add merge_safety module",
  "blockers": [],
  "steps": [
    {"name": "sin_checkpoint", "ok": true, "snapshot_id": "snap-..."},
    {"name": "git_status", "ok": true, "clean": true, "changes_count": 0},
    {"name": "codocs_check", "ok": true, "broken": 0},
    {"name": "ceo_audit", "ok": true, "grade": "B", "cache_hit": false}
  ],
  "base": "main",
  "head": "HEAD",
  "timestamp": "2026-06-05T..."
}
```

## Files that touch this

- `mcp_server.py` — exposes `sin_programming_workflow` MCP tool
- `~/.config/opencode/hooks/pre-commit.sh` — calls `action=pre_commit`
- `~/.config/opencode/hooks/post-write.sh` — calls `action=post_write`
- `~/.config/opencode/hooks/pre-write.sh` — calls `action=pre_write`

## Inputs

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `action` | str | (required) | One of pre_write, write, post_write, pre_commit, refactor, session_warmup |
| `target` | str | `""` | File path (pre_write/write/post_write) or symbol name (refactor) |
| `content` | str | `""` | File content (write only) |
| `message` | str | `""` | Commit message (pre_commit only — generates a suggestion if empty) |
| `checkpoint_name` | str | `""` | Snapshot name (pre_commit / refactor — auto-generated if empty) |
| `base` | str | `"main"` | Base ref (pre_commit) |
| `head` | str | `"HEAD"` | Head ref (pre_commit) |

## Caching

The `pre_commit` action caches the ceo-audit result in-process for
5 minutes per ``(profile, base, head)`` triple. A dry-run + a real
commit made back-to-back will only run the audit once.

## Suggested message heuristic (pre_commit)

When `message` is empty, the tool auto-generates a Conventional Commits
string from `git diff --name-only HEAD`:

- All test files → `test: update tests for <file>`
- All doc files → `docs: update <file>`
- New file → `feat: add <name>`
- Otherwise → `chore: update N file(s)`

Always review and edit the suggestion before committing.

## Known caveats

- **Cross-tool imports are best-effort.** If `gitnexus` is not
  installed, the `refactor` action returns ``ok=false`` for the
  impact step (verdict falls through to PROCEED).
- **Verdict is a heuristic.** A `READY` verdict does not guarantee
  the commit is safe — always run `sin_merge_safety` before pushing.
- **Suggested message is dumb.** It looks at file paths only. For
  semantically meaningful commit messages, the agent should write
  its own and pass it via `message=...`.
