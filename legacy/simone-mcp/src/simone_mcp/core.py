from __future__ import annotations

import ast
import asyncio
import json
import logging
import os
import re
import threading
from pathlib import Path
from typing import Any
from urllib.parse import urlparse, urlunparse

try:
    import libcst as cst  # type: ignore[import-not-found]
    HAS_LIBCST = True
except ImportError:
    HAS_LIBCST = False

try:
    import jedi  # type: ignore[import-not-found]
    HAS_JEDI = True
except ImportError:
    HAS_JEDI = False

try:
    import tree_sitter_python  # type: ignore[import-not-found]  # noqa: F401
    from tree_sitter import Language, Parser  # type: ignore[import-not-found]
    HAS_TREE_SITTER = True
except ImportError:
    HAS_TREE_SITTER = False

try:
    import tree_sitter_typescript as tstypescript  # type: ignore[import-not-found]
    HAS_TREE_SITTER_TS = True
except ImportError:
    HAS_TREE_SITTER_TS = False

logger = logging.getLogger(__name__)


AGENT_NAME = "simone-mcp"
AGENT_DISPLAY_NAME = "Simone MCP"
AGENT_VERSION = "2026.06.30"
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
_JSON_SCHEMA_2020_12 = "https://json-schema.org/draft/2020-12/schema"

