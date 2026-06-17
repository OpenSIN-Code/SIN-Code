---
name: skill-memory-honcho-rollback
description: Snapshot, diff, and rollback sin-brain / Honcho memory with merge/exact/patch strategies and an audit log.
license: MIT
compatibility:
  - sin-code
  - opencode
  - claude-code
  - codex
metadata:
  author: SIN-Code
  version: 3.20.0
  sources: "OpenSIN-Code/Infra-SIN-OpenCode-Stack/skills/honcho-rollback"
required_tools:
  - sin_memory_search
lifecycle: external
---

# skill-memory-honcho-rollback

## Overview

Add undo capability to sin-brain memory: named snapshots, diffs, restores, and audit logs.

## When to Use

- User says "take a snapshot", "rollback memory", "memory diff", "audit log", "undo memory change".

## When NOT to Use

- Memory is not persisted yet.
- User only wants to read memory.

## Core Process

```
SNAPSHOT → DIFF → RESTORE → LOG
```

1. Create a named snapshot of current memory.
2. Diff between two snapshots.
3. Restore to a snapshot with merge, exact, or patch strategy.
4. Record every mutation in an append-only audit log.

## Strategies

- **merge**: Safe, keeps current additions if no conflict.
- **exact**: Destructive, replace exactly.
- **patch**: Apply only changes in diff.

## Verification

- [ ] Snapshot created and named.
- [ ] Diff shows added/removed/modified.
- [ ] Restore strategy chosen and errors checked.
- [ ] Audit log updated.
