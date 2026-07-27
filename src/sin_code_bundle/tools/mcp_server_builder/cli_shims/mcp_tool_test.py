# SPDX-License-Identifier: MIT
# Purpose: CLI shim for mcp_tool_test
# Docs: mcp-tool-test.doc.md
"""CLI: mcp-tool-test — generate pytest tests for an MCP tool.

Usage: mcp-tool-test --server-path PATH --tool-name NAME [--output-path PATH]
"""
from __future__ import annotations

import argparse
import sys

from ..mcp_server import mcp_tool_test


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="mcp-tool-test", description="Generate pytest tests for an MCP tool.")
    parser.add_argument("--server-path", required=True)
    parser.add_argument("--tool-name", required=True)
    parser.add_argument("--output-path", default="")
    args = parser.parse_args(argv)
    try:
        print(mcp_tool_test(
            server_path=args.server_path,
            tool_name=args.tool_name,
            output_path=args.output_path,
        ))
    except Exception as e:
        print(f"[mcp-tool-test] error: {e}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