TOOL_DEFINITIONS = [
    {
        "name": "sin_simone_mcp_health",
        "title": "Health Check",
        "description": "Check Simone MCP readiness, model, and capabilities.",
        "inputSchema": {
            "$schema": _JSON_SCHEMA_2020_12,
            "type": "object",
            "additionalProperties": False,
        },
        "outputSchema": {
            "$schema": _JSON_SCHEMA_2020_12,
            "type": "object",
            "properties": {
                "ok": {"type": "boolean"},
                "status": {"type": "string"},
                "name": {"type": "string"},
                "version": {"type": "string"},
                "transport": {"type": "string"},
                "memory": {"type": "string"},
            },
        },
        "annotations": {
            "readOnlyHint": True,
            "destructiveHint": False,
            "idempotentHint": True,
            "openWorldHint": False,
        },
        "execution": {"taskSupport": "forbidden"},
    },
    {
        "name": "sin_simone_mcp_symbol_search",
        "title": "Symbol Search",
        "description": "Search for symbols across the codebase using LSP-powered semantic analysis.",
        "inputSchema": {
            "$schema": _JSON_SCHEMA_2020_12,
            "type": "object",
            "properties": {
                "query": {"type": "string", "description": "Symbol search query"},
                "root": {"type": "string", "description": "Workspace root path"},
            },
            "required": ["query"],
        },
        "outputSchema": {
            "$schema": _JSON_SCHEMA_2020_12,
            "type": "object",
            "properties": {
                "ok": {"type": "boolean"},
                "symbol": {"type": "string"},
                "count": {"type": "integer"},
                "matches": {
                    "type": "array",
                    "items": {
                        "type": "object",
                        "properties": {
                            "symbol": {"type": "string"},
                            "kind": {"type": "string"},
                            "file": {"type": "string"},
                            "line": {"type": "integer"},
                            "column": {"type": "integer"},
                            "endLine": {"type": "integer"},
                            "endColumn": {"type": "integer"},
                        },
                    },
                },
            },
        },
        "annotations": {
            "readOnlyHint": True,
            "destructiveHint": False,
            "idempotentHint": True,
            "openWorldHint": True,
        },
        "execution": {"taskSupport": "forbidden"},
    },
    {
        "name": "sin_simone_mcp_structural_edit",
        "title": "Structural Edit",
        "description": "Perform structural code edits using LSP-grade symbol resolution and refactoring.",
        "inputSchema": {
            "$schema": _JSON_SCHEMA_2020_12,
            "type": "object",
            "properties": {
                "editPayload": {"type": "string", "description": "Structural edit payload in JSON or natural language"},
            },
            "required": ["editPayload"],
        },
        "outputSchema": {
            "$schema": _JSON_SCHEMA_2020_12,
            "type": "object",
            "properties": {
                "ok": {"type": "boolean"},
                "symbol": {"type": "string"},
                "file": {"type": "string"},
                "engine": {"type": "string"},
                "error": {"type": "string"},
            },
        },
        "annotations": {
            "readOnlyHint": False,
            "destructiveHint": True,
            "idempotentHint": False,
            "openWorldHint": True,
        },
        "execution": {"taskSupport": "forbidden"},
    },
    {
        "name": "sin_simone_mcp_memory_query",
        "title": "Memory Query",
        "description": "Query the cloud semantic memory for code context and prior analysis results.",
        "inputSchema": {
            "$schema": _JSON_SCHEMA_2020_12,
            "type": "object",
            "properties": {
                "query": {"type": "string", "description": "Semantic memory query"},
            },
            "required": ["query"],
        },
        "outputSchema": {
            "$schema": _JSON_SCHEMA_2020_12,
            "type": "object",
            "properties": {
                "ok": {"type": "boolean"},
                "results": {"type": "array"},
                "error": {"type": "string"},
            },
        },
        "annotations": {
            "readOnlyHint": True,
            "destructiveHint": False,
            "idempotentHint": True,
            "openWorldHint": False,
        },
        "execution": {"taskSupport": "forbidden"},
    },
    {
        "name": "sin_simone_mcp_find_references",
        "title": "Find References",
        "description": "Find textual references to a symbol across a workspace.",
        "inputSchema": {
            "$schema": _JSON_SCHEMA_2020_12,
            "type": "object",
            "properties": {
                "symbol": {"type": "string", "description": "Symbol name to search references for"},
                "root": {"type": "string", "description": "Workspace root path"},
            },
            "required": ["symbol"],
        },
        "outputSchema": {
            "$schema": _JSON_SCHEMA_2020_12,
            "type": "object",
            "properties": {
                "ok": {"type": "boolean"},
                "symbol": {"type": "string"},
                "count": {"type": "integer"},
                "matches": {
                    "type": "array",
                    "items": {
                        "type": "object",
                        "properties": {
                            "file": {"type": "string"},
                            "hits": {
                                "type": "array",
                                "items": {
                                    "type": "object",
                                    "properties": {
                                        "line": {"type": "integer"},
                                        "text": {"type": "string"},
                                    },
                                },
                            },
                        },
                    },
                },
                "engine": {"type": "string"},
            },
        },
        "annotations": {
            "readOnlyHint": True,
            "destructiveHint": False,
            "idempotentHint": True,
            "openWorldHint": True,
        },
        "execution": {"taskSupport": "forbidden"},
    },
    {
        "name": "sin_simone_mcp_project_overview",
        "title": "Project Overview",
        "description": "Summarize the workspace footprint and primary file types.",
        "inputSchema": {
            "$schema": _JSON_SCHEMA_2020_12,
            "type": "object",
            "properties": {
                "root": {"type": "string", "description": "Workspace root path"},
            },
        },
        "outputSchema": {
            "$schema": _JSON_SCHEMA_2020_12,
            "type": "object",
            "properties": {
                "ok": {"type": "boolean"},
                "root": {"type": "string"},
                "fileCount": {"type": "integer"},
                "topExtensions": {
                    "type": "array",
                    "items": {
                        "type": "object",
                        "properties": {
                            "extension": {"type": "string"},
                            "count": {"type": "integer"},
                        },
                    },
                },
            },
        },
        "annotations": {
            "readOnlyHint": True,
            "destructiveHint": False,
            "idempotentHint": True,
            "openWorldHint": True,
        },
        "execution": {"taskSupport": "forbidden"},
    },
    {
        "name": "sin_simone_mcp_graphify_query",
        "title": "Graphify Query",
        "description": "Ask a question about a codebase using its knowledge graph (graphify BFS traversal).",
        "inputSchema": {
            "$schema": _JSON_SCHEMA_2020_12,
            "type": "object",
            "properties": {
                "query": {"type": "string", "description": "Natural language question about the codebase"},
                "root": {"type": "string", "description": "Workspace root path"},
                "budget": {"type": "integer", "description": "Max output tokens (default 2000)"},
            },
            "required": ["query", "root"],
        },
        "outputSchema": {
            "$schema": _JSON_SCHEMA_2020_12,
            "type": "object",
            "properties": {
                "ok": {"type": "boolean"},
                "output": {"type": "string"},
                "error": {"type": "string"},
            },
        },
        "annotations": {
            "readOnlyHint": True,
            "destructiveHint": False,
            "idempotentHint": True,
            "openWorldHint": True,
        },
        "execution": {"taskSupport": "forbidden"},
    },
    {
        "name": "sin_simone_mcp_graphify_update",
        "title": "Graphify Update",
        "description": "Re-extract code files and update the knowledge graph for a workspace.",
        "inputSchema": {
            "$schema": _JSON_SCHEMA_2020_12,
            "type": "object",
            "properties": {
                "root": {"type": "string", "description": "Workspace root path"},
            },
            "required": ["root"],
        },
        "outputSchema": {
            "$schema": _JSON_SCHEMA_2020_12,
            "type": "object",
            "properties": {
                "ok": {"type": "boolean"},
                "output": {"type": "string"},
                "error": {"type": "string"},
            },
        },
        "annotations": {
            "readOnlyHint": False,
            "destructiveHint": True,
            "idempotentHint": False,
            "openWorldHint": True,
        },
        "execution": {"taskSupport": "forbidden"},
    },
    {
        "name": "sin_simone_mcp_graphify_explain",
        "title": "Graphify Explain",
        "description": "Plain-language explanation of a graph node and its neighbors in a codebase.",
        "inputSchema": {
            "$schema": _JSON_SCHEMA_2020_12,
            "type": "object",
            "properties": {
                "node": {"type": "string", "description": "Node name or label to explain"},
                "root": {"type": "string", "description": "Workspace root path"},
            },
            "required": ["node", "root"],
        },
        "outputSchema": {
            "$schema": _JSON_SCHEMA_2020_12,
            "type": "object",
            "properties": {
                "ok": {"type": "boolean"},
                "output": {"type": "string"},
                "error": {"type": "string"},
            },
        },
        "annotations": {
            "readOnlyHint": True,
            "destructiveHint": False,
            "idempotentHint": True,
            "openWorldHint": True,
        },
        "execution": {"taskSupport": "forbidden"},
    },
    {
        "name": "sin_simone_mcp_graphify_path",
        "title": "Graphify Path",
        "description": "Find the shortest path between two nodes in the codebase knowledge graph.",
        "inputSchema": {
            "$schema": _JSON_SCHEMA_2020_12,
            "type": "object",
            "properties": {
                "source": {"type": "string", "description": "Source node name"},
                "target": {"type": "string", "description": "Target node name"},
                "root": {"type": "string", "description": "Workspace root path"},
            },
            "required": ["source", "target", "root"],
        },
        "outputSchema": {
            "$schema": _JSON_SCHEMA_2020_12,
            "type": "object",
            "properties": {
                "ok": {"type": "boolean"},
                "output": {"type": "string"},
                "error": {"type": "string"},
            },
        },
        "annotations": {
            "readOnlyHint": True,
            "destructiveHint": False,
            "idempotentHint": True,
            "openWorldHint": True,
        },
        "execution": {"taskSupport": "forbidden"},
    },
]
CAPABILITIES = [tool["name"] for tool in TOOL_DEFINITIONS] + [
    "graphify",
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
            {"id": "sin_simone_mcp_health", "name": "Health Check"},
            {"id": "sin_simone_mcp_symbol_search", "name": "Symbol Search"},
            {"id": "sin_simone_mcp_structural_edit", "name": "Structural Edit"},
            {"id": "sin_simone_mcp_memory_query", "name": "Memory Query"},
            {"id": "sin_simone_mcp_find_references", "name": "Find References"},
            {"id": "sin_simone_mcp_project_overview", "name": "Project Overview"},
            {"id": "sin_simone_mcp_graphify_query", "name": "Graphify Query"},
            {"id": "sin_simone_mcp_graphify_update", "name": "Graphify Update"},
            {"id": "sin_simone_mcp_graphify_explain", "name": "Graphify Explain"},
            {"id": "sin_simone_mcp_graphify_path", "name": "Graphify Path"},
        ],
        "defaultInputModes": ["application/json", "text/plain"],
        "defaultOutputModes": ["application/json", "text/plain"],
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


