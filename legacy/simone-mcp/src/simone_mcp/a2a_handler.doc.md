# `a2a_handler.py` — A2A JSON-RPC Request Handler

What this file does: implements the A2A v1 method surface for the `/a2a/v1` endpoint. The agent-to-agent protocol uses JSON-RPC; this module dispatches incoming requests to the right handler and builds the response.

## Dependency map

- Imports: `core.TOOL_DEFINITIONS`, `core.build_agent_card`, `core.execute_simone_action`, `correlation.correlation_manager`, `schemas.JsonRpcRequest`, `schemas.MessageSendParams`, `schemas.ToolCallParams`.
- Imported by: `http_app.py` (the `/a2a/v1` route).

## A2A methods implemented

| Method            | Purpose                                              |
|-------------------|------------------------------------------------------|
| `agent.discover`  | Return the agent card (name, version, endpoints, auth) |
| `agent.ping`      | Liveness probe with timestamp                        |
| `tool.list`       | Return the MCP tool definitions                      |
| `tool.call`       | Dispatch a tool call and return the result + correlation id |
| `message/send`    | Accept a text message, treat as JSON action or action name |
| `tasks/get`       | Return a stub completed task record                  |

## Important config / limits

- **Always returns JSON-RPC 2.0** (never raises). Errors use standard codes: -32600 (invalid request), -32601 (method not found), -32602 (invalid params), -32603 (internal error).
- **Correlation IDs** are auto-generated for `tool.call` from `(tool, args, timestamp)` — 8 hex chars of SHA-256 + unix timestamp.
- **`message/send` interprets the text** as either a JSON action dict or a bare action name (e.g. `"agent.help"`).

## Design decisions

- **Why a stub `tasks/get`?** The heavy lifting is in the MCP transport. For the A2A view, "completed" is enough; A2A clients who want real task state should use the MCP `tasks/get` directly.
- **Why include `correlation_id` in the response?** Lets clients match a tool call to its result across async boundaries (e.g. when the call is queued and the response comes back via SSE).
- **Why text-as-action in `message/send`?** A2A's canonical payload is a `Message` with `parts`. Some clients send raw text. Falling back to "treat text as action name" is the most forgiving interpretation.

## Usage

This module is wired automatically by `http_app.create_app()`. To call directly:

```python
import asyncio
from simone_mcp.a2a_handler import handle_a2a_request

result = asyncio.run(handle_a2a_request(
    {"jsonrpc": "2.0", "id": 1, "method": "agent.ping", "params": {}},
    base_url="http://localhost:8234",
))
print(result)
```

## Caveats / footguns

- The module never raises — all exceptions are caught and returned as JSON-RPC errors. Wrap a test in `try/except` only if you want to verify the "never raises" contract.
- `correlation_id` is shared across A2A and MCP transports; cross-check it in dashboards, not in tool logic.
