# SIN-Code Hooks

The lifecycle-hook subsystem. Hooks fire at **Phases** in the agent
loop; a `PreToolUse` hook may `Block` (the ECC exit-code-2 equivalent)
to veto a tool call; other phases may only `Warn`. Per-hook timeout
and panic recovery: a misbehaving hook never breaks a session.

> **Code:** `cmd/sin-code/internal/hooklife/` (~770 LOC).
> Native Go, no Node dependency.
>
> **CLI:** `sin hooks list | test [phase]`.
>
> **Spec:** AGENTS.md §1 (this repo's source of truth).

## Phases

| Phase | When | May Block? |
|---|---|---|
| `PreToolUse` | before a tool runs | **yes** |
| `PostToolUse` | after a tool runs | no (Warn only) |
| `Stop` | turn finished | no |
| `SessionStart` | session begins | no |
| `UserPrompt` | a user prompt was submitted | yes |
| `SessionEnd` | session ends | no |
| `PreCompact` | before context compaction | no |

`Block` outside `PreToolUse` is recorded as a warning in the aggregate —
the runner cannot actually stop a lifecycle point that has already
started.

## Built-in hooks (default set, registered at startup)

| ID | Phase | Purpose |
|---|---|---|
| `block-no-verify` | PreToolUse | refuse `git commit --no-verify` |
| `config-protection` | PreToolUse | block edits to `.git/`, `go.sum`, `.env` |
| `post-edit-format` | PostToolUse | run `gofmt` / `prettier` / `ruff format` |
| `post-edit-typecheck` | PostToolUse | surface LSP diagnostics as a warning |
| `quality-gate` | PreToolUse | run verifier before `git commit` |
| `cost-tracker` | PostToolUse | record per-tool spend to ledger |
| `suggest-compact` | Stop, PreCompact | warn when context is large |

Configure via `Options{Workdir, LLM, Model, Memory, VerifyGate}` passed
to `learning.New(...)` in the wiring layer. The `quality-gate` hook
is the only one that needs the `verify.Gate` wired; all others work
out of the box.

## CLI

```bash
# List every registered hook with its phase set
sin hooks list

# Dispatch a synthetic event through the runner
sin hooks test PreToolUse --tool Bash --command "git commit --no-verify"
# expected: verdict=block, message="git commit --no-verify is not allowed; ..."

sin hooks test PostToolUse --tool Edit --path "cmd/sin-code/main.go"
# expected: verdict=allow (or warn if gofmt would change something)
```

## Adding a new hook

A `Hook` is a Go interface:

```go
type Hook interface {
    ID() string
    Phases() []Phase
    Run(ctx context.Context, ev Event) Decision
}
```

Three steps to add one:

1. **Implement the interface** in your package (or in
   `internal/hooklife/builtin.go` if it's general-purpose).
2. **Register it** at startup:
   ```go
   reg := hooklife.NewRegistry()
   reg.Register(hooklife.BlockNoVerify{})
   reg.Register(myCustomHook{})  // ← here
   runner := hooklife.NewRunner(reg).WithTimeout(10 * time.Second)
   ```
3. **Wire the runner** into the agent loop (see
   `internal/learning/learner.go` for the canonical place).

## Decision semantics

| `Verdict` | Effect on runner |
|---|---|
| `Allow` | proceed silently |
| `Warn` | proceed + surface `Message` to the user/agent |
| `Block` | veto the action (PreToolUse only) |

For `PreToolUse`, the **first** `Block` verdict short-circuits the
dispatch. For every other phase, `Block` is folded into a warning.

Warnings from all hooks of a phase are joined with `[hook-id] message`
format and surfaced in a single `Decision.Message`. The agent loop
appends this to the next user-facing turn.

## Per-hook isolation

Each hook runs in its own goroutine with `context.WithTimeout` (default
10s). If the hook returns, panics, or times out, the runner still
proceeds to the next hook. A misbehaving hook can never break a
session.

```go
type Decision struct {
    Verdict Verdict
    Message string
    HookID  string
}
```

## Why two hook systems (and how they relate)

`cmd/sin-code/internal/hooks/hooks.go` is the **legacy** hook system
(user-configured shell commands and webhooks via
`~/.config/sin/sin-code.toml` legacy `[hooks]` section; see
`docs/HOOKS.md`). It has the same Phase names
but a YAML-driven configuration.

`cmd/sin-code/internal/hooklife/` is the **native Go** hook system
(structured `Hook` interface; runtime-wired via the `learning` package).
The two fire in parallel; the legacy one runs first because it is
user-configurable and may want to veto before the Go layer sees the
event. They never conflict — both return a `Decision`; the agent
loop takes the most restrictive.

If you are building a new feature, **default to `hooklife`**. The
legacy `hooks` system is for end-user customisation only.

## Related

- AGENTS.md §1 (this repo's source of truth)
- `docs/HOOKS.md` — the legacy user-configurable hook system
- `cmd/sin-code/internal/hooks/hooks.go` — the legacy implementation
- `cmd/sin-code/internal/hooklife/` — the native Go implementation
- `cmd/sin-code/internal/learning/learner.go` — the bridge
- `docs/INSTINCTS.md` — the instinct subsystem that consumes hook events
