"""MCP server re-exports — the public tool surface for MCP clients.

Re-exports the core functions from `simone_mcp.core` so external
clients (OpenCode, Claude, etc.) can import them as a flat module.

Docs: mcp_server.doc.md
"""
from simone_mcp.core import (
    _build_realtime_url,
    build_agent_card,
    dashboard,
    execute_simone_action,
    find_references,
    find_symbol,
    get_project_overview,
    insert_after_symbol,
    process_lsp_task,
    replace_symbol_body,
)

__all__ = [
    "_build_realtime_url",
    "build_agent_card",
    "dashboard",
    "execute_simone_action",
    "find_references",
    "find_symbol",
    "get_project_overview",
    "insert_after_symbol",
    "process_lsp_task",
    "replace_symbol_body",
]
