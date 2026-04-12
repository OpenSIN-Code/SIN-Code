from __future__ import annotations

import ast
import asyncio
import json
import os
import re
from pathlib import Path
from typing import Any
from urllib.parse import urlparse, urlunparse


AGENT_NAME = "simone-mcp"
AGENT_DISPLAY_NAME = "Simone MCP"
AGENT_VERSION = "2026.04.12"
AGENT_DESCRIPTION = "Production-grade MCP 2.0 code worker with symbol operations, streamable HTTP transport, OAuth 2.1 readiness, and hybrid memory integrations."
MCP_ENDPOINT = "/mcp"
A2A_ENDPOINT = "/a2a/v1"
OPEN_PATHS = {
    "/",
    "/health",
    "/dashboard",
    "/.well-known/agent-card.json",
    "/.well-known/agent.json",
    "/.well-known/oauth-client.json",
    "/.well-known/oauth-authorization-server",
}
TOOL_DEFINITIONS = [
    {
        "name": "code.find_symbol",
        "description": "Locate symbol definitions across a workspace.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "symbol": {"type": "string"},
                "root": {"type": "string"},
            },
            "required": ["symbol"],
        },
        "annotations": {
            "title": "Find Symbol",
            "readOnlyHint": True,
            "idempotentHint": True,
        },
    },
    {
        "name": "code.find_references",
        "description": "Find textual references to a symbol across a workspace.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "symbol": {"type": "string"},
                "root": {"type": "string"},
            },
            "required": ["symbol"],
        },
        "annotations": {
            "title": "Find References",
            "readOnlyHint": True,
            "idempotentHint": True,
        },
    },
    {
        "name": "code.replace_symbol_body",
        "description": "Replace the body of a Python function or async function.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "symbol": {"type": "string"},
                "file": {"type": "string"},
                "body": {"type": "string"},
            },
            "required": ["symbol", "file", "body"],
        },
        "annotations": {
            "title": "Replace Symbol Body",
            "readOnlyHint": False,
            "idempotentHint": False,
        },
    },
    {
        "name": "code.insert_after_symbol",
        "description": "Insert text immediately after a Python symbol block.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "symbol": {"type": "string"},
                "file": {"type": "string"},
                "text": {"type": "string"},
            },
            "required": ["symbol", "file", "text"],
        },
        "annotations": {
            "title": "Insert After Symbol",
            "readOnlyHint": False,
            "idempotentHint": False,
        },
    },
    {
        "name": "code.project_overview",
        "description": "Summarize the workspace footprint and primary file types.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "root": {"type": "string"},
            },
        },
        "annotations": {
            "title": "Project Overview",
            "readOnlyHint": True,
            "idempotentHint": True,
        },
    },
]
CAPABILITIES = [tool["name"] for tool in TOOL_DEFINITIONS] + [
    "memory.hybrid",
    "transport.streamable_http",
    "auth.oauth2.1",
]


def build_agent_card(base_url: str) -> dict[str, Any]:
    normalized_base_url = base_url.rstrip("/")
    return {
        "name": AGENT_NAME,
        "displayName": AGENT_DISPLAY_NAME,
        "description": AGENT_DESCRIPTION,
        "version": AGENT_VERSION,
        "url": normalized_base_url,
        "capabilities": CAPABILITIES,
        "endpoints": {
            "health": "/health",
            "dashboard": "/dashboard",
            "a2a": A2A_ENDPOINT,
            "mcp": MCP_ENDPOINT,
        },
        "auth": {
            "type": "oauth2.1",
            "jwksUrl": os.getenv("SIMONE_OAUTH_JWKS_URL", ""),
            "issuer": os.getenv("SIMONE_OAUTH_ISSUER", ""),
            "audience": os.getenv("SIMONE_OAUTH_AUDIENCE", AGENT_NAME),
        },
        "skills": [
            {"id": "agent.help", "name": "Help"},
            {"id": "simone.mcp.health", "name": "Health"},
            {"id": "code.find_symbol", "name": "Find Symbol"},
            {"id": "code.find_references", "name": "Find References"},
            {"id": "code.replace_symbol_body", "name": "Replace Symbol Body"},
            {"id": "code.insert_after_symbol", "name": "Insert After Symbol"},
            {"id": "code.project_overview", "name": "Project Overview"},
        ],
    }


