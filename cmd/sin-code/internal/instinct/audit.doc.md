# instinct/audit.go — provenance

JSONL event log under `<base>/audit.jsonl`. Append-only. The `sin
instinct history` command reads it back.

## Events recorded

| Kind | When |
|---|---|
| `created` | new instinct persisted (via `Observer.Flush`) |
| `reinforced` | existing instinct's confidence increased |
| `contradicted` | existing instinct's confidence decreased |
| `evolved` | member of an evolution proposal that was applied |
| `promoted` | project instinct moved to global scope |
| `pruned` | deleted by TTL or by `sin instinct forget` |

## Why best-effort

Audit failures must not block the learning loop. A locked or full disk
should still let the agent continue; the audit gap will show up in
`sin instinct history` and the operator can investigate. We do **not**
want a buggy append to wedge the hook dispatcher.

## Storage cost

A typical session writes 5-20 events. The JSONL file grows at ~1KB
per event and is never rotated by the package. If that becomes a
problem, point your logrotate at it.
