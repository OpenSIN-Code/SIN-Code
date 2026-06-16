# prp/types.go — data model

A PRP is the plan-of-record for a single change. It is:

- a goal (what we're trying to do)
- a context (what we already know)
- a plan (the approach)
- acceptance criteria (how we know we're done)
- a list of tasks (the units of work)
- a phase (where we are in the lifecycle)

The file lives in the repo (`.sin/prp/<id>.md`) so the agent's
plan is reviewable, diffable, and resumable across sessions.

## Phase machine

```
draft ─► planned ─► implementing ─► verifying ─► ready ─► shipped
                          ▲              │
                          └──────────────┘  (verify failed -> back to implementing)
```

## Why persistent + in-repo

- **Reviewable** — humans can read the plan before any code changes
- **Resumable** — a session crash or context compaction doesn't lose
  the plan; the next session picks up at the last persisted phase
- **Auditable** — `git log .sin/prp/` shows the history of the
  engineering process, not just the code