def build_oauth_client_metadata(base_url: str) -> dict[str, Any]:
    callback = f"{base_url.rstrip('/')}/oauth/callback"
    return {
        "client_name": AGENT_NAME,
        "application_type": "native",
        "grant_types": ["authorization_code", "refresh_token"],
        "response_types": ["code"],
        "redirect_uris": [callback, "http://127.0.0.1/callback"],
        "token_endpoint_auth_method": "none",
    }


def build_authorization_server_metadata(base_url: str) -> dict[str, Any]:
    normalized_base_url = base_url.rstrip("/")
    issuer = os.getenv("SIMONE_OAUTH_ISSUER") or normalized_base_url
    authorization_endpoint = (
        os.getenv("SIMONE_OAUTH_AUTHORIZATION_ENDPOINT") or f"{issuer}/authorize"
    )
    token_endpoint = os.getenv("SIMONE_OAUTH_TOKEN_ENDPOINT") or f"{issuer}/token"
    registration_endpoint = (
        os.getenv("SIMONE_OAUTH_REGISTRATION_ENDPOINT") or f"{issuer}/register"
    )
    jwks_uri = os.getenv("SIMONE_OAUTH_JWKS_URL") or f"{issuer}/.well-known/jwks.json"
    return {
        "issuer": issuer,
        "authorization_endpoint": authorization_endpoint,
        "token_endpoint": token_endpoint,
        "registration_endpoint": registration_endpoint,
        "jwks_uri": jwks_uri,
        "response_types_supported": ["code"],
        "grant_types_supported": ["authorization_code", "refresh_token"],
        "token_endpoint_auth_methods_supported": ["none"],
        "code_challenge_methods_supported": ["S256"],
    }


def _build_realtime_url(supabase_url: str) -> str:
    parsed = urlparse(supabase_url.rstrip("/"))
    scheme = "wss" if parsed.scheme == "https" else "ws"
    path = parsed.path.rstrip("/")
    if not path.endswith("/realtime/v1"):
        path = f"{path}/realtime/v1" if path else "/realtime/v1"
    return urlunparse((scheme, parsed.netloc, path, "", "", ""))


def _workspace_root(value: str | None) -> Path:
    return Path(value).expanduser().resolve() if value else Path.cwd()


def _candidate_files(root: Path) -> list[Path]:
    blocked = {
        ".git",
        ".venv",
        "venv",
        "node_modules",
        "__pycache__",
        ".serena",
        ".pcpm",
    }
    paths: list[Path] = []
    for path in root.rglob("*.py"):
        if any(part in blocked for part in path.parts):
            continue
        if path.is_file():
            paths.append(path)
    return sorted(paths)


def _parse_file(path: Path) -> ast.AST | None:
    try:
        return ast.parse(path.read_text(encoding="utf-8"), filename=str(path))
    except (OSError, SyntaxError, UnicodeDecodeError):
        return None


def _iter_symbol_nodes(tree: ast.AST) -> list[ast.AST]:
    return [
        node
        for node in ast.walk(tree)
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef))
    ]


def _symbol_kind(node: ast.AST) -> str:
    if isinstance(node, ast.ClassDef):
        return "class"
    if isinstance(node, ast.AsyncFunctionDef):
        return "async_function"
    return "function"


