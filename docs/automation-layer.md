# Go Automation Core-Layer (issue #108)

Issue #108 is a system & integration report for the wider OpenSIN-Code
ecosystem. Most of it (opencode.jsonc, pre/post-session shell hooks, Hugging
Face Spaces monitoring) belongs to *infra* repos, not to this Go binary. The
**actionable, in-repo Go part** is a native automation core: a tool
**registry**, a lifecycle **hook manager**, and a **chain engine** that links
tools into self-repairing loops. Those are implemented here under `pkg/`.

> The Go snippets in the issue contained copy/paste defects (e.g.
> `Capabilitiesstring`, `hm.hooks =HookFunc{}`, the wrong module path
> `OpenSIN-Code/sin`). This implementation is the corrected, compiling,
> race-safe, fully-tested version.

## Packages

### `pkg/tools` — tool registry

A process-wide registry of agent tools. Each tool implements `Tool`
(`Execute` + `GetMetadata`) and registers its semantic metadata and
capabilities. The registry deterministically generates the system-prompt
fragment listing every tool, so an agent can never miss one.

```go
r := tools.GetRegistry()
_ = r.RegisterTool(myTool)
prompt := r.GenerateAgentPrompt()   // stable, sorted listing
names  := r.ToolsByCapability("fs") // capability -> tool routing
```

### `pkg/hooks` — lifecycle hook manager

Hooks intercept tool calls at `BeforeToolExecution`, `AfterToolExecution`,
and `OnLoopFailure`. A hook can mutate the threaded `*Context`; the first
error aborts the phase. A default hook fires the validation chain after a
successful `write_file`.

```go
hm := hooks.GetManager()
hm.Register(hooks.AfterToolExecution, func(hc *hooks.Context) (*hooks.Context, error) {
    if hc.Err != nil { /* evaluate self-repair chain */ }
    return hc, nil
})
```

### `pkg/chains` — chain engine

Links registry tools into closed loops. On a step error or a validator
rejection, the failure context is fed into the next loop's input for
self-repair, up to a loop budget.

```go
e := chains.NewEngine()
out, err := e.ExecuteLoopChain("fix-lint",
    []chains.Step{{ToolName: "edit"}, {ToolName: "lint", InputMapper: mapPrev}},
    initialInput,
    func(o interface{}) bool { return o == "clean" },
    5, // maxLoops
)
```

## Design properties

- **Native Go:** runs inside the compiled CLI binary — no Node.js runtime
  overhead for tool composition.
- **Determinism:** prompt generation and capability listings are sorted, so
  output is stable across runs and map-iteration order.
- **Race-safe:** registry and hook manager guard state with `sync.RWMutex`;
  `Trigger`/`Execute` snapshot the hook slice before running.
- **Testable:** singletons (`GetRegistry`, `GetManager`) are paired with
  constructors (`NewRegistry`, `NewManager`, `NewEngineWith`) so tests stay
  hermetic and never touch global state.

## Out of scope for this repo

The report's infra automation (global `opencode.jsonc`, `pre-session.sh` /
`post-session.sh`, v0-pool container restarts, HF Spaces keep-alive) targets
the `Infra-SIN-OpenCode-Stack` and deployment repos and is intentionally not
vendored into the SIN-Code Go binary.
