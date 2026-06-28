---
name: skill-process-create
description: Teaches the SIN-Code / OpenCode skill creation process from intent to a validated, bundled skill. Use when the user asks "create skill", "new skill", "how to build a skill", or wants to add a reusable agent capability to the repository.
license: MIT
compatibility:
  - sin-code
  - opencode
  - claude-code
  - codex
metadata:
  author: SIN-Code
  version: 3.23.0
lifecycle: native
---

# skill-process-create

## Overview

Create a new, valid SIN-Code / OpenCode skill. This skill walks through the full process from identifying the need to shipping a bundled skill inside the repository. It is process-oriented: it focuses on decisions, conventions, and validation rather than on code implementation details (see `skill-code-create` for the scaffold-level view).

## When to Use

- User says "create skill", "new skill", "skill-process-create" or "/skill-process-create".
- User asks how to build a skill.
- A new, reusable agent capability should be packaged as a skill and bundled into the repository.

## When NOT to Use

- Updating an existing skill (edit the skill's files directly).
- Running a one-off prompt that does not need a reusable skill.
- Building an MCP server (use `sin-mcp-server-builder` instead).
- Adding code-level features outside the skill framework.

## Core Process

```
GATHER INTENT → CHOOSE CATEGORY & NAME → SCAFFOLD STRUCTURE →
WRITE SKILL.md → FILL CONTEXT/FRAMEWORKS/TASKS/TEMPLATES →
LICENSE → VALIDATE → UPDATE DOCS → BUILD & TEST → COMMIT
```

1. Clarify the skill's purpose, trigger phrases, and scope.
2. Pick a category from the canonical list and a valid `skill-<category>-<name>` name.
3. Create the directory under `skills/<category>-skills/<skill-name>/`.
4. Write `SKILL.md` with YAML frontmatter, overview, triggers, process, and verification checklist.
5. Add at least one `.md` file to each of `context/`, `frameworks/`, `tasks/`, `templates/`.
6. Add a `LICENSE` file (MIT for bundled skills).
7. Validate with `python3 scripts/validate_skill.py <skill-dir> --strict`.
8. Validate all bundled skills with `python3 scripts/validate_skill.py --all-bundled --strict`.
9. Run `go build ./cmd/sin-code/...` and `go test ./cmd/sin-code -race -count=1`.
10. Update `README.md`, `AGENTS.md`, `CHANGELOG.md`, and `ECOSYSTEM.md` for bundled skills.
11. Commit with a conventional commit message and push to `main`.

## Skill Structure

```
skill-process-create/
├── SKILL.md
├── LICENSE
├── context/
│   └── triggers.md
├── frameworks/
│   └── standards.md
├── tasks/
│   └── workflow.md
└── templates/
    ├── prompt.md
    └── output.md
```

## Naming Rules

- Bundled skills: `skill-<category>-<descriptive-name>` (e.g., `skill-process-create`, `skill-code-ceo-audit`).
- Local skills: any valid kebab-case name, e.g., `skill-create`.
- Directory name and `name:` field in `SKILL.md` frontmatter must match exactly.
- Valid kebab-case: `^[a-z0-9]+(-[a-z0-9]+)*$`.

## Lifecycle Values

| Value | Meaning |
|---|---|
| `native` | Embedded in the `sin-code` binary (default for bundled skills). |
| `external` | Ported from an external source; requires `sources:` in metadata. |
| `deprecated` | No longer recommended; retained for compatibility. |
| `internal` | Local or process-focused skill; accepted by the bundled validator. |

## Verification

- [ ] `python3 scripts/validate_skill.py <skill-dir> --strict` passes.
- [ ] `python3 scripts/validate_skill.py --all-bundled --strict` passes.
- [ ] `SKILL.md` has valid YAML frontmatter with `name`, `description`, and `lifecycle`.
- [ ] All required directories exist and contain `.md` files.
- [ ] `LICENSE` file is present.
- [ ] Name matches the directory and frontmatter.
- [ ] `go build ./cmd/sin-code/...` passes.
- [ ] `go test ./cmd/sin-code -race -count=1` passes (or the relevant test scope).
- [ ] `README.md` / `AGENTS.md` / `CHANGELOG.md` / `ECOSYSTEM.md` are updated for bundled skills.
