# Tasks: Workflow

Docs: ../SKILL.md

## Pre-flight

- [ ] Understand the skill's purpose and triggers.
- [ ] Choose a valid category from the canonical list.
- [ ] Choose a valid kebab-case name: `skill-<category>-<descriptive-name>`.
- [ ] Confirm the directory path: `skills/<category>-skills/<name>/`.

## Execution

- [ ] Task 1: Scaffold directory.
  - Acceptance: `context/`, `frameworks/`, `tasks/`, `templates/` exist and `LICENSE` is present.
  - Verify: Directory listing.
- [ ] Task 2: Write `SKILL.md`.
  - Acceptance: Frontmatter + overview + when to use + core process + naming rules + verification.
  - Verify: Validator parses frontmatter.
- [ ] Task 3: Fill `context/triggers.md`.
  - Acceptance: Triggers, boundaries, required input, tone documented.
  - Verify: File not empty.
- [ ] Task 4: Fill `frameworks/standards.md`.
  - Acceptance: Standards, layout, constraints, quality gates documented.
  - Verify: File not empty.
- [ ] Task 5: Fill `tasks/workflow.md`.
  - Acceptance: Pre-flight, execution, post-flight tasks documented.
  - Verify: File not empty.
- [ ] Task 6: Fill `templates/output.md` and `templates/prompt.md`.
  - Acceptance: Reusable templates provided, including category and naming convention.
  - Verify: File not empty.
- [ ] Task 7: Validate.
  - Acceptance: `python3 scripts/validate_skill.py --all-bundled --strict` passes.
  - Verify: Exit code 0.
- [ ] Task 8: Build and test.
  - Acceptance: `go build ./...` and `go test ./... -race -count=1` pass.
  - Verify: Exit code 0.
- [ ] Task 9: Update project docs for bundled skills.
  - Acceptance: `README.md`, `AGENTS.md`, `CHANGELOG.md`, `ECOSYSTEM.md` updated.
  - Verify: All four files mention the new skill/category.

## Post-flight

- [ ] Create `.claude/skills/` symlink if the user wants local OpenCode/Claude discovery (not required for binary-embedded bundled skills).
- [ ] Summarize the new skill, its category, and its validation status.
- [ ] Remind the user to rebuild and test the `sin-code` binary.
