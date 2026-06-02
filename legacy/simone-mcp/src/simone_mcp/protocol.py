from __future__ import annotations

import asyncio
import base64
import logging
import threading
import time
import uuid
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Callable, Awaitable

from .core import TOOL_DEFINITIONS, execute_simone_action, json_dumps
from .schemas import TOOL_ARG_MODELS

logger = logging.getLogger(__name__)

PROTOCOL_VERSION = "2026-06-30"
SUPPORTED_VERSIONS = ["2026-06-30", "2025-11-25", "2025-03-26"]
SSE_RETRY_MS = 5000
_TASK_MAX_AGE_MS = 3600000
_TASK_CLEANUP_EVERY = 64
_TASK_DEFAULT_POLL_MS = 5000
_TASK_MAX_CONCURRENT = 100

_JSON_SCHEMA_2020_12 = "https://json-schema.org/draft/2020-12/schema"

_TOOL_ARG_ALIASES: dict[str, dict[str, str]] = {
    "sin_simone_mcp_symbol_search": {"query": "symbol"},
    "sin_simone_mcp_find_references": {},
    "sin_simone_mcp_structural_edit": {"editPayload": "edit_payload"},
    "sin_simone_mcp_memory_query": {"query": "query"},
    "sin_simone_mcp_project_overview": {},
    "sin_simone_mcp_health": {},
    "sin_simone_mcp_graphify_query": {},
    "sin_simone_mcp_graphify_update": {},
    "sin_simone_mcp_graphify_explain": {},
    "sin_simone_mcp_graphify_path": {},
    "sin_simone_mcp_write_file": {},
    "sin_simone_mcp_edit_file": {},
    "sin_simone_mcp_patch_file": {},
    "sin_simone_mcp_read_file": {},
}

SIMONE_INSTRUCTIONS = (
    "Simone MCP provides LSP-grade code intelligence. "
    "Use symbol_search to find definitions, find_references for usage sites, "
    "structural_edit for safe refactoring, and memory_query for semantic recall. "
    "graphify_query answers questions about a codebase using its knowledge graph — "
    "run graphify_update first to build the graph, then query it. "
    "graphify_explain explains a node's neighbors; graphify_path finds shortest path. "
    "Resources expose project files; prompts offer guided workflows. "
    "Tasks enable long-running operations — poll tasks/get for inline results. "
    "Always specify a 'root' path when targeting a specific project."
)

PROMPT_DEFINITIONS: list[dict[str, Any]] = [
    {
        "name": "code_review",
        "title": "Code Review",
        "description": "Structured code review prompt — analyzes quality, security, and performance.",
        "arguments": [
            {"name": "code", "description": "The code to review", "required": True},
            {"name": "language", "description": "Programming language (e.g. python, typescript)", "required": False},
            {"name": "focus", "description": "Focus area: security, performance, readability, all", "required": False},
        ],
        "annotations": {"audience": ["assistant"], "priority": 0.8},
    },
    {
        "name": "debug_assistant",
        "title": "Debug Assistant",
        "description": "Guided debugging workflow — step-by-step error diagnosis and fix suggestions.",
        "arguments": [
            {"name": "error", "description": "Error message or stack trace", "required": True},
            {"name": "code", "description": "Relevant code snippet", "required": False},
            {"name": "language", "description": "Programming language", "required": False},
        ],
        "annotations": {"audience": ["assistant"], "priority": 0.7},
    },
    {
        "name": "refactor_plan",
        "title": "Refactor Plan",
        "description": "Generate a refactoring plan with step-by-step instructions.",
        "arguments": [
            {"name": "description", "description": "What to refactor", "required": True},
            {"name": "files", "description": "Comma-separated list of files affected", "required": False},
            {"name": "priority", "description": "Priority: low, medium, high", "required": False},
        ],
        "annotations": {"audience": ["assistant"], "priority": 0.6},
    },
    {
        "name": "test_generator",
        "title": "Test Generator",
        "description": "Generate test cases for a function or module.",
        "arguments": [
            {"name": "code", "description": "The code to test", "required": True},
            {"name": "framework", "description": "Test framework: pytest, unittest, jest, vitest", "required": False},
            {"name": "style", "description": "Style: unit, integration, property", "required": False},
        ],
        "annotations": {"audience": ["assistant"], "priority": 0.7},
    },
]

