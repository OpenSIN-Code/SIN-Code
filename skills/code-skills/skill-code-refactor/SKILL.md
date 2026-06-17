---
name: skill-code-refactor
description: Refactor a symbol with full SIN impact analysis and Oracle verification. Use when the user asks to refactor, rename, or restructure a symbol while preserving behavior.
license: MIT
compatibility:
  - sin-code
  - opencode
  - claude-code
  - codex
metadata:
  author: SIN-Code
  version: 3.20.0
required_tools:
  - sin_sckg
  - sin_oracle
  - sin_test
lifecycle: native
---

# skill-code-refactor

## Overview

Perform a behavior-preserving refactor of a symbol using SIN-Code impact analysis, semantic diff, and Oracle verification.

## When to Use

- User asks to refactor, rename, or restructure a symbol.
- The change should not alter observable behavior.

## When NOT to Use

- Feature additions or bug fixes.
- Changes that intentionally alter behavior.

## Core Process

```
IMPACT → REFACTOR → SEMANTIC DIFF → DEBT CHECK → VERIFY → REPORT
```

1. Call `impact("{{symbol}}")`. Read callers, fan-in, and risk.
   - If `touches_public_api` is true or risk is high, state the blast radius to the user.
2. Make the smallest refactor that satisfies the goal. Do not change behavior.
3. For each edited file, call `semantic_diff(before, after)`.
   - If any diff reports more than one intent, split the change.
4. Call `architectural_debt()`. If the score regressed, simplify before moving on.
5. Call `verify_tests(...)` (and `prove(...)` for critical pure functions).
6. Do NOT report done until the Oracle verdict is `pass`.

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "The refactor is safe because the tests still pass." | Tests passing is necessary but not sufficient; use impact analysis and semantic diff. |
| "I can change a few related things while I'm here." | One change per refactor. Mixed intents hide regressions. |
| "The user didn't ask for a report, just the result." | The blast radius and debt delta are part of the result. |

## Red Flags

- Skipping impact analysis.
- Changing behavior under the guise of refactoring.
- Reporting done with a red Oracle verdict.

## Verification

- [ ] Impact analysis completed and blast radius reported.
- [ ] Semantic diff shows exactly one intent per file.
- [ ] Architectural debt did not regress (or was justified and fixed).
- [ ] Oracle verdict is `pass`.
- [ ] Final report includes blast radius, intents, debt delta, and verdict.
