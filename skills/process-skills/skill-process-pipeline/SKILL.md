---
name: skill-process-pipeline
description: >
  Full SIN-Code pipeline in one shot: grill-me (design) → plan v2 (strategy)
  → GSD (phases) → delegate-subagents (parallel execution) → self-review
  (CEO review) → dodone check (machine gate). Triggers on "full pipeline",
  "end to end", "do everything", "complete workflow", "/pipeline", or when
  user wants the entire SIN-Code stack applied to a task.
license: MIT
compatibility:
  - sin-code
  - opencode
  - claude-code
  - codex
metadata:
  author: SIN-Code
  version: 3.29.0
required_tools:
  - sin_bash
  - sin_run_loop
  - sin_goal_add
lifecycle: native
---

# skill-process-pipeline — Full SIN-Code Stack in One Shot

## Overview

Chains all 6 SIN-Code process skills into a single deterministic pipeline.
Each stage feeds the next. No stage is skipped. No stage self-declares success
without the next stage's gate passing.

## When to Use

- User says "full pipeline", "/pipeline", "do everything", "end to end"
- User wants the complete SIN-Code quality stack applied to a task
- High-stakes task that needs design review + planning + parallel execution + CEO review + machine gate

## When NOT to Use

- Quick one-liner task (just do it)
- Only need one stage (use the individual skill instead)
- Task is purely informational (no code changes)

## The Pipeline

```
STAGE 1: GRILL     — adversarial design interview (before code)
STAGE 2: PLAN      — plan v2 with research, review, risk scoring
STAGE 3: GSD       — break plan into phases, init project
STAGE 4: EXECUTE   — delegate-subagents per wave, parallel
STAGE 5: REVIEW    — self-review (CEO reads diffs, checks scope)
STAGE 6: DONE GATE — dodone check (machine: grep+test+build, exit 0)
```

### Stage 1: GRILL (Design Review)

**Skill:** `grill-me` / `skill-process-grill`
**Input:** User's task description
**Output:** Decision tree with resolved and open points

1. Start grilling: `grill_start(topic="$TASK", context="$CODEBASE_CONTEXT")`
2. Ask 5+ adversarial questions, one at a time, with recommended answers
3. Record each answer: `grill_record_answer(session_id, question, answer)`
4. Synthesize: `grill_synthesize(session_id)` → decisions + assumptions
5. **Gate:** At least 5 questions asked, all branches resolved or explicitly open

If user skips grilling (already has clear design), proceed to Stage 2 with note.

### Stage 2: PLAN (Strategy + Execution Plan)

**Skill:** `plan v2` (use `--lite` for simple tasks, full for complex)
**Input:** Grill decisions + task description
**Output:** Execution-ready plan with phases, tasks, risks, done criteria

1. Check for existing plan (Stage 0 of plan v2)
2. Research (parallel: librarian + explore)
3. Draft plan with OKRs/lite structure
4. Critical review (hard critique gate)
5. Quality score (full mode) or simple review (lite mode)
6. **Gate:** Plan has phases, tasks with validation, done criteria, risks

### Stage 3: GSD (Project Lifecycle)

**Skill:** `skill-process-gsd`
**CLI:** `sin-code gsd`
**Input:** Approved plan
**Output:** `.gsd/` state with phases, plans per phase

1. `sin-code gsd init --name "$PROJECT" --description "$TASK"`
2. For each plan phase: `sin-code gsd phase add "$TITLE" --priority $PRIORITY`
3. For each phase: create task plan (save to `.gsd/plans/`)
4. `sin-code gsd status` — verify project structure
5. **Gate:** All phases created, each has a plan

### Stage 4: EXECUTE (Parallel Wave Execution)

**Skill:** `delegate-subagents`
**Input:** Phase plan with waves
**Output:** Code changes, test results

1. `sin-code gsd execute $PHASE_ID` — analyze waves
2. For each wave (dependency-ordered):
   a. Launch all tasks in wave as parallel subagents (`delegate-subagents`)
   b. Each subagent gets: full context, file assignment, validation criteria
   c. Wait for all subagents in wave to complete
   d. Verify each result (build + test)
   e. `sin-code gsd execute $PHASE_ID --task $TASK_ID --status done`
