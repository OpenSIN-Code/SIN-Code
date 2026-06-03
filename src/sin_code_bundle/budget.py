"""Keep MCP tool outputs compact so they don't blow the agent's context window.

Every tool result is passed through `trim()` before returning. Lists are capped,
long strings truncated, and an explicit `_truncated` flag is added so the agent
knows more data exists.

Docs: budget.doc.md
"""
from __future__ import annotations

from typing import Any

MAX_LIST = 25
MAX_STR = 2000


def trim(value: Any, max_list: int = MAX_LIST, max_str: int = MAX_STR) -> Any:
    """Recursively trim a tool output to safe sizes."""
    if isinstance(value, str):
        return value if len(value) <= max_str else value[:max_str] + " ...[truncated]"
    if isinstance(value, list):
        trimmed = [trim(v, max_list, max_str) for v in value[:max_list]]
        if len(value) > max_list:
            trimmed.append({"_truncated": True, "_omitted": len(value) - max_list})
        return trimmed
    if isinstance(value, dict):
        return {k: trim(v, max_list, max_str) for k, v in value.items()}
    return value