RESOURCE_TEMPLATES: list[dict[str, Any]] = [
    {
        "uriTemplate": "file:///{path}",
        "name": "Project Files",
        "title": "Project Files",
        "description": "Access files in the project directory.",
        "mimeType": "application/octet-stream",
        "annotations": {"audience": ["assistant", "user"], "priority": 0.5},
    },
    {
        "uriTemplate": "source://{root}/{relpath}",
        "name": "Source Code",
        "title": "Source Code",
        "description": "Read source code files with line metadata.",
        "mimeType": "text/plain",
        "annotations": {"audience": ["assistant"], "priority": 0.8},
    },
]

_LIST_TTL_MS = 300000
_LIST_CACHE_SCOPE = "session"
PAGE_SIZE = 50

_log_level: str = "info"
_log_level_lock = threading.Lock()

_subscriptions: set[str] = set()
_subscriptions_lock = threading.Lock()

_tool_versions: int = 0
_tool_versions_lock = threading.Lock()

_session_store: dict[str, dict[str, Any]] = {}
_session_store_lock = threading.Lock()

_sse_event_log: dict[str, list[dict[str, Any]]] = {}
_sse_event_log_lock = threading.Lock()

_task_store: dict[str, dict[str, Any]] = {}
_task_store_lock = threading.Lock()
_task_op_count = 0

_completion_registry: dict[str, Callable[..., Awaitable[list[str]]]] = {}


def _now_iso() -> str:
    return datetime.now(timezone.utc).isoformat()


def _paginate(items: list[Any], cursor: str | None) -> tuple[list[Any], str | None]:
    start = 0
    if cursor:
        try:
            start = int(base64.b64decode(cursor).decode())
        except Exception:
            start = 0
    page = items[start:start + PAGE_SIZE]
    next_start = start + PAGE_SIZE
    next_cursor = None
    if next_start < len(items):
        next_cursor = base64.b64encode(str(next_start).encode()).decode()
    return page, next_cursor


def _get_log_level() -> str:
    with _log_level_lock:
        return _log_level


def _set_log_level(level: str) -> None:
    global _log_level
    with _log_level_lock:
        _log_level = level


def _register_session(session_id: str) -> None:
    with _session_store_lock:
        if session_id not in _session_store:
            _session_store[session_id] = {"id": session_id, "created": _now_iso()}


def _remove_session(session_id: str) -> None:
    with _session_store_lock:
        _session_store.pop(session_id, None)
    with _task_store_lock:
        stale = [tid for tid, t in _task_store.items() if t.get("sessionId") == session_id]
        for tid in stale:
            del _task_store[tid]


def _subscribe_resource(uri: str) -> None:
    with _subscriptions_lock:
        _subscriptions.add(uri)


def _unsubscribe_resource(uri: str) -> None:
    with _subscriptions_lock:
        _subscriptions.discard(uri)


def _get_subscriptions() -> set[str]:
    with _subscriptions_lock:
        return set(_subscriptions)


def _bump_tool_version() -> int:
    global _tool_versions
    with _tool_versions_lock:
        _tool_versions += 1
        return _tool_versions


def _log_event(session_id: str, event_id: str, data: dict[str, Any]) -> None:
    with _sse_event_log_lock:
        if session_id not in _sse_event_log:
            _sse_event_log[session_id] = []
        _sse_event_log[session_id].append({"id": event_id, "data": data})


def _get_events_after(session_id: str, last_event_id: str) -> list[dict[str, Any]]:
    with _sse_event_log_lock:
        events = _sse_event_log.get(session_id, [])
        for i, e in enumerate(events):
            if e["id"] == last_event_id:
                return events[i + 1:]
        return events


def _tool_task_support(name: str) -> str:
    for tool in TOOL_DEFINITIONS:
        if tool["name"] == name:
            execution = tool.get("execution")
            if execution and isinstance(execution, dict):
                val = execution.get("taskSupport", "forbidden")
                return str(val)
    return "forbidden"


def _build_task_obj(task: dict[str, Any]) -> dict[str, Any]:
    obj: dict[str, Any] = {
        "taskId": task["id"],
        "status": task["status"],
        "statusMessage": task.get("statusMessage"),
        "createdAt": task["createdAt"],
        "lastUpdatedAt": task["lastUpdatedAt"],
        "ttl": task.get("ttl"),
        "pollInterval": task.get("pollInterval", _TASK_DEFAULT_POLL_MS),
    }
    if task["status"] in ("completed", "failed", "cancelled"):
        if task.get("result") is not None:
            obj["result"] = task["result"]
        if task.get("error") is not None:
            obj["error"] = task["error"]
    return obj


