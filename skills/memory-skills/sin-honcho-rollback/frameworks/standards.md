# Frameworks: Standards & Constraints

Docs: ../SKILL.md

## Strategies

- merge: safe, conflict-aware.
- exact: destructive.
- patch: delta-only.

## Constraints

- Always dry-run when possible.
- Audit log is append-only.
- Never delete audit log.
