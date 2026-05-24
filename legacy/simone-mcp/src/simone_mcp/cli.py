from __future__ import annotations

import asyncio
import json
import os
import sys
from pathlib import Path
from typing import Any

from .core import TOOL_DEFINITIONS, build_agent_card, execute_simone_action, get_project_overview
from .mcp_stdio import serve_stdio


def _print(payload: Any) -> None:
    sys.stdout.write(f"{json.dumps(payload, indent=2)}\n")


def _read_action_argument() -> dict[str, Any]:
    if len(sys.argv) > 2:
        raw = sys.argv[2]
    else:
        raw = sys.stdin.read().strip()
    if not raw:
        raise SystemExit("missing_action_json")
    return json.loads(raw)


def main() -> None:
    command = sys.argv[1] if len(sys.argv) > 1 else "help"
    if command in {"serve", "serve-a2a"}:
        import uvicorn

        from .http_app import create_app

        port = int(sys.argv[2]) if len(sys.argv) > 2 and sys.argv[2].isdigit() else 8234
        host = os.getenv("SIMONE_HOST", "0.0.0.0")
        base_url = os.getenv("SIMONE_BASE_URL", f"http://localhost:{port}")
        print(f"Simone-MCP HTTP/SSE server on {host}:{port}")
        print(f"  MCP:    {base_url}/mcp")
        print(f"  A2A:    {base_url}/a2a/v1")
        print(f"  Health: {base_url}/health")
        print(f"  Docs:   {base_url}/docs")
        uvicorn.run(create_app(), host=host, port=port, log_level="info")
        return
    if command == "serve-mcp":
        asyncio.run(serve_stdio())
        return
    if command == "print-card":
        base_url = os.getenv("SIMONE_BASE_URL", "http://localhost:8234")
        _print(build_agent_card(base_url))
        return
    if command == "run-action":
        _print(asyncio.run(execute_simone_action(_read_action_argument())))
        return
    if command == "index":
        root = sys.argv[2] if len(sys.argv) > 2 else str(Path.cwd())
        overview = get_project_overview({"root": root})
        _print(overview)
        return
    if command == "validate":
        _validate_config()
        return
    if command == "tool-list":
        _print({"tools": TOOL_DEFINITIONS})
        return
    sys.stderr.write(
        "Usage:\n"
        "  simone serve              Start HTTP/A2A server (port 8234)\n"
        "  simone serve-mcp          Start MCP stdio server\n"
        "  simone print-card         Print agent discovery card\n"
        "  simone run-action JSON    Execute a tool action\n"
        "  simone index [PATH]       Show project overview\n"
        "  simone validate           Validate server configuration\n"
        "  simone tool-list          List available MCP tools\n"
    )


def _validate_config() -> None:
    issues: list[str] = []
    qdrant_url = os.getenv("QDRANT_URL", "")
    neo4j_uri = os.getenv("NEO4J_URI", "")
    neo4j_password = os.getenv("NEO4J_PASSWORD", "")
    auth_required = os.getenv("SIMONE_AUTH_REQUIRED", "false").lower() in {"1", "true", "yes"}
    jwks_url = os.getenv("SIMONE_OAUTH_JWKS_URL", "")

    if auth_required and not jwks_url:
        issues.append("SIMONE_AUTH_REQUIRED=true but SIMONE_OAUTH_JWKS_URL not set")
    if neo4j_uri and not neo4j_password:
        issues.append("NEO4J_URI set but NEO4J_PASSWORD missing")
    if qdrant_url:
        try:
            from qdrant_client import QdrantClient

            client = QdrantClient(url=qdrant_url)
            client.get_collections()
        except Exception as e:
            issues.append(f"Qdrant connection failed: {e}")
    if neo4j_uri and neo4j_password:
        try:
            from neo4j import GraphDatabase

            driver = GraphDatabase.driver(neo4j_uri, auth=("neo4j", neo4j_password))
            driver.verify_connectivity()
            driver.close()
        except Exception as e:
            issues.append(f"Neo4j connection failed: {e}")

    if issues:
        for issue in issues:
            sys.stderr.write(f"  ISSUE: {issue}\n")
        sys.exit(1)
    _print({"ok": True, "status": "valid", "memory": "hybrid" if qdrant_url and neo4j_uri else "basic"})


if __name__ == "__main__":
    main()
