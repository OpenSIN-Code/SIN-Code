# Frameworks: Standards & Constraints

Docs: ../SKILL.md

## Technology Stack

- SIN-Code Go stack (or project language).
- `go fmt`, `go vet`, `go test --race`.

## Coding Standards

- One task per focused change.
- Tests live next to production code.
- Handle errors explicitly.

## Security Constraints

- No secrets in code.
- Validate external inputs.

## Quality Gates

- Tests pass.
- Linter clean.
- Coverage does not decrease.
- Race detector clean (`go test --race`).
