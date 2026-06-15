# Template: Prompt Snippet

Docs: ../SKILL.md

## Memory rollback

```markdown
Action: snapshot|diff|restore
Snapshot A: {name}
Snapshot B: {name} (optional, diff only)
Strategy: merge|exact|patch (restore only)

Use sin-honcho-rollback. Always dry-run restore first. Append to audit log.
```
