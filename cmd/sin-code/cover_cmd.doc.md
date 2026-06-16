# `sin-code cover`

Coverage-Drohne — a state-of-the-art coverage automation tool for SIN-Code.

## Subcommands

- `scan` — print package coverage table (text or JSON).
- `check` — fail CI if any package is below `--min` coverage.
- `gaps` — list uncovered functions/blocks per package.
- `generate` — write an AI test-generation request JSON.
- `hook` — print or install a git pre-commit coverage gate.

## SOTA design

Coverage-Drohne combines the best practices from industrial LLM test-generation
(Meta TestGen-LLM, Qodo Cover) with Go-native tooling:

1. **Coverage-driven**: every run produces a machine-readable coverage profile.
2. **Gap-aware**: `gaps` pinpoints the exact functions and blocks that are
   not covered.
3. **AI test-gen requests**: `generate` emits a JSON prompt that can be fed to
   the agent loop or to an external LLM to produce the missing tests.
4. **CI gate**: `check --min 100` is deterministic and can be used as a
   merge-blocking gate.

Future versions will add mutation-testing integration (`go-mutesting`),
coverage-guided fuzzing (`go test -fuzz`), and a file-watcher that auto-triggers
test generation after `sin-code` writes or edits a Go file.

## Usage

```bash
sin-code cover scan
sin-code cover scan --json
sin-code cover check --min 100
sin-code cover gaps --package coverdrohne
sin-code cover generate --package coverdrohne --out req.json
sin-code cover hook
sin-code cover hook --install
```

## Exit codes

- `0` — scan/check passed, no package below threshold.
- `1` — check found a package below threshold (text mode) or a command error.

## Machine-readable output

`cover check --json` always exits `0` and prints a JSON envelope:

```json
{
  "passed": false,
  "min": 100,
  "failed": [
    {"import_path": "...", "coverage": 42.0, "statements": 100, "covered": 42}
  ]
}
```
