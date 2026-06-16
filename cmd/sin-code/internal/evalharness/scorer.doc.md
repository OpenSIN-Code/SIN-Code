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
| `CompileAndRun` | extract, compile, and run a self-check on generated code |

## `CompileAndRun`

`CompileAndRun` is the SIN-Code equivalent of ponytail's `correctness.js`
gate. It extracts the first fenced code block from the model output,
compiles it, and runs a self-check in a sandboxed subprocess.

Supported languages: `go`, `python`, `javascript`, `bash`.

Score semantics:
- No code block → `0.0` / fail.
- Compile failure → `0.0` / fail.
- `SkipTest=true` and compile passes → `1.0` / pass (YAGNI for trivial
  one-liners).
- Compile passes but no `SelfCheck` and `SkipTest=false` → `0.5` / fail.
- Compile + self-check pass → `1.0` / pass.

The default timeout is 30 seconds; set `Timeout` to override.
`Binary` optionally pins the compiler/interpreter executable.

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
