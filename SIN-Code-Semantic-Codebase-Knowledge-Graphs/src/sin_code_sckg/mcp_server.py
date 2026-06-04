"""MCP-Server fuer Agent-Integration."""

from __future__ import annotations

import json
from pathlib import Path

import yaml

from .graph import KnowledgeGraph


def _load_graph() -> KnowledgeGraph:
    cfg = {"graph": {"storage": "./.sin/knowledge.graph"}}
    cfg_path = Path("config.yaml")
    if cfg_path.exists():
        with open(cfg_path) as f:
            cfg = yaml.safe_load(f)
    return KnowledgeGraph(storage_path=cfg["graph"]["storage"])


def build_server():
    """Erzeugt die FastMCP-Instanz (ohne sie zu starten) - testbar."""
    from mcp.server.fastmcp import FastMCP

    mcp = FastMCP("sin-code-sckg")

    @mcp.tool()
    def find_symbol(name: str) -> str:
        """Find a symbol in the codebase by name."""
        return json.dumps(_load_graph().find_symbol(name), indent=2)

    @mcp.tool()
    def impact_analysis(fqid: str) -> str:
        """Return blast-radius / impact analysis for a fully-qualified symbol id."""
        return json.dumps(_load_graph().impact_analysis(fqid), indent=2)

    @mcp.tool()
    def architecture_overview() -> str:
        """Return high-level architecture stats and hubs."""
        return json.dumps(_load_graph().explain_architecture(), indent=2)

    @mcp.tool()
    def downstream_deps(fqid: str) -> str:
        """Return downstream dependencies (what uses this symbol)."""
        return json.dumps(_load_graph().downstream(fqid))

    return mcp


def main():
    build_server().run()


if __name__ == "__main__":
    main()
