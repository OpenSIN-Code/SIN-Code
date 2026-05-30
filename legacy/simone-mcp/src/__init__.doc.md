# `src/__init__.py` — Simone MCP Package Entry

Partner file: `src/__init__.py`

## Purpose
Empty package-level `__init__.py` for the `src/` namespace. This module is the root package entry point for the Simone MCP project. It intentionally contains no exports to keep the namespace clean.

## Relationship
- `src/cli.py` — imports from `simone_mcp.cli` directly, not through this file
- `src/main.py` — creates the FastAPI app via `simone_mcp.http_app`
- `src/mcp_server.py` — re-exports core functions from `simone_mcp.core`

## Dependencies
None.

## Notes
This file is empty by design. All meaningful exports are handled by `src/simone_mcp/__init__.py`.
