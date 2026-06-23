# evalharness/scorer.go — pluggable scoring

A `Scorer` is a pure function from `(EvalCase, Output)` to
`(score 0..1, passed, detail string)`. The runner does not care which
implementation it uses.

## Built-ins

| Scorer | Use case |
|---|---|
| `ExactMatch` | regression-style: output must equal expected verbatim |
| `ContainsAll` | snippet-style: each line of `expected` is a required substring |
| `SuccessFlag` | use the subject's own pass/fail boolean (verify gate) |
| `LLMJudge` | delegate to a model via the `Judge` function field |
| `Composite` | weighted average of multiple scorers |

## `LLMJudge` and circularity

`LLMJudge` deliberately accepts a function field rather than a
`Completer` interface. This keeps the evalharness package dependency-
free: wire the actual model client at the call site (in
`wiring/eval.go`) where you already have the model adapter.

## Why `Composite`

Two scorers often disagree (e.g. `ExactMatch` says 0, `LLMJudge` says
0.8 because the answer is correct but rephrased). `Composite` lets
you express "I trust the LLM judge 70%, the exact match 30%".

## Related files

- `runner.go` — applies the Scorer
- `types.go` — `Result` carries the score
