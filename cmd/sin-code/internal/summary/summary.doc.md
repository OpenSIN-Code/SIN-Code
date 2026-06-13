# Auto-Summary Builder (`internal/summary`)

Docs: `summary.go`

## What
A deterministic, rule-based builder that turns a session ledger into a
human-readable summary. No LLM call is required, so summaries are fast,
reproducible, and keep M2 (no external dependencies) intact.

## Why
- Agents need to report what they did in a compact form.
- Humans need to review sessions without reading every turn.
- Oracle-style verification needs an evidence string that can be quoted or
  cross-checked.

## How it works
`Build(ctx, store, sessionID)` loads the ledger entries for that session and
walks them chronologically:
- Counts `tool_call` entries as turns.
- Collects unique tool names.
- Records the last `verify_pass`/`verify_fail` event as verification status.
- Uses the `task_complete` summary as the one-liner, or falls back to the first
  user prompt.

## Output formats
- `Format(s)` returns markdown with sections for tools, prompts, and status.
- `Evidence(s)` returns a compact one-line evidence string suitable for
  Oracle cross-checks or commit trailers.

## Usage
```go
sum, err := summary.Build(ctx, store, "sess-123")
if err != nil { ... }
fmt.Println(summary.Format(sum))
fmt.Println("Evidence:", summary.Evidence(sum))
```

## Maintenance
- When adding new `ledger.EntryType` values, update the switch in
  `buildFromEntries` if they affect the summary.
- Keep the summary deterministic; no randomness or external calls.
- If an LLM-based summary is ever added, it must be an optional mode behind a
  flag and must not break the default deterministic path.

## Caveats
- The builder is only as good as the ledger entries. Missing `task_complete`
  or `verify_*` events will produce weaker summaries.
- Very long prompts are truncated to 80 characters in the one-liner fallback.
