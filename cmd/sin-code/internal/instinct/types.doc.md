# instinct/types.go — domain model

Single learned behavior with a confidence score. The model mirrors the
schema of `affaan-m/ecc` continuous-learning-v2 (frontmatter fields,
confidence 0.3–0.9, scope, project-hash, status lifecycle) but is a
clean-room reimplementation in Go — **no foreign code is vendored**.

## What this file owns

- `Instinct` struct (frontmatter + body)
- `Scope` (`project` | `global`) and `Status` (`pending` | `active` | `evolved` | `archived`)
- Confidence math: `Reinforce`, `Contradict`, `Decay`
- `SignatureKey()` for cross-project dedup
- `Slugify`, `SortByConfidence` helpers

## Why these numbers

| Constant | Value | Reason |
|---|---|---|
| `MinConfidence` | 0.30 | Floor; below this the instinct is noise |
| `MaxConfidence` | 0.90 | Cap; even a habit shouldn't dominate over user intent |
| `ActivationThreshold` | 0.60 | `pending → active` cutoff |
| `EvolveThreshold` | 0.70 | Cluster-eligible cutoff (with `Observations ≥ 3`) |
| Reinforce step | 25% of remaining gap | Diminishing returns — no runaway |
| Contradict step | 40% of gap-to-floor | Faster decay than growth |
| Decay rate | 5%/30 days | Slow forget for stale habits |

## Related files

- `frontmatter.go` — Markdown serialization
- `store.go` — disk persistence
- `manager.go` — high-level API consumed by `internal/learning/`
- `extract.go` — heuristic + LLM-backed candidate generation
- `inject.go` — `SystemBlockForProject` builds the prompt block

## Tests

`types_test.go` (in this package) covers:
- confidence convergence
- contradiction clamping to floor
- decay monotonicity
- signature key stability across scope changes
