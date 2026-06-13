# `sin-code ledger` CLI binding

Docs: `ledger_cmd.go`

## What
Cobra command binding for the semantic session ledger. Provides two
subcommands: `list` (recent sessions) and `show` (entries for one session).

## Why
Gives users a direct way to audit agent activity without writing SQL or
calling the internal API.

## Usage
```bash
sin-code ledger list          # show recent session IDs
sin-code ledger show <id>     # show all entries for that session
```

## Environment
- `SIN_CODE_LEDGER` overrides the default ledger SQLite path.

## Maintenance
- Keep subcommands read-only. The ledger is append-only; mutation is the
  responsibility of the agent loop.
- Match the output format of `sin-code summary` where possible for UX
  consistency.
