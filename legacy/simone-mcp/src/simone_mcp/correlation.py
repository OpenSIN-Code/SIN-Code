from __future__ import annotations

import hashlib
import json
from datetime import datetime
from typing import Any


class ToolCallCorrelation:
    def __init__(self, max_age_seconds: int = 3600):
        self.active_calls: dict[str, dict[str, Any]] = {}
        self.max_age_seconds = max_age_seconds

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
        self.active_calls[correlation_id] = {
            "tool_name": tool_name,
            "arguments": arguments,
            "started_at": datetime.now().isoformat(),
            "status": "in_progress",
        }
        return correlation_id

    def complete_call(self, correlation_id: str, result: Any, error: str | None = None) -> None:
        if correlation_id not in self.active_calls:
            return
        self.active_calls[correlation_id].update(
            {
                "status": "completed" if not error else "failed",
                "completed_at": datetime.now().isoformat(),
                "result": result,
                "error": error,
            }
        )

    def get_call_status(self, correlation_id: str) -> dict[str, Any] | None:
        return self.active_calls.get(correlation_id)

    def cleanup_old_calls(self) -> int:
        now = datetime.now()
        to_remove = [
            cid
            for cid, data in self.active_calls.items()
            if (now - datetime.fromisoformat(data["started_at"])).total_seconds()
            > self.max_age_seconds
        ]
        for cid in to_remove:
            del self.active_calls[cid]
        return len(to_remove)


correlation_manager = ToolCallCorrelation()
