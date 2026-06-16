# dataset/dataset.go

## What

JSON parser + schema validator for Golden Datasets (issue #75).

A Golden Dataset is one JSON file under `evals/*.json`:

```json
{
  "name": "Critical Path Tests",
  "version": "1.0.0",
  "test_cases": [
    {
      "id": "auth_refactor",
      "prompt": "Refactor the auth system…",
      "constraints": {"must_use_tools": ["sin_edit"], "max_turns": 10},
      "expected": {"contains_keywords": ["argon2"], "min_quality": 0.8},
      "scorer": {"type": "compile_and_run", "language": "go", "self_check": "assert foo() == 1"}
    }
  ]
}
```

## API surface

| Symbol | Purpose |
|--------|---------|
| `LoadDataset(path)` | read + parse + validate one JSON file |
| `(*Dataset).Validate()` | schema-only check; cheap to call repeatedly |
| `(*Dataset).FilterByTag(tag)` | subset by case-insensitive tag |
| `ListDatasets(dir)` | walk for `*.json` files (relative to `dir`) |

## Constraints

- Stdlib only (`encoding/json`, `fs`, `os`, `path/filepath`, `strings`,
  `time`). No YAML, no third-party schema lib — mandate M2 + "NOT a
  place to vendor" (AGENTS.md §2).
- TestCase IDs must be unique within a Dataset.
- `require_verify=true` requires a non-empty `verify_cmd`.
- `min_quality` and `max_tokens` are bounded; out-of-range values
  fail validation at load time.
- `scorer` selects a post-hoc output scorer. `type: compile_and_run`
  requires a supported `language` (`go`, `python`, `javascript`, `bash`)
  and optionally `self_check`, `skip_test`, `timeout`, and `binary`.

## Without the issue's reference library

The issue body mentioned unexported verification details (custom
runner factory in `runner.go`); this file deliberately does NOT
expose those — the runner adapter lives in `runner.go` where the
real AgentLoop signature lives.
