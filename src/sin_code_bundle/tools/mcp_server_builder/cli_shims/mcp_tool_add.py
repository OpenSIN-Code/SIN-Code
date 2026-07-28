# SPDX-License-Identifier: MIT
# Purpose: CLI shim for mcp_tool_add
# Docs: mcp-tool-add.doc.md
"""CLI: mcp-tool-add — add a new tool to an existing MCP server.

Usage: mcp-tool-add --server-path PATH --tool-name NAME --description DESC
                   [--params JSON] [--body CODE]
"""

from __future__ import annotations

import argparse
import sys

from ..mcp_server import mcp_tool_add


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="mcp-tool-add", description="Add a new tool to an existing MCP server."
    )
    parser.add_argument("--server-path", required=True, help="Path to the mcp_server.py file.")
    parser.add_argument(
        "--tool-name", required=True, help="Snake-case tool name (valid Python identifier)."
    )
    parser.add_argument("--description", required=True, help="One-line docstring for the tool.")
    parser.add_argument("--params", default="[]", help="JSON list of [name, type, default] tuples.")
    parser.add_argument(
        "--body", default='result = {"ok": True}', help="Python statements for the tool body."
    )
    args = parser.parse_args(argv)
    try:
        print(
            mcp_tool_add(
                server_path=args.server_path,
                tool_name=args.tool_name,
                description=args.description,
                params=args.params,
                body=args.body,
            )
        )
    except Exception as e:
        print(f"[mcp-tool-add] error: {e}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
