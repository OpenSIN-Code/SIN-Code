---
name: skill-github-actions
description: One-command GitHub Actions workflow deployment for OpenSIN-Code repos. Provisions canonical workflows, branch protection, dependabot, and release automation.
license: MIT
compatibility:
  - sin-code
  - opencode
  - claude-code
  - codex
metadata:
  author: SIN-Code
  version: 3.20.0
lifecycle: native
---

# skill-github-actions

## Overview

Provision and maintain GitHub Actions workflows, branch protection, dependabot, and release automation for OpenSIN-Code repositories.

## When to Use

- User says "deploy workflow", "add GitHub Action", "set up CI", "fix release.yml", "fix the broken pipeline", "branch protection", "require skill-code-ceo-audit check", "rollout to all repos", "deploy org-wide", "dependabot", "SBOM in CI", "release automation".

## When NOT to Use

- Repository is not hosted on GitHub.
- User does not have admin access.

## Core Process

```
AUDIT → PROVISION → VERIFY → PUSH
```

1. Audit existing workflows and branch protection.
2. Provision canonical files (skill-code-ceo-audit.yml, release.yml, dependabot.yml, sbom.yml, branch-protection.json, app-integration.yml).
3. Verify via dry-run or PR.
4. Push to repo or batch-rollout.

## Canonical Files

- `.github/workflows/skill-code-ceo-audit.yml`
- `.github/workflows/release.yml`
- `.github/dependabot.yml`
- `.github/workflows/sbom.yml`
- `.github/branch-protection.json`
- `.github/workflows/app-integration.yml`

## Verification

- [ ] Files exist and are valid YAML/JSON.
- [ ] Branch protection rules reference required checks.
- [ ] Workflow runs without errors.
- [ ] Commit uses Conventional Commits.
