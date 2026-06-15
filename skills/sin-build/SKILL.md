---
name: sin-build
description: Implement a feature from an approved plan with tests and verification. Use when the user asks to build, implement, or code a feature.
license: MIT
compatibility:
  - opencode
  - sin-code
metadata:
  author: SIN-Code
  version: 1.0.0
---

# sin-build

## Overview

Implement a feature from an approved plan. Write code, tests, and verify before reporting done.

## When to Use

- User asks to build or implement a feature.
- An approved plan exists in `.sin/plans/`.

## When NOT to Use

- No approved plan exists (use `sin-plan` / `sin-spec` first).
- The task is a single-line fix.

## Core Process

```
LOAD PLAN → IMPLEMENT TASK → TEST → VERIFY → ITERATE
```

1. Load the approved plan from `.sin/plans/`.
2. For each task, write the necessary code.
3. Write unit tests covering the change.
4. Run linter and formatter (`go fmt`, `go vet`).
5. Run the full test suite.
6. Verify acceptance criteria are met.
7. If verification fails, iterate.

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "Tests are optional for this simple change." | Every change must have tests. |
| "I'll fix linter issues later." | Later never comes. Fix now. |
| "The plan is just a suggestion." | The plan is the contract. Deviations need user approval. |

## Red Flags

- Implementing without a plan.
- Skipping tests or verification.
- Decreasing code coverage.

## Verification

- [ ] All plan tasks completed.
- [ ] Unit tests cover success and failure paths.
- [ ] Linter and formatter pass.
- [ ] Full test suite passes.
- [ ] Acceptance criteria met.
- [ ] Code coverage did not decrease.
