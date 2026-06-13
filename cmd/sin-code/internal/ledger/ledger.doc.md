# Semantic Session Ledger (`internal/ledger`)

Docs: `store.go`

## What
An append-only SQLite store of agent-loop events. Every prompt, tool call,
verification result, and completion is recorded with a JSON payload and a
human-readable summary line. The ledger is the evidence layer for
auto-summaries and cross-session learning.

## Why
- **Auditability:** trace what the agent did, when, and why.
- **Summaries:** the `summary` package reads the ledger to produce session
  recaps.
- **Verification evidence:** Oracle/PoC results are stored next to the turns
  they gate, so a summary can claim “verified by poc” with proof.

## Schema (v1)
Table `ledger`:
- `id TEXT PRIMARY KEY`
- `session_id TEXT NOT NULL` (indexed)
- `type TEXT NOT NULL` (indexed)
- `data TEXT NOT NULL` (JSON payload)
- `summary TEXT NOT NULL` (human line)
- `created_at TEXT NOT NULL` (RFC3339Nano)

## Entry types
- `user_prompt` — user message that started a turn
- `tool_call` — local tool executed by the agent
- `tool_error` — tool returned an error
- `verify_pass` — verification gate passed
- `verify_fail` — verification gate failed
- `verification_mode` — gate mode selected (poc/oracle)
- `task_complete` — agent loop finished with verified result
- `task_abort` — max turns exceeded or context cancelled

## Usage
```go
store, err := ledger.Open(ledger.DefaultPath())
if err != nil { ... }
defer store.Close()

id, err := store.Record(ctx, ledger.Entry{
    SessionID: "sess-123",
    Type:      ledger.TypeToolCall,
    Data:      map[string]any{"tool": "sin_edit", "args": map[string]any{"path": "foo.go"}},
    Summary:   "edited foo.go",
})

entries, err := store.List(ctx, "sess-123", 1000)
```

## Maintenance
- Keep `id` generation deterministic-free (uses crypto/rand + UnixNano).
- Do not delete rows; the ledger is append-only by contract. If compaction is
  ever needed, archive into a separate table first.
- When adding a new `EntryType`, update `agentloop/loop.go` call sites and the
  `summary` builder.

## Caveats
- Uses `modernc.org/sqlite` (CGo-free, mandate M2).
- `DefaultPath()` writes to `~/.local/share/sin-code/ledger.db` unless
  `SIN_CODE_HOME` is set. Tests must use `t.TempDir()`.
- Concurrent writers are serialized by `SetMaxOpenConns(1)`.
