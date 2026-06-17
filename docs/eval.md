# `sin-code eval` User Guide

Run Golden Datasets against the SIN-Code agent loop and produce CI-ready
JSON reports.

## Quick start

```bash
# Run a dataset and print a human-readable report
sin-code eval run --dataset evals/critical.json

# Run with a minimum pass-rate gate and JSON output
sin-code eval run --dataset evals/critical.json \
    --min-pass-rate 0.9 \
    --json

# List all datasets
sin-code eval list --dir evals
```

## Datasets

Datasets live under `evals/` and are JSON files following the schema in
`cmd/sin-code/internal/dataset/dataset.go`. See `evals/README.md` for the
full catalog.

## Subcommands

| Subcommand | Purpose |
|------------|---------|
| `eval run` | Run a single dataset and report pass rate. |
| `eval compare` | Run the four-arm comparator (baseline / terse / lazy_skill / skill). |
| `eval snapshot` | Write a deterministic snapshot for CI diffing. |
| `eval diff` | Compare two snapshot files. |
| `eval list` | List datasets in a directory. |

## Scorers

By default, `eval run` uses the `contains` scorer for output checks. You can
override the scorer per run:

```bash
# Compile and run a fenced code block with a self-check
sin-code eval run --dataset evals/fuzzing.json \
    --scorer compile_and_run --language go \
    --self-check "func main() { if Add(1,2) != 3 { panic(\"fail\") } }"

# Exact match
sin-code eval run --dataset evals/critical.json --scorer exact
```

## Four-arm comparator

```bash
sin-code eval compare --dataset evals/three-arm-example.json --skill skill-code-create
sin-code eval snapshot --dataset evals/three-arm-example.json --out snap.json
sin-code eval diff --snapshot snap-base.json --snapshot-b snap-head.json
```

## Tracing

```bash
sin-code eval run --dataset evals/critical.json \
    --trace --trace-exporter otlp --trace-endpoint localhost:4318
```

## LLM-as-a-Judge

```bash
sin-code eval run --dataset evals/critical.json \
    --judge-model gpt-4o \
    --judge-endpoint https://api.openai.com/v1 \
    --judge-key-env OPENAI_API_KEY
```

## Real-model completion (issue #261)

By default, `sin-code eval run` uses the offline stub (`eval_cmd.co`
echoes the prompt back). Use `--use-model` (or `SIN_EVAL_USE_MODEL=1`) to
route each case through the configured OpenAI-compatible chat completion
instead.

```bash
sin-code eval run --dataset evals/test-generation.json \
    --use-model \
    --min-pass-rate 0.5
```

Requires either:

- `LLM_API_KEY` env var, or
- `llm.api_key` in user/project config (see `sin-code config get llm.api_key`).

`LLM_MODEL` env var or `llm.model` config is also required. The endpoint
defaults to `https://integrate.api.nvidia.com/v1`; override via
`llm.base_url`.

Byte-stability: without `--use-model` (the default) the same CI runs as
before (`sin-code eval list` payloads, structural-keyword assertions,
`compile_and_run` scorers, etc.). With `--use-model`, results become
model-dependent — `eval snapshot`/`eval diff` retain their determinism
because the snapshot is taken on top of whatever the model produced.


## CI integration

The `eval-n8n.yml` GitHub Actions workflow delegates dataset runs to the
n8n-managed OCI VM (M1). The webhook sends a comma-separated list of
datasets and a minimum pass rate; the n8n workflow runs each dataset and
reports the result.

## Exit codes

- `0` — pass rate meets or exceeds `--min-pass-rate`.
- `1` — pass rate below the threshold, or an error occurred.

## See also

- `evals/README.md` — dataset catalog and schema
- `docs/RFC-test-automation.md` — Test-First Verify-Loop design
- `cmd/sin-code/internal/eval/eval.doc.md` — internal judge/metrics docs
- `cmd/sin-code/internal/evalharness/comparator.doc.md` — four-arm comparator
