---
name: skill-process-grill
description: Use when user says 'grill me', 'stress test', 'interrogate design', 'poke holes', 'challenge plan'. Adversarial design-review interview. Relentlessly questions plans to surface hidden assumptions before implementation.
license: MIT
compatibility:
  - sin-code
  - opencode
  - claude-code
  - codex
metadata:
  author: SIN-Code
  version: 3.20.0
  sources: "OpenSIN-Code/Infra-SIN-OpenCode-Stack/skills/grill-me"
required_tools:
  - sin_sckg
  - sin_grasp
lifecycle: external
---

# skill-process-grill

## Overview

Stress-test a plan, design, or decision before building it.

## When to Use

- User says "grill me", "stress test this plan", "interrogate my design", "poke holes in my idea", "challenge my approach".

## When NOT to Use

- The user has already decided and only wants implementation.
- There is no plan or decision to challenge.

## Core Process

```
LISTEN → CHALLENGE → FOLLOW UP → SYNTHESIZE
```

1. Listen to the user's plan.
2. Ask adversarial questions that surface assumptions, risks, and edge cases.
3. Follow up on evasive or weak answers.
4. Synthesize a decision tree with resolved and unresolved points.

## Tactics

- Ask for the worst-case scenario.
- Demand evidence for assumptions.
- Question trade-offs not explicitly stated.
- Test for reversibility.

## Companion Tools

- **plan v2** (`/plan`) — after grilling, turn resolved decisions into a full execution-ready plan. For quick tasks use `plan --lite`.
- **sin-doc-coauthoring** — turn the resolved decisions into a SPEC/ADR/PRD
- **sin-goal-mode** — break the implementation into goals + subtasks
- **ceo-audit** — for the technical review (security, performance, etc.)

## Related Skills

- **skill-process-dodone** — after plan v2 executes, dodone checks deterministically whether the work is truly done
- **self-review** — CEO-grade evidence-driven review after code is written
- **plan v2** — full plan-and-execute skill with --lite (quick) and --cli (deterministic) modes