def _create_task(name: str, arguments: dict[str, Any], session_id: str, ttl_ms: int | None = None) -> dict[str, Any]:
    global _task_op_count
    task_id = str(uuid.uuid4())
    now = _now_iso()
    now_mono = time.monotonic()
    effective_ttl = ttl_ms if ttl_ms is not None else _TASK_MAX_AGE_MS
    with _task_store_lock:
        active = sum(1 for t in _task_store.values() if t["status"] == "working" and t.get("sessionId") == session_id)
        if active >= _TASK_MAX_CONCURRENT:
            raise ValueError(f"Max concurrent tasks ({_TASK_MAX_CONCURRENT}) reached for session")
        _task_store[task_id] = {
            "id": task_id,
            "status": "working",
            "statusMessage": "The operation is now in progress.",
            "result": None,
            "error": None,
            "toolName": name,
            "arguments": arguments,
            "sessionId": session_id,
            "createdAt": now,
            "lastUpdatedAt": now,
            "ttl": effective_ttl,
            "pollInterval": _TASK_DEFAULT_POLL_MS,
            "_createdMono": now_mono,
        }
        _task_op_count += 1
        if _task_op_count % _TASK_CLEANUP_EVERY == 0:
            _cleanup_tasks()
        return dict(_task_store[task_id])


def _cleanup_tasks() -> None:
    now = time.monotonic()
    stale = [
        tid for tid, t in _task_store.items()
        if (now - t.get("_createdMono", 0)) > (t.get("ttl", _TASK_MAX_AGE_MS) / 1000.0)
        and t["status"] in ("completed", "failed", "cancelled")
    ]
    for tid in stale:
        del _task_store[tid]


def _update_task(task_id: str, status: str, result: Any = None, error: str | None = None, statusMessage: str | None = None) -> dict[str, Any] | None:
    with _task_store_lock:
        task = _task_store.get(task_id)
        if task is None:
            return None
        task["status"] = status
        if result is not None:
            task["result"] = result
        if error is not None:
            task["error"] = error
        if statusMessage is not None:
            task["statusMessage"] = statusMessage
        elif status == "completed":
            task["statusMessage"] = "The operation completed successfully."
        elif status == "failed":
            task["statusMessage"] = f"Tool execution failed: {error}"
        task["lastUpdatedAt"] = _now_iso()
        return dict(task)


def _get_task(task_id: str) -> dict[str, Any] | None:
    with _task_store_lock:
        task = _task_store.get(task_id)
        return dict(task) if task else None


def _cancel_task(task_id: str) -> dict[str, Any] | None:
    with _task_store_lock:
        task = _task_store.get(task_id)
        if task is None:
            return None
        if task["status"] in ("completed", "failed", "cancelled"):
            return None
        task["status"] = "cancelled"
        task["statusMessage"] = "The task was cancelled by request."
        task["lastUpdatedAt"] = _now_iso()
        return dict(task)


async def _run_task(task_id: str, action: dict[str, Any], send_notification: Callable[[dict[str, Any]], Awaitable[None]] | None = None) -> None:
    _update_task(task_id, "working")
    if send_notification:
        try:
            task = _get_task(task_id)
            if task:
                await send_notification({
                    "jsonrpc": "2.0",
                    "method": "notifications/tasks",
                    "params": _build_task_obj(task),
                })
        except Exception:
            pass
    try:
        result_data = await execute_simone_action(action)
        is_error = not result_data.get("ok", False)
        if is_error:
            _update_task(task_id, "failed", result=result_data, error=result_data.get("error", "unknown"))
        else:
            _update_task(task_id, "completed", result=result_data)
    except Exception as e:
        _update_task(task_id, "failed", error=str(e))
    if send_notification:
        try:
            task = _get_task(task_id)
            if task:
                await send_notification({
                    "jsonrpc": "2.0",
                    "method": "notifications/tasks",
                    "params": _build_task_obj(task),
                })
        except Exception:
            pass


def _negotiate_version(client_version: str | None) -> str:
    if not client_version:
        return PROTOCOL_VERSION
    if client_version in SUPPORTED_VERSIONS:
        return client_version
    sorted_versions = sorted(SUPPORTED_VERSIONS, reverse=True)
    for v in sorted_versions:
        if v <= client_version:
            return v
    return PROTOCOL_VERSION


def _extract_request_meta(params: dict[str, Any]) -> dict[str, Any]:
    meta = params.get("_meta")
    return meta if isinstance(meta, dict) else {}


