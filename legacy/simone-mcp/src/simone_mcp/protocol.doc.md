# `src/simone_mcp/protocol.py` — MCP Protocol Implementation

Partner file: `src/simone_mcp/protocol.py`

## Purpose
Full MCP 2.0 protocol implementation with SEP-2663 Tasks Extension v2. Handles initialize, tools, resources, prompts, tasks, logging, completions, and SSE event streaming. Supports task deferral, progress notifications, and protocol version negotiation.

## Key Symbols
| Symbol | Kind | Purpose |
|--------|------|---------|
| `handle_mcp_request()` | async function | Main MCP request dispatcher |
| `_build_initialize_result()` | function | Build initialize response with capabilities |
| `_create_task()` | function | Create async task for deferred execution |
| `_run_task()` | async function | Execute task with notification callbacks |
| `_update_task()` | function | Update task status/result/error |
| `_get_task()` | function | Get task by ID |
| `_cancel_task()` | function | Cancel a running task |
| `_build_task_obj()` | function | Build task object for MCP responses |
| `_negotiate_version()` | function | Protocol version negotiation |
| `_list_resources()` | function | List workspace files as resources |
| `_read_resource()` | function | Read file content by URI |
| `_generate_prompt()` | function | Generate prompt messages |
| `_complete_arguments()` | async function | Argument completion for prompts/resources |
| `_paginate()` | function | Base64 cursor pagination |
| `_log_event()` | function | SSE event logging |
| `_get_events_after()` | function | Replay SSE events after last ID |
| `_remove_session()` | function | Clean up session and associated tasks |

## Supported Methods
| Method | Description |
|--------|-------------|
| `initialize` | Protocol handshake, session creation |
| `ping` | Health check |
| `tools/list` | Paginated tool listing |
| `tools/call` | Tool execution (sync or deferred) |
| `tasks/get` | Get task status with inline result |
| `tasks/update` | Resume task with input |
| `tasks/cancel` | Cancel running task |
| `resources/list` | List workspace resources |
| `resources/templates/list` | List resource templates |
| `resources/read` | Read file content |
| `resources/subscribe` | Subscribe to resource |
| `resources/unsubscribe` | Unsubscribe from resource |
| `prompts/list` | List prompts |
| `prompts/get` | Get prompt with args |
| `logging/setLevel` | Set log level |
| `completion/complete` | Argument completion |
| `sampling/createMessage` | Not supported (returns error) |
| `elicitation/create` | Not supported (returns error) |

## Task System (SEP-2663)
- Tasks are created for tools with `taskSupport != "forbidden"`
- Tasks store inline results (no separate `tasks/result` method)
- Tasks can be updated with input when `status == "input_required"`
- Notifications sent via `notifications/tasks` (not `notifications/tasks/status`)

## Constants
| Constant | Value | Purpose |
|----------|-------|---------|
| `PROTOCOL_VERSION` | "2026-06-30" | Current MCP version |
| `SUPPORTED_VERSIONS` | 3 versions | Backward compatibility |
| `SSE_RETRY_MS` | 5000 | SSE retry interval |
| `_TASK_MAX_AGE_MS` | 3600000 | Task TTL (1 hour) |
| `_TASK_MAX_CONCURRENT` | 100 | Max concurrent tasks per session |
| `PAGE_SIZE` | 50 | Pagination page size |

## Relationship
- `src/simone_mcp/core.py` — `execute_simone_action()`, `TOOL_DEFINITIONS`, `json_dumps()`
- `src/simone_mcp/schemas.py` — `JsonRpcRequest`, `TOOL_ARG_MODELS`
- `src/simone_mcp/http_app.py` — calls `handle_mcp_request()` for POST /mcp
- `src/simone_mcp/mcp_stdio.py` — calls `handle_mcp_request()` for stdio
- `src/simone_mcp/correlation.py` — `correlation_manager` for tool tracking
- `tests/test_simone_mcp.py` — tests SEP-2663, SEP-2243, SEP-2549, protocol methods

## Dependencies
- `core`: `TOOL_DEFINITIONS`, `execute_simone_action`, `json_dumps`
- `schemas`: `JsonRpcRequest`, `TOOL_ARG_MODELS`
- Standard lib: `asyncio`, `base64`, `threading`, `time`, `uuid`, `datetime`

## Broken Links Check
- No internal links to other `.doc.md` files in this module.
