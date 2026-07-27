# SPDX-License-Identifier: MIT
# Purpose: CLI shim for mcp_audit
# Docs: mcp-audit.doc.md
"""CLI: mcp-audit — run a ceo-audit (47 quality gates) on a new MCP server.

Usage: mcp-audit <PROJECT_DIR> [--grade B] [--profile QUICK|FULL]
"""
from __future__ import annotations

import argparse
import sys

from ..mcp_server import mcp_audit


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="mcp-audit", description="Run a ceo-audit (47 quality gates) on a new MCP server.")
    parser.add_argument("project_dir")
    parser.add_argument("--grade", default="B", help="Minimum acceptable grade (default: B).")
    parser.add_argument("--profile", default="QUICK", choices=["QUICK", "FULL"])
    args = parser.parse_args(argv)
    try:
        print(mcp_audit(
            project_dir=args.project_dir,
            grade=args.grade,
            profile=args.profile,
        ))
    except Exception as e:
        print(f"[mcp-audit] error: {e}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
