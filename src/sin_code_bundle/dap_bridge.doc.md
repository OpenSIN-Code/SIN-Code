# DAP Bridge — Runtime Debugger Bridge

## Purpose

DAP (Debug Adapter Protocol) runtime bridge — spawns language debuggers as
subprocesses, attaches a known port, and stores session facts in memory so
agents can attach a real debugger instead of guessing from logs.

## What is DAP?

The Debug Adapter Protocol is the Microsoft-standard JSON-RPC contract that
VS Code, Cursor, OpenCode, and similar editors use to talk to language
debuggers. The bridge here is the *server* side: it spawns a language-specific
debugger, exposes it on a TCP port, and lets any DAP client connect.

### Languages & debuggers (hardcoded mapping)

| Language     | Binary / command            | Default port |
|--------------|-----------------------------|--------------|
| `python`     | `python -m debugpy --listen` | `5678`      |
| `go`         | `dlv debug --headless`       | `2345`      |
| `node` / `javascript` / `typescript` | `node --inspect-brk=PORT` | `9229` |

Any other language returns `{"error": "Unsupported language for DAP: ..."}`.

## Public API

### `DAPSession`

A single attached debugger instance. One DAP session per language/target.

- `__init__(language: str, target: str, repo_root: Path)`
  - `language` — one of `python`, `go`, `node`, `javascript`, `typescript`
  - `target` — entry-point file or module to run under the debugger
  - `repo_root` — working directory for the spawned process
- `start() -> dict` — spawns the debugger, returns `{"success", "port",
  "message"}` or `{"error": "..."}`. Catches `FileNotFoundError` separately
  (e.g. `debugpy` not installed) and returns a friendly diagnostic.
- `stop() -> None` — best-effort `process.terminate()` and clears the handle.
  Exceptions during terminate are swallowed.

### `SINRuntimeTrace`

High-level orchestrator. Holds a dict of `DAPSession` keyed by
`f"{language}_{function_name}"`.

- `trace_function(file_path, function_name, language="python",
  store_in_memory=True) -> dict`
  - Creates a `DAPSession`, calls `start()`, and on success registers the
    session.
  - If `store_in_memory=True`, calls `sin_code_bundle.memory.remember(...)`
    with `kind="runtime"`, `scope="repo"`. Wrapped in try/except — memory
    failures are silently ignored.
  - Returns `{"success", "session_id", "port", "message"}` (message tells
    the client where to attach).
- `get_session_status(session_id) -> dict` — `{"active", "port"}` or
  `{"active": False, "error": "Session not found"}`.
- `stop_trace(session_id) -> dict` — stops the session, removes it from
  the dict, returns `{"success", "message"}`.

## Dependencies

- **Stdlib only** for the core: `subprocess`, `pathlib`, `typing`.
- **Optional** `sin_code_bundle.memory` — only imported inside
  `trace_function` and only when `store_in_memory=True`. Imported lazily
  (inside the function, not at module top) so a broken memory backend
  does not block DAP usage.
- **No Honcho.** **No SCKG.** **No LSP.** Pure subprocess + Python debugpy.

## Graceful degradation

- `debugpy` not installed → `FileNotFoundError` caught, returns
  `{"error": "Debugger for python not found (install debugpy/dlv/node)."}`.
- `dlv` not on PATH → same pattern.
- `node` not on PATH → same pattern.
- Memory backend unavailable → trace still works, only the
  `remember()` call is skipped (wrapped in `except Exception: pass`).
- `stop()` failures are silently swallowed (debugger may have already
  exited on its own).

## Usage example

```python
from pathlib import Path
from sin_code_bundle.dap_bridge import SINRuntimeTrace

tracer = SINRuntimeTrace(repo_root=Path("/repo"))
result = tracer.trace_function("src/app.py", "main", language="python")
# {"success": True, "session_id": "python_main", "port": 5678,
#  "message": "Attach DAP client to localhost:5678 to inspect main"}

# Later
tracer.stop_trace("python_main")
```

Also exposed via MCP: `sin_runtime_trace(file_path, function_name, language)`
and `sin_stop_trace(session_id)` (see `mcp_server.py`).

## Known caveats

- `start()` does not wait for the debugger to actually be ready — the
  port is open once `subprocess.Popen` returns, but `debugpy` may still
  be initializing. A robust DAP client should retry the attach.
- One session per `(language, function_name)` — calling `trace_function`
  twice with the same args overwrites the previous session in
  `self.sessions` without stopping it.
- `stop_trace` does not verify the process actually exited.