def find_symbol(payload: dict[str, Any]) -> dict[str, Any]:
    symbol = str(payload.get("symbol") or "").strip()
    root = _workspace_root(payload.get("root"))
    matches: list[dict[str, Any]] = []
    for path in _candidate_files(root):
        tree = _parse_file(path)
        if tree is None:
            continue
        for node in _iter_symbol_nodes(tree):
            if getattr(node, "name", None) != symbol:
                continue
            matches.append(
                {
                    "symbol": symbol,
                    "kind": _symbol_kind(node),
                    "file": str(path),
                    "line": getattr(node, "lineno", 0),
                    "column": getattr(node, "col_offset", 0),
                    "endLine": getattr(node, "end_lineno", getattr(node, "lineno", 0)),
                    "endColumn": getattr(
                        node, "end_col_offset", getattr(node, "col_offset", 0)
                    ),
                }
            )
    return {"ok": True, "symbol": symbol, "count": len(matches), "matches": matches}


def find_references(payload: dict[str, Any]) -> dict[str, Any]:
    symbol = str(payload.get("symbol") or "").strip()
    root = _workspace_root(payload.get("root"))
    pattern = re.compile(rf"\b{re.escape(symbol)}\b")
    matches: list[dict[str, Any]] = []
    total = 0
    for path in _candidate_files(root):
        try:
            content = path.read_text(encoding="utf-8")
        except (OSError, UnicodeDecodeError):
            continue
        file_hits = []
        for number, line in enumerate(content.splitlines(), start=1):
            if not pattern.search(line):
                continue
            hit_count = len(pattern.findall(line))
            total += hit_count
            file_hits.append({"line": number, "text": line.strip(), "count": hit_count})
        if file_hits:
            matches.append({"file": str(path), "hits": file_hits})
    return {"ok": True, "symbol": symbol, "count": total, "matches": matches}


def _find_named_node(path: Path, symbol: str) -> ast.AST:
    tree = _parse_file(path)
    if tree is None:
        raise ValueError("unparseable_python_file")
    for node in _iter_symbol_nodes(tree):
        if getattr(node, "name", None) == symbol:
            return node
    raise ValueError("symbol_not_found")


def _preserve_trailing_newline(text: str, updated: str) -> str:
    return (
        f"{updated}\n"
        if text.endswith("\n") and not updated.endswith("\n")
        else updated
    )


def replace_symbol_body(payload: dict[str, Any]) -> dict[str, Any]:
    try:
        symbol = str(payload.get("symbol") or "").strip()
        file_path = Path(str(payload.get("file") or "")).expanduser().resolve()
        body = str(payload.get("body") or "pass")
        original = file_path.read_text(encoding="utf-8")
        lines = original.splitlines()
        node = _find_named_node(file_path, symbol)
        if (
            not isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef))
            or not node.body
        ):
            raise ValueError("replace_symbol_body_requires_function")
        first_statement = node.body[0]
        last_statement = node.body[-1]
        indent = " " * (node.col_offset + 4)
        replacement = [f"{indent}{line}" if line else "" for line in body.splitlines()]
        lines[first_statement.lineno - 1 : last_statement.end_lineno] = replacement
        updated = _preserve_trailing_newline(original, "\n".join(lines))
        file_path.write_text(updated, encoding="utf-8")
        return {"ok": True, "symbol": symbol, "file": str(file_path)}
    except Exception as error:
        return {"ok": False, "error": str(error), "symbol": payload.get("symbol")}


def insert_after_symbol(payload: dict[str, Any]) -> dict[str, Any]:
    try:
        symbol = str(payload.get("symbol") or "").strip()
        file_path = Path(str(payload.get("file") or "")).expanduser().resolve()
        text = str(payload.get("text") or "")
        original = file_path.read_text(encoding="utf-8")
        lines = original.splitlines()
        node = _find_named_node(file_path, symbol)
        insertion = text.splitlines() or [text]
        lines[node.end_lineno : node.end_lineno] = insertion
        updated = _preserve_trailing_newline(original, "\n".join(lines))
        file_path.write_text(updated, encoding="utf-8")
        return {"ok": True, "symbol": symbol, "file": str(file_path)}
    except Exception as error:
        return {"ok": False, "error": str(error), "symbol": payload.get("symbol")}