class PathTraversalError(ValueError):
    pass


def _validate_file_in_workspace(file_path: Path, root: Path | None = None) -> Path:
    resolved = file_path.resolve()
    if root is not None:
        workspace = root.resolve()
        try:
            resolved.relative_to(workspace)
        except ValueError:
            raise PathTraversalError(f"Path {resolved} is outside workspace {workspace}")
    return resolved


_PY_BLOCKED = {
    ".git", ".venv", "venv", "node_modules", "__pycache__",
    ".serena", ".pcpm", "data", "profiles", "forensics",
    "cache", ".pytest_cache", "site-packages",
}
_JS_TS_EXTENSIONS = {".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs", ".mts", ".cts"}
_TSX_QUERY_NAMES = frozenset({
    "function_declaration", "method_definition", "class_declaration",
    "arrow_function", "variable_declarator",
})
_TS_PARSER_LOCK = threading.Lock()
_TS_PARSER_CACHE: tuple[Any, Any] | None = None


def _candidate_files(root: Path) -> list[Path]:
    resolved_root = root.resolve()
    paths: list[Path] = []
    for path in root.rglob("*"):
        if not path.is_file():
            continue
        if path.is_symlink() and not path.resolve().is_relative_to(resolved_root):
            continue
        if any(part in _PY_BLOCKED for part in path.parts):
            continue
        if path.suffix in {".py", ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs", ".mts", ".cts"}:
            paths.append(path)
    return sorted(paths)


def _ts_parsers() -> tuple[Any, Any] | None:
    global _TS_PARSER_CACHE
    with _TS_PARSER_LOCK:
        if _TS_PARSER_CACHE is not None:
            return _TS_PARSER_CACHE
        if not HAS_TREE_SITTER or not HAS_TREE_SITTER_TS:
            _TS_PARSER_CACHE = None
            return None
        try:
            ts_lang = Language(tstypescript.language_typescript())
            tsx_lang = Language(tstypescript.language_tsx())
            _TS_PARSER_CACHE = (Parser(ts_lang), Parser(tsx_lang))
            return _TS_PARSER_CACHE
        except Exception:
            logger.debug("tree-sitter TS/TSX parser init failed", exc_info=True)
            _TS_PARSER_CACHE = None
            return None


def _extract_symbols_treesitter(path: Path) -> list[dict[str, Any]]:
    parsers = _ts_parsers()
    if parsers is None:
        return _extract_symbols_js_regex(path)
    ts_parser, tsx_parser = parsers
    parser = tsx_parser if path.suffix in {".tsx", ".jsx"} else ts_parser
    try:
        source = path.read_bytes()
        tree = parser.parse(source)
    except (OSError, UnicodeDecodeError):
        logger.debug("Failed to read %s for tree-sitter", path, exc_info=True)
        return []
    matches: list[dict[str, Any]] = []
    stack = [tree.root_node]
    while stack:
        node = stack.pop()
        if node.type in _TSX_QUERY_NAMES:
            name_node = node.child_by_field_name("name")
            if name_node is not None:
                symbol_name = source[name_node.start_byte:name_node.end_byte].decode("utf-8", errors="replace")
                kind = "class" if "class" in node.type else "function"
                matches.append({
                    "symbol": symbol_name,
                    "kind": kind,
                    "file": str(path),
                    "line": node.start_point[0] + 1,
                    "column": node.start_point[1],
                    "endLine": node.end_point[0] + 1,
                    "endColumn": node.end_point[1],
                })
        if hasattr(node, "children"):
            stack.extend(node.children)
    return matches


_JS_SYMBOL_PATTERN = re.compile(
    r"^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?"
    r"(?:function\s+(?P<func>\w+)"
    r"|class\s+(?P<class>\w+)"
    r"|const\s+(?P<const>\w+)\s*=\s*(?:async\s+)?(?:\([^)]*\)|[\w]+)\s*=>)"
)


def _extract_symbols_js_regex(path: Path) -> list[dict[str, Any]]:
    try:
        content = path.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError):
        return []
    matches: list[dict[str, Any]] = []
    for line_no, line in enumerate(content.splitlines(), start=1):
        m = _JS_SYMBOL_PATTERN.match(line)
        if not m:
            continue
        symbol_name = m.group("func") or m.group("class") or m.group("const")
        if symbol_name:
            kind = "class" if m.group("class") else "function"
            matches.append({
                "symbol": symbol_name,
                "kind": kind,
                "file": str(path),
                "line": line_no,
                "column": m.start(),
                "endLine": line_no,
                "endColumn": m.end(),
            })
    return matches


