# Trigger Phrases — skill-process-disk-clean

## Primary Triggers (always activate)

- "clean disk"
- "free up space"
- "disk full"
- "mac has no space"
- "no space left"
- "storage full"
- "reclaim disk"
- "clean up my mac"
- "why is my disk 97% full"
- "opencode.db is huge"
- "why is opencode 60gb"
- "clean caches"
- "clean yarn cache"
- "clean npm cache"
- "clean bun cache"
- "clean go build cache"
- "vacuum opencode db"

## Context Triggers

- `df -h /` shows capacity > 80%
- `du -sh ~/.local/share/opencode/` reports > 10 GB
- `ls -lh ~/.local/share/opencode/opencode.db` reports > 5 GB
- User complains about build/test failures due to "no space left on device"

## Anti-Triggers (do NOT activate)

- "delete this one file" — single known target, just do it
- "clean production server" — different risk profile, confirm first
- "wipe my whole disk" — never
- "delete node_modules in my project" — that affects source builds, ask first
