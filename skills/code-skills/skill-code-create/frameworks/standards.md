# Frameworks: Standards & Constraints

Docs: ../SKILL.md

## Technology Stack

- SIN-Code skill standard.
- `scripts/validate_skill.py` for validation.
- Optional `scripts/create_skill.py` for scaffolding.

## Skill Standard

- SKILL.md with YAML frontmatter (`name`, `description`, `license`, `compatibility`, `metadata`).
- Required directories: `context/`, `frameworks/`, `tasks/`, `templates/`.
- Recommended directories: `scripts/`, `tests/`, `lib/`.
- `compatibility` must be a YAML list.

## Constraints

- Skill name must match directory name.
- No copyrighted material without license.
- Keep templates actionable.

## Quality Gates

- Strict validator passes.
- Frontmatter valid.
- All required directories populated.
- LICENSE file present.
