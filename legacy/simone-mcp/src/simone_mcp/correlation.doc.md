# `src/simone_mcp/correlation.py` — Tool Call Correlation Tracker

Partner file: `src/simone_mcp/correlation.py`

## Purpose
Tracks tool call lifecycle with correlation IDs. Generates deterministic IDs from SHA-256 hash of tool name + arguments. Stores call history with LRU eviction (max 1024 calls) and automatic stale cleanup (max age 1 hour).

## Key Symbols
| Symbol | Kind | Purpose |
|--------|------|---------|
| `ToolCallCorrelation` | class | Correlation tracking manager |
| `generate_correlation_id()` | method | Generate or use provided correlation ID |
| `complete_call()` | method | Mark call as completed/failed |
| `get_call_status()` | method | Get call status by ID |
| `cleanup_old_calls()` | method | Remove stale calls manually |
| `correlation_manager` | global | Singleton instance used by HTTP + A2A |

## Constants
| Constant | Value | Purpose |
|----------|-------|---------|
| `MAX_ACTIVE_CALLS` | 1024 | LRU eviction limit |
| `CLEANUP_EVERY_N` | 64 | Auto-cleanup interval |

## Relationship
- `src/simone_mcp/a2a_handler.py` — uses `correlation_manager` for A2A tool calls
- `src/simone_mcp/http_app.py` — uses `correlation_manager` for HTTP MCP tool calls
- `src/simone_mcp/protocol.py` — uses `correlation_manager` for task tracking
- `tests/test_simone_mcp.py` — tests `ToolCallCorrelation` extensively (thread safety, eviction, cleanup)

## Dependencies
- Standard lib: `hashlib`, `json`, `threading`, `collections.OrderedDict`, `datetime`

## Thread Safety
All methods are protected by `threading.Lock()`. `cleanup_old_calls()` and `complete_call()` are atomic. Tested with 4 concurrent threads × 50 calls each.

## Broken Links Check
- No internal links to other `.doc.md` files in this module.
