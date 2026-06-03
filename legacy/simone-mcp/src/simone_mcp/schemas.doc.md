# `schemas.py` — Pydantic Schemas for MCP Payloads

What this file does: Pydantic models for JSON-RPC payloads and per-tool arguments. The `TOOL_ARG_MODELS` dict maps tool names to their argument models, which `protocol.py` uses to validate `tools/call` arguments before dispatch.

## Dependency map

- Imports: `pydantic` (`BaseModel`, `Field`, `model_validator`).
- Imported by: `protocol.py`.

## Public API

| Symbol                            | Purpose                                                          |
|-----------------------------------|------------------------------------------------------------------|
| `JsonRpcRequest`                  | Validates a JSON-RPC 2.0 request (method, params, id)            |
| `JsonRpcResponse`                 | Mirrors the request shape with `result` / `error`                 |
| `ToolCallParams`                  | A2A-shaped tool call (`name`, `arguments`, `correlation_id`)     |
| `MessagePart` / `A2AMessage` / `MessageSendParams` | A2A message shape                          |
| `FindSymbolArgs` / `FindReferencesArgs` / ... | One per tool                          |
| `TOOL_ARG_MODELS`                 | Dict mapping tool name → arg model class                         |

## Per-tool argument models

Each MCP tool has a dedicated `*Args` Pydantic model:

- `FindSymbolArgs`, `FindReferencesArgs` — symbol search
- `ReplaceSymbolBodyArgs`, `InsertAfterSymbolArgs` — structural edit
- `ProjectOverviewArgs` — workspace summary (also used for `health`)
- `SymbolSearchArgs`, `StructuralEditArgs`, `MemoryQueryArgs` — by-name aliases
- `GraphifyQueryArgs`, `GraphifyUpdateArgs`, `GraphifyExplainArgs`, `GraphifyPathArgs`
- `WriteFileArgs`, `EditFileArgs`, `PatchFileArgs`, `ReadFileArgs`
- `TaskGetArgs`, `TaskUpdateArgs`, `TaskCancelArgs`

## Important config / limits

- **All `*Args` models use `Field(min_length=1)` for required string fields** — empty strings are rejected.
- **`ToolCallParams` uses `Field(alias="name")`** to accept the MCP `name` field; `populate_by_name` allows both `name` and `tool_name`.
- **Field names are case-sensitive** — `editPayload` is NOT the same as `editpayload`.

## Design decisions

- **Why Pydantic and not dataclasses?** Pydantic gives free JSON validation, alias support, and clear error messages. The cost is a dep, which we already have.
- **Why a model per tool?** Strict validation per tool surface catches typos in the JSON-RPC body before they hit the dispatcher.
- **Why use `model_config = {"populate_by_name": True}`?** Lets the same model accept both `name` and `tool_name`, useful for clients that send either form.

## Usage

Models are used internally by `protocol.py`. For tests:

```python
from simone_mcp.schemas import FindSymbolArgs

args = FindSymbolArgs(symbol="my_func", root="/tmp/proj")
print(args.symbol, args.root)
```

## Caveats / footguns

- **Adding a new tool?** Add both the `*Args` class AND the entry in `TOOL_ARG_MODELS`. Without the entry, the protocol layer skips validation.
- **`min_length=1` rejects empty strings** — if you want a default for a field, use `Field(default=...)` and explicitly allow empty in the validator.
