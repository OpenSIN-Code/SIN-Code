# Tasks: Workflow

Docs: ../SKILL.md

## Pre-flight

- [ ] Confirm the approved spec exists.
- [ ] Identify the target plan path in `.sin/plans/`.

## Execution

- [ ] Task 1: Load spec.
  - Acceptance: Spec content is available.
  - Verify: Requirements extracted.
- [ ] Task 2: Decompose into atomic work units.
  - Acceptance: Each unit is one file change or test.
  - Verify: No multi-file task without sub-tasks.
- [ ] Task 3: Order by dependencies.
  - Acceptance: Dependent tasks come after prerequisites.
  - Verify: Dependency graph is acyclic.
- [ ] Task 4: Define contracts and acceptance criteria.
  - Acceptance: Each task has input/output contract.
  - Verify: Criteria are testable.
- [ ] Task 5: Save plan to `.sin/plans/`.
  - Acceptance: Plan file exists and is readable.
  - Verify: File written successfully.
- [ ] Task 6: Review plan.
  - Acceptance: Critic or user approves.
  - Verify: Approval recorded.

## Post-flight

- [ ] Summarize plan tasks and dependencies.
- [ ] Hand off to `sin-build` if ready.
