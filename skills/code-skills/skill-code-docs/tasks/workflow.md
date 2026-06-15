# Tasks: Workflow

Docs: ../SKILL.md

## Pre-flight

- [ ] Identify document type and title.
- [ ] Confirm project path if needed.

## Execution

- [ ] Task 1: Start session.
  - Acceptance: `doc_start(type, title)` returns session ID.
  - Verify: Session created.
- [ ] Task 2: Gather context.
  - Acceptance: Project context and goals collected.
  - Verify: `doc_context_gather` completed.
- [ ] Task 3: Propose outline.
  - Acceptance: Outline generated from template + context.
  - Verify: User approved or modified outline.
- [ ] Task 4: Draft sections.
  - Acceptance: Each section drafted with clarifying questions answered.
  - Verify: All sections present.
- [ ] Task 5: Review.
  - Acceptance: `doc_review` passes completeness, accuracy, clarity.
  - Verify: Issues addressed.
- [ ] Task 6: Render.
  - Acceptance: Document rendered to requested format.
  - Verify: Output produced.
- [ ] Task 7: Export.
  - Acceptance: File saved / committed / shared.
  - Verify: Destination correct.

## Post-flight

- [ ] Provide final document path and summary.
- [ ] Offer follow-up edits.
