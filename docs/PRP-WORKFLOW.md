# SIN-Code PRP (Product Requirement Prompt)

The **Product Requirement Prompt** workflow. A PRP is a persistent,
reviewable plan-of-record for a change: a goal, context, decomposed
tasks, and acceptance criteria, stored as Markdown under
`.sin/prp/<id>.md`. The Engine drives it through phases —
**draft → planned → implementing → verifying → ready → shipped** —
persisting after every step so a run is interruptible and resumable.

> **Code:** `cmd/sin-code/internal/prp/` (~1,100 LOC, ~140 LOC tests).
>
> **CLI:** `sin prp new | run | status | plan | implement | verify | pr`.
>
> **Spec:** AGENTS.md §1 (this repo's source of truth).

## Why this exists

Three problems it solves:

1. **Resumability.** A long-running change survives `Ctrl-C`,
   process crash, context compaction, even a power outage. The
   next session reads `.sin/prp/<id>.md` and resumes from the
   last persisted phase.
2. **Reviewability.** The plan lives in the repo (`.sin/prp/`),
   diffable in `git log`, reviewable in a PR. The "why" is
   preserved alongside the "what".
3. **Bounded autonomy.** Verification failure kicks the PRP back
   to `implementing` automatically; the system never reports
   "done" without a green gate.

## Phase machine

```
draft   ──RunPlan──►  planned
planned ──RunImplement──►  implementing (or back to planned if blocked)
implementing ──RunVerify──►  verifying
verifying ──(pass)──►  ready
verifying ──(fail)──►  implementing  (kick back for fixes)
ready    ──RunPR──►  shipped
```

Each transition persists the PRP. A crash mid-run loses at most
one step.

## PRP file format

`.sin/prp/<id>.md`:

```markdown
---
id: my-change
title: My change
phase: planned
goal: ...
created_at: 2026-06-16T00:00:00Z
updated_at: 2026-06-16T01:23:45Z
tasks:
  - id: t1
    title: ...
    state: todo
  - id: t2
    title: ...
    state: todo
---

# My change

## Goal

...

## Context

...

## Plan

...

## Acceptance Criteria

...
```

The frontmatter is the **machine** view (parsed by the engine).
The body is the **human** view (read by reviewers, agents, and
`sin prp show <id>` if you add that subcommand).

## Four collaborators (interfaces you implement)

The engine delegates the hard work to four interfaces wired by the
host:

| Interface | Method | Implement against |
|---|---|---|
| `Planner` | `Plan(ctx, goal, context) ([]Task, plan, err)` | an agent / model call that decomposes a goal |
| `Implementer` | `Implement(ctx, p, t) (notes, err)` | an agent that executes one task, returns notes; error → task becomes `blocked` |
| `Verifier` | `Verify(ctx, workdir) (passed, report, err)` | **wired** — `wiring.PRPDeps(verifier, ...)` wraps your `verify.Gate` |
| `PRController` | `OpenPR(ctx, p) (url, err)` | git/gh glue: create branch, commit, push, open PR |

The default CLI registers `prp.Deps{}` (zero values), so `sin prp
status` and `sin prp new` work without any collaborators wired.
`sin prp run` will fail with a clear "no planner wired" error until
you supply a planner, implementer, and PR controller.

## CLI

```bash
# Create a draft PRP
sin prp new "Add rate limiting to API" \
  --goal "Limit each user to 100 req/min" \
  --context "Current API has no rate limit; abuse risk is high"

# Run the full pipeline (plan → implement → verify → pr)
sin prp run <id>

# Or step-by-step:
sin prp plan <id>        # Planner decomposes goal into tasks
sin prp implement <id>   # Implementer executes tasks (one by one)
sin prp verify <id>       # Verifier runs the quality gate
sin prp pr <id>          # PRController opens the pull request

# Status
sin prp status           # list all PRPs
sin prp status <id>      # show one PRP with task states
```

`sin prp new` generates an ID by slugifying the title + a 5-digit
suffix (`my-change-12345`). Same title in the same second collides
on disk; if that matters, rename the file before pushing.

## State transitions in detail

### `RunPlan(ctx, p)`

Calls `Planner.Plan(ctx, p.Goal, p.Context)`. The returned tasks
have their `ID` filled in (`t1`, `t2`, ...) and `State` set to
`todo` if missing. Sets `p.Plan` and `p.Phase = PhasePlanned`.
Persists.

### `RunImplement(ctx, p)`

For each task in order:
- Sets `State = doing`, persists.
- Calls `Implementer.Implement(ctx, p, t)`.
- On success: `State = done`, `Notes = result`. Persists.
- On error: `State = blocked`, `Notes = err.Error()`. Returns
  the error to the caller; run stops here.

When all tasks are `done`, sets `Phase = PhaseVerifying` and persists.

### `RunVerify(ctx, p)`

Calls `Verifier.Verify(ctx, workdir)`. On pass: `Phase = PhaseReady`.
On fail: `Phase = PhaseImplementing` (kick back so the implementer
can fix). On error: returns the error to the caller.

### `RunPR(ctx, p)`

Requires `Phase = PhaseReady`. Calls `PRController.OpenPR(ctx, p)`.
On success: `Phase = PhaseShipped` and persists.

### `RunAll(ctx, p)`

The convenience. Runs plan if `Phase = PhaseDraft`, then
implement, then verify, then PR. Stops at the first failure.

## Why verification failure kicks back to implementing

A failed verification means the implementation is not done, even
if every task is marked `done`. The system needs a way to retry
without losing progress. Setting `Phase = PhaseImplementing` puts
the PRP in a state where `sin prp implement <id>` can re-run from
the top of the task list (skipping already-done tasks), and
`sin prp verify <id>` can re-run the gate.

## Common failure modes

- **PRController.OpenPR returns a 409** (PR already exists for
  this branch). The engine persists `Phase = PhaseShipped` anyway,
  with the URL it received. Manual cleanup: close the duplicate
  PR.
- **Verifier returns an error** (not a fail, an error). The run
  stops with the error. Re-run `sin prp run <id>` after fixing
  the verifier.
- **Task blocked for 3+ retries.** The engine does not auto-retry.
  It surfaces the `Notes` field so the operator can decide whether
  to retry the task manually or skip it.

## Why not just use `sin-code daemon` for everything?

The daemon runs goals in the background. PRPs are **reviewable
plans** that survive across sessions, with explicit phase
transitions. A daemon task is ephemeral; a PRP is durable. The
two complement each other: the daemon could *use* PRPs as
its task source, the PRP `Implementer` could call the daemon
to actually run the work.

## Related

- AGENTS.md §1 (this repo's source of truth)
- `cmd/sin-code/internal/prp/` — package source
- `cmd/sin-code/internal/wiring/prp.go` — the default verifier wiring
- `docs/HOOKS-NEW.md` — the `Verifier` interface PRPs depend on
- `docs/CI-RUNBOOK.md` — recovery procedures
- `examples/eval-sets/` — how to validate PRP outcomes against an eval set