def _parse_file(path: Path) -> ast.AST | None:
    try:
        return ast.parse(path.read_text(encoding="utf-8"), filename=str(path))
    except (OSError, SyntaxError, UnicodeDecodeError):
        logger.debug("Failed to parse %s", path, exc_info=True)
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
        if path.suffix in _JS_TS_EXTENSIONS:
            matches.extend(
                [m for m in _extract_symbols_treesitter(path) if m["symbol"] == symbol]
            )
            continue
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
    if HAS_JEDI:
        return _find_references_jedi(symbol, root)
    return _find_references_regex(symbol, root)


def _find_references_jedi(symbol: str, root: Path) -> dict[str, Any]:
    project = jedi.Project(path=root)
    matches: list[dict[str, Any]] = []
    total = 0
    seen: set[tuple[str, int]] = set()
    for path in _candidate_files(root):
        if path.suffix in _JS_TS_EXTENSIONS:
            continue
        try:
            content = path.read_text(encoding="utf-8")
        except (OSError, UnicodeDecodeError):
            continue
        file_hits: list[dict[str, Any]] = []
        try:
            script = jedi.Script(code=content, path=str(path), project=project)
        except (ValueError, OSError):
            continue
        for number, line in enumerate(content.splitlines(), start=1):
            stripped = line.strip()
            if not stripped or stripped.startswith("#"):
                continue
            col = line.find(symbol)
            if col < 0:
                continue
            try:
                defs = script.goto(number, col, follow_imports=True)
            except (jedi.utils.UncaughtAttributeError, ValueError, TypeError):
                continue
            for d in defs:
                if d.name == symbol:
                    key = (str(path), number)
                    if key not in seen:
                        seen.add(key)
                        total += 1
                        file_hits.append({"line": number, "text": stripped})
                    break
        if file_hits:
            matches.append({"file": str(path), "hits": file_hits})
    return {"ok": True, "symbol": symbol, "count": total, "matches": matches, "engine": "jedi"}


