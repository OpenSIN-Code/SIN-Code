# build

## Overview
Implement a feature from a plan: write code, tests, and verify.

## Steps
1. Load the current plan from `.sin/plans/`.
2. For each task in the plan:
   a. Write the necessary Go code (or other language).
   b. Write unit tests covering the change.
   c. Run linter and formatter (go fmt, go vet).
3. Run the full test suite.
4. Verify that acceptance criteria are met.
5. If verification fails, go back to step 2 (self-correct).

## Verification
- [ ] All tests pass.
- [ ] No lint warnings.
- [ ] Code coverage does not decrease.
- [ ] Build succeeds.

## Anti-Rationalization
| Excuse | Rebuttal |
|--------|----------|
| "Tests are optional for this simple change." | Every change must have tests. |
| "I'll fix linter issues later." | Fix now; later never comes. |

## Quality Gates
- Use `sin-code test --race` to catch data races.
- Use `sin-code security` to scan for vulnerabilities.
- Governor enforces hard invariant: no commit without passing tests.
