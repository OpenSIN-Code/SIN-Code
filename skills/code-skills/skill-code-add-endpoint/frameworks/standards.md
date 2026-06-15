# Frameworks: Standards & Constraints

Docs: ../SKILL.md

## Technology Stack

- EFM (Ephemeral Full-Stack Mocking) for contract verification.
- Project's HTTP framework (e.g., Gin, FastAPI, Express).

## Standards

- Define request/response contract before implementation.
- Mock first, then implement.
- Tests for success and failure paths.
- Update API docs in the same PR.

## Constraints

- No production endpoint without a verified contract.
- No breaking changes to existing consumers without migration.

## Quality Gates

- Mock verification passes.
- Tests pass.
- Docs updated.
