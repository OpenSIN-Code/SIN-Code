# Purpose: CLI shim for simone execute_simone_action
# Docs: simone-execute-action.doc.md
"""CLI: simone-execute-action — dispatch a simone action by name.

Mirrors the async MCP tool `execute_simone_action`. The JSON payload
can be passed inline (--payload) or via a file (--payload-file).
If neither is given, defaults to {"action": "agent.help"}.

Usage:
    simone-execute-action [--action NAME] [--payload JSON] [--payload-file PATH]
"""
from __future__ import annotations
import argparse
import asyncio
import json
import sys
from pathlib import Path

from simone_mcp.core import execute_simone_action


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="simone-execute-action",
        description="Dispatch a Simone action by name (health, help, find_symbol, etc.).",
    )
    parser.add_argument("--action", default=None)
    parser.add_argument("--payload", default=None)
    parser.add_argument("--payload-file", default=None)
    args = parser.parse_args(argv)
    payload: dict = {}
    if args.payload_file:
        payload = json.loads(Path(args.payload_file).read_text(encoding="utf-8"))
    elif args.payload:
        payload = json.loads(args.payload)
    if args.action:
        payload["action"] = args.action
    if "action" not in payload:
        payload["action"] = "agent.help"
    result = asyncio.run(execute_simone_action(payload))
    print(json.dumps(result, indent=2, default=str))
    return 0 if result.get("ok") else 1


if __name__ == "__main__":
    raise SystemExit(main())
