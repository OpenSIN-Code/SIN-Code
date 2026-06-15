# evalharness/types.go — data model

The data model: cases, sets, what the subject produced, what the
harness scored, the complete pass over a set, and the aggregate.

## Type relationships

```
EvalSet ── many ──▶ EvalCase
   │
   ▼
Runner.Execute(set) ──▶ Run
                          │
                          ├── many Results
                          └── Aggregate() → (weightedScore, passRate)
```

## Why weighted score + pass rate

Two numbers tell different stories:

- **weighted score** = how *good* the answers are (0..1, continuous)
- **pass rate** = how *often* they cleared the bar (0..1, binary)

A model can have pass rate 1.0 with weighted score 0.51 (always just
clearing the bar), or weighted score 0.95 with pass rate 0.10
(great answers, rarely matches the expected format). You almost
always want to see both.

## Related files

- `scorer.go` — produces `Result` from `(EvalCase, Output)`
- `runner.go` — produces `Run` from `(Subject, EvalSet)`
- `store.go` — persists both
- `regression.go` — diffs two `Run`s
