"""MCP server entry point for simone-mcp.

Docs: server.doc.md
"""
from __future__ import annotations

import asyncio

from .mcp_stdio import serve_stdio


def main():
    asyncio.run(serve_stdio())


if __name__ == "__main__":
    main()
