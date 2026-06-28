---
name: skill-process-scheduler
description: Use when user says 'schedule job', 'cron', 'interval', 'cancel job', 'run job now', 'job logs'. Job scheduling with cron expressions and human-readable intervals via MCP server and CLI. Schedule, list, cancel, run, and inspect job execution logs.
license: MIT
compatibility:
  - sin-code
  - opencode
  - claude-code
  - codex
metadata:
  author: SIN-Code
  version: 3.20.0
  sources: "OpenSIN-Code/Infra-SIN-OpenCode-Stack/skills/scheduler"
required_tools:
  - sin_execute
lifecycle: external
---

# skill-process-scheduler

## Overview

Schedule recurring or one-off jobs using cron expressions or intervals. Manage jobs and inspect execution logs through MCP or CLI.

## When to Use

- Schedule a recurring task (cron or interval).
- List, cancel, or trigger scheduled jobs.
- Inspect execution history and logs.
- Run ad-hoc jobs.

## When NOT to Use

- One-shot commands that need no persistence (use `sin_execute`).
- Complex workflow orchestration (use `sin-orchestrate`).
- Long-running daemon processes without scheduling semantics.

## Core Process

```
DEFINE JOB → SCHEDULE → MONITOR → RUN/CANCEL → REVIEW LOGS
```

1. Define job name, command, and schedule (cron or interval).
2. Schedule the job.
3. Monitor status.
4. Run immediately or cancel as needed.
5. Review execution logs.

## MCP Tools

| Tool | Purpose |
|---|---|
| `schedule_job` | Create a scheduled job |
| `schedule_list` | List jobs |
| `schedule_cancel` | Remove a job |
| `schedule_status` | Show job status |
| `schedule_run_now` | Trigger a job immediately |
| `schedule_logs` | Show recent execution logs |

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "I'll just run it manually." | Recurring tasks need persistence and logs. |
| "Cron syntax is too complex." | Intervals like `5m` are human-readable. |
| "Logs are optional." | Logs are essential for debugging scheduled jobs. |

## Red Flags

- Jobs without clear timeout.
- Commands that depend on unverified environment.
- No log inspection after failures.
- Jobs scheduled but never monitored.

## Verification

- [ ] Job name and command are clear.
- [ ] Schedule is valid (cron or interval).
- [ ] Timeout set appropriately.
- [ ] Job listed after creation.
- [ ] Logs inspected after first run.
