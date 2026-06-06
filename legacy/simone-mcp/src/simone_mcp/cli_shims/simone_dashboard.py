# Purpose: CLI shim for simone dashboard
# Docs: simone-dashboard.doc.md
"""CLI: simone-dashboard — render the Simone HTML dashboard.

Mirrors the async MCP tool `dashboard`. Writes the HTML to stdout
(or to a file with --out). Useful for previewing the dashboard
without spinning up uvicorn.

Usage:
    simone-dashboard [--out PATH]
"""
from __future__ import annotations
import argparse
import asyncio
import sys
from pathlib import Path

from simone_mcp.core import dashboard as dashboard_fn


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="simone-dashboard",
        description="Render the Simone HTML dashboard to stdout (or --out file).",
    )
    parser.add_argument("--out", default=None)
    args = parser.parse_args(argv)
    html = asyncio.run(dashboard_fn())
    if args.out:
        Path(args.out).write_text(html, encoding="utf-8")
        print(f"[simone-dashboard] wrote {len(html)} bytes to {args.out}")
    else:
        sys.stdout.write(html)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
