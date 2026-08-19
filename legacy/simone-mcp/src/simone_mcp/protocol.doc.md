# `protocol.py` — MCP Protocol Layer

What this file does: the JSON-RPC dispatcher for tools, resources, prompts, and tasks. The `handle_mcp_request` function is the single entry point used by both transports (`mcp_stdio` and `http_app`).

## Dependency map

- Imports: stdlib (`asyncio`, `base64`, `logging`, `threading`, `time`, `uuid`, `datetime`, `pathlib`, `typing`).
- Internal: `core.TOOL_DEFINITIONS`, `core.execute_simone_action`, `core.json_dumps`, `schemas.TOOL_ARG_MODELS`.
- Imported by: `http_app.py`, `mcp_stdio.py`.

## Public API

| Symbol                            | Purpose                                                          |
|-----------------------------------|------------------------------------------------------------------|
| `PROTOCOL_VERSION`                | Current MCP version (`"2026-06-30"`)                            |
| `SUPPORTED_VERSIONS`              | List of accepted client versions for negotiation                 |
| `SSE_RETRY_MS`                    | 5s — sent in SSE `retry:` fields                                 |
| `PROMPT_DEFINITIONS`              | 4 prompts: code_review, debug_assistant, refactor_plan, test_generator |
| `RESOURCE_TEMPLATES`              | `file:///{path}` and `source://{root}/{relpath}`                  |
| `handle_mcp_request(payload, session_id, send_notification?, client_protocol_version?)` | Single MCP request handler |
| `_log_event(session_id, event_id, data)` | Add to SSE event log (for replay)                         |
| `_get_events_after(session_id, last_event_id)` | Replay events after a cursor                       |

## MCP methods handled

| Method                       | Behavior                                                  |
|------------------------------|-----------------------------------------------------------|
| `initialize`                 | Negotiate version, register session, return capabilities  |
| `initialized` / `notifications/initialized` | No-op                                     |
| `ping`                       | Empty result                                              |
| `tools/list`                 | Paginated tool definitions                                |
| `tools/call`                 | Validate args, dispatch (sync or async via tasks)         |
| `tasks/get`                  | Return task object (status, result, error)                 |
| `tasks/update`               | Resume a task awaiting input                              |
| `tasks/cancel`               | Cancel a working task                                     |
| `resources/list`             | Paginated file list                                       |
| `resources/templates/list`   | Paginated URI templates                                   |
| `resources/read`             | Read a file by URI                                        |
| `resources/subscribe` / `unsubscribe` | Add/remove URI from the subscription set       |
| `prompts/list`               | Paginated prompt definitions                              |
| `prompts/get`                | Render a prompt with arguments                            |
| `logging/setLevel`           | Set the Python logger level                               |
| `completion/complete`        | Argument-value completions                                |
| `sampling/createMessage`     | **Unsupported in stdio**                                  |
| `elicitation/create`         | **Unsupported** — error with hint to rephrase             |

## Important config / limits

- **Default page size: 50** (`PAGE_SIZE`).
- **List result TTL: 5 min** (`_LIST_TTL_MS`); cache scope: `session`.
- **Task retention: 1 hour** (`_TASK_MAX_AGE_MS`), cleanup every 64 ops.
- **Max concurrent tasks per session: 100.**
- **Task poll interval (suggested to clients): 5s.**

## Design decisions

- **Why a `tasks/` namespace?** MCP 2.0 added long-running operations as first-class entities. Tools that opt in (via `execution.taskSupport != "forbidden"` in their tool definition) get queued and tracked.
- **Why version negotiation on `initialize`?** Clients from different MCP versions can connect. We pick the highest version both sides support.
- **Why tool arg aliases?** The tool surface evolved; the alias map lets old clients send `edit_payload` while new ones send `editPayload` (or vice versa).

## Usage

`handle_mcp_request` is called by the transports, not directly. For tests:

```python
import asyncio
from simone_mcp.protocol import handle_mcp_request

response, sid, notifs = asyncio.run(handle_mcp_request(
    {"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": {}},
    session_id=None,
))
```

## Caveats / footguns

- **`handle_mcp_request` never raises** — all errors are returned as JSON-RPC error responses. Catch in tests only to assert the "never raises" contract.
- **The task store is in-process** — restart = lost tasks.
- **`sampling/createMessage` returns a custom error** in stdio. Use HTTP transport with client-side sampling if you need it.
- **The `_meta` field is round-tripped** for tool calls but ignored for most other methods.
