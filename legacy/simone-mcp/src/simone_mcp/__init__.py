"""Simone MCP — production-grade code-intelligence MCP server.

Re-exports the public API: the CLI entry point and the core tool
functions. See `core.doc.md` for the tool surface and `http_app.doc.md`
for the FastAPI application.

Docs: __init__.doc.md
"""
from .cli import main
from .core import (
    _build_realtime_url,
    build_agent_card,
    dashboard,
    execute_simone_action,
    find_references,
    find_symbol,
    get_project_overview,
    insert_after_symbol,
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
    "main",
    "replace_symbol_body",
]

import logging

# `NullHandler` so importing this package doesn't print a "No handlers
# could be found" warning when the consumer hasn't configured logging.
logging.getLogger(__name__).addHandler(logging.NullHandler())