def _find_references_regex(symbol: str, root: Path) -> dict[str, Any]:
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
    return {"ok": True, "symbol": symbol, "count": total, "matches": matches, "engine": "regex"}


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
        explicit_root = payload.get("root")
        if explicit_root:
            root = _workspace_root(explicit_root)
        else:
            root = None
        file_path = Path(str(payload.get("file") or "")).expanduser().resolve()
        _validate_file_in_workspace(file_path, root)
        body = str(payload.get("body") or "pass")
        if HAS_LIBCST:
            return _replace_symbol_body_libcst(symbol, file_path, body)
        return _replace_symbol_body_ast(symbol, file_path, body)
    except Exception as error:
        return {"ok": False, "error": str(error), "symbol": payload.get("symbol")}


def _replace_symbol_body_libcst(symbol: str, file_path: Path, body: str) -> dict[str, Any]:
    source = file_path.read_text(encoding="utf-8")

    def _parse_body(code: str) -> list[cst.BaseStatement]:
        lines = code.splitlines()
        dedented_lines: list[str] = []
        min_indent: int | None = None
        for line in lines:
            if not line.strip():
                dedented_lines.append("")
                continue
            stripped = line.lstrip()
            indent = len(line) - len(stripped)
            if min_indent is None or indent < min_indent:
                min_indent = indent
            dedented_lines.append(stripped)
        if min_indent and min_indent > 0:
            dedented_lines = [
                line[min_indent:] if len(line) >= min_indent and line.strip() else line
                for line in lines
            ]
            dedented_lines = [
                line.lstrip() if line.strip() else "" for line in dedented_lines
            ]
        dedented = "\n".join(dedented_lines)
        return cst.parse_module(dedented).body  # type: ignore[no-any-return]

    class BodyReplacer(cst.CSTTransformer):  # type: ignore[misc]
        def leave_FunctionDef(self, original_node: cst.FunctionDef, updated_node: cst.FunctionDef) -> cst.FunctionDef:
            if original_node.name.value == symbol:
                try:
                    new_stmts = _parse_body(body)
                except Exception as e:
                    raise ValueError(f"Invalid Python code in new body: {e}")
                return updated_node.with_changes(body=cst.IndentedBlock(body=new_stmts))
            return updated_node

    tree = cst.parse_module(source)
    new_tree = tree.visit(BodyReplacer())
    if new_tree.code != source:
        updated = _preserve_trailing_newline(source, new_tree.code)
        file_path.write_text(updated, encoding="utf-8")
        return {"ok": True, "symbol": symbol, "file": str(file_path), "engine": "libcst"}
    raise ValueError("symbol_not_found_or_unchanged")


