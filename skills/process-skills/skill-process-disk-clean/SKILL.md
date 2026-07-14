---
name: skill-process-disk-clean
description: Use when user says 'clean disk', 'free up space', 'disk full', 'opencode.db is huge', 'clean caches', 'vacuum opencode db'. Safely reclaim disk space by cleaning developer caches and VACUUMing the bloated opencode.db (WAL freelist) with zero session loss.
license: MIT
compatibility:
  - sin-code
  - opencode
  - claude-code
  - codex
metadata:
  author: SIN-Code
  version: 3.29.0
  sources:
    - Infra-SIN-OpenCode-Stack
required_tools: []
lifecycle: native
---

# skill-process-disk-clean

## Overview

Safely reclaim disk space on a developer Mac by cleaning opencode-related and
general developer caches, logs, and the bloated `opencode.db` SQLite database.

The skill detects large directories, classifies them by risk (safe / ask /
locked), and runs a deterministic cleanup. The single biggest win is usually
`~/.local/share/opencode/opencode.db`: in WAL mode it grows to tens of GB while
~90% of the file is freelist pages. A `VACUUM` after stopping opencode shrinks
it back to a few GB with **zero session loss**.

## When to Use

- "clean disk", "free up space", "disk full", "mac has no space"
- "opencode.db is huge", "why is opencode 60gb"
- "clean caches", "clean yarn/npm/bun/go cache"
- Disk capacity is above ~80% and the user wants headroom.

## When NOT to Use

- A one-off `rm` of a single known file (just do it).
- Cleaning a production server or someone else's machine without confirmation.
- The user explicitly wants to keep old sessions/logs for audit.

## Core Process

```
CHECK DISK → SURVEY DIRS → CLASSIFY RISK → CLEAN SAFE → HANDLE DB → REPORT
```

1. Report current `df -h /` capacity.
2. Survey candidate directories with `du -sh`.
3. Classify each as `safe` (cache, regenerable), `ask` (auth/state, old
   backups), or `locked` (opencode.db while opencode runs).
4. Delete `safe` targets immediately.
5. For `opencode.db`: stop all opencode processes, then `VACUUM`.
6. Print before/after `df -h /`.

## Risk Classes

| Class | Examples | Action |
|-------|----------|--------|
| safe | `~/Library/Caches/*`, `~/.npm/_cacache`, `~/.bun/install/cache`, `~/.claude/transcripts`, `~/.claude/plugins/cache`, `chrome_pipeline_*`, `webauto-*` | delete without asking |
| ask | `~/.config/sin-solver/authd/state-backups`, old opencode `sessions/` you want kept | ask user first |
| locked | `~/.local/share/opencode/opencode.db` | stop opencode, then `VACUUM` |

## opencode.db Growth Explained

- WAL mode + no `VACUUM` → deleted rows go to the freelist, never returned to OS.
- Hundreds of sessions × thousands of tool-output parts (10–100 KB each) bloat
  the live data to ~6 GB; the file reaches 60 GB because the freelist holds
  ~53 GB.
- `VACUUM` rewrites the file and releases the freelist: 60 GB → ~6 GB.
- **No sessions are lost** — `VACUUM` only reclaims empty pages.

## Critical Rules

- Never delete `~/.local/share/opencode/opencode.db` itself.
- Never `VACUUM` while opencode processes hold the lock — it hangs. Stop them
  first (`pkill -9 -f opencode`).
- Never delete `.git`, project source, or `~/.ssh`.
- Always print before/after free space.

## Verification

- [ ] `df -h /` before and after shows reclaimed space.
- [ ] `opencode.db` size reduced after VACUUM (if run).
- [ ] No project source or git history deleted.
- [ ] User confirmed any `ask`-class deletion.
