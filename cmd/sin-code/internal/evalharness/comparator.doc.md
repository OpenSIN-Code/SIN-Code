# evalharness/comparator.go — four-arm Compare

`Compare(ctx, EvalSet, []Arm, CompareOptions) (CompareReport, error)`
runs every supplied arm against every case in the EvalSet, and
returns a matrix whose rows are arms and whose columns are
ponytail-shaped (LOC, USD, latency, correctness).

## The arms

| Reserved ID | Source | When to use |
|---|---|---|
| `__baseline__` | empty system prompt | the "no instruction" control for raw model behaviour |
| `__terse__` | `"Answer concisely."` | the honest control for any skill (caveman evals/README.md §3) |
| `__lazy_skill__` | terese-prefixed `skill-code-lazy` body (issue #178) | the lazy-skill control |
| `<user-supplied>` | terse-prefixed SKILL.md body | the candidate |

Every non-baseline arm is prefixed with the terse instruction so
the resulting delta = `(<arm> − terse)` isolates the skill's own
contribution from the generic "be terse" effect.

## Result extension (backward-compatible)

`Result` carries five new `omitempty` fields: `ArmID`,
`PromptTokens`, `CompletionTokens`, `TotalTokens`, `LOC`, `USD`.
Old consumers that don't read these fields see no change.

## Aggregates

`CompareReport.TotalsByArm` is per-arm rollup across cases:

- `PassRate()` = `Passed / TotalCases`
- `WeightedScore` = mean `Result.Score`
- `LOC`, `USD`, `Tokens`, `LatencyMS` arrays so the snapshot
  builder can compute medians without extra plumbing

`CompareReport.PerCase` is per-case rollup across arms; each row
is keyed by `arm.ID` for jq diffing.

`CompareReport.ByArm()` pivots for downstream tools that expect
`<arm-id> → []run`.

## Concurrency

The default `Compare` is serial — exactly four (or five) calls
to `Subject.Run`, in arm declaration order. `CompareParallel`
exposes the parallel path; it is verified to produce the same
TotalsByArm counts under `-race`.

## Related files

- `arms.go` — built-in arm constructors (`DefaultArms`, `SkillArm`,
  `LazySkillArm`, `VerbosityArm`).
- `prices.go` — USD/1k token price book (`stub`, `gpt-4o-mini`, …).
- `snapshot.go` — byte-stable snapshot serialisation +
  `DiffSnapshots` for two-snapshot diff.
- `regression.go` — single-arm `CompareRuns` regression
  comparator (the legacy `Compare` signature is preserved under
  the renamed name).
- `types.go` — `Arm`, `Result` extensions.
