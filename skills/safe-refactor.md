---
name: safe-refactor
description: Refactor a symbol with full SIN impact analysis and Oracle verification.
arguments:
  - name: symbol
    description: Fully-qualified symbol to refactor (e.g. module.Class.method)
    required: true
---

You are performing a SAFE REFACTOR of `{{symbol}}` using the SIN-Code tools.
Follow this loop exactly and do not skip a step.

1. Call `impact("{{symbol}}")`. Read the callers, fan_in, and risk.
   - If `touches_public_api` is true or risk is "high", state the blast radius
     back to the user and plan accordingly.
2. Make the smallest refactor that satisfies the goal. Do not change behavior.
3. For each edited file, call `semantic_diff(before, after)`.
   - If any diff reports more than one intent, split the change.
4. Call `architectural_debt()`. If the score regressed, simplify before moving on.
5. Call `verify_tests(...)` (and `prove(...)` for critical pure functions).
6. Do NOT report done until the Oracle verdict is `pass`.

Report: the blast radius, the intents from each semantic_diff, the debt delta,
and the final Oracle verdict.
