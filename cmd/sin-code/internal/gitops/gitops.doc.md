# `gitops.doc.md` — Autonomous Commit / Push / PR

Commits verified agent work to git so the human never has to. Part of the
SIN-Code Loop System (loop-007). Invoked by the daemon after a goal passes the
stop-gate when `--auto-commit` (or `SIN_AUTO_COMMIT=1`) is set.

## What it does

`AutoCommit(ctx, CommitOptions)`:

1. **Stages** everything in the workspace (`git add -A`).
2. **No-ops cleanly** when there is nothing to commit (returns nil).
3. **Commits** with the provided message. Identity falls back to a bot identity
   (`SIN_GIT_AUTHOR_NAME`/`_EMAIL`, default `sin-code-agent`) via per-invocation
   `-c user.name=… -c user.email=…` so it works in a fresh repo with no config.
4. **Pushes** to `PushRemote` (current branch, `--set-upstream`) when set.
5. **Opens a PR** via the GitHub REST API when `CreatePR` is set (requires
   `GH_TOKEN`/`GITHUB_TOKEN`; owner/repo/base auto-detected). Skipped silently
   when no token is present.

## CommitOptions

| Field | Meaning |
|---|---|
| `Workspace` | repo root (required) |
| `Message` | full commit message |
| `PushRemote` | remote to push to (empty = commit only) |
| `CreatePR` | open a PR after push |
| `PRTitle` / `PRBody` | PR metadata |
| `BaseBranch` | PR base (empty = repo default/`main`) |

## Surfaces

- **Daemon flags:** `--auto-commit`, `--push-remote`, `--open-pr` (the latter
  implies `--push-remote=origin`). Env: `SIN_AUTO_COMMIT=1`.
- Commit message format:
  `feat(agent): complete goal #<id> in <n> turns\n\n<summary>\n\n[sin-code goal-id: <id>]`.

## Important limits / footguns

- **Fail-soft in the daemon.** A commit/push/PR error logs a warning and never
  blocks goal finalization — the loop keeps moving.
- **Never force-pushes.** A rejected push surfaces as a warning; resolve by hand.
- **Respects mandate M3.** Auto-commit only runs *after* the verify-gate and
  stop-gate have both passed.
- **No branch creation here.** It commits on the current branch; create/checkout
  a feature branch upstream if you don't want commits on the checked-out branch.
