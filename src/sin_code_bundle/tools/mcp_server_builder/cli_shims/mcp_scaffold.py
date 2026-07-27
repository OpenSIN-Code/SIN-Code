# SPDX-License-Identifier: MIT
# Purpose: CLI shim for mcp_scaffold
# Docs: mcp-scaffold.doc.md
"""CLI: mcp-scaffold — scaffold a new MCP server from a spec.

Usage: mcp-scaffold --name NAME --description DESC [--target DIR] [--tools CSV] [--template T]
"""
from __future__ import annotations

import argparse
import sys

from ..mcp_server import mcp_scaffold


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="mcp-scaffold", description="Scaffold a new MCP server from a spec.")
    parser.add_argument("--name", required=True, help='Human-readable name (e.g. "My Cool Tool").')
    parser.add_argument("--description", required=True, help="One-line description for README/pyproject.")
    parser.add_argument("--target", default=None, help="Output directory (must be empty).")
    parser.add_argument("--tools", default="ping", help='Comma-separated tool names (default: "ping").')
    parser.add_argument("--template", default="python-fastmcp", choices=["python-fastmcp", "node-mcp", "go-mcp"])
    args = parser.parse_args(argv)
    try:
        kwargs: dict = {"name": args.name, "description": args.description, "tools": args.tools, "template": args.template}
        if args.target:
            kwargs["target"] = args.target
        print(mcp_scaffold(**kwargs))
    except Exception as e:
        print(f"[mcp-scaffold] error: {e}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
