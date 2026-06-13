# `update_rollback.doc.md` — Rollback Logic

Restores a previous snapshot after a failed or undesired update.

## What it does
- **Discovers latest snapshot** via `BackupManager.Latest()`.
- **Reads manifest** to identify pre-update Go binary versions.
- **Restores each binary** from the snapshot dir to `$SIN_CODE_BIN_DIR`.
- **Handles partial restores**: missing backup files are logged as warnings,
  not fatal errors.

## Restore sequence
1. If no snapshot exists → prints "No snapshot to rollback to.", exits 0.
2. Loads `manifest.json` from the snapshot dir.
3. For each Go binary in `manifest.Pre.GoBins`:
   - Copies `<snapshot>/<tool>` → `<binDir>/<tool>` with `0755` perms.
   - If the source file is missing → warning, Failed++, continues.
4. Reports summary: Updated count, Failed count.

## Safety guarantees
- **Never deletes the snapshot dir** — rollback is read-only on the source.
- **Writes to binDir only** — pipx packages are NOT restored (pipx has its own
  pin/rollback mechanism).
- **Non-fatal on partial restore**: one missing backup doesn't block others.
