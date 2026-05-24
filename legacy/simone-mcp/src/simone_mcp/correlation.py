from __future__ import annotations

import hashlib
import json
from collections import OrderedDict
from datetime import datetime
from typing import Any

MAX_ACTIVE_CALLS = 1024
CLEANUP_EVERY_N = 64


class ToolCallCorrelation:
    def __init__(self, max_age_seconds: int = 3600, max_calls: int = MAX_ACTIVE_CALLS):
        self._calls: OrderedDict[str, dict[str, Any]] = OrderedDict()
        self.max_age_seconds = max_age_seconds
        self.max_calls = max_calls
        self._op_count = 0

    def generate_correlation_id(
        self, tool_name: str, arguments: dict[str, Any], provided_id: str | None = None
    ) -> str:
        if provided_id:
            correlation_id = provided_id
        else:
            canonical = json.dumps(
                {"tool": tool_name, "args": arguments}, sort_keys=True, separators=(",", ":")
            )
            digest = hashlib.sha256(canonical.encode()).hexdigest()
            correlation_id = f"auto_{digest[:8]}_{int(datetime.now().timestamp())}"

        self._calls[correlation_id] = {
            "tool_name": tool_name,
            "arguments": arguments,
            "started_at": datetime.now().isoformat(),
            "status": "in_progress",
        }
        self._calls.move_to_end(correlation_id)
        self._maybe_evict()
        return correlation_id

    def complete_call(self, correlation_id: str, result: Any, error: str | None = None) -> None:
        if correlation_id not in self._calls:
            return
        self._calls[correlation_id].update(
            {
                "status": "completed" if not error else "failed",
                "completed_at": datetime.now().isoformat(),
                "result": result,
                "error": error,
            }
        )
        self._calls.move_to_end(correlation_id)
        self._op_count += 1
        if self._op_count % CLEANUP_EVERY_N == 0:
            self._cleanup_stale()

    def get_call_status(self, correlation_id: str) -> dict[str, Any] | None:
        return self._calls.get(correlation_id)

    def cleanup_old_calls(self) -> int:
        return self._cleanup_stale()

    def _maybe_evict(self) -> None:
        while len(self._calls) > self.max_calls:
            self._calls.popitem(last=False)

    def _cleanup_stale(self) -> int:
        now = datetime.now()
        to_remove = [
            cid
            for cid, data in self._calls.items()
            if (now - datetime.fromisoformat(data["started_at"])).total_seconds()
            > self.max_age_seconds
        ]
        for cid in to_remove:
            del self._calls[cid]
        return len(to_remove)


correlation_manager = ToolCallCorrelation()
