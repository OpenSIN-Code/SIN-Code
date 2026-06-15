# Frameworks: Standards & Constraints

Docs: ../SKILL.md

## Security Rules

- No .env files in repos.
- No tokens in shell history.
- No secret values in CI logs.
- Rotate exposed tokens.

## Constraints

- Use `infisical` CLI.
- Degrade gracefully if Infisical is unreachable.
