# `sin-code eval` User Guide

Run Golden Datasets against the SIN-Code agent loop and produce CI-ready
JSON reports. See `evals/README.md` for the dataset catalog.

## Quick start

```bash
sin-code eval run --dataset evals/critical.json
sin-code eval run --dataset evals/critical.json --min-pass-rate 0.9 --json
sin-code eval list --dir evals
```

## Golden Dataset JSON schema

```json
{
  "name": "My Dataset",           // required
  "version": "1.0.0",             // required
  "description": "optional",
  "test_cases": [
    {
      "id": "unique-id",          // required
      "prompt": "What to do",     // required
      "tags": ["smoke"],
      "constraints": {
        "max_turns": 5, "timeout": "120s",
        "must_use_tools": ["sin_test_generate"],
        "forbidden_tools": ["sin_bash"],
        "require_verify": false
      },
      "expected": {
        "output_contains": ["PASS"],
        "output_avoids": ["error"],
        "contains_keywords": ["stub echo"],
        "min_quality": 0.0,
        "custom_criteria": "optional LLM-judge rubric"
      },
      "scorer": {
        "type": "compile_and_run",
        "language": "go",
        "self_check": "func main() { ... }"
      },
      "metadata": { "priority": "high" }
    }
  ]
}
```

**Scorer types:** `exact` (verbatim), `contains` (substring, default),
`compile_and_run` (extract, compile, run self-check).

## CLI reference — `eval run`

| Flag | Default | Purpose |
|------|---------|---------|
| `--dataset` `-d` | *(required)* | Golden Dataset JSON path |
| `--min-pass-rate` | `0.9` | CI gate (exit 1 if below) |
| `--json` | `false` | JSON output for CI |
| `--timeout` | `5m` | Per-case timeout |
| `--scorer` | *(dataset)* | Override: `compile_and_run\|exact\|contains` |
| `--language` | | For `compile_and_run` (go/python/javascript/bash) |
| `--self-check` | | Self-check code for `compile_and_run` |
| `--skip-test` | `false` | YAGNI: accept compile-only |
| `--arm` | | Comma-separated arms for four-arm comparator |
| `--skill` | `skill-code-create` | User-skill arm name |
| `--trace` | `false` | OpenTelemetry tracing |
| `--trace-exporter` | `stdout` | `stdout\|otlp\|noop` |
| `--judge-model` | | LLM-as-a-Judge model id |
| `--use-model` | `false` | Real LLM instead of offline stub |

### `eval compare` / `snapshot` / `diff`

```bash
sin-code eval compare  --dataset evals/three-arm-example.json
sin-code eval snapshot --dataset evals/three-arm-example.json --out snap.json
sin-code eval diff     --snapshot snap-a.json --snapshot-b snap-b.json
```

## Compile-and-run worked example

```bash
sin-code eval run --dataset evals/fuzzing.json \
    --scorer compile_and_run --language go \
    --self-check "func main() { if Add(1,2) != 3 { panic(\"fail\") } }"
```

Extracts the first fenced code block, compiles it, runs the self-check.
No code / compile fail = 0.0; compile + self-check pass = 1.0; compile
only (no self-check, `--skip-test=false`) = 0.5.

## Four-arm comparator (issue #171)

Runs the same dataset against four system-prompt arms, comparing LOC,
USD, latency, and pass-rate:

| Arm | ID | System prompt |
|-----|----|---------------|
| baseline | `__baseline__` | empty (raw model) |
| terse | `__terse__` | `"Answer concisely."` |
| lazy_skill | `__lazy_skill__` | terse + `skill-code-lazy` |
| user-skill | `<name>` | terse + your SKILL.md |

The **honest delta** = `(user-skill − terse)` isolates the skill's
contribution from the generic "be terse" effect.

## Tracing & LLM-as-a-Judge

```bash
sin-code eval run --dataset evals/critical.json \
    --trace --trace-exporter otlp --trace-endpoint langfuse.local:4318

sin-code eval run --dataset evals/critical.json \
    --judge-model gpt-4o --judge-key-env OPENAI_API_KEY
```

Tracing uses a pure-Go OTel SDK (no CGO, mandate M2). Missing judge API
key degrades gracefully — JSON report still emitted with judge fields zeroed.

## Real-model completion (issue #261)

By default `eval run` uses an offline stub. Use `--use-model` (or
`SIN_EVAL_USE_MODEL=1`) to route through a configured chat completion.
Requires `LLM_API_KEY`/`llm.api_key` + `LLM_MODEL`/`llm.model`.

## CI integration

The `eval-n8n.yml` workflow delegates to the n8n OCI VM (mandate M1):

```bash
sin-code eval run --dataset evals/critical.json --min-pass-rate 0.95 --json
echo $?  # 0 = pass, 1 = below threshold
```

## Exit codes

- `0` — pass rate meets or exceeds `--min-pass-rate`.
- `1` — pass rate below threshold or error.

## See also

- `evals/README.md` — dataset catalog and schema
- `docs/RFC-test-automation.md` — Test-First Verify-Loop design
- `cmd/sin-code/internal/eval/eval.doc.md` — judge/metrics internals
- `cmd/sin-code/internal/evalharness/comparator.doc.md` — comparator
