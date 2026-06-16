# internal/style — verbosity / compression mode (issue #167)

System-prompt renderer for agent output style. Lets the user trade off
between token cost and response richness without losing technical
substance — the ruleset only compresses *prose*, never code, URLs,
file paths, or error strings.

## Levels

| Mode | Empty block? | Effect |
|---|---|---|
| `default` | yes | No ruleset injected; the agent uses its native voice. |
| `verbose` | yes | Explicit alias for `default`; no change in behavior. |
| `normal` | no | Drop pleasantries and tool-call narration. Keep headings and code. |
| `terse` | no | One-line answers when one line is enough. Fragments OK. Drop articles. |
| `ultra` | no | Tightest compression. Causal chains with `→`. Abbreviate only prose. |

All non-empty rulesets share the **auto-clarity** clause:

> When the agent is about to do something destructive, security-relevant,
> or multi-step where fragment order matters, drop to normal prose,
> label the section, then resume.

## Auto-clarity

Three triggers force the model to drop terseness **for that section only**:

1. Destructive write (`rm -rf`, force-push, schema drop, irreversible vendor action)
2. Security-relevant (token rotation, secret exposure, audit-trail change)
3. Multi-step where order matters (database migrations, lock ordering)

This satisfies SIN-Code mandate **M3 (verification gate is sacred)**:
terse output is not an excuse to skip the careful prose around a
destructive operation.

## API

```go
mode := style.ParseMode("terse")
block := style.RenderRules(mode, "")     // style ruleset alone
block := style.RenderRules(mode, body)   // style + optional skill body
block := style.RenderSystemBlock("terse") // shorthand for ParseMode+RenderRules
```

Composition primitive:

```go
combined := style.AppendVerbosity(existingBlock, mode)
// existingBlock is always preserved verbatim
// combined = existingBlock + "\n\n" + rules when mode != default|verbose
```

Functional-option pattern for callers that build the system prompt
incrementally:

```go
var b strings.Builder
instinct.RenderSystemBlockInto(&b, active, 15, style.WithVerbosity(mode))
```

## Determinism

Every ruleset is a `const` string. Output is byte-stable across
builds for a given `(mode, skillBody)` pair. This is required so the
future system-prompt hash metric (issue #2) can lock it down with a
golden test.

## Race safety

The package owns no mutable state. All exported functions are pure.
Safe under `go test -race`.

## Wiring

| Caller | Hook |
|---|---|
| `instinct/inject.go` | `RenderSystemBlockWithOptions` appends the verbosity block after the instinct list. |
| `learning.Learner.BeforeTurn` | Pulls mode from `Options.Style` (defaulted from `LLMStyle`) and calls `RenderSystemBlockWithOptions`. |
| `chat_cmd.go --style / --verbosity` | CLI flag overrides the config for the running REPL/headless call. |
| `internal/config.go LLMStyle` | `sin-code config set llm.style terse`. |

## References

- Issue #167 — Verbosity / Compression Mode
- JuliusBrussee/caveman `skills/caveman/SKILL.md` — the ruleset source
  of truth adapted to SIN-Code's declarative style.
- `docs/STYLE-VERBOSITY.md` — user-facing spec.
