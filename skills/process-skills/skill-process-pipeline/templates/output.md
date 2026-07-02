# Pipeline Templates — Output Format

## Pipeline Start

```
=== SIN-Code Full Pipeline (10 Stages) ===
Task: <task description>
Time: <timestamp>
```

## Stage Progress

```
Stage 0: PRE-FLIGHT ✅ (doctor OK, goal #42, checkpoint created)
Stage 1: GRILL ✅ (7 questions, 5 resolved, 2 open)
Stage 2: PLAN ✅ (3 phases, 15 tasks, risk score 24/100)
Stage 3: GSD ✅ (3 phases, 3 plans, 3 goal subtasks)
Stage 4: EXECUTE ⏳ (wave 2/3, 8/12 tasks done)
Stage 5: REVIEW ⏸
Stage 6: DONE GATE ⏸
...
```

## Pipeline Complete

```
=== PIPELINE COMPLETE (10/10 stages) ===
Task: <task description>
Goal: goal-abc123
Commit: feat: add X (goal #42, pipeline 10/10, dodone exit 0)
PR: https://github.com/org/repo/pull/123
Cost: $0.42 (14k tokens)
Duration: 23 minutes
Log: /tmp/sin-pipeline-<name>-<timestamp>.log
```
