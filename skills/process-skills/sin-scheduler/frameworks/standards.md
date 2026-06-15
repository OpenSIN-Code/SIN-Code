# Frameworks: Standards & Constraints

Docs: ../SKILL.md

## Technology Stack

- `sin-scheduler` MCP server / CLI.
- SQLite database at `~/.sin_scheduler/scheduler.db`.
- `schedule` and `croniter` libraries.

## Standards

- Every job has a clear name and timeout.
- Cron expressions must be valid.
- Interval expressions must be human-readable (e.g., `5m`, `1h`).
- Logs inspected after failures.

## Constraints

- Daemon runs in background thread.
- Logs retained for 30 days by default.
- Jobs persisted across restarts.

## Quality Gates

- Job created and listed.
- First execution logged.
- Logs reviewed for errors.
