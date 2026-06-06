# Purpose: CLI shim for simone build_agent_card
# Docs: simone-build-agent-card.doc.md
"""CLI: simone-build-agent-card — build the A2A agent discovery card.

Mirrors the MCP tool `build_agent_card`. Prints the agent card as
JSON to stdout. Useful for serving the card at /.well-known/agent.json
without booting the FastAPI app.

Usage:
    simone-build-agent-card --base-url URL
"""
from __future__ import annotations
import argparse
import json
import sys

from simone_mcp.core import build_agent_card


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="simone-build-agent-card",
        description="Build the A2A agent discovery card for /.well-known/agent.json.",
    )
    parser.add_argument("--base-url", required=True)
    args = parser.parse_args(argv)
    card = build_agent_card(args.base_url)
    print(json.dumps(card, indent=2, default=str))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
