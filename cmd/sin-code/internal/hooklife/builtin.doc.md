# hooklife/builtin.go — default hook set

Seven built-in hooks, all written against the small interfaces in this
file. Wire the interfaces through `internal/adapters/*` to attach
SIN-Code's real `lsp`, `verify`, `ledger`, `headroom` subsystems.

## List

| ID | Phase | Purpose |
|---|---|---|
| `block-no-verify` | PreToolUse | refuse `git commit --no-verify` |
| `config-protection` | PreToolUse | block edits to `.git/`, `go.sum`, `.env` |
| `post-edit-format` | PostToolUse | run `gofmt`/`prettier`/`ruff format` |
| `post-edit-typecheck` | PostToolUse | surface LSP diagnostics as a warning |
| `quality-gate` | PreToolUse | run verifier before `git commit` |
| `cost-tracker` | PostToolUse | record per-tool spend to ledger |
| `suggest-compact` | Stop, PreCompact | warn when context is large |

## Why so many, all in one file

Each hook is short (10-30 lines). Splitting them into separate files
adds noise without adding clarity. The single file makes the
"default policy" auditable in one read.

## Related files

- `event.go` — the `Decision` contract
- `runner.go` — applies the verdicts
- `internal/adapters/` — concrete implementations
