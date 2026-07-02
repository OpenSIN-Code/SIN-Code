# Pipeline Context — SIN-Code Full Stack Integration

## Skill Chain

```
grill-me → plan v2 → skill-process-gsd → delegate-subagents → self-review → skill-process-dodone
```

Each skill's output is the next skill's input. The chain is deterministic —
no stage may self-declare success without the downstream gate passing.

## Stage Boundaries

| Stage | Skill | Input | Output | Gate |
|---|---|---|---|---|
| 1 | grill-me | Task description | Decision tree | 5+ questions, all branches resolved |
| 2 | plan v2 | Grill decisions | Execution plan | Plan has phases + tasks + done criteria |
| 3 | skill-process-gsd | Approved plan | .gsd/ state | All phases created with plans |
| 4 | delegate-subagents | Phase plan + waves | Code changes | Build + test pass |
| 5 | self-review | Git diff | Review report | 0 BLOCKER + 0 MAJOR |
| 6 | skill-process-dodone | Final codebase | Exit 0/2/3 | Exit 0 = WIRKLICH FERTIG |

## Degradation Rules

- grill-me: can be skipped if user has clear design (note it, proceed)
- plan v2: can use --lite for simple tasks (skip PERT/Monte-Carlo/OKR)
- GSD: can be skipped for single-phase tasks (plan v2 handles directly)
- delegate-subagents: can be skipped for single-file tasks (do it inline)
- self-review: NEVER skipped — CEO review is mandatory
- dodone: NEVER skipped — machine gate is mandatory (M3)

## Loop-Back

When Stage 6 (dodone) fails with Exit 2 or 3, findings feed back to Stage 4
(Execute). The agent fixes the issues and re-runs Stage 5 + 6. Maximum 3
loop-back iterations before escalating to user.
