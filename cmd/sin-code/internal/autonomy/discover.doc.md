# `discover.doc.md` — Autonomous Backlog Discovery

Turns latent work in a repository into queued goals so the daemon finds its own
work instead of waiting for a human prompt. Part of the loop-engineering layer.

## What it does

- **Scans source comments** for `TODO`/`FIXME`/`XXX`/`HACK` markers (comment
  prefixes only, so string literals containing the word are not false positives).
- **Scans `MASTER_TODO.md`** for unchecked GitHub-style checklist items
  (`- [ ]` / `* [ ]`).
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
- **CLI:** `sin-code goal discover [--dry-run] [--retries N]`.

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

- **No GH-issue source yet.** Designed to extend (add a source + dedup key);
  current sources are comments and MASTER_TODO.
- **Marker scan is line-based.** Block comments spanning lines only match on the
  line bearing the marker.
- **A re-opened TODO after a verified goal re-enqueues.** This is intentional —
  the work reappeared — but can surprise if the marker is left in deliberately.
