# Tasks: Workflow

Docs: ../SKILL.md

## Pre-flight

- [ ] Understand the skill's purpose and triggers.
- [ ] Choose a valid name.

## Execution

- [ ] Task 1: Scaffold directory.
  - Acceptance: `context/`, `frameworks/`, `tasks/`, `templates/` exist.
  - Verify: Directory listing.
- [ ] Task 2: Write SKILL.md.
  - Acceptance: Frontmatter + overview + when to use + core process + verification.
  - Verify: Validator parses frontmatter.
- [ ] Task 3: Fill context/triggers.md.
  - Acceptance: Triggers and boundaries documented.
  - Verify: File not empty.
- [ ] Task 4: Fill frameworks/standards.md.
  - Acceptance: Standards and constraints documented.
  - Verify: File not empty.
- [ ] Task 5: Fill tasks/workflow.md.
  - Acceptance: Pre-flight, execution, post-flight tasks documented.
  - Verify: File not empty.
- [ ] Task 6: Fill templates/output.md and templates/prompt.md.
  - Acceptance: Reusable templates provided.
  - Verify: File not empty.
- [ ] Task 7: Validate.
  - Acceptance: `validate_skill.py --strict` passes.
  - Verify: Exit code 0.

## Post-flight

- [ ] Create `.claude/skills/` symlink if needed.
- [ ] Summarize the new skill and its validation status.
