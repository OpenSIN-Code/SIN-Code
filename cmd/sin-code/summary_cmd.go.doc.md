# `sin-code summary` CLI binding

Docs: `summary_cmd.go`

## What
Cobra command binding that builds a deterministic markdown summary of a
session from the ledger. Optionally prints a one-line evidence string.

## Why
Allows users and downstream tools to quickly understand what happened in a
session and whether it was verified.

## Usage
```bash
sin-code summary <session-id>           # markdown summary
sin-code summary <session-id> --evidence # one-line evidence
```

## Environment
- `SIN_CODE_LEDGER` overrides the default ledger SQLite path.

## Maintenance
- If LLM-based summaries are added later, keep the deterministic path as the
  default and add a `--llm` flag for the optional mode.
- The evidence format is a public API for scripts; changing it requires a
  major version bump or deprecation cycle.
