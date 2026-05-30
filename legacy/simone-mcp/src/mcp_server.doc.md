# `src/mcp_server.py` — MCP Server Re-exports

Partner file: `src/mcp_server.py`

## Purpose
Re-exports core Simone MCP functions for external consumption. This is the module imported by MCP client configurations (OpenCode, Claude, etc.) to access the tool surface.

## Key Symbols
| Symbol | Source | Purpose |
|--------|--------|---------|
| `_build_realtime_url` | `core.py` | Supabase realtime URL builder |
| `build_agent_card` | `core.py` | A2A agent discovery card |
| `dashboard` | `core.py` | HTML dashboard generator |
| `execute_simone_action` | `core.py` | Main action dispatcher |
| `find_references` | `core.py` | Symbol reference search |
| `find_symbol` | `core.py` | Symbol definition search |
| `get_project_overview` | `core.py` | Workspace footprint summary |
| `insert_after_symbol` | `core.py` | Insert code after symbol |
| `process_lsp_task` | `core.py` | LSP task processor |
| `replace_symbol_body` | `core.py` | Replace symbol body safely |

## Relationship
- `src/simone_mcp/core.py` — all functions sourced from here
- `clients/opencode-mcp.json` — references this module for MCP tool registration

## Dependencies
- `simone_mcp.core`

## Usage
```python
from mcp_server import find_symbol, find_references
```