def _replace_symbol_body_ast(symbol: str, file_path: Path, body: str) -> dict[str, Any]:
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
    return {"ok": True, "symbol": symbol, "file": str(file_path), "engine": "ast"}


def insert_after_symbol(payload: dict[str, Any]) -> dict[str, Any]:
    try:
        symbol = str(payload.get("symbol") or "").strip()
        explicit_root = payload.get("root")
        if explicit_root:
            root = _workspace_root(explicit_root)
        else:
            root = None
        file_path = Path(str(payload.get("file") or "")).expanduser().resolve()
        _validate_file_in_workspace(file_path, root)
        text = str(payload.get("text") or "")
        original = file_path.read_text(encoding="utf-8")
        lines = original.splitlines()
        node = _find_named_node(file_path, symbol)
        insertion = text.splitlines() or [text]
        lines[getattr(node, "end_lineno", 0) : getattr(node, "end_lineno", 0)] = insertion
        updated = _preserve_trailing_newline(original, "\n".join(lines))
        file_path.write_text(updated, encoding="utf-8")
        return {"ok": True, "symbol": symbol, "file": str(file_path), "engine": "libcst" if HAS_LIBCST else "ast"}
    except Exception as error:
        return {"ok": False, "error": str(error), "symbol": payload.get("symbol")}


def get_project_overview(payload: dict[str, Any]) -> dict[str, Any]:
    root = _workspace_root(payload.get("root"))
    counts: dict[str, int] = {}
    file_count = 0
    for path in root.rglob("*"):
        if any(part in _PY_BLOCKED for part in path.parts):
            continue
        if not path.is_file():
            continue
        file_count += 1
        suffix = path.suffix or "[none]"
        counts[suffix] = counts.get(suffix, 0) + 1
    top_extensions = sorted(counts.items(), key=lambda item: (-item[1], item[0]))[:10]
    result: dict[str, Any] = {
        "ok": True,
        "root": str(root),
        "fileCount": file_count,
        "topExtensions": [
            {"extension": extension, "count": count}
            for extension, count in top_extensions
        ],
    }
    graph_summary = _graphify_summary_impl(str(root))
    if graph_summary.get("has_graph"):
        result["graphify"] = graph_summary
    return result


from .hybrid_memory import query_hybrid_memory as _query_hybrid_memory_impl  # noqa: E402
from .graphify_service import (  # noqa: E402
    graphify_query as _graphify_query_impl,
    graphify_update as _graphify_update_impl,
    graphify_explain as _graphify_explain_impl,
    graphify_path as _graphify_path_impl,
    graphify_summary as _graphify_summary_impl,
    graphify_available as _graphify_available_impl,
)


