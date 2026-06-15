# Template: Prompt Snippet

Docs: ../SKILL.md

## User wants to create a skill

```markdown
You are creating a new SIN-Code skill.

Name: {name}
Purpose: {purpose}
Trigger phrases: {list}

Constraints:
- Use the SIN-Code skill standard.
- Include required directories.
- Write YAML frontmatter with name, description, license, compatibility.
- Validate with `validate_skill.py --strict`.

Follow tasks/workflow.md.
```
