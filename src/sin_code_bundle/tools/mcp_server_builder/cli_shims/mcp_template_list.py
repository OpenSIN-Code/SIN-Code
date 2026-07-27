# SPDX-License-Identifier: MIT
# Purpose: CLI shim for mcp_template_list
# Docs: mcp-template-list.doc.md
"""CLI: mcp-template-list — list available MCP server templates.

Usage: mcp-template-list
"""
from __future__ import annotations

import sys

from ..mcp_server import mcp_template_list


def main(argv: list[str] | None = None) -> int:
    try:
        print(mcp_template_list())
    except Exception as e:
        print(f"[mcp-template-list] error: {e}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
