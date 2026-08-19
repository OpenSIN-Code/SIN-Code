# Purpose: CLI shim for simone get_project_overview
# Docs: simone-project-overview.doc.md
"""CLI: simone-project-overview — summarize the workspace.

Mirrors the MCP tool `get_project_overview`. Returns file count,
top-10 extensions, and a graphify summary (if graph.json exists).

Usage:
    simone-project-overview [--root PATH]
"""
from __future__ import annotations
import argparse
import json
import sys

from simone_mcp.core import get_project_overview


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="simone-project-overview",
        description="Summarize the workspace: file count, top extensions, graphify summary.",
    )
    parser.add_argument("--root", default=None)
    args = parser.parse_args(argv)
    payload: dict = {}
    if args.root:
        payload["root"] = args.root
    result = get_project_overview(payload)
    print(json.dumps(result, indent=2, default=str))
    return 0 if result.get("ok") else 1


if __name__ == "__main__":
    raise SystemExit(main())
