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

## Dual-mode contract (issue #264)

Every dataset in this directory MUST stay green in two evaluation modes:

1. **Offline stub mode** (the default): the runner uses
   `cmd/sin-code.evalCmd.stubRunOverride`, an echo-only loop that does not
   call a real LLM. Byte-stable; safe in CI.
2. **Real-model mode** (`--use-model`, issue #261): the loop routes every
   case through a configured chat-completion provider.

The two modes gain their signal from different scorers:

- Stub mode → `expected.output_contains` is the primary verifier.
- Model mode → `scorer.type: "compile_and_run"` plus `language: "go"`
  is the primary verifier.

To keep both modes green simultaneously, every `compile_and_run` Scorer
sets `requires_model: true` — `Runner.applyScorer` then skips the scorer
when `RunnerConfig.UseModel` is false. Only `expected.output_contains`
runs in stub mode. Removing `requires_model: true` from such a case will
fail offline CI; restoring it is the fix.

The convention is enforced by the v1.1.0 JSON files in this directory;

every dataset carries at least one `model-only` tag + `compile_and_run`
case so that real-model CI has something concrete to verify, while the
existing stub-friendly cases keep their historical keyword checks.

## CI

The `eval-n8n.yml` GitHub Actions workflow delegates these datasets to the
n8n-managed OCI VM on every push/PR affecting `evals/` or eval-related code.
