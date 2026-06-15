# Frameworks: Standards & Constraints

Docs: ../SKILL.md

## Canonical Workflows

- ceo-audit.yml
- release.yml
- sbom.yml
- app-integration.yml
- dependabot.yml

## Branch Protection

- Require PRs.
- Require `ceo-audit` check.
- Restrict pushes to main.

## Constraints

- Idempotent.
- Conventional Commits.
- Use real `gh` CLI + REST API.
