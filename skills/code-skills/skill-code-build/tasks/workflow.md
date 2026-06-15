# Tasks: Workflow

Docs: ../SKILL.md

## Pre-flight

- [ ] Confirm the approved plan exists.
- [ ] Identify the files to touch per task.

## Execution

- [ ] Task 1: Load plan.
  - Acceptance: Plan is read and tasks are understood.
  - Verify: Task list extracted.
- [ ] Task 2: Implement task N.
  - Acceptance: Code compiles and satisfies task contract.
  - Verify: Manual check or targeted test.
- [ ] Task 3: Write tests for task N.
  - Acceptance: Success and failure paths covered.
  - Verify: Tests run.
- [ ] Task 4: Run linter/formatter.
  - Acceptance: No lint errors.
  - Verify: `go fmt`, `go vet` clean.
- [ ] Task 5: Run full test suite with race detector.
  - Acceptance: All tests pass.
  - Verify: `go test --race` passes.
- [ ] Task 6: Verify acceptance criteria.
  - Acceptance: Criteria met.
  - Verify: Checklist complete.
- [ ] Task 7: Iterate if any verification failed.
  - Acceptance: All gates green.
  - Verify: No failures.

## Post-flight

- [ ] Summarize what was built and verified.
- [ ] Report coverage and any risks.
