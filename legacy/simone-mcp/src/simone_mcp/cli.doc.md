# `src/simone_mcp/cli.py` — CLI Implementation

Partner file: `src/simone_mcp/cli.py`

## Purpose
Provides the `simone` command-line interface with subcommands for server startup, tool execution, project indexing, and configuration validation.

## Key Symbols
| Symbol | Kind | Purpose |
|--------|------|---------|
| `main()` | function | CLI entry point and command dispatcher |
| `_print()` | function | JSON output to stdout |
| `_read_action_argument()` | function | Read action JSON from argv or stdin |
| `_validate_config()` | function | Validate external service connectivity (Qdrant, Neo4j, OAuth) |

## Supported Commands
| Command | Description |
|---------|-------------|
| `serve` | Start HTTP/A2A server (port 8234) |
| `serve-a2a` | Alias for `serve` |
| `serve-mcp` | Start MCP stdio server |
| `print-card` | Print agent discovery card as JSON |
| `run-action JSON` | Execute a single tool action |
| `index [PATH]` | Show project overview |
| `validate` | Validate configuration |
| `tool-list` | List available MCP tools |

## Relationship
- `src/simone_mcp/core.py` — `TOOL_DEFINITIONS`, `execute_simone_action`, `get_project_overview`, `build_agent_card`
- `src/simone_mcp/mcp_stdio.py` — `serve_stdio()` for `serve-mcp`
- `src/simone_mcp/http_app.py` — `create_app()` for `serve`
- `src/cli.py` — thin wrapper that calls this module

## Dependencies
- `core`: `TOOL_DEFINITIONS`, `build_agent_card`, `execute_simone_action`, `get_project_overview`
- `mcp_stdio`: `serve_stdio`
- `http_app`: `create_app` (lazy import)
- Standard lib: `asyncio`, `json`, `os`, `sys`
- External: `uvicorn` (lazy import)

## Environment Variables
| Variable | Purpose |
|----------|---------|
| `SIMONE_HOST` | Server bind host (default: 0.0.0.0) |
| `SIMONE_BASE_URL` | Base URL for agent card (default: http://localhost:{port}) |
| `QDRANT_URL` | Qdrant vector DB URL |
| `NEO4J_URI` | Neo4j graph DB URI |
| `NEO4J_PASSWORD` | Neo4j password |
| `SIMONE_AUTH_REQUIRED` | Enable OAuth 2.1 (true/false) |
| `SIMONE_OAUTH_JWKS_URL` | JWKS endpoint for token validation |

## Broken Links Check
- No internal links to other `.doc.md` files in this module.
