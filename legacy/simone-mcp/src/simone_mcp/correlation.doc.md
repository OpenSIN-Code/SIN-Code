# `correlation.py` — Per-tool-call Correlation Tracking

What this file does: a thread-safe, in-memory store of in-flight and recent tool calls. Each call gets a correlation id (caller-provided or auto-generated) and a status (`in_progress` → `completed` / `failed`).

## Dependency map

- Imports: stdlib (`hashlib`, `json`, `threading`, `collections.OrderedDict`, `datetime`, `typing`).
- Imported by: `a2a_handler.py`, `http_app.py` (for MCP correlation).

## Public API

| Symbol                              | Purpose                                                          |
|-------------------------------------|------------------------------------------------------------------|
| `MAX_ACTIVE_CALLS` / `CLEANUP_EVERY_N` | Module-level config: 1024 max, cleanup every 64 ops           |
| `ToolCallCorrelation(...)`          | Class; construct for custom limits                               |
| `.generate_correlation_id(tool, args, provided_id?)` | Register a call; return the id       |
| `.complete_call(id, result, error?)`| Mark a call complete (or failed)                                 |
| `.get_call_status(id)`              | Look up a record, or `None`                                      |
| `.cleanup_old_calls()`             | Force a stale-record cleanup; returns count removed               |
| `correlation_manager`               | Module-level singleton (use this!)                               |

## Important config / limits

- **Default `max_calls=1024`** — bounds in-memory growth under load. Excess calls LRU-evict.
- **Default `max_age_seconds=3600`** — 1 hour. Stale records cleaned up every `CLEANUP_EVERY_N=64` ops.
- **Auto-generated ids look like `auto_<8hex>_<unix_ts>`** — opaque to callers; never parse.
- **Thread-safe** via a single `threading.Lock`; contention is rare (read-mostly workload).

## Design decisions

- **Why an LRU `OrderedDict`?** Eviction is O(1) at both ends. The cap of 1024 caps memory at a few hundred KB.
- **Why amortize cleanup?** A full scan on every operation is O(n). Doing it every 64th op keeps the per-op cost low and the worst-case scan rate bounded.
- **Why `complete_call` is a no-op for unknown ids?** Records can be evicted between `generate` and `complete` (e.g. a long batch of older calls). Silently dropping the completion is the right call.

## Usage example

```python
from simone_mcp.correlation import correlation_manager

# At the start of a tool call
cid = correlation_manager.generate_correlation_id("find_symbol", {"symbol": "x"})

# At the end
try:
    result = do_work()
    correlation_manager.complete_call(cid, result)
except Exception as e:
    correlation_manager.complete_call(cid, None, str(e))
```

## Caveats / footguns

- **`complete_call` is fire-and-forget.** If a tool crashes, the manager still records the failure — but only if the call site catches the exception.
- The store is in-process and not durable. Restart = lost correlations.
- For a long-running tool, the record stays in `in_progress` state until `complete_call` fires. Don't rely on `max_age_seconds` for "abandoned call" detection; a watchdog is the right tool.
