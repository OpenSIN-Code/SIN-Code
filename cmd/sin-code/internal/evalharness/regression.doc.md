# evalharness/regression.go — diff two runs

`Compare` produces a case-by-case diff between a baseline and a
candidate `Run`. The CLI exposes `--fail-on-regress` so this can act
as a CI gate.

## Delta kinds

| Kind | Meaning |
|---|---|
| `added` | case exists in candidate but not baseline |
| `removed` | case exists in baseline but not candidate |
| `improved` | score went up by more than `epsilon` |
| `regressed` | score went down by more than `epsilon` |
| `unchanged` | within `epsilon` of baseline |

## Epsilon

Default 0.001 — ignores float noise. Pass a higher value to be
more tolerant of small wobble in stochastic models.

## `HasRegressions` semantics

True if **either** any case regressed **or** the aggregate score
went down. This is conservative: a stable aggregate with a single
zero-score case is still a regression worth flagging.

## Related files

- `types.go` — `Run`, `Result`
- `cli.go` — `compare` subcommand
