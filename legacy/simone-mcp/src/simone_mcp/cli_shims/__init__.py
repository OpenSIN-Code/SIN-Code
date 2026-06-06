# Purpose: CLI shim package for Simone MCP tools
# Docs: __init__.doc.md
"""CLI shim package — thin wrappers exposing Simone MCP tools as CLI binaries.

Each shim is a single-tool wrapper that:
  1. Parses CLI args (argparse)
  2. Calls the corresponding function in `simone_mcp.core` directly
  3. Prints the result as JSON

This bypasses the FastAPI/MCP transport layer entirely, so callers don't
need the MCP server running. For tools that are defined as `async def`,
we run them in a small asyncio loop here.
"""