def _inject_meta(result: dict[str, Any], request_meta: dict[str, Any]) -> dict[str, Any]:
    if request_meta:
        result["_meta"] = dict(request_meta)
    return result


def _list_resources(root: str | None = None) -> list[dict[str, Any]]:
    ws = Path(root) if root else Path.cwd()
    resources: list[dict[str, Any]] = []
    for p in ws.rglob("*"):
        if p.is_file() and not p.name.startswith(".") and ".git" not in p.parts:
            try:
                rel = p.relative_to(ws)
                resources.append({
                    "uri": f"file:///{rel}",
                    "name": rel.name,
                    "description": str(rel),
                    "mimeType": _guess_mime(p),
                    "size": p.stat().st_size,
                    "annotations": {"audience": ["assistant", "user"], "priority": 0.5, "lastModified": p.stat().st_mtime},
                })
            except (ValueError, OSError):
                continue
    return resources[:500]


def _read_resource(uri: str, root: str | None = None) -> dict[str, Any] | None:
    ws = Path(root) if root else Path.cwd()
    if uri.startswith("file:///"):
        rel = uri[len("file:///"):]
        target = ws / rel
        try:
            resolved = target.resolve()
            if root and not str(resolved).startswith(str(ws.resolve())):
                return None
            if not resolved.is_file():
                return None
            text = resolved.read_text(errors="replace")
            return {"uri": uri, "mimeType": _guess_mime(resolved), "text": text}
        except OSError:
            return None
    if uri.startswith("source://"):
        rest = uri[len("source://"):]
        parts = rest.split("/", 1)
        if len(parts) != 2:
            return None
        base, relpath = parts
        target = Path(base) / relpath
        try:
            resolved = target.resolve()
            if root and not str(resolved).startswith(str(ws.resolve())):
                return None
            if not resolved.is_file():
                return None
            text = resolved.read_text(errors="replace")
            return {"uri": uri, "mimeType": _guess_mime(resolved), "text": text}
        except OSError:
            return None
    return None


def _guess_mime(p: Path) -> str:
    ext = p.suffix.lower()
    mime_map = {
        ".py": "text/x-python", ".js": "text/javascript", ".ts": "text/typescript",
        ".tsx": "text/typescript", ".jsx": "text/javascript", ".json": "application/json",
        ".yaml": "text/yaml", ".yml": "text/yaml", ".toml": "text/toml",
        ".md": "text/markdown", ".html": "text/html", ".css": "text/css",
        ".rs": "text/x-rust", ".go": "text/x-go", ".java": "text/x-java",
        ".c": "text/x-c", ".cpp": "text/x-c++", ".h": "text/x-c",
    }
    return mime_map.get(ext, "application/octet-stream")


def _generate_prompt(name: str, arguments: dict[str, str]) -> dict[str, Any]:
    prompts_map: dict[str, Callable[..., dict[str, Any]]] = {
        "code_review": _prompt_code_review,
        "debug_assistant": _prompt_debug_assistant,
        "refactor_plan": _prompt_refactor_plan,
        "test_generator": _prompt_test_generator,
    }
    fn = prompts_map.get(name)
    if fn:
        return fn(arguments)
    return {
        "description": f"Unknown prompt: {name}",
        "messages": [{"role": "user", "content": {"type": "text", "text": f"Prompt '{name}' not found."}}],
    }


def _prompt_code_review(args: dict[str, str]) -> dict[str, Any]:
    code = args.get("code", "")
    language = args.get("language", "unknown")
    focus = args.get("focus", "all")
    return {
        "description": "Code review prompt",
        "messages": [
            {
                "role": "user",
                "content": {
                    "type": "text",
                    "text": (
                        f"Please perform a {focus} code review on the following {language} code.\n\n"
                        f"```\n{code}\n```\n\n"
                        "Analyze for: correctness, security vulnerabilities, performance issues, "
                        "code style, and suggest improvements with specific refactorings."
                    ),
                },
            }
        ],
    }


def _prompt_debug_assistant(args: dict[str, str]) -> dict[str, Any]:
    error = args.get("error", "")
    code = args.get("code", "")
    language = args.get("language", "unknown")
    code_section = f"\nRelevant code:\n```\n{code}\n```" if code else ""
    return {
        "description": "Debug assistant prompt",
        "messages": [
            {
                "role": "user",
                "content": {
                    "type": "text",
                    "text": (
                        f"I'm encountering the following error in {language}:\n\n"
                        f"```\n{error}\n```\n"
                        f"{code_section}\n\n"
                        "Help me debug this step by step: 1) Identify the root cause, "
                        "2) Explain why it's happening, 3) Suggest a fix, 4) Suggest how to prevent it."
                    ),
                },
            }
        ],
    }


