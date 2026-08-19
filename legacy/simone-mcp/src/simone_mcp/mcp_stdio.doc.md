# `mcp_stdio.py` — MCP Stdio Transport

What this file does: line-delimited JSON-RPC over stdin/stdout. The canonical transport for local MCP clients (OpenCode, Claude Desktop). Each line is one request (or a batch array); each response is one line on stdout.

## Dependency map

- Imports: stdlib (`json`, `logging`, `sys`); internal: `protocol.handle_mcp_request`.
- Imported by: `cli.py` (the `serve-mcp` subcommand), `server.py` (the `main()` entry).

## Public API

| Function                | Purpose                                                          |
|-------------------------|------------------------------------------------------------------|
| `serve_stdio()`         | Async coroutine; blocks until stdin closes                       |

## Important config / limits

- **One request per line.** A blank line is skipped (defensive).
- **JSON parse errors** return a JSON-RPC `-32700` (parse error) on stdout and continue.
- **Notifications** (`method: "notifications/..."` with no `id`) are dispatched but produce no response line — only their out-of-band notifications are emitted.
- **Session state is per-process** — `session_id` and `client_protocol_version` are module-local variables in the stdio loop.

## Design decisions

- **Why one line per message?** Newline-delimited JSON is the simplest framing for a process-to-process pipe. Clients that want batching can send a JSON array on a single line.
- **Why no explicit `initialize` enforcement?** Many clients send `initialize` before any other call; the loop simply remembers the session id. Subsequent calls without `initialize` get a fresh session id (which is fine for stateless tools).

## Usage

```python
import asyncio
from simone_mcp.mcp_stdio import serve_stdio
asyncio.run(serve_stdio())
```

In practice, run via the CLI: `simone serve-mcp`.

## Caveats / footguns

- **The server is single-process and single-session.** Multiple clients on the same process share state. Don't expect isolation.
- **Stderr is for logging only.** All MCP responses go to stdout — the protocol layer will be confused if a tool writes a non-JSON line.
- **No reconnection / replay.** When the process dies, the session is gone. The client should restart.
