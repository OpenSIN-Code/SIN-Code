# SIN-Code Golden Datasets

Versioned JSON datasets for `sin-code eval` (see `docs/eval.md`).

## Available datasets

| Dataset | Cases | Scorer | Purpose |
|---------|-------|--------|---------|
| `critical.json` | 3 | contains | Smoke / structural tests for the eval runner |
| `test-generation.json` | — | contains | `sin_test_generate` + `sin_test` workflow |
| `mutation.json` | — | contains | `sin_mutation` invocation and report |
| `fuzzing.json` | — | compile_and_run | Generating fuzz-style Go programs |
| `property.json` | — | compile_and_run | Generating property-based checks |
| `quality-gate.json` | — | contains | `sin_quality_gate` invocation and report |
| `three-arm-example.json` | 3 | comparator | Four-arm bench (baseline/terse/lazy/skill) |
| `skill-code.json` | 3 | comparator | Four-arm eval for code-skills (build, refactor, plan). Run with `--arm baseline,terse,lazy_skill,skill-code-build` |
| `skill-debug.json` | 2 | comparator | Four-arm eval for debug-skills (deep RCA). Run with `--arm baseline,terse,lazy_skill,skill-debug-deep` |
| `skill-github.json` | 2 | comparator | Four-arm eval for github-skills (actions, readme). Run with `--arm baseline,terse,lazy_skill,skill-github-actions` |
| `skill-browser.json` | 2 | comparator | Four-arm eval for browser-skills (automation, page inspection). Run with `--arm baseline,terse,lazy_skill,skill-browser-tools` |
| `skill-design.json` | 2 | comparator | Four-arm eval for design-skills (frontend component, image generation). Run with `--arm baseline,terse,lazy_skill,skill-design-frontend` |
| `skill-ecosystem.json` | 2 | comparator | Four-arm eval for ecosystem-skills (context bridge, marketplace). Run with `--arm baseline,terse,lazy_skill,skill-ecosystem-context` |
| `skill-infrastructure.json` | 2 | comparator | Four-arm eval for infrastructure-skills (cloudflare tunnel, supabase setup). Run with `--arm baseline,terse,lazy_skill,skill-infrastructure-cloudflare` |
| `skill-memory.json` | 2 | comparator | Four-arm eval for memory-skills (honcho preferences, infisical secrets). Run with `--arm baseline,terse,lazy_skill,skill-memory-honcho` |
| `skill-planning.json` | 2 | comparator | Four-arm eval for planning-skills (enterprise architecture, task decomposition). Run with `--arm baseline,terse,lazy_skill,skill-planning-enterprise` |
| `skill-process.json` | 2 | comparator | Four-arm eval for process-skills (goal tracking, adversarial grill). Run with `--arm baseline,terse,lazy_skill,skill-process-goal` |
| `skill-shop.json` | 2 | comparator | Four-arm eval for shop-skills (stripe integration, CJ dropshipping). Run with `--arm baseline,terse,lazy_skill,skill-shop-stripe` |

Run `sin-code eval list --dir evals` to discover datasets programmatically.

## Dataset format

```json
{
  "name": "My Dataset",
  "version": "1.0.0",
  "description": "optional description",
  "test_cases": [
    {
      "id": "unique-id",
      "prompt": "What the agent should do",
      "tags": ["smoke"],
      "constraints": {
        "max_turns": 5,
        "timeout": "120s",
        "must_use_tools": ["sin_test_generate"],
        "forbidden_tools": ["sin_bash"],
        "require_verify": false
      },
      "expected": {
        "output_contains": ["PASS"],
        "output_avoids": ["error"],
        "contains_keywords": ["stub echo"],
        "avoids_keywords": ["panic"],
        "min_quality": 0.0,
        "custom_criteria": "optional LLM-judge rubric"
      },
      "scorer": {
        "type": "compile_and_run",
        "language": "go",
        "self_check": "func main() { ... }",
        "requires_model": true
      },
      "metadata": { "priority": "high" }
    }
  ]
}
```

### Field reference

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | Dataset name |
| `version` | yes | Dataset version (free-form string) |
| `test_cases[].id` | yes | Unique test case identifier |
| `test_cases[].prompt` | yes | Prompt sent to the agent |
| `constraints.max_turns` | no | Max agent loop turns (0 = unlimited) |
| `constraints.timeout` | no | Per-case timeout (e.g. `"30s"`, `"5m"`) |
| `constraints.must_use_tools` | no | Tools the agent must invoke |
| `constraints.forbidden_tools` | no | Tools the agent must not invoke |
| `expected.output_contains` | no | Required substrings in output |
| `expected.output_avoids` | no | Forbidden substrings in output |
| `expected.min_quality` | no | LLM-judge threshold (0.0–1.0) |
| `scorer.type` | no | `exact` / `contains` / `compile_and_run` |
| `scorer.requires_model` | no | Skip scorer in stub mode (issue #264) |

### Scorer types

| Scorer | Pass condition |
|--------|----------------|
| `contains` (default) | All `output_contains` substrings present |
| `exact` | Output equals expected verbatim |
| `compile_and_run` | Extract code block, compile, run self-check |

## Adding a new dataset

1. Create a new JSON file in this directory.
2. Follow the schema above.
3. Run `sin-code eval list --dir evals` to verify discovery.
4. Run `sin-code eval run --dataset evals/<name>.json --min-pass-rate 0.8 --json`.
5. Update this README.

## Dual-mode contract (issue #264)

Every dataset MUST stay green in two modes:

1. **Offline stub mode** (default): echo-only loop, no LLM. Byte-stable.
2. **Real-model mode** (`--use-model`, issue #261): routes through chat
   completion.

To keep both green, `compile_and_run` scorers set `requires_model: true`
so the scorer is skipped in stub mode. Only `expected.output_contains`
runs in stub mode.

## CI

The `eval-n8n.yml` GitHub Actions workflow delegates these datasets to the
n8n-managed OCI VM on every push/PR affecting `evals/` or eval code (M1).
