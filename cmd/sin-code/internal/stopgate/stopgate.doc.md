# `stopgate.doc.md` — Independent Completion Authority

The stop-gate is the harness that decides whether a goal is **truly** done. It
decouples completion authority from the worker (2026 agent-loop best practice):
the worker only *proposes* completion (no more tool calls AND the verify-gate
passed); the stop-gate *confirms* it against a `GoalContract`, or forces the loop
to keep working with the open criteria injected back into the conversation. This
is the core anti-babysitting mechanism.

## Mode: HYBRID

1. **Deterministic checks first** (build/test/lint/predicate/diff-scope, via
   `internal/orchestrator`). Reproducible and free. **Fail-closed:** every failed
   check blocks completion — not just the orchestrator's mandatory kinds, so a
   failing `done-when` or `no-new-todos` predicate also blocks.
2. **Semantic judge second** (`internal/eval`, a strong/equal model). Only reached
   when deterministic checks are green. The judge can **reject** a green result
   but can **never** resurrect a red one (we return before consulting it).

## Decision matrix

| Deterministic | Judge | Result |
|---|---|---|
| any fail | (not consulted) | **continue** (open criteria = failed checks) |
| all pass | reject | **continue** (open criteria = judge reason/feedback) |
| all pass | accept | **complete** |
| all pass | error (fail-open, default) | **complete** (judge unavailable) |
| all pass | error (fail-closed opt-in) | **continue** |

## Files that import / touch it

- `cmd/sin-code/internal/agentloop/loop.go` — calls `Loop.StopGate` after verify-pass.
- `cmd/sin-code/internal/loopbuilder/builder.go` — constructs the Hybrid + wires the judge.
- `cmd/sin-code/internal/goalcontract` — supplies the contract.
- `cmd/sin-code/internal/eval` — the semantic Judge (interface, so tests inject fakes).

## Config & env

| Knob | Default | Meaning |
|---|---|---|
| `SIN_EVALUATOR_MODEL` | worker model | model used for the semantic judge |
| `SIN_EVALUATOR_BASE_URL` / `SIN_EVALUATOR_API_KEY` | worker client | separate evaluator endpoint |
| `WithFailClosedOnJudgeError()` | off (fail-open) | block instead of accept on judge infra error |

## Known caveats / footguns

- **Fail-open on judge error by default.** A flaky evaluator network must never
  trap the loop forever; deterministic failures are always fail-closed regardless.
- **A nil judge disables semantic evaluation.** The gate is then a strict superset
  of the verify-gate (deterministic only).
- **The gate runs only after verify-pass.** It tightens, never loosens, the
  existing completion bar; a nil `Loop.StopGate` preserves exact legacy behavior.
