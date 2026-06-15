# Frameworks: Standards & Constraints

Docs: ../SKILL.md

## Technology Stack

- SIN-Code tool suite: `impact`, `semantic_diff`, `architectural_debt`, `verify_tests`, `prove`.

## Coding Standards

- Preserve exact behavior.
- Make the smallest change possible.
- One intent per changed file.

## Security Constraints

- Do not introduce new vulnerabilities during refactoring.
- Keep access modifiers unchanged unless explicitly required.

## Quality Gates

- Impact analysis risk must be acceptable or explicitly approved.
- `semantic_diff` must not split intents.
- `architectural_debt` must not regress.
- Oracle verdict must be `pass`.
