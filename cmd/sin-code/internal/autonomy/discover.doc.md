# `discover.doc.md` — Autonomous Backlog Discovery

Turns latent work in a repository into queued goals so the daemon finds its own
work instead of waiting for a human prompt. Part of the loop-engineering layer.

## What it does

- **Scans source comments** for `TODO`/`FIXME`/`XXX`/`HACK` markers (comment
  prefixes only, so string literals containing the word are not false positives).
- **Scans `MASTER_TODO.md`** for unchecked GitHub-style checklist items
  (`- [ ]` / `* [ ]`).
- **Scans open GitHub issues** (loop-003) via the REST API when
  `ScanGitHubIssues` is set — owner/repo auto-detected from the `origin` git
  remote, auth from `GH_TOKEN`/`GITHUB_TOKEN`. Pull requests are filtered out.
  See `discover_github.go`.
- **Scans failing CI check runs** (loop-008) for a commit when `ScanCIChecks`
  is set, turning each failure into a high-priority fix-goal. See
  `discover_ci.go`.
- **Deduplicates** each finding with a stable `DedupKey` so repeated scans never
  pile up duplicates of the same item.
- **EnqueueFindings** persists findings via `Queue.AddDiscovered`, which skips
  any finding whose key already has a live (non-terminal) goal.

## Dedup semantics

- Key for comments: `<marker>:<relpath>:<note>` — path-scoped, so the same note
  in two files is two findings.
- Key for MASTER_TODO: `master_todo:<normalized item text>`.
- A key is only suppressed while a matching goal is **not** in
  `verified`/`failed`/`exhausted`. After a goal terminates, a re-discovered item
  may legitimately enqueue again.
- An empty key never dedupes.

## Surfaces

- **Trigger:** `{"type":"discover","every":"30m"}` in `.sin-code/triggers.json`
  (handled by `autonomy.Runner.runDiscover`; runs once on startup then on the tick).
  Per-source toggles: `scan_comments`, `scan_master`, `scan_github_issues`,
  `github_labels`, `scan_ci_checks`, `ci_branch`.
- **CLI:** `sin-code goal discover [--dry-run] [--retries N] [--github-issues
  [--github-labels …]] [--ci-checks [--ci-branch …]]`.

## Files that import / touch it

- `cmd/sin-code/internal/autonomy/triggers.go` — `discover` trigger type.
- `cmd/sin-code/internal/autonomy/queue.go` — `AddDiscovered` + `dedup_key` column.
- `cmd/sin-code/goal_cmd.go` — `goal discover` command.

## Important limits

| Config | Default | Meaning |
|---|---|---|
| `MaxFindings` | 50 | cap per scan |
| `MaxRetries` | 3 | retry budget for enqueued goals |
| `discover` trigger `every` | — | must be ≥ 1m |

- Skips `.git`, `node_modules`, `.sin-code`, `vendor`, `dist`, `build`.
- Scanned extensions: common source files only (see `codeExts`); docs/config skipped.

## Known caveats / footguns

- **GitHub/CI sources are network-bound and fail-open.** A failed API call logs
  a warning and returns the local findings; it never aborts a scan.
- **Marker scan is line-based.** Block comments spanning lines only match on the
  line bearing the marker.
- **A re-opened TODO after a verified goal re-enqueues.** This is intentional —
  the work reappeared — but can surprise if the marker is left in deliberately.
