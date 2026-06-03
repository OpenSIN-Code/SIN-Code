"""Keep MCP tool outputs compact so they don't blow the agent's context window.

Every tool result is passed through `trim()` before returning. Lists are capped,
long strings truncated, and an explicit `_truncated` flag is added so the agent
knows more data exists.

Docs: budget.doc.md
"""
from __future__ import annotations

from typing import Any

# Default ceilings sized to fit comfortably in a 200K-token agent context even
# when many tools are called per turn — strings dominate token cost so we cap
# them harder than list arity. Override per-call via `trim(value, max_list=…)`.
MAX_LIST = 25       # max items kept per list; rest collapsed into _truncated sentinel
MAX_STR = 2000      # max characters per string; rest replaced with " ...[truncated]"


def trim(value: Any, max_list: int = MAX_LIST, max_str: int = MAX_STR) -> Any:
    """Recursively trim a tool output to safe sizes.

    Walks any JSON-shaped value (str / list / dict / scalar) and enforces the
    `max_list` and `max_str` ceilings. Non-container scalars pass through
    untouched. Lists longer than `max_list` get an extra trailing dict
    ``{"_truncated": True, "_omitted": N}`` so the agent can see that more
    data existed without being forced to render it.

    Args:
        value: Any JSON-serialisable Python value (typically the result of
            an MCP tool call).
        max_list: Maximum list length to keep before truncating.
        max_str: Maximum string length (in characters) before truncating.

    Returns:
        A new value of the same shape as ``value`` but capped to the limits.
        Original input is never mutated.
    """
    if isinstance(value, str):
        return value if len(value) <= max_str else value[:max_str] + " ...[truncated]"
    if isinstance(value, list):
        trimmed = [trim(v, max_list, max_str) for v in value[:max_list]]
        if len(value) > max_list:
            # Sentinel must be a dict (not a string) so JSON consumers can detect
            # truncation programmatically without scanning text content.
            trimmed.append({"_truncated": True, "_omitted": len(value) - max_list})
        return trimmed
    if isinstance(value, dict):
        return {k: trim(v, max_list, max_str) for k, v in value.items()}
    return value
