# prp/cli.go — `sin prp ...`

## Subcommands

| Command | Purpose |
|---|---|
| `prp new [title] --goal --context` | create a draft PRP |
| `prp run [id]` | full pipeline: plan → implement → verify → pr |
| `prp status [id]` | show one PRP (or list all) |
| `prp plan [id]` | run only the planning phase |
| `prp implement [id]` | run only the implementation phase |
| `prp verify [id]` | run only the verification phase |
| `prp pr [id]` | run only the PR-opener phase |

## Why per-phase commands

A user may want to:

- re-plan after a context change (`prp plan <id>` only)
- re-implement after a verification failure (`prp implement <id>`)
- open the PR by hand and just mark ready (`prp pr <id>`)

Splitting the commands keeps each one auditable and lets the CLI
be a thin shell over the engine.

## ID slug

`slugID(title)` produces `<slug>-<5-digit-suffix>` from the title.
The suffix is `time.Now().Unix() % 100000` — same title in the same
second collides, but that's unlikely and easy to fix with
`mv .sin/prp/<id>.md <new-id>.md`.

## Related files

- `engine.go` — the `Engine` the CLI wraps
- `store.go` — `Load` / `List`
