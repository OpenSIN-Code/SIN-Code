# `update_backup.doc.md` — BackupManager Snapshot Lifecycle

Creates, lists, prunes update snapshots in the state directory.

## What it does
- **Create** snapshot directories under `~/.local/state/sin-code/updates/<timestamp>/`.
- **List** all snapshots, newest first (reverse-sorted by timestamp).
- **Latest** returns the most recent snapshot dir.
- **Prune** keeps at most N snapshots, removing older ones (best-effort).

## State root convention
- Default: `~/.local/state/sin-code/` (based on `$HOME`).
- Override: set `SIN_CODE_STATE_ROOT` env var.
- Snapshot path: `<StateRoot>/updates/<timestamp>/`

## Who calls what
- `runUpdate()` in update_cmd.go calls `Create()` before phases.
- `runRollback()` in update_rollback.go calls `Latest()` to find restore target.
- `runUpdate()` calls `Prune()` after successful update.

## Known caveats
- **Timestamps must be sortable**: `defaultNow()` returns Unix epoch seconds.
  Tests inject `Now` to control ordering.
- **Prune is non-fatal**: disk-full or permission errors during `RemoveAll` are
  silently swallowed.
