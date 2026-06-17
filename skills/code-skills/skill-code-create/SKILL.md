---
name: skill-code-create
description: Creates and validates new SIN-Code / OpenCode skills. Use when the user says "create skill", "new skill", "skill-code-create", "/skill-code-create", or asks how to build a skill.
license: MIT
compatibility:
  - sin-code
  - opencode
metadata:
  author: SIN-Code
  version: 3.20.0
required_tools:
  - sin_write
lifecycle: native
---

# skill-code-create

## Overview

Create a new SIN-Code / OpenCode compatible skill from a template. Produces a valid skill directory with `SKILL.md`, `context/`, `frameworks/`, `tasks/`, `templates/`, and optional `scripts/` / `tests/` / `lib/`.

As of **v3.20.0**, SIN-Code ships **37 bundled skills** embedded in the `sin-code` binary under `skills/<category>-skills/`. All bundled skills follow the naming convention `skill-<category>-<descriptive-name>`. New skills must be created in the correct category directory and added to the registry/docs before they are discoverable by agents.

## When to Use

- User says "create skill", "new skill", "skill-code-create", or "/skill-code-create".
- User asks how to build a skill.
- A new agent capability needs to be packaged as a skill.
- A new bundled skill needs to be added to `skills/<category>-skills/`.

## When NOT to Use

- The user wants to update an existing skill (edit the skill's own files directly).
- The task is a one-off prompt, not a reusable skill.
- The user wants a skill that lives outside the SIN-Code ecosystem (point them to the MCP server builder instead).

## Core Process

```
GATHER INTENT → CHOOSE CATEGORY & NAME → SCAFFOLD → WRITE SKILL.md →
ADD CONTEXT/FRAMEWORKS/TASKS/TEMPLATES → VALIDATE → UPDATE REGISTRY & DOCS → COMMIT
```

1. Gather the skill's purpose, trigger phrases, and scope.
2. Choose a category from the canonical list and a valid `skill-<category>-<name>` name.
3. Create the directory under `skills/<category>-skills/<skill-name>/`.
4. Write `SKILL.md` with frontmatter + overview + when to use + core process + verification.
5. Fill `context/triggers.md`, `frameworks/standards.md`, `tasks/workflow.md`, `templates/output.md`, `templates/prompt.md`.
6. Add a `LICENSE` file.
7. Run `python3 scripts/validate_skill.py --all-bundled --strict`.
8. Update `README.md`, `AGENTS.md`, `CHANGELOG.md`, and `ECOSYSTEM.md` if the skill is bundled.
9. Verify `go build ./...` still works and `go test ./...` passes.

## Bundled Skill Categories (v3.20.0)

| Category | Directory | Examples |
|---|---|---|
| Code | `code-skills/` | `skill-code-create`, `skill-code-audit`, `skill-code-build` |
| Browser | `browser-skills/` | `skill-browser-tools` |
| Debug | `debug-skills/` | `skill-debug-deep` |
| Design | `design-skills/` | `skill-design-frontend`, `skill-design-image` |
| Ecosystem | `ecosystem-skills/` | `skill-ecosystem-context`, `skill-ecosystem-marketplace` |
| GitHub | `github-skills/` | `skill-github-actions`, `skill-github-account` |
| Infrastructure | `infrastructure-skills/` | `skill-infrastructure-supabase`, `skill-infrastructure-cloudflare` |
| Memory | `memory-skills/` | `skill-memory-honcho`, `skill-memory-infisical` |
| Planning | `planning-skills/` | `skill-planning-enterprise` |
| Process | `process-skills/` | `skill-process-goal`, `skill-process-grill`, `skill-code-lazy` |
| Shop | `shop-skills/` | `skill-shop-stripe`, `skill-shop-cj-dropshipping` |

## Naming Rules

- Bundled skills: `skill-<category>-<descriptive-name>` (e.g., `skill-code-create`, `skill-github-actions`).
- Name must match the directory name and the `name:` field in `SKILL.md` frontmatter.
- Must be valid kebab-case: `^[a-z0-9]+(-[a-z0-9]+)*$`.
- Skills ported from external repos must include `lifecycle: external` and `sources:` in `SKILL.md` metadata.

### `required_tools` Frontmatter Field

- **What:** An optional YAML frontmatter field that binds a skill to specific SIN tools. Example:
  ```yaml
  required_tools: [sin_edit, sin_test, sin_quality_gate]
  ```
  or as a YAML block list:
  ```yaml
  required_tools:
    - sin_edit
    - sin_test
  ```
- **When to use:** When the skill has a deterministic tool dependency — i.e., the skill's workflow cannot be completed without calling a specific SIN tool. Omit the field if the skill has no hard tool requirement.
- **How it works:** Parsed by `cmd/sin-code/internal/skillmgr/required_tools.go` and merged into `CoverageRequiredTools` on the agent loop. Enforced at runtime by the `ToolCoverageEnforcer` (issue #248): if the model completes a run without invoking every listed tool, the loop rejects the completion and re-injects the violation as open criteria.
- **Validation:** `scripts/validate_skill.py --strict` validates that `required_tools` (if present) is a YAML list of known SIN tool names.

## External Skills

- External skills live in `~/.config/opencode/skills/` or are registered as MCP servers.
- They must still follow the `SKILL.md` standard and include `lifecycle: external` + `sources:`.
- Bundled external skills (e.g., from `Infra-SIN-OpenCode-Stack`,
  or `DietrichGebert/ponytail` → `skill-code-lazy` in `process-skills/`)
  are copied into `skills/<category>-skills/` with attribution.

## Pairing with `skill-code-lazy`

`skill-code-create` *defaults to* lazy: when you scaffold a skill,
start from the simplest template, add complexity only when needed.
For the review/refactor side of the same philosophy, see
`skills/process-skills/skill-code-lazy/` (SIN-Code variant of
ponytail, gated by `verify.pass` per M3). Use both together:

1. `skill-code-create` to build the skeleton (lazy-by-default).
2. `skill-code-lazy` in the review phase to trim over-engineering
   **after** the verify-gate passes.

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "A single SKILL.md is enough." | SIN-Code requires `context/`, `frameworks/`, `tasks/`, `templates/` for maintainability. |
| "I'll skip validation." | Invalid skills fail CI and won't be discovered by agents. |
| "The name doesn't matter." | Must match `skill-<category>-<name>` and the directory name. |
| "I don't need to update docs." | Bundled skills require `README.md`, `AGENTS.md`, `CHANGELOG.md`, and `ECOSYSTEM.md` updates. |
| "External skills don't need sources." | External skills must include `lifecycle: external` and `sources:` metadata. |

## Red Flags

- Missing required directories.
- Broken YAML frontmatter.
- No verification checklist.
- Name mismatch between frontmatter, directory, and file system.
- Wrong category directory (e.g., `skill-github-actions` in `code-skills/`).
- Missing `LICENSE` file.

## Verification

- [ ] `python3 scripts/validate_skill.py <skill-dir> --strict` passes.
- [ ] `python3 scripts/validate_skill.py --all-bundled --strict` still passes.
- [ ] `go build ./...` passes.
- [ ] `go test ./... -race -count=1` passes (or at least the relevant packages).
- [ ] `SKILL.md` is clear and complete.
- [ ] `README.md` / `AGENTS.md` / `CHANGELOG.md` / `ECOSYSTEM.md` updated for bundled skills.
- [ ] Symlink created in `.claude/skills/` if the user wants local OpenCode/Claude discovery (not required for binary-embedded skills).
