# wiring/prp.go — PRP/verify bridge

`PRPDeps(verifier, planner, impl, pr)` returns a `prp.Deps` ready
to feed into `prp.NewCommand`.

## The four collaborators

| Field | What it does | Where to wire it |
|---|---|---|
| `Planner` | decomposes goal + context into `[]Task` + plan text | an agent / model call |
| `Implementer` | executes one task, returns notes | an agent / model call |
| `Verifier` | runs the quality gate | **wired** — `hooklife.Verifier` adapter |
| `PRController` | opens the pull request | git/gh glue |

The Verifier is the only one that comes pre-wired; the other three
are interfaces you implement against your agent/orchestrator/git
layer. The `prpVerifier` adapter is a no-op when the `hooklife.Verifier`
is nil — useful for tests.

## Related files

- `prp/cli.go` — the consumer
- `prp/engine.go` — the engine
- `hooklife/builtin.go` — the `Verifier` interface
