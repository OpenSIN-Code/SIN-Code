# Template: Prompt Snippet

Docs: ../SKILL.md

## User asks for a plan

```markdown
You are creating an implementation plan for SIN-Code.

Spec: {spec path}

Constraints:
- Atomic tasks: one file change or test per task.
- Order by dependencies.
- Every task needs acceptance criteria.
- Save to `.sin/plans/`.
- Get Critic or user review.

Follow tasks/workflow.md.
```