def get_project_overview(payload: dict[str, Any]) -> dict[str, Any]:
    root = _workspace_root(payload.get("root"))
    blocked = {
        ".git",
        ".venv",
        "venv",
        "node_modules",
        "__pycache__",
        ".serena",
        ".pcpm",
    }
    counts: dict[str, int] = {}
    file_count = 0
    for path in root.rglob("*"):
        if any(part in blocked for part in path.parts):
            continue
        if not path.is_file():
            continue
        file_count += 1
        suffix = path.suffix or "[none]"
        counts[suffix] = counts.get(suffix, 0) + 1
    top_extensions = sorted(counts.items(), key=lambda item: (-item[1], item[0]))[:10]
    return {
        "ok": True,
        "root": str(root),
        "fileCount": file_count,
        "topExtensions": [
            {"extension": extension, "count": count}
            for extension, count in top_extensions
        ],
    }


def query_hybrid_memory(payload: dict[str, Any]) -> dict[str, Any]:
    query = str(payload.get("query") or "").strip()
    qdrant_url = os.getenv("QDRANT_URL", "")
    neo4j_uri = os.getenv("NEO4J_URI", "")
    enabled = bool(qdrant_url and neo4j_uri)
    return {
        "ok": True,
        "enabled": enabled,
        "query": query,
        "vectorStore": qdrant_url or None,
        "graphStore": neo4j_uri or None,
        "matches": [],
    }


async def execute_simone_action(payload: dict[str, Any]) -> dict[str, Any]:
    try:
        action = str(payload.get("action") or "agent.help")
        if action in {"agent.help", "simone.mcp.help"}:
            return {
                "ok": True,
                "name": AGENT_NAME,
                "actions": [
                    "agent.help",
                    "simone.mcp.health",
                    "code.find_symbol",
                    "code.find_references",
                    "code.replace_symbol_body",
                    "code.insert_after_symbol",
                    "code.project_overview",
                    "memory.query",
                ],
            }
        if action in {"simone.mcp.health", "sin.simone.mcp.health"}:
            return {
                "ok": True,
                "status": "ok",
                "name": AGENT_NAME,
                "version": AGENT_VERSION,
                "transport": "streamable-http+stdio",
                "memory": "hybrid",
            }
        if action in {
            "code.find_symbol",
            "simone.mcp.symbol.search",
            "sin.simone.mcp.symbol.search",
        }:
            return find_symbol(payload)
        if action in {"code.find_references", "simone.mcp.references.search"}:
            return find_references(payload)
        if action in {"code.replace_symbol_body", "simone.mcp.structural.edit"}:
            return replace_symbol_body(payload)
        if action in {"code.insert_after_symbol"}:
            return insert_after_symbol(payload)
        if action in {"code.project_overview"}:
            return get_project_overview(payload)
        if action in {"memory.query", "sin.simone.mcp.memory.query"}:
            return query_hybrid_memory(payload)
        return {"ok": False, "error": "unknown_action", "action": action}
    except Exception as error:
        return {"ok": False, "error": str(error), "action": payload.get("action")}


async def process_lsp_task(payload: dict[str, Any]) -> dict[str, Any]:
    await asyncio.sleep(0)
    if payload.get("action"):
        return await execute_simone_action(payload)
    return {"ok": True, "symbol": payload.get("symbol"), "engine": "python-ast"}


async def dashboard() -> str:
    return """
<html>
  <head>
    <title>Simone MCP</title>
    <meta charset=\"utf-8\" />
  </head>
  <body>
    <main>
      <h1>Simone MCP</h1>
      <section>
        <h2>Quick Actions</h2>
        <ul>
          <li><code>activate_simone</code></li>
          <li><code>activate_simone serve-mcp</code></li>
          <li><code>activate_simone print-card</code></li>
          <li><code>activate_simone run-action '{\"action\":\"simone.mcp.health\"}'</code></li>
        </ul>
      </section>
    </main>
  </body>
</html>
""".strip()


def json_dumps(payload: Any) -> str:
    return json.dumps(payload, indent=2, sort_keys=False)
