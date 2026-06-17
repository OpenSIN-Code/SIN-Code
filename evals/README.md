# SIN-Code Golden Datasets

This directory contains versioned Golden Datasets for `sin-code eval` (see
`docs/eval.md` and `docs/RFC-test-automation.md`).

## Available datasets

| Dataset | Purpose | Typical scorer |
|---------|---------|----------------|
| `critical.json` | Smoke / structural tests for the eval runner itself. | contains (default) |
| `test-generation.json` | `sin_test_generate` + `sin_test` workflow. | contains |
| `mutation.json` | `sin_mutation` invocation and report interpretation. | contains |
| `fuzzing.json` | Generating fuzz-style Go programs. | compile_and_run |
| `property.json` | Generating property-based checks. | compile_and_run |
| `quality-gate.json` | `sin_quality_gate` invocation and report interpretation. | contains |
| `three-arm-example.json` | Four-arm comparator example (baseline/terse/lazy_skill/skill). | comparator |

## Adding a new dataset

1. Create a new JSON file in this directory.
2. Follow the schema in `cmd/sin-code/internal/dataset/dataset.go`.
3. Run `sin-code eval list --dir evals` to verify it is discovered.
4. Run `sin-code eval run --dataset evals/<name>.json --min-pass-rate 0.8 --json` locally.
5. Update this README and `docs/eval.md`.

## Schema quick reference

```json
{
  "name": "My Dataset",
  "version": "1.0.0",
  "description": "...",
  "test_cases": [
    {
      "id": "unique-id",
      "prompt": "What the agent should do",
      "tags": ["tag"],
      "constraints": {
        "max_turns": 5,
        "timeout": "120s",
        "must_use_tools": ["sin_test_generate"]
      },
      "expected": {
        "output_contains": ["PASS"],
        "output_avoids": ["error"]
      },
      "scorer": {
        "type": "compile_and_run",
        "language": "go",
        "self_check": "func main() { ... }"
      }
    }
  ]
}
```

## CI

The `eval-n8n.yml` GitHub Actions workflow delegates these datasets to the
n8n-managed OCI VM on every push/PR affecting `evals/` or eval-related code.
