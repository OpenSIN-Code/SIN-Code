# SPDX-License-Identifier: MIT
# Purpose: CLI shim for mcp_publish
# Docs: mcp-publish.doc.md
"""CLI: mcp-publish — publish an MCP server to PyPI (Python) or npm (Node).

Usage: mcp-publish <PROJECT_DIR> [--template T] [--test] [--no-dry-run] [--registry URL]
"""

from __future__ import annotations

import argparse
import sys

from ..mcp_server import mcp_publish


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="mcp-publish", description="Publish an MCP server to PyPI or npm."
    )
    parser.add_argument("project_dir")
    parser.add_argument(
        "--template", default="python-fastmcp", choices=["python-fastmcp", "node-mcp", "go-mcp"]
    )
    parser.add_argument("--test", action="store_true", help="Publish to TestPyPI (Python only).")
    parser.add_argument(
        "--no-dry-run",
        dest="dry_run",
        action="store_false",
        help="Actually upload (default: dry-run).",
    )
    parser.add_argument("--registry", default="", help="Optional npm registry URL (Node only).")
    args = parser.parse_args(argv)
    try:
        print(
            mcp_publish(
                project_dir=args.project_dir,
                template=args.template,
                test=args.test,
                dry_run=args.dry_run,
                registry=args.registry,
            )
        )
    except Exception as e:
        print(f"[mcp-publish] error: {e}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