3. After all waves: run full build + test
4. **Gate:** All tasks done, build passes, tests pass

### Stage 5: REVIEW (CEO Self-Review)

**Skill:** `self-review` / `skill-process-self-review`
**Input:** All code changes (git diff)
**Output:** Review report with BLOCKER/MAJOR/MINOR/NIT findings

1. Reconstruct scope from original task + grill decisions
2. Requirement check: every requirement covered?
3. File-by-file check: read every changed file
4. Run `scripts/verify.sh` or equivalent (build + test + lint)
5. Severity assessment: BLOCKER / MAJOR / MINOR / NIT
6. Fix all BLOCKER + MAJOR immediately, re-verify
7. **Gate:** 0 open BLOCKER, 0 open MAJOR

### Stage 6: DONE GATE (Machine Verification)

**Skill:** `skill-process-dodone`
**CLI:** `dodone check` or `scripts/dodone-check.sh`
**Input:** Final codebase state
**Output:** Exit 0 (WIRKLICH FERTIG) or Exit 2/3 (back to Stage 4)

1. Generate DoD contract from task + plan done criteria
2. Inject system prompt (agent knows machine check is pending)
3. Run `dodone check` — deterministic pillars:
   - P1: No placeholders (grep TODO/FIXME/panic)
   - P2: Error handling (no empty catch/pass/ignore)
   - P3: Tests pass (go test / pytest / npm test)
   - P4: Build + lint clean
   - P5: Required artifacts present
   - P6: Requirements coverage
   - P7: No dead code
   - P8: PoC invariants (if poc tool available)
   - P9: Architecture debt (if adw tool available)
   - P10: Security scan (if sin-security available)
   - P11: SCKG dead code (if sckg tool available)
4. **Gate:** Exit 0 = WIRKLICH FERTIG
5. If Exit 2/3: feed findings back to Stage 4, fix, re-run

## Post-Pipeline

After Exit 0:
- `sin-code gsd phase edit $PHASE_ID --status completed`
- `sin-goal-mode goal_complete()` if goal tracking active
- `sin-brain remember("Pipeline completed for $TASK")`
- Move to next phase (back to Stage 2) or close project

## Failure Handling

| Stage | Failure | Action |
|---|---|---|
| 1 Grill | Unresolved design | User decides, then proceed |
| 2 Plan | Review rejects | 1 revision max, then user decides |
| 3 GSD | Phase conflict | User resolves, re-init |
| 4 Execute | Subagent fails | Re-launch with fixed prompt (max 2 retries) |
| 5 Review | BLOCKER found | Fix immediately, re-review |
| 6 Done Gate | Exit 2/3 | Feed findings to Stage 4, fix, re-run |

## Cost Awareness

| Stage | LLM Calls | Duration |
|---|---|---|
| 1 Grill | 3-10 | 1-3 min |
| 2 Plan | 4-5 | 2-5 min |
| 3 GSD | 0 (CLI) | <1s |
| 4 Execute | 5-50 (task-dependent) | 5-60 min |
| 5 Review | 1-3 | 1-5 min |
| 6 Done Gate | 0 (deterministic) | <30s |
| **Total** | **13-70** | **10-70 min** |

## Slash Command

```
/pipeline <task description>
```

Triggers this skill with the task as input. See `commands/pipeline.md` in Infra.

## Verification

- [ ] Stage 1 completed (grill synthesize produced decisions)
- [ ] Stage 2 completed (plan has phases, tasks, done criteria)
- [ ] Stage 3 completed (gsd status shows all phases)
- [ ] Stage 4 completed (all tasks marked done, build+test pass)
- [ ] Stage 5 completed (0 BLOCKER, 0 MAJOR)
- [ ] Stage 6 completed (dodone check exit 0)
- [ ] Phase marked completed in GSD
- [ ] Goal completed (if tracking)
