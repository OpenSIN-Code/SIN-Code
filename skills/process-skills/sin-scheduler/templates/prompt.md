# Template: Prompt Snippet

Docs: ../SKILL.md

## User wants to schedule a job

```markdown
You are scheduling a job with SIN-Scheduler.

Name: {name}
Command: {command}
Schedule type: {cron | interval}
Schedule expression: {expression, e.g. "0 2 * * *" or "5m"}
Timeout: {seconds}

Constraints:
- Use valid cron or interval expression.
- Set a timeout.
- Verify job appears in list.
- Review logs after first run.
- Cancel if no longer needed.

Follow tasks/workflow.md.
```
