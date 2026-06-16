# prp/engine.go — phase machine

The engine runs the four collaborators in order, persisting after
every step. The phase machine is straightforward:

```
draft   ──RunPlan──►  planned
planned ──RunImplement──►  implementing (or back to planned if blocked)
implementing ──RunVerify──►  verifying
verifying ──(pass)──►  ready
verifying ──(fail)──►  implementing (kick back)
ready    ──RunPR──►  shipped
```

## Per-step persistence

`save(p)` is called after every state change. A crash, context
compaction, or `Ctrl-C` mid-run loses at most one step. The next
session loads the PRP from disk and resumes from its current phase.

## `RunAll` short-circuits

Stops at the first failure:

- `RunPlan` error → engine returns the planner error
- `RunImplement` blocked task → engine returns the task error
- `RunVerify` failed gate → engine returns the report
- `RunPR` error → engine returns the controller error

The user can fix the underlying issue and re-run `sin prp run <id>`.

## Why a `Log` field, not `log.Printf`

Different hosts (CLI, TUI, tests) want different output. A
`func(format, args...)` is the simplest abstraction that lets all
three plug in without coupling to `log` or `fmt`.

## Related files

- `types.go` — `Phase`, `TaskState`, `PRP`
- `store.go` — `Store.Save` / `Store.Load`
- `cli.go` — the consumer
