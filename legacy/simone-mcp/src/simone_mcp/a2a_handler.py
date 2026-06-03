"""A2A (Agent-to-Agent) JSON-RPC request handler.

Implements the A2A v1 method surface:
  - `agent.discover` — return the agent card for discovery
  - `agent.ping`     — liveness probe with timestamp
  - `tool.list`      — return the MCP tool definitions
  - `tool.call`      — dispatch a tool call and return the result
  - `message/send`   — accept a text message and treat it as an action
  - `tasks/get`      — return a stub completed task

Docs: a2a_handler.doc.md
"""
from __future__ import annotations

import json
import logging
import uuid
from datetime import datetime
from typing import Any

from .core import TOOL_DEFINITIONS, build_agent_card, execute_simone_action
from .correlation import correlation_manager
from .schemas import JsonRpcRequest, MessageSendParams, ToolCallParams

logger = logging.getLogger(__name__)


# ── Public entry point ─────────────────────────────────────────────────
async def handle_a2a_request(
    payload: dict[str, Any], base_url: str
) -> dict[str, Any]:
    """Dispatch an A2A JSON-RPC request to the right handler.

    Args:
        payload: Parsed JSON-RPC body (already deserialized).
        base_url: The server's public base URL — used to build the
                  `url` field in the agent card.

    Returns:
        A JSON-RPC 2.0 response dict (result or error). Always
        JSON-serializable; never raises.
    """
    try:
        rpc = JsonRpcRequest(**payload)
    except Exception as e:
        return _error_response(
            payload.get("id"), -32600, f"Invalid request: {e}"
        )

    request_id = rpc.id
    method = rpc.method
    # `params` can be a list per JSON-RPC, but A2A uses object form.
    params = rpc.params if isinstance(rpc.params, dict) else {}

    try:
        if method == "agent.discover":
            return _ok_response(request_id, build_agent_card(base_url))

        if method == "agent.ping":
            return _ok_response(
                request_id, {"status": "alive", "timestamp": datetime.now().isoformat()}
            )

        if method == "tool.list":
            return _ok_response(request_id, {"tools": TOOL_DEFINITIONS})

        if method == "tool.call":
            return await _handle_tool_call(request_id, params)

        if method == "message/send":
            return await _handle_message_send(request_id, params)

        if method == "tasks/get":
            task_id = params.get("id")
            if not task_id:
                return _error_response(request_id, -32602, "Missing task id")
            # A2A's tasks/get stub: the heavy lifting is in the MCP
            # transport; for the A2A view we just say "completed".
            return _ok_response(
                request_id,
                {
                    "id": task_id,
                    "kind": "task",
                    "status": {"state": "completed"},
                },
            )

        return _error_response(request_id, -32601, f"Method not found: {method}")

    except Exception as e:
        # Last-resort guard: NEVER raise out of an HTTP handler.
        # Log with traceback for debuggability, return JSON-RPC error.
        logger.exception("A2A handler error for method=%s", method)
        return _error_response(request_id, -32603, str(e))


# ── Tool dispatch ──────────────────────────────────────────────────────
async def _handle_tool_call(request_id: Any, params: dict[str, Any]) -> dict[str, Any]:
    """Handle `tool.call` — validate, dispatch, return result + correlation id."""
    try:
        validated = ToolCallParams(**params)
        tool_name = validated.tool_name
        arguments = validated.arguments
    except Exception as e:
        return _error_response(request_id, -32602, f"Invalid tool call params: {e}")
    action = dict(arguments)
    action["action"] = tool_name
    # Generate a correlation id so the caller can match this response
    # to their request. Re-use a provided id if the caller already has one.
    correlation_id = correlation_manager.generate_correlation_id(
        tool_name, arguments, params.get("correlation_id")
    )
    try:
        result = await execute_simone_action(action)
        correlation_manager.complete_call(correlation_id, result)
        return _ok_response(
            request_id, {"correlation_id": correlation_id, "data": result}
        )
    except Exception as e:
        # Record the failure in the correlation store; the caller still
        # gets a structured error response.
        correlation_manager.complete_call(correlation_id, None, str(e))
        return _error_response(request_id, -32603, str(e))


# ── Message dispatch ──────────────────────────────────────────────────
async def _handle_message_send(request_id: Any, params: dict[str, Any]) -> dict[str, Any]:
    """Handle `message/send` — interpret the text as a JSON action or raw action name."""
    try:
        validated = MessageSendParams(**params)
        parts = validated.message.parts
    except Exception as e:
        return _error_response(request_id, -32602, f"Invalid message/send params: {e}")
    text = " ".join(str(part.text) for part in parts).strip()
    # Default: treat the message as a bare action name (e.g. "agent.help").
    action = {"action": text or "agent.help"}
    # If the message is valid JSON with an "action" key, use that.
    try:
        parsed = json.loads(text) if text else None
        if isinstance(parsed, dict) and isinstance(parsed.get("action"), str):
            action = parsed
    except json.JSONDecodeError:
        # Free-text message — keep the default action.
        pass
    result = await execute_simone_action(action)
    return _ok_response(
        request_id,
        {
            "id": str(uuid.uuid4()),
            "kind": "task",
            "status": {
                "state": "completed",
                "message": {
                    "role": "agent",
                    "parts": [{"type": "text", "text": "completed"}],
                },
            },
            "artifacts": [
                {
                    "id": str(uuid.uuid4()),
                    "name": action.get("action", "agent.help"),
                    "parts": [{"type": "data", "data": result}],
                }
            ],
        },
    )


# ── Response helpers ──────────────────────────────────────────────────
def _ok_response(request_id: Any, result: Any) -> dict[str, Any]:
    """Build a JSON-RPC 2.0 success response."""
    return {"jsonrpc": "2.0", "id": request_id, "result": result}


def _error_response(request_id: Any, code: int, message: str) -> dict[str, Any]:
    """Build a JSON-RPC 2.0 error response.

    Standard JSON-RPC error codes:
      -32700 = parse error, -32600 = invalid request, -32601 = method not found,
      -32602 = invalid params, -32603 = internal error.
    """
    return {"jsonrpc": "2.0", "id": request_id, "error": {"code": code, "message": message}}
