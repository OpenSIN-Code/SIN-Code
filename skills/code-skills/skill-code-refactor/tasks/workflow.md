# Tasks: Workflow

Docs: ../SKILL.md

## Pre-flight

- [ ] Capture the symbol to refactor.
- [ ] Confirm the goal is behavior-preserving.

## Execution

- [ ] Task 1: Impact analysis.
  - Acceptance: `impact(symbol)` returns callers, fan-in, and risk.
  - Verify: Blast radius is reported to the user.
- [ ] Task 2: Apply smallest refactor.
  - Acceptance: Code compiles and tests still run.
  - Verify: No behavior changes (same inputs → same outputs).
- [ ] Task 3: Semantic diff per file.
  - Acceptance: Each file has exactly one intent.
  - Verify: No split-intent warnings.
- [ ] Task 4: Architectural debt check.
  - Acceptance: Debt score is stable or improved.
  - Verify: `architectural_debt()` output reviewed.
- [ ] Task 5: Verification.
  - Acceptance: `verify_tests` passes; pure functions also use `prove`.
  - Verify: Oracle verdict is `pass`.
- [ ] Task 6: Report.
  - Acceptance: Report includes blast radius, intents, debt delta, and verdict.
  - Verify: User acknowledges or approves.

## Post-flight

- [ ] Commit or summarize the change.
- [ ] Note any follow-up refactors if debt could not be reduced.