def _prompt_refactor_plan(args: dict[str, str]) -> dict[str, Any]:
    description = args.get("description", "")
    files = args.get("files", "")
    priority = args.get("priority", "medium")
    files_section = f"\nAffected files: {files}" if files else ""
    return {
        "description": "Refactoring plan prompt",
        "messages": [
            {
                "role": "user",
                "content": {
                    "type": "text",
                    "text": (
                        f"Create a detailed refactoring plan (priority: {priority}) for:\n\n"
                        f"{description}\n"
                        f"{files_section}\n\n"
                        "Provide: 1) Current architecture analysis, 2) Target architecture, "
                        "3) Step-by-step migration plan, 4) Risk assessment, 5) Testing strategy."
                    ),
                },
            }
        ],
    }


def _prompt_test_generator(args: dict[str, str]) -> dict[str, Any]:
    code = args.get("code", "")
    framework = args.get("framework", "pytest")
    style = args.get("style", "unit")
    return {
        "description": "Test generator prompt",
        "messages": [
            {
                "role": "user",
                "content": {
                    "type": "text",
                    "text": (
                        f"Generate {style} tests using {framework} for the following code:\n\n"
                        f"```\n{code}\n```\n\n"
                        "Cover: 1) Happy path, 2) Edge cases, 3) Error cases, "
                        "4) Boundary values. Use descriptive test names and assertions."
                    ),
                },
            }
        ],
    }


async def _complete_arguments(ref: dict[str, Any], argument: dict[str, str], context_arguments: dict[str, str] | None = None) -> list[str]:
    ref_type = ref.get("type", "")
    arg_name = argument.get("name", "")
    arg_value = argument.get("value", "")

    completions: list[str] = []
    if ref_type == "ref/prompt":
        if arg_name == "language":
            completions = [lang for lang in ["python", "typescript", "javascript", "rust", "go", "java"] if lang.startswith(arg_value)]
        elif arg_name == "framework":
            completions = [f for f in ["pytest", "unittest", "jest", "vitest", "mocha"] if f.startswith(arg_value)]
        elif arg_name == "focus":
            completions = [f for f in ["security", "performance", "readability", "all"] if f.startswith(arg_value)]
        elif arg_name == "priority":
            completions = [p for p in ["low", "medium", "high"] if p.startswith(arg_value)]
        elif arg_name == "style":
            completions = [s for s in ["unit", "integration", "property"] if s.startswith(arg_value)]
    elif ref_type == "ref/resource":
        root = context_arguments.get("root", "") if context_arguments else ""
        if arg_name == "path" or arg_name == "relpath":
            ws = Path(root) if root else Path.cwd()
            try:
                matches = [str(p.relative_to(ws)) for p in ws.rglob(f"{arg_value}*") if p.is_file()]
                completions = matches[:20]
            except (ValueError, OSError):
                pass

    return completions


def _build_resource_links(result_data: dict[str, Any]) -> list[dict[str, Any]]:
    links = []
    matches = result_data.get("matches", [])
    for match in matches:
        if isinstance(match, dict):
            file_path = match.get("file", "")
            if file_path:
                p = Path(file_path)
                links.append({
                    "type": "resource_link",
                    "uri": f"file:///{file_path}",
                    "name": p.name,
                    "mimeType": _guess_mime(p),
                })
    return links[:10]


def _build_initialize_result(session_id: str, client_version: str | None = None) -> dict[str, Any]:
    _register_session(session_id)
    negotiated = _negotiate_version(client_version)
    return {
        "protocolVersion": negotiated,
        "capabilities": {
            "tools": {"listChanged": True},
            "resources": {"subscribe": True, "listChanged": True},
            "prompts": {"listChanged": True},
            "logging": {},
            "completions": {},
            "tasks": {
                "update": {},
                "cancel": {},
            },
            "extensions": [
                {
                    "uri": "io.modelcontextprotocol/tasks",
                    "description": "Tasks Extension v2 — long-running operations with inline results (SEP-2663)",
                },
            ],
        },
        "serverInfo": {
            "name": "simone-mcp",
            "version": "2026.06.30",
            "description": "Production-grade MCP code worker with LSP-grade symbol operations, hybrid memory, and streamable HTTP transport.",
        },
        "instructions": SIMONE_INSTRUCTIONS,
        "sessionId": session_id,
    }


