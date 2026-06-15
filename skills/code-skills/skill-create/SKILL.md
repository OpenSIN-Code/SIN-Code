---
name: skill-create
description: Creates and validates new SIN-Code / OpenCode skills. Use when the user says "create skill", "new skill", "skill-create", "/skill-create", or asks how to build a skill.
license: MIT
compatibility:
  - opencode
  - sin-code
metadata:
  author: SIN-Code
  version: 1.0.0
---

# skill-create

## Overview

Create a new SIN-Code / OpenCode compatible skill from a template. Produces a valid skill directory with SKILL.md, context/, frameworks/, tasks/, templates/, and optional scripts/tests/lib.

## When to Use

- User says "create skill", "new skill", "skill-create", or "/skill-create".
- User asks how to build a skill.
- A new agent capability needs to be packaged as a skill.

## When NOT to Use

- The user wants to update an existing skill (use the skill's own files).
- The task is a one-off prompt, not a reusable skill.

## Core Process

```
GATHER INTENT → CHOOSE NAME → SCAFFOLD → WRITE SKILL.md → ADD CONTEXT/FRAMEWORKS/TASKS/TEMPLATES → VALIDATE → COMMIT
```

1. Gather the skill's purpose, trigger phrases, and scope.
2. Pick a valid kebab-case name.
3. Scaffold the directory with `create_skill.py`.
4. Write SKILL.md with frontmatter + overview + when to use + when NOT to use + core process + verification.
5. Fill context/triggers.md, frameworks/standards.md, tasks/workflow.md, templates/output.md, templates/prompt.md.
6. Run `validate_skill.py --strict` on the new skill.
7. Optionally add scripts/tests/lib.

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "A single SKILL.md is enough." | SIN-Code requires context/frameworks/tasks/templates for maintainability. |
| "I'll skip validation." | Invalid skills fail CI and won't be discovered by agents. |
| "The name doesn't matter." | Must match `^[a-z0-9]+(-[a-z0-9]+)*$` and the directory name. |

## Red Flags

- Missing required directories.
- Broken YAML frontmatter.
- No verification checklist.
- Name mismatch between frontmatter and directory.

## Verification

- [ ] `validate_skill.py <skill-dir> --strict` passes.
- [ ] `validate_skill.py --all-bundled --strict` still passes.
- [ ] SKILL.md is clear and complete.
- [ ] Symlink created in `.claude/skills/` if the user wants OpenCode/Claude discovery.
