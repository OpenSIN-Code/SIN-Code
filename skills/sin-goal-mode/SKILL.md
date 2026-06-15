---
name: sin-goal-mode
description: Track long-running goals with subtasks, dependencies, checkpoints, and rollback. Use when work spans multiple sessions or subtasks.
license: MIT
compatibility:
  - opencode
  - sin-code
metadata:
  author: SIN-Code
  version: 1.0.0
---

# sin-goal-mode

## Overview

Track complex, multi-step work using a goal with subtasks, dependencies, checkpoints, and rollback.

## When to Use

- Work spans multiple sessions or subtasks.
- User asks for a roadmap or phase plan.

## When NOT to Use

- The task is a single-step fix.
- No multi-step tracking needed.

## Core Process

```
START GOAL → ADD SUBTASKS → EXECUTE → CHECKPOINT → COMPLETE
```

1. Create a named goal with subtasks.
2. Add dependencies between subtasks.
3. Execute subtasks.
4. Create checkpoints before risky steps.
5. Mark subtasks complete.
6. Complete the goal.

## Subtask States

- pending
- in_progress
- completed
- blocked
- cancelled

## Verification

- [ ] Goal is specific and measurable.
- [ ] Subtasks are actionable.
- [ ] Dependencies are acyclic.
- [ ] Checkpoints created before risky work.
- [ ] Goal is only marked complete when all subtasks are done.
