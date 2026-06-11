# SPDX-License-Identifier: MIT
"""sin-brain integration with graceful degradation.

If sin-brain is installed, every run feeds back pitfalls and decisions
and recalls them on the next run. If it is not installed, all calls
are silent no-ops — the agent still works, just without persistent memory.
"""

from __future__ import annotations

from typing import Any

try:
    from sin_brain import recall as _recall, remember as _remember
    _AVAILABLE = True
except Exception:  # pragma: no cover
    _AVAILABLE = False


def available() -> bool:
    return _AVAILABLE


def recall_pitfalls(goal: str, limit: int = 5) -> list[str]:
    if not _AVAILABLE:
        return []
    try:
        hits: list[dict[str, Any]] = _recall(
            query=goal, kinds=["pitfall", "fix"], limit=limit)
        return [h.get("text", "") for h in hits if h.get("text")]
    except Exception:
        return []


def remember_pitfall(goal: str, task_title: str, detail: str) -> None:
    if not _AVAILABLE:
        return
    try:
        _remember(
            kind="pitfall",
            text=(f"[delegate] goal={goal!r} task={task_title!r}: "
                  f"{detail}"),
            tags=["sin-delegate"],
        )
    except Exception:
        pass


def remember_decision(goal: str, detail: str) -> None:
    if not _AVAILABLE:
        return
    try:
        _remember(
            kind="decision",
            text=f"[delegate] goal={goal!r}: {detail}",
            tags=["sin-delegate"],
        )
    except Exception:
        pass
