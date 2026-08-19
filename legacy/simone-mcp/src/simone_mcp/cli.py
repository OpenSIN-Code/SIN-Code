"""Command-line interface for the `simone` command.

Supports: `serve`, `serve-mcp`, `print-card`, `run-action`, `index`,
`validate`, `integrate`, `tool-list`. The `src/cli.py` shim is a
1-line delegate to `main()`.

Docs: cli.doc.md
"""
from __future__ import annotations

import asyncio
import json
import logging
import os
import shutil
import sys
from pathlib import Path
from typing import Any

from .core import TOOL_DEFINITIONS, build_agent_card, execute_simone_action, get_project_overview
from .mcp_stdio import serve_stdio

AGENTS_RULES = """

## Simone MCP — Tool Replacements

Use Simone MCP tools INSTEAD of OpenCode built-ins:

| Instead of | Use |
|------------|-----|
| `grep` / `Grep` for symbol search | `sin_simone_mcp_symbol_search` |
| `edit` / `Edit` for code changes | `sin_simone_mcp_structural_edit` |
| manual grep for references | `sin_simone_mcp_find_references` |
| project exploration | `sin_simone_mcp_project_overview` |

"""


def _integrate_opencode() -> dict[str, str | int]:
    OPCODE_CONFIG = Path.home() / ".config" / "opencode" / "opencode.json"
    OPCODE_AGENTS = Path.home() / ".config" / "opencode" / "AGENTS.md"
    results: list[str] = []

    if not OPCODE_CONFIG.exists():
        results.append(f"SKIP: {OPCODE_CONFIG} not found")
        return {"status": "skipped", "messages": results}

    with open(OPCODE_CONFIG) as f:
        config = json.load(f)

    backup = OPCODE_CONFIG.with_suffix(".json.bak")
    if not backup.exists():
        shutil.copy2(OPCODE_CONFIG, backup)
        results.append(f"BACKUP: {backup}")

    changed = False

    mcp = config.setdefault("mcp", {})
    if "sin-simone-mcp" not in mcp:
        repo_root = Path(__file__).resolve().parent.parent.parent
        mcp["sin-simone-mcp"] = {
            "type": "local",
            "command": [sys.executable or "python3", str(repo_root / "src" / "cli.py"), "serve-mcp"],
            "enabled": True,
        }
        changed = True
        results.append("ADD: sin-simone-mcp MCP server")
    else:
        results.append("OK: sin-simone-mcp already configured")

    perm = config.setdefault("permission", {})
    for tool in ("grep", "glob"):
        if perm.get(tool) != "deny":
            perm[tool] = "deny"
            changed = True
            results.append(f"DENY: built-in {tool}")

    if perm.get("sin_simone_mcp_*") != "allow":
        perm["sin_simone_mcp_*"] = "allow"
        changed = True
        results.append("ALLOW: sin_simone_mcp_*")

    if changed:
        with open(OPCODE_CONFIG, "w") as f:
            json.dump(config, f, indent=2)
        results.append(f"SAVED: {OPCODE_CONFIG}")

    if OPCODE_AGENTS.exists():
        with open(OPCODE_AGENTS) as f:
            content = f.read()
        if "## Simone MCP — Tool Replacements" not in content:
            with open(OPCODE_AGENTS, "a") as f:
                f.write(AGENTS_RULES)
            results.append(f"PATCHED: {OPCODE_AGENTS}")

    messages = "\n".join(results)
    print(messages)
    return {"status": "ok", "changed": changed, "messages": results}

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s [%(name)s] %(message)s",
    stream=sys.stderr,
)
logger = logging.getLogger(__name__)


def _print(payload: Any) -> None:
    sys.stdout.write(f"{json.dumps(payload, indent=2)}\n")


def _read_action_argument() -> dict[str, Any]:
    if len(sys.argv) > 2:
        raw = sys.argv[2]
    else:
        raw = sys.stdin.read().strip()
    if not raw:
        raise SystemExit("missing_action_json")
    return json.loads(raw)  # type: ignore[no-any-return]


def main() -> None:
    command = sys.argv[1] if len(sys.argv) > 1 else "help"
    if command in {"serve", "serve-a2a"}:
        import uvicorn

        from .http_app import create_app

        port = int(sys.argv[2]) if len(sys.argv) > 2 and sys.argv[2].isdigit() else 8234
        host = os.getenv("SIMONE_HOST", "0.0.0.0")
        base_url = os.getenv("SIMONE_BASE_URL", f"http://localhost:{port}")
        logger.info("Simone-MCP HTTP/SSE server on %s:%s", host, port)
        logger.info("  MCP:    %s/mcp", base_url)
        logger.info("  A2A:    %s/a2a/v1", base_url)
        logger.info("  Health: %s/health", base_url)
        logger.info("  Docs:   %s/docs", base_url)
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
    if command == "integrate":
        _print(_integrate_opencode())
        return
    sys.stderr.write(
        "Usage:\n"
        "  simone serve              Start HTTP/A2A server (port 8234)\n"
        "  simone serve-mcp          Start MCP stdio server\n"
        "  simone print-card         Print agent discovery card\n"
        "  simone run-action JSON    Execute a tool action\n"
        "  simone index [PATH]       Show project overview\n"
        "  simone validate           Validate server configuration\n"
        "  simone integrate          Integrate with OpenCode (disable grep, add MCP)\n"
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
