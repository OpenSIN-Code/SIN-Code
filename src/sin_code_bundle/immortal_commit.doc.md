# immortal_commit.py

One-call immortal commit — Conventional Commits + tag + push in 1 MCP tool call.

## What it does

`sin_immortal_commit` is the single tool agents should use INSTEAD of running
raw `git commit` + `git tag` + `git push`. It enforces four rules in one go:

1. **Conventional Commits** — the message must match `type(scope): subject`
   (subject >= 5 chars). Valid types: `feat`, `fix`, `docs`, `chore`,
   `style`, `test`, `refactor`, `perf`, `ci`, `build`.
2. **No secrets in message** — cheap substring scan for `BEGIN PRIVATE KEY`,
   `sk-`, `ghp_`, `github_pat_`, `xoxb-`/`xoxp-`, `AIza`, `AKIA`/`ASIA`.
3. **Main only** — refuses to run on any branch other than `main` (configurable
   via `main_branch=...`).
4. **Pre-commit snapshot** — creates a `sin-honcho-rollback snapshot` so the
   user can roll back to the pre-commit state if needed. Independent of the
   commit; failure is non-fatal.

After validation it runs `git add -A`, `git commit`, optional `git tag -a`,
and `git push origin <branch>` (and `git push origin <tag>` if tagged).

## Files that touch this

- `mcp_server.py` — exposes `sin_immortal_commit` MCP tool
- `~/.config/opencode/hooks/post-commit.sh` — calls this on every commit
  completion (for the post-tag + push phase)
- `~/.config/opencode/agents/SIN-Zeus.md` — agent rule "use sin_immortal_commit
  instead of raw git commit"

## Inputs

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `message` | str | (required) | Conventional Commits message |
| `tag` | str | `""` | Optional annotated tag (e.g. `v0.8.0`) |
| `push` | bool | `True` | Push to origin after commit |
| `force_main` | bool | `True` | Refuse to run on any branch other than `main` |
| `main_branch` | str | `"main"` | Branch name to enforce (override for `master`) |
| `snapshot_first` | bool | `True` | Create a pre-commit rollback snapshot |

## Outputs

```json
{
  "success": true,
  "sha": "abc123...",
  "branch": "main",
  "tag": "v0.8.0",
  "pushed": true,
  "warnings": [],
  "steps": [
    {"step": "validate_format", "ok": true},
    {"step": "secret_scan", "ok": true},
    {"step": "branch_check", "ok": true, "branch": "main"},
    {"step": "snapshot", "ok": true, "id": "snap-..."},
    {"step": "git_add", "ok": true},
    {"step": "git_commit", "ok": true},
    {"step": "git_tag", "ok": true},
    {"step": "git_push", "ok": true},
    {"step": "git_push_tag", "ok": true}
  ],
  "snapshot": {"ok": true, "snapshot_id": "snap-..."},
  "timestamp": "2026-06-05T..."
}
```

## Error cases

| Error | HTTP-ish status | Recoverable? |
|-------|-----------------|--------------|
| Message not Conventional Commits format | 400 | Fix message, retry |
| Possible secret in message | 400 | Remove secret, retry |
| On non-main branch (force_main=True) | 409 | `git checkout main` first |
| Working tree clean after `git add` | 409 | Nothing to commit (soft error) |
| `git push` failed | 502 | Resolve remote issue, retry |
| `sin-honcho-rollback` not installed | (warning) | Tool still works, just no snapshot |

## Known caveats

- **Snapshot is best-effort.** If `sin-honcho-rollback` is not on PATH (and
  the fallback location doesn't have it), the snapshot step is skipped
  silently — the commit still happens. Install `sin-honcho-rollback` from
  `pip install sin-brain` for full coverage.
- **No interactive rebase / amend.** The tool is intentionally narrow: it
  only does "add + commit + optional tag + push". For amend/squash, fall
  back to raw `git` via `sin_bash` (with a clear comment why).
- **Dirty tree is allowed.** Per the AUTONOMOUS-AGENT mandate, the tool does
  NOT block when the working tree has uncommitted changes — it just adds
  them all to the commit. A warning is included in the result so the user
  can see what was bundled.
