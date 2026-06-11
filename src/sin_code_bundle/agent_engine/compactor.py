# SPDX-License-Identifier: MIT
"""Context Compactor — keeps long agent sessions inside any context budget.

Three-tier rolling working set:
    HOT   — recent, verbatim (last N entries, full fidelity)
    WARM  — older entries compressed to structured digests
    COLD  — evicted from the working set entirely
"""

from __future__ import annotations

import json
from collections import deque
from dataclasses import dataclass, field
from typing import Any


@dataclass(slots=True)
class _Entry:
    step_id: str
    kind: str
    content: str
    digest: str | None = None

    def size(self) -> int:
        return len(self.digest if self.digest is not None else self.content)


def _digest_tool_result(step_id: str, content: str) -> str:
    head = content[:300]
    try:
        data = json.loads(content)
        facts: dict[str, Any] = {"step_id": step_id}
        for key in ("exit_code", "path", "bytes", "total_lines",
                    "replaced", "truncated"):
            if key in data:
                facts[key] = data[key]
        if isinstance(data.get("hits"), list):
            facts["hit_count"] = len(data["hits"])
            facts["hit_files"] = sorted(
                {h.get("file", "?") for h in data["hits"][:20]}
            )
        if data.get("stderr"):
            facts["error_head"] = str(data["stderr"])[:200]
        return json.dumps(facts, ensure_ascii=False)
    except (json.JSONDecodeError, TypeError, AttributeError):
        return json.dumps({"step_id": step_id, "head": head},
                          ensure_ascii=False)


@dataclass
class ContextCompactor:
    budget_chars: int = 60_000
    hot_count: int = 8
    _entries: deque[_Entry] = field(default_factory=deque)
    _evicted: list[str] = field(default_factory=list)

    def append(self, step_id: str, content: str,
               kind: str = "tool_result") -> None:
        self._entries.append(_Entry(step_id=step_id, kind=kind,
                                    content=content))
        self._enforce()

    def _enforce(self) -> None:
        n = len(self._entries)
        for i, entry in enumerate(self._entries):
            if i < n - self.hot_count and entry.digest is None:
                entry.digest = (
                    _digest_tool_result(entry.step_id, entry.content)
                    if entry.kind == "tool_result"
                    else entry.content[:300]
                )
        while self.total_size() > self.budget_chars and len(self._entries) > 0:
            if len(self._entries) <= self.hot_count:
                for entry in self._entries:
                    if entry.digest is None:
                        entry.digest = (
                            _digest_tool_result(entry.step_id, entry.content)
                            if entry.kind == "tool_result"
                            else entry.content[:300]
                        )
                if self.total_size() <= self.budget_chars:
                    break
            evicted = self._entries.popleft()
            self._evicted.append(evicted.step_id)

    def total_size(self) -> int:
        return sum(e.size() for e in self._entries)

    def render(self) -> str:
        parts: list[str] = []
        if self._evicted:
            parts.append(
                f"[{len(self._evicted)} older results evicted — "
                f"retrievable by step_id from telemetry log: "
                f"{', '.join(self._evicted[-10:])}]"
            )
        n = len(self._entries)
        for i, entry in enumerate(self._entries):
            if i < n - self.hot_count and entry.digest is not None:
                parts.append(f"[digest {entry.step_id}] {entry.digest}")
            else:
                parts.append(f"[{entry.step_id}] {entry.content}")
        return "\n".join(parts)

    def stats(self) -> dict[str, int]:
        return {
            "entries": len(self._entries),
            "evicted": len(self._evicted),
            "size_chars": self.total_size(),
            "budget_chars": self.budget_chars,
        }
