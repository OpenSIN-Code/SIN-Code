# wiring/eval.go — eval subject bridge

`EvalSubjectFactory` returns a closure that the `sin eval` CLI
calls with a subject name and receives a `(Subject, Scorer, error)`
back.

## Default subject: `verify`

A `verify` subject wraps the `hooklife.Verifier` interface (the
same one `hooklife.QualityGate` uses). The workdir is taken from
`case.Meta["workdir"]`, falling back to `case.Prompt` if absent.

The scorer is `SuccessFlag` — the verifier already decides
pass/fail, so we just surface its boolean.

## Extending

Add new subjects by extending the `switch` in `EvalSubjectFactory`:

```go
case "agent":
    return myAgentSubject{...}, evalharness.ContainsAll{...}, nil
```

The factory pattern keeps the wiring layer out of the CLI.

## Related files

- `evalharness/cli.go` — the consumer
- `hooklife/builtin.go` — the `Verifier` interface
- `wiring.go` (other files in this package) — pipeline construction
