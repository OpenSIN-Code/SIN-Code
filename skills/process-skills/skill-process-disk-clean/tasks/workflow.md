# Workflows — skill-process-disk-clean

## Workflow 1: Full safe cleanup (no opencode restart)

Use when the user wants space now and opencode must keep running.

```
1. df -h /
2. Survey safe targets:
   du -sh ~/Library/Caches/Yarn/ ~/.bun/install/cache/ ~/.npm/_cacache/ \
         ~/Library/Caches/go-build/ ~/Library/Caches/trivy/ \
         ~/.claude/transcripts/ ~/.claude/plugins/cache/ \
         ~/.config/chrome_pipeline_*/ ~/.config/webauto-oci-debug/ \
         ~/.local/share/webauto-nodriver-mcp/ ~/.local/share/opencode/log/
3. rm -rf each safe target
4. df -h /  (report GB freed)
```

Note: caches may regenerate while opencode runs (Yarn can regrow to 8 GB).
Re-run as needed; never delete `opencode.db`.

## Workflow 2: opencode.db VACUUM (max reclamation)

Use when capacity is critical and opencode can be stopped.

```
1. df -h /
2. pkill -9 -f opencode
3. sleep 3
4. ps aux | grep opencode | grep -v grep   # must be empty
5. sqlite3 ~/.local/share/opencode/opencode.db "VACUUM;"
6. ls -lh ~/.local/share/opencode/opencode.db
7. df -h /  (report GB freed — typically 50+ GB)
```

## Workflow 3: Ask-class cleanup

```
1. du -sh ~/.config/sin-solver/authd/state-backups/
2. Present size + age to user
3. On confirmation: rm -rf <target>
4. df -h /
```

## Workflow 4: Diagnostic (no deletion)

When the user just wants to know what is using space:

```
df -h /
du -sh ~/Library/* ~/.local/* ~/.config/* 2>/dev/null | sort -rh | head -25
du -sh ~/.local/share/opencode/opencode.db
sqlite3 ~/.local/share/opencode/opencode.db \
  "PRAGMA page_count; PRAGMA page_size; PRAGMA freelist_count;" \
  # if freelist_count >> page_count, VACUUM will reclaim most of it
```
