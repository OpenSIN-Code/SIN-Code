# Template: Prompt Snippet

Docs: ../SKILL.md

## User wants to create a skill

```markdown
You are creating a new SIN-Code bundled skill.

Category: {category} (must be one of: code, browser, debug, design, ecosystem, github, infrastructure, memory, planning, process, shop)
Name: skill-{category}-{descriptive-name}
Purpose: {purpose}
Trigger phrases: {list}

Constraints:
- Directory must be `skills/{category}-skills/{name}/`.
- Write YAML frontmatter with `name`, `description`, `license`, `compatibility`, `metadata`.
- For external/port skills, include `lifecycle: external` and `sources:` in metadata.
- If the skill has deterministic tool dependencies, add `required_tools:` as a YAML list (e.g. `[sin_edit, sin_test]`).
- Include required directories: `context/`, `frameworks/`, `tasks/`, `templates/`.
- Add a `LICENSE` file.
- Validate with `python3 scripts/validate_skill.py --all-bundled --strict`.
- Build and test with `go build ./... && go test ./... -race -count=1`.
- Update `README.md`, `AGENTS.md`, `CHANGELOG.md`, and `ECOSYSTEM.md` for bundled skills.

Follow `tasks/workflow.md`.
```
