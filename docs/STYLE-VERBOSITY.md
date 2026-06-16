# Verbosity / Compression Mode

A first-class feature for trading off between token cost and response
richness. Inspired by the [caveman](https://github.com/JuliusBrussee/caveman)
prompt-compression pattern, adapted to SIN-Code's declarative style.

## Levels

| Mode | What changes | When to use it |
|---|---|---|
| `default` | nothing (pass-through) | keep-around; identical to `verbose` |
| `verbose` | nothing (pass-through) | default today; legacy behavior |
| `normal` | drops pleasantries and tool-call narration | everyday interactive |
| `terse` | fragments OK; drops articles/hedging | batch operations, scripted runs |
| `ultra` | causal chains via `→`; tightest valid compression | cost ceilings, big backlogs |

## What is byte-preserved in every mode

Code blocks, URLs, file paths, error strings, commit-type keywords,
exact line numbers, and `func`/`var`/`const` names are byte-preserved
in every mode. Only prose is compressed.

## Auto-clarity (mandate M3)

Every non-default ruleset carries an **auto-clarity** clause. When
the next action is:

- **Destructive** — `rm -rf`, force-push, schema drop, irreversible vendor action
- **Security-relevant** — token rotation, secret exposure, audit-trail change
- **Order-sensitive** — database migration, lock ordering, multi-step sequence

…the model is required to drop to **normal prose**, label the
section, then resume terse prose after. This satisfies mandate **M3
(the verification gate is sacred)**: terse output is never an
excuse to skip the careful prose around a destructive operation.

## CLI usage

```bash
# User-level: persist across runs
sin-code config set llm.style terse

# Project-level: override for one repo
echo 'llm.style = "ultra"' >> .sin-code/config.toml

# Verify
sin-code config show             # human-readable table
sin-code config show --toml      # exact TOML
sin-code config show --json      # JSON
sin-code config validate         # check for typos
```

```bash
# Runtime override for one shot (added in a follow-up PR, wire-up
# tracked separately): sin-code chat --style terse or --terse.
```

## Reference

- **Issue**: #167
- **Source of truth**: `internal/style/style.go`
- **Rulesets**: 3 `const` strings, byte-stable per `(mode, skillBody)`
- **Tests**: `internal/style/style_test.go` (race-safe),
  `internal/instinct/inject_test.go` (composition tests)
- **Auto-clarity rationale**: AGENTS.md §3 (M3 verification gate)
- **Config schema**: `cmd/sin-code/internal/config.doc.md`
