# SIN-Code Eval-Harness

Eval-driven development: define an **EvalSet** (named cases with
prompts and optional expectations), run it against a **Subject**
(an agent run, a verify gate, a model call) using a **Scorer**
(exact, contains, success, LLM-judge, composite), and persist each
**Run**. `Compare` diffs two runs case-by-case to surface
improvements and regressions — that becomes a CI gate.

> **Code:** `cmd/sin-code/internal/evalharness/` (~770 LOC, ~140 LOC tests).
>
> **CLI:** `sin evalset run | list | compare`.
> (Note: `sin eval` is the older Golden-Dataset runner from issue #75;
> the new harness lives at `sin evalset` to avoid a cobra `Use:`
> collision.)
>
> **Spec:** AGENTS.md §1 (this repo's source of truth).

## Data model

```
EvalSet ── many ──► EvalCase
   │
   ▼
Runner.Execute(set) ──► Run
                          │
                          ├── many Results
                          └── Aggregate() → (weightedScore, passRate)
```

| Type | Purpose |
|---|---|
| `EvalSet` | Named collection of cases. Loaded from `<base>/sets/<name>.json` |
| `EvalCase` | One test scenario: `id`, `prompt`, optional `expected`, `tags`, `weight`, `meta` |
| `Output` | What the subject produced: `text`, `success`, `duration`, `meta` |
| `Subject` | The thing under evaluation. Implement `Run(ctx, EvalCase) (Output, error)`. |
| `Scorer` | Maps `(EvalCase, Output)` → `(score 0..1, passed, detail)` |
| `Result` | Per-case scored outcome: `score`, `passed`, `weight`, `output`, `detail`, `duration`, `error` |
| `Run` | One pass over an EvalSet: `id`, `set_name`, `subject`, `started_at`, `results` |

## Built-in Scorers

| Scorer | Use case |
|---|---|
| `ExactMatch` | regression-style: output must equal expected verbatim |
| `ContainsAll{PassThreshold}` | snippet-style: each line of `expected` is a required substring |
| `SuccessFlag` | use the subject's own pass/fail boolean (verify gate) |
| `LLMJudge{Judge func}` | delegate to a model via the `Judge` function field |
| `Composite{Scorers, Weights, PassThreshold}` | weighted average of multiple scorers |

`LLMJudge` deliberately accepts a function field rather than a
`Completer` interface. This keeps the package dependency-free: wire
the actual model client at the call site (in `internal/wiring/eval.go`)
where you already have the model adapter.

## CLI

```bash
# List the recorded runs (newest first)
sin evalset list
#   example-set-1747442400  2026-06-16 01:20  score=0.857 pass=75%

# Run an eval set against the chosen subject
sin evalset run go-quality --subject verify
#   [1/4] builds              score=1.00 pass=true
#   [2/4] vet-clean           score=1.00 pass=true
#   [3/4] tests-pass          score=0.50 pass=false
#   [4/4] no-secrets          score=1.00 pass=true
#   Run go-quality-1747442400 — score=0.875 pass-rate=75% (4 cases)

# Compare two runs (case-by-case delta)
sin evalset compare go-quality-1747442400 go-quality-1747442500
#   score: 0.875 -> 0.857  (improved=0 regressed=1)
#     regressed tests-pass   1.00 -> 0.50 (-0.50)

# Use --fail-on-regress as a CI gate
sin evalset compare base-run-id candidate-run-id --fail-on-regress
#   exit 0 if no regressions, exit 1 if any case regressed or aggregate dropped
```

## Wiring a Subject

The CLI takes a `subjectFactory func(name) (Subject, Scorer, error)`.
The wiring layer (`internal/wiring/eval.go`) provides the default:

```go
case "verify", "gate":
    return verifySubject{verifier: ...}, evalharness.SuccessFlag{}, nil
```

`verifySubject` wraps a `hooklife.Verifier` (your real
`verify.Gate`) and runs the quality check for the `workdir` named in
the case's `meta` field (or in `case.Prompt` as a fallback).

To plug in an agent-run subject, add a new case to the switch:

```go
case "agent":
    return myAgentSubject{...}, evalharness.ContainsAll{}, nil
```

## EvalSet file format

```json
{
  "name": "go-quality",
  "description": "Baseline quality checks for Go changes in SIN-Code",
  "cases": [
    {
      "id": "builds",
      "prompt": ".",
      "expected": "PASS",
      "tags": ["build"],
      "weight": 2.0,
      "meta": { "workdir": "." }
    },
    {
      "id": "tests-pass",
      "prompt": "./internal/instinct/... ./internal/hooklife/...",
      "expected": "ok",
      "tags": ["testing"],
      "weight": 2.0,
      "meta": { "workdir": "." }
    }
  ]
}
```

`weight` defaults to 1.0. `tags` are free-form and currently used
for filtering, not for routing. `expected` is optional — pass an
empty string to score purely on the subject's success flag.

Storage: `<base>/sets/<name>.json` and `<base>/runs/<run-id>.json`.
`<base>` resolution: `SIN_EVAL_DIR` → `$XDG_DATA_HOME/sin-code-eval` →
`~/.local/share/sin-code-eval`.

## Regression detection (CI gate)

`Compare(baseline, candidate, epsilon)` produces a `Comparison` with
`Deltas[]` (one per case, kind = `improved` / `regressed` / `unchanged`
/ `added` / `removed`). The `Comparison.HasRegressions()` method
returns true if **either** any case regressed **or** the aggregate
score went down — conservative by design.

```bash
# In CI:
RUN_ID=$(sin evalset run go-quality --subject verify 2>&1 | grep -oE '[a-z-]+-[0-9]+' | head -1)
BASE_ID=$(sin evalset list | head -1 | awk '{print $1}')

# ... store the base ID somewhere durable (git tag, env var, etc.) ...

sin evalset compare "$BASE_ID" "$RUN_ID" --fail-on-regress
```

## When to use eval-harness vs the older `sin eval`

| Use case | Command |
|---|---|
| Regression detection across PRs | **`sin evalset`** (this package) |
| Golden dataset with LLM-as-judge | `sin eval` (issue #75, LLMJudge path) |
| `tracing` + dataset visualization | `sin eval` (uses `internal/dataset` + `internal/trace`) |
| Custom scorers, per-case timeouts, JSONL run history | **`sin evalset`** (this package) |
| Cross-model A/B comparison | `sin eval` (designed for that) |
| Smoke test of the verify engine | **`sin evalset`** with `--subject verify` |

Both coexist. Use both; they answer different questions.

## Related

- AGENTS.md §1 (this repo's source of truth)
- `cmd/sin-code/internal/evalharness/` — package source
- `cmd/sin-code/internal/wiring/eval.go` — the default subject factory
- `examples/eval-sets/` — two ready-to-run example sets
- `docs/HOOKS-NEW.md` — the hook system that the verify Subject depends on
- `docs/CI-RUNBOOK.md` — recovery procedures for CI failures
