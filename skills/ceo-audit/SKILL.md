---
name: ceo-audit
description: CEO-grade SOTA repository audit. Runs 47 quality gates (security, performance, code quality, dependencies, tests, docs, compliance) and produces a board-ready Markdown + SARIF report.
license: MIT
compatibility:
  - opencode
  - sin-code
metadata:
  author: SIN-Code
  version: 1.0.0
---

# ceo-audit

## Overview

Run a comprehensive audit on a repository and produce executive-grade reports.

## When to Use

- User says "ceo audit", "audit this repo", "is this production-ready", "pre-release check", "security audit", "compliance audit", "boss-level review".

## When NOT to Use

- The task is a quick lint run.
- The repository is not code.

## Core Process

```
SCAN → ANALYZE → SCORE → REPORT
```

1. Run dependency, security, lint, test, and quality scanners.
2. Analyze findings against OWASP/ASVS v5.0.
3. Score risk per gate and overall.
4. Produce Markdown + SARIF report.

## Tools Used

- bandit, mypy, ruff, gosec, govulncheck, golangci-lint, npm-audit.
- SIN-Code tools: discover, map, grasp, scout, sckg, adw, oracle.

## Verification

- [ ] All 47 gates ran.
- [ ] Findings mapped to CWE IDs.
- [ ] Risk score computed.
- [ ] Report saved.
