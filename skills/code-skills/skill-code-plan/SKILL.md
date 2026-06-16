---
name: skill-code-plan
description: Break an approved specification into concrete, executable tasks. Use when the user has a spec and needs an implementation plan.
license: MIT
compatibility: 
metadata: 
lifecycle: native
---

# skill-code-plan

## Overview

Break an approved specification into atomic, dependency-ordered tasks that an autonomous agent can execute.

## When to Use

- User has an approved spec and asks for a plan.
- A feature needs to be decomposed into executable tasks.

## When NOT to Use

- No spec exists yet (use `skill-code-spec` first).
- The task is already small enough to implement directly.

## Core Process

```
LOAD SPEC → DECOMPOSE → ORDER → DEFINE CONTRACTS → SAVE → REVIEW
```

1. Load the approved specification from `.sin/specs/`.
2. Identify atomic work units (one file change or test per unit).
3. Order tasks by dependencies.
4. For each task, define input/output contracts and acceptance criteria.
5. Save the plan to `.sin/plans/`.
6. Review the plan with the user or Critic agent.

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "I can keep the plan in my head." | Written plans enable parallel work and review. |
| "Just generate code directly." | Planning first reduces hallucinations and rework. |
| "Tasks can be vague." | Every task needs a contract and acceptance criteria. |

## Red Flags

- Tasks that touch more than one responsibility.
- Missing dependency ordering.
- No acceptance criteria.

## Verification

- [ ] Each task references a spec requirement.
- [ ] Tasks are executable by an autonomous agent.
- [ ] No task takes more than 10 minutes of agent time.
- [ ] Plan is saved to `.sin/plans/`.
- [ ] Plan was reviewed by Critic or user.
