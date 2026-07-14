# Standards — skill-process-disk-clean

## 1. Always measure before and after

Print `df -h /` at the start and end. Report capacity % and GB freed. Never
claim space was reclaimed without proof.

## 2. Classify every target before deleting

| Class | Definition | Action |
|-------|------------|--------|
| safe | Regenerable cache, no user state | delete immediately |
| ask | Auth tokens, state backups, old sessions the user may want | ask first |
| locked | `opencode.db` while opencode holds a write lock | stop processes, then VACUUM |

## 3. Never touch source or secrets

Forbidden under all circumstances:

- `~/.git`, any project `.git`
- `~/.ssh`
- Project source trees
- `~/.local/share/opencode/opencode.db` (the file itself — only VACUUM it)

## 4. opencode.db VACUUM protocol

The DB uses WAL mode. Deleted rows accumulate in the freelist and are never
returned to the OS until `VACUUM`. A 60 GB file may hold only ~6 GB of real
data.

Steps:

1. Stop ALL opencode processes — `VACUUM` hangs while a lock is held:
   ```bash
   pkill -9 -f opencode
   sleep 3
   ```
2. Confirm no opencode process remains: `ps aux | grep opencode | grep -v grep`
3. Run: `sqlite3 ~/.local/share/opencode/opencode.db "VACUUM;"`
4. Takes 2–3 minutes for a 60 GB file. Verify size dropped.

`VACUUM` does **not** delete sessions — it only releases empty freelist pages.

## 5. Safe targets (regenerable, delete without asking)

- `~/Library/Caches/Yarn/`
- `~/Library/Caches/go-build/`
- `~/.npm/_cacache/`
- `~/.bun/install/cache/`
- `~/Library/Caches/trivy/`
- `~/.claude/transcripts/`
- `~/.claude/plugins/cache/`
- `~/.config/chrome_pipeline_*/`
- `~/.config/webauto-oci-debug/`
- `~/.local/share/webauto-nodriver-mcp/`
- `~/.local/share/opencode/log/`
- `~/.local/share/opencode/sessions/` (only if user does not need old session files)

## 6. Ask-class targets (confirm with user)

- `~/.config/sin-solver/authd/state-backups/` — 3-month-old auth state backups
- `~/Library/Containers/` — app data, may contain user documents

## 7. Ensure free space before VACUUM

`VACUUM` rewrites the whole DB. Ensure at least the size of the *real* data
(~6 GB) is free before starting. If not, clean safe caches first.