def query_hybrid_memory(payload: dict[str, Any]) -> dict[str, Any]:
    return _query_hybrid_memory_impl(payload)


def _graphify_query(payload: dict[str, Any]) -> dict[str, Any]:
    question = str(payload.get("query") or "").strip()
    root = str(payload.get("root") or _workspace_root(None))
    budget = int(payload.get("budget", 2000))
    return _graphify_query_impl(question, root, budget=budget)


def _graphify_update(payload: dict[str, Any]) -> dict[str, Any]:
    root = str(payload.get("root") or _workspace_root(None))
    return _graphify_update_impl(root)


def _graphify_explain(payload: dict[str, Any]) -> dict[str, Any]:
    node = str(payload.get("node") or "").strip()
    root = str(payload.get("root") or _workspace_root(None))
    return _graphify_explain_impl(node, root)


def _graphify_path(payload: dict[str, Any]) -> dict[str, Any]:
    source = str(payload.get("source") or "").strip()
    target = str(payload.get("target") or "").strip()
    root = str(payload.get("root") or _workspace_root(None))
    return _graphify_path_impl(source, target, root)


_SYNC_ACTIONS = frozenset({
    "sin_simone_mcp_symbol_search",
    "sin_simone_mcp_find_references",
    "sin_simone_mcp_structural_edit",
    "sin_simone_mcp_memory_query",
    "sin_simone_mcp_project_overview",
    "sin_simone_mcp_health",
    "sin_simone_mcp_insert_after",
    "sin_simone_mcp_graphify_query",
    "sin_simone_mcp_graphify_update",
    "sin_simone_mcp_graphify_explain",
    "sin_simone_mcp_graphify_path",
})


async def execute_simone_action(payload: dict[str, Any]) -> dict[str, Any]:
    try:
        action = str(payload.get("action") or "agent.help")
        if action in {"agent.help", "simone.mcp.help"}:
            return {
                "ok": True,
                "name": AGENT_NAME,
                "actions": [
                    "agent.help",
                    "sin_simone_mcp_health",
                    "sin_simone_mcp_symbol_search",
                    "sin_simone_mcp_structural_edit",
                    "sin_simone_mcp_memory_query",
                    "sin_simone_mcp_find_references",
                    "sin_simone_mcp_project_overview",
                    "sin_simone_mcp_graphify_query",
                    "sin_simone_mcp_graphify_update",
                    "sin_simone_mcp_graphify_explain",
                    "sin_simone_mcp_graphify_path",
                ],
            }
        if action in {"simone.mcp.health", "sin.simone.mcp.health", "sin_simone_mcp_health"}:
            return {
                "ok": True,
                "status": "ok",
                "name": AGENT_NAME,
                "version": AGENT_VERSION,
                "transport": "streamable-http+stdio",
                "memory": "hybrid",
            }
        if action in _SYNC_ACTIONS:
            return await asyncio.to_thread(_execute_sync_action, action, payload)
        return {"ok": False, "error": "unknown_action", "action": action}
    except Exception as error:
        return {"ok": False, "error": str(error), "action": payload.get("action")}


def _execute_sync_action(action: str, payload: dict[str, Any]) -> dict[str, Any]:
    if action == "sin_simone_mcp_symbol_search":
        return find_symbol(payload)
    if action == "sin_simone_mcp_find_references":
        return find_references(payload)
    if action == "sin_simone_mcp_structural_edit":
        return replace_symbol_body(payload)
    if action == "sin_simone_mcp_project_overview":
        return get_project_overview(payload)
    if action == "sin_simone_mcp_memory_query":
        return query_hybrid_memory(payload)
    if action == "sin_simone_mcp_insert_after":
        return insert_after_symbol(payload)
    if action == "sin_simone_mcp_graphify_query":
        return _graphify_query(payload)
    if action == "sin_simone_mcp_graphify_update":
        return _graphify_update(payload)
    if action == "sin_simone_mcp_graphify_explain":
        return _graphify_explain(payload)
    if action == "sin_simone_mcp_graphify_path":
        return _graphify_path(payload)
    return {"ok": False, "error": "unknown_action", "action": action}


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