async def handle_mcp_request(
    payload: dict[str, Any],
    session_id: str | None,
    send_notification: Callable[[dict[str, Any]], Awaitable[None]] | None = None,
    client_protocol_version: str | None = None,
) -> tuple[dict[str, Any] | None, str | None, list[dict[str, Any]]]:
    notifications: list[dict[str, Any]] = []
    try:
        from .schemas import JsonRpcRequest
        rpc = JsonRpcRequest(**payload)
    except Exception as e:
        return {
            "jsonrpc": "2.0",
            "id": payload.get("id"),
            "error": {"code": -32600, "message": f"Invalid request: {e}"},
        }, session_id, notifications

    request_id = rpc.id
    method = rpc.method
    params = rpc.params if isinstance(rpc.params, dict) else {}
    request_meta = _extract_request_meta(params)

    if method == "initialize":
        session_id = session_id or str(uuid.uuid4())
        client_ver = client_protocol_version or params.get("protocolVersion")
        result = _build_initialize_result(session_id, client_ver)
        _inject_meta(result, request_meta)
        return {"jsonrpc": "2.0", "id": request_id, "result": result}, session_id, notifications

    if method in {"initialized", "notifications/initialized"}:
        return None, session_id, notifications

    if method == "ping":
        ping_result: dict[str, Any] = {}
        _inject_meta(ping_result, request_meta)
        return {"jsonrpc": "2.0", "id": request_id, "result": ping_result}, session_id, notifications

    if method == "tools/list":
        cursor = params.get("cursor")
        page, next_cursor = _paginate(TOOL_DEFINITIONS, cursor)
        tools_result: dict[str, Any] = {"tools": page, "ttlMs": _LIST_TTL_MS, "cacheScope": _LIST_CACHE_SCOPE}
        if next_cursor:
            tools_result["nextCursor"] = next_cursor
        _inject_meta(tools_result, request_meta)
        return {"jsonrpc": "2.0", "id": request_id, "result": tools_result}, session_id, notifications

    if method == "tools/call":
        name = params.get("name", "")
        arguments = dict(params.get("arguments") or {})
        progress_token = request_meta.get("progressToken")

        arg_model = TOOL_ARG_MODELS.get(name)
        if arg_model:
            try:
                validated = arg_model(**arguments)
                arguments = validated.model_dump(by_alias=True)
            except Exception as e:
                return {
                    "jsonrpc": "2.0",
                    "id": request_id,
                    "result": {
                        "content": [{"type": "text", "text": f"Invalid arguments: {e}"}],
                        "isError": True,
                        "structuredContent": {"ok": False, "error": f"Invalid arguments: {e}"},
                    },
                }, session_id, notifications

        aliases = _TOOL_ARG_ALIASES.get(name, {})
        remapped: dict[str, Any] = {}
        for k, v in arguments.items():
            new_key = aliases.get(k, k) if isinstance(k, str) else k
            remapped[new_key] = v
        action = dict(remapped)
        action["action"] = name

        task_support = _tool_task_support(name)
        should_defer = task_support != "forbidden"

        if should_defer:
            try:
                task = _create_task(name, arguments, session_id or "")
            except ValueError as e:
                return {
                    "jsonrpc": "2.0",
                    "id": request_id,
                    "error": {"code": -32603, "message": str(e)},
                }, session_id, notifications

            asyncio.create_task(_run_task(task["id"], action, send_notification))
            create_result = {
                "resultType": "task",
                "task": _build_task_obj(task),
                "_meta": {
                    "io.modelcontextprotocol/tasks": f"Task {task['id']} created for {name}. Poll with tasks/get — result is inline.",
                },
            }
            return {"jsonrpc": "2.0", "id": request_id, "result": create_result}, session_id, notifications

        if progress_token and send_notification:
            notifications.append({
                "jsonrpc": "2.0",
                "method": "notifications/progress",
                "params": {"progressToken": progress_token, "progress": 0.0, "total": 1.0, "message": f"Starting {name}"},
            })

        try:
            result_data = await execute_simone_action(action)
        except Exception as e:
            return {
                "jsonrpc": "2.0",
                "id": request_id,
                "result": {
                    "content": [{"type": "text", "text": f"Execution error: {e}"}],
                    "isError": True,
                    "structuredContent": {"ok": False, "error": str(e)},
                    "_meta": request_meta if request_meta else {},
                },
            }, session_id, notifications

        if progress_token and send_notification:
            notifications.append({
                "jsonrpc": "2.0",
                "method": "notifications/progress",
                "params": {"progressToken": progress_token, "progress": 1.0, "total": 1.0, "message": f"Completed {name}"},
            })

        content_items: list[dict[str, Any]] = [{"type": "text", "text": json_dumps(result_data)}]
        resource_links = _build_resource_links(result_data)
        content_items.extend(resource_links)

        tool_result: dict[str, Any] = {
            "content": content_items,
            "isError": not result_data.get("ok", False),
            "structuredContent": result_data,
            "_meta": request_meta if request_meta else {},
        }
        return {"jsonrpc": "2.0", "id": request_id, "result": tool_result}, session_id, notifications

    if method == "tasks/get":
        task_id = params.get("taskId") or params.get("id", "")
        task_obj: dict[str, Any] | None = _get_task(task_id)
        if task_obj is None:
            return {
                "jsonrpc": "2.0",
                "id": request_id,
                "error": {"code": -32602, "message": "Failed to retrieve task: Task not found"},
            }, session_id, notifications
        task_result = _build_task_obj(task_obj)
        _inject_meta(task_result, request_meta)
        return {"jsonrpc": "2.0", "id": request_id, "result": task_result}, session_id, notifications

    if method == "tasks/update":
        task_id = params.get("taskId") or params.get("id", "")
        update_task: dict[str, Any] | None = _get_task(task_id)
        if update_task is None:
            return {
                "jsonrpc": "2.0",
                "id": request_id,
                "error": {"code": -32602, "message": "Failed to retrieve task: Task not found"},
            }, session_id, notifications
        if update_task["status"] != "input_required":
            return {
                "jsonrpc": "2.0",
                "id": request_id,
                "error": {"code": -32602, "message": f"Task is not awaiting input (status: {update_task['status']})"},
            }, session_id, notifications
        update_input = params.get("input", {})
        _update_task(task_id, "working", statusMessage="Resuming with provided input.")
        if send_notification:
            try:
                updated_task = _get_task(task_id)
                if updated_task:
                    await send_notification({
                        "jsonrpc": "2.0",
                        "method": "notifications/tasks",
                        "params": _build_task_obj(updated_task),
                    })
            except Exception:
                pass
        asyncio.create_task(_run_task(task_id, {"action": update_task.get("toolName", ""), **update_task.get("arguments", {}), "_update_input": update_input}, send_notification))
        update_result = _build_task_obj(_get_task(task_id) or {})
        _inject_meta(update_result, request_meta)
        return {"jsonrpc": "2.0", "id": request_id, "result": update_result}, session_id, notifications

    if method == "tasks/cancel":
        task_id = params.get("taskId") or params.get("id", "")
        existing = _get_task(task_id)
        if existing is None:
            return {
                "jsonrpc": "2.0",
                "id": request_id,
                "error": {"code": -32602, "message": "Failed to retrieve task: Task not found"},
            }, session_id, notifications
        if existing["status"] in ("completed", "failed", "cancelled"):
            return {
                "jsonrpc": "2.0",
                "id": request_id,
                "error": {"code": -32602, "message": f"Cannot cancel task: already in terminal status '{existing['status']}'"},
            }, session_id, notifications
        cancelled = _cancel_task(task_id)
        cancel_result = _build_task_obj(cancelled or {})
        _inject_meta(cancel_result, request_meta)
        return {"jsonrpc": "2.0", "id": request_id, "result": cancel_result}, session_id, notifications

    if method == "resources/list":
        cursor = params.get("cursor")
        root = params.get("root")
        resources = _list_resources(root)
        page, next_cursor = _paginate(resources, cursor)
        result = {"resources": page, "ttlMs": _LIST_TTL_MS, "cacheScope": _LIST_CACHE_SCOPE}
        if next_cursor:
            result["nextCursor"] = next_cursor
        _inject_meta(result, request_meta)
        return {"jsonrpc": "2.0", "id": request_id, "result": result}, session_id, notifications

    if method == "resources/templates/list":
        cursor = params.get("cursor")
        page, next_cursor = _paginate(RESOURCE_TEMPLATES, cursor)
        result = {"resourceTemplates": page, "ttlMs": _LIST_TTL_MS, "cacheScope": _LIST_CACHE_SCOPE}
        if next_cursor:
            result["nextCursor"] = next_cursor
        _inject_meta(result, request_meta)
        return {"jsonrpc": "2.0", "id": request_id, "result": result}, session_id, notifications

    if method == "resources/read":
        uri = params.get("uri", "")
        root = params.get("root")
        content = _read_resource(uri, root)
        if content is None:
            return {
                "jsonrpc": "2.0",
                "id": request_id,
                "error": {"code": -32002, "message": "Resource not found", "data": {"uri": uri}},
            }, session_id, notifications
        result = {"contents": [content]}
        _inject_meta(result, request_meta)
        return {"jsonrpc": "2.0", "id": request_id, "result": result}, session_id, notifications

    if method == "resources/subscribe":
        uri = params.get("uri", "")
        _subscribe_resource(uri)
        result = {}
        _inject_meta(result, request_meta)
        return {"jsonrpc": "2.0", "id": request_id, "result": result}, session_id, notifications

    if method == "resources/unsubscribe":
        uri = params.get("uri", "")
        _unsubscribe_resource(uri)
        result = {}
        _inject_meta(result, request_meta)
        return {"jsonrpc": "2.0", "id": request_id, "result": result}, session_id, notifications

    if method == "prompts/list":
        cursor = params.get("cursor")
        page, next_cursor = _paginate(PROMPT_DEFINITIONS, cursor)
        result = {"prompts": page, "ttlMs": _LIST_TTL_MS, "cacheScope": _LIST_CACHE_SCOPE}
        if next_cursor:
            result["nextCursor"] = next_cursor
        _inject_meta(result, request_meta)
        return {"jsonrpc": "2.0", "id": request_id, "result": result}, session_id, notifications

    if method == "prompts/get":
        name = params.get("name", "")
        arguments = params.get("arguments") or {}
        prompt_def = next((p for p in PROMPT_DEFINITIONS if p["name"] == name), None)
        if prompt_def is None:
            return {
                "jsonrpc": "2.0",
                "id": request_id,
                "error": {"code": -32602, "message": f"Unknown prompt: {name}"},
            }, session_id, notifications
        required_args = [a["name"] for a in prompt_def.get("arguments", []) if a.get("required")]
        for req in required_args:
            if req not in arguments:
                return {
                    "jsonrpc": "2.0",
                    "id": request_id,
                    "error": {"code": -32602, "message": f"Missing required argument: {req}"},
                }, session_id, notifications
        prompt_result = _generate_prompt(name, arguments)
        _inject_meta(prompt_result, request_meta)
        return {"jsonrpc": "2.0", "id": request_id, "result": prompt_result}, session_id, notifications

    if method == "logging/setLevel":
        level = params.get("level", "info")
        valid_levels = {"debug", "info", "notice", "warning", "error", "critical", "alert", "emergency"}
        if level not in valid_levels:
            return {
                "jsonrpc": "2.0",
                "id": request_id,
                "error": {"code": -32602, "message": f"Invalid log level: {level}. Valid: {valid_levels}"},
            }, session_id, notifications
        _set_log_level(level)
        logging.getLogger("simone_mcp").setLevel(getattr(logging, level.upper(), logging.INFO))
        result = {}
        _inject_meta(result, request_meta)
        if send_notification:
            notifications.append({
                "jsonrpc": "2.0",
                "method": "notifications/message",
                "params": {"level": level, "data": f"Log level set to {level}", "logger": "simone_mcp"},
            })
        return {"jsonrpc": "2.0", "id": request_id, "result": result}, session_id, notifications

    if method == "completion/complete":
        ref = params.get("ref", {})
        argument = params.get("argument", {})
        context_arguments = params.get("contextArguments")
        completions = await _complete_arguments(ref, argument, context_arguments)
        result = {"completion": {"values": completions, "total": len(completions), "hasMore": False}}
        _inject_meta(result, request_meta)
        return {"jsonrpc": "2.0", "id": request_id, "result": result}, session_id, notifications

    if method == "sampling/createMessage":
        return {
            "jsonrpc": "2.0",
            "id": request_id,
            "error": {"code": -1, "message": "Sampling not supported in stdio mode — use HTTP transport with client-side sampling."},
        }, session_id, notifications

    if method == "elicitation/create":
        params.get("message", "")
        schema = params.get("schema", {})
        enum_values = schema.get("enum")
        detail = f" Allowed values: {enum_values}" if enum_values and isinstance(enum_values, list) else ""
        return {
            "jsonrpc": "2.0",
            "id": request_id,
            "error": {"code": -1, "message": f"Elicitation not supported — rephrase as a tool call.{detail}"},
        }, session_id, notifications

    return {
        "jsonrpc": "2.0",
        "id": request_id,
        "error": {"code": -32601, "message": "Method not found"},
    }, session_id, notifications
