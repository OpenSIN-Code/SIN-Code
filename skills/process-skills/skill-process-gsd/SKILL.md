---
name: skill-process-gsd
description: >
  GSD — Get Shit Done. Phased project lifecycle management with plan-and-execute
  workflow, wave-based parallel execution, and deterministic state tracking.
  Replaces the 8 npm-based GSD skills (gsd-discuss-phase, gsd-execute-phase,
  gsd-help, gsd-new-project, gsd-phase, gsd-plan-phase, gsd-surface, gsd-update)
  with a single unified Go-native skill backed by `sin-code gsd`.
  Triggers on "gsd", "get shit done", "project lifecycle", "phase planning",
  "roadmap", "wave execution", "project init".
license: MIT
compatibility:
  - sin-code
  - opencode
  - claude-code
  - codex
metadata:
  author: SIN-Code
  version: 3.29.0
  replaces:
    - gsd-discuss-phase
    - gsd-execute-phase
    - gsd-help
    - gsd-new-project
    - gsd-phase
    - gsd-plan-phase
    - gsd-surface
    - gsd-update
required_tools:
  - sin_bash
lifecycle: native
---

# skill-process-gsd — Get Shit Done

## Overview

Phased project lifecycle: init → phases → plans → execute → verify.
All state persists in `.gsd/` as markdown (PROJECT.md, ROADMAP.md, STATE.md).
Powered by the `sin-code gsd` Go binary — no npm, no Node, no external deps.

## When to Use

- Starting a new project that needs structured phases
- Breaking a large task into phases with dependencies
- Tracking execution progress across waves of parallel work
- User says "gsd", "get shit done", "project lifecycle", "phase plan"

## When NOT to Use

- Single quick task (use `plan v2 --lite` instead)
- Just need a plan without phases (use `plan v2` instead)
- Already have a spec and need task decomposition (use `plan v2 --from-spec`)

## CLI Reference

All commands via `sin-code gsd`:

### Project

```bash
sin-code gsd init --name "MyProject" --description "Build X"
sin-code gsd status              # show project status + completion %
sin-code gsd status --json       # machine-readable output
```

### Phase CRUD

```bash
sin-code gsd phase add "Backend API" --priority P0
sin-code gsd phase add "Frontend" --priority P1
sin-code gsd phase insert 1 "Database Migration" --priority P0
sin-code gsd phase remove 2
sin-code gsd phase edit 1 --title "Core API" --priority P0 --status in-progress
sin-code gsd phase list
sin-code gsd phase list --json
```

### Plan

```bash
sin-code gsd plan 1              # show plan for phase 1
```

### Execute

```bash
sin-code gsd execute 1           # show waves + progress for phase 1
sin-code gsd execute 1 --task T1 --status done
sin-code gsd execute 1 --json
```

## Core Process

```
INIT → PHASES → PLAN → EXECUTE → VERIFY → NEXT PHASE
```

1. `gsd init` — create project, establish `.gsd/` state
2. `gsd phase add` — break project into phases (P0 critical → P3 optional)
3. Per phase: create plan (use `plan v2` or `plan v2 --from-spec`)
4. `gsd execute` — analyze waves, run tasks in dependency order
5. After each wave: verify with `dodone check` + `self-review`
6. Mark phase completed, move to next

## Wave-Based Execution

Tasks within a phase are grouped into waves by dependencies:
- Wave 1: tasks with no dependencies (run in parallel)
- Wave 2: tasks depending only on Wave 1 (run in parallel after Wave 1)
- Wave 3: etc.

Use `delegate-subagents` to fan out parallel tasks within a wave.

## Integration with SIN-Code Stack

| Stage | Tool | Purpose |
|---|---|---|
| Design | `grill-me` | Adversarial design interview before phases |
| Planning | `plan v2` | Full 13-stage plan per phase |
| Execution | `delegate-subagents` | Parallel subagents within waves |
| Review | `self-review` | CEO-grade evidence review after code |
| Done Gate | `skill-process-dodone` | Deterministic DoD check (grep+test+build) |
| Memory | `sin-goal-mode` | Goal tracking with checkpoints |

### Full Pipeline

```
grill-me (design) → gsd init → gsd phase add → plan v2 (per phase)
  → delegate-subagents (wave execution) → self-review → dodone check
  → gsd phase edit --status completed → next phase
```

## State Files

| File | Purpose |
|---|---|
| `.gsd/PROJECT.md` | Project name, description, tech stack |
| `.gsd/ROADMAP.md` | All phases with priority and status |
| `.gsd/STATE.md` | Current phase, history |
| `.gsd/plans/phase-N.md` | Task plan for phase N |

## Verification

- [ ] Project initialized with `gsd init`
- [ ] At least 1 phase exists
- [ ] Each phase has a plan (via `plan v2`)
- [ ] Execution waves analyzed
- [ ] All tasks in current phase marked done
- [ ] `dodone check` passes before marking phase completed
