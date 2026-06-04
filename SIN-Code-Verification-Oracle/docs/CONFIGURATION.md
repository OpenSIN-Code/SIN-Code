# Configuration — Verification Oracle

The oracle is configured through CLI flags, the eval suite JSON file, and the
external tools you choose to install. There is no global config file.

## CLI flags

| Command | Flag | Default | Description |
|---------|------|---------|-------------|
| `oracle verify` | `--test` | — | Test command treated as ground truth. |
| `oracle verify` | `--build` | — | Optional build command. |
| `oracle verify` | `--no-diagnostics` | off | Skip the diagnostics oracle. |
| `oracle diagnostics` | `--path` | `.` | Path to analyze. |
| `oracle trace-capture` | `--out` | — | File to write the captured trace to. |
| `oracle trace-diff` | `--before` | — | Previously captured trace to compare against. |
| `oracle eval` | `--label` | `baseline` | Label for the run. |

## External oracles (optional)

The diagnostics oracle shells out to tools if present, and degrades gracefully
otherwise. Install the ones relevant to your stack:

```bash
pip install pyright ruff       # Python type-checking + linting
npm install -g typescript      # tsc for TypeScript
```

| Tool | Language | Role |
|------|----------|------|
| pyright | Python | type errors / diagnostics |
| ruff | Python | lint diagnostics |
| tsc | TypeScript | type errors |

## Eval suite format

A suite is a JSON array of tasks. Each task points to a workspace and carries a
**hidden** verification command the agent never sees:

```json
[
  {
    "id": "fix-add-bug",
    "workspace": "examples/workspaces/fix-add-bug",
    "verify_cmd": "python -m pytest -q",
    "description": "Fix the addition bug so tests pass."
  }
]
```

See [examples/suite.example.json](../examples/suite.example.json).

## Verdict semantics

| Status | Meaning |
|--------|---------|
| `PASS` | Ground-truth signals confirm success. |
| `FAIL` | At least one ground-truth signal failed. |
| `UNVERIFIED` | No ground-truth signal was available — never treated as success. |

`confidence` reflects how many independent signals agreed.
