# SPDX-License-Identifier: MIT
# Purpose: CLI shim for mcp_validate
# Docs: mcp-validate.doc.md
"""CLI: mcp-validate — validate an MCP server (tools, type hints, docstrings, CoDocs).

Usage: mcp-validate <PROJECT_DIR>
"""
from __future__ import annotations

import argparse
import sys

from ..mcp_server import mcp_validate


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="mcp-validate", description="Validate an MCP server (tools, type hints, docstrings, CoDocs).")
    parser.add_argument("project_dir")
    args = parser.parse_args(argv)
    try:
        print(mcp_validate(args.project_dir))
    except Exception as e:
        print(f"[mcp-validate] error: {e}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
