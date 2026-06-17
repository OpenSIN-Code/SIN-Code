---
name: skill-planning-enterprise
description: Agent-first enterprise planning skill with a strict JSON CLI, deterministic validation, idempotent execution, and governance-aware rollback.
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
sources: 
---

# /plan v2.1 — Agent-first enterprise planning

## Contract
- Use `opencode-plan`; JSON is the machine contract.
- Never guess. Validate before execute.
- Exit codes are authoritative: `0` ok, `1` validation, `2` execution, `3` approval, `4` drift, `5` unknown.
- Keep execution state under `.plan/` and make execution idempotent.
- Keep the skill docs and the packaged runtime aligned.

## Commands
- `opencode-plan init --template <enterprise|agile|compliance> --out plan.json`
- `opencode-plan validate plan.json --strict --auto-fix`
- `opencode-plan simulate plan.json --iterations 10000`
- `opencode-plan execute plan.json --mode plan-only|plan-and-execute|continuous`
- `opencode-plan audit plan.json --format json|sarif|markdown`
- `opencode-plan rollback plan.json --to latest --dry-run`
- `opencode-plan schema --target plan|response`

## Plan model
Every plan should include:
- outcomes / OKRs
- current state analysis
- decisions and assumptions
- phases with concrete tasks
- explicit dependency graph
- risk register and rollback plan
- done criteria and approvals
- metrics and learning fields

## Workflow
1. Check for an approved plan.
2. Validate the plan and repair trivial dependency cycles.
3. Simulate risk and duration.
4. Approve or request approval.
5. Execute one bounded step at a time.
6. Audit each state change.
7. Roll back on drift.
8. Learn from actual vs planned duration.

## Governance
- Approval gates must be explicit.
- Audit records must be machine-readable.
- Drift must pause execution.
- Rollback must be dry-runable before apply.

## Legacy note
The old stage-heavy design is still valid as the planning policy, but the runtime is now the packaged CLI under `src/plan_cli/`.
