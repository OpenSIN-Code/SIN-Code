"""Per-tool-call correlation tracking for the A2A / MCP transport.

Every tool call gets a correlation id (caller-provided or auto-generated
from the tool name + arguments). The id maps to a record that tracks
status, result, and timing. Records evict on age or count.

Docs: correlation.doc.md
"""
from __future__ import annotations

import hashlib
import json
import threading
from collections import OrderedDict
from datetime import datetime
from typing import Any

# `MAX_ACTIVE_CALLS=1024` bounds the in-memory dict so a runaway client
# can't OOM us. `CLEANUP_EVERY_N=64` amortizes the stale-record scan.
MAX_ACTIVE_CALLS = 1024
CLEANUP_EVERY_N = 64


# ── Correlation manager ───────────────────────────────────────────────
class ToolCallCorrelation:
    """Thread-safe in-memory store of in-flight and recent tool calls.

    Records are LRU-evicted on size (`max_calls`) and age (`max_age_seconds`).
    Use `correlation_manager` (module-level singleton) for the default
    instance; instantiate this class directly for custom limits.
    """

    def __init__(self, max_age_seconds: int = 3600, max_calls: int = MAX_ACTIVE_CALLS):
        # OrderedDict is used so we can LRU-evict by popping the oldest.
        self._calls: OrderedDict[str, dict[str, Any]] = OrderedDict()
        # Single lock guards the dict; read-mostly workload, contention is rare.
        self._lock = threading.Lock()
        # 1 hour default — long enough to trace a multi-step agent loop.
        self.max_age_seconds = max_age_seconds
        self.max_calls = max_calls
        # `_op_count` drives the periodic stale-record cleanup so we
        # don't scan on every operation.
        self._op_count = 0

    def generate_correlation_id(
        self, tool_name: str, arguments: dict[str, Any], provided_id: str | None = None
    ) -> str:
        """Register a new call and return its correlation id.

        Args:
            tool_name: Name of the tool being called.
            arguments: Arguments passed to the tool.
            provided_id: Use this id if given (caller-managed);
                         otherwise auto-generate from tool+args+timestamp.

        Returns:
            The correlation id (caller can pass it to `complete_call`).
        """
        if provided_id:
            correlation_id = provided_id
        else:
            # Canonical JSON keeps (tool, args) pairs that compare equal
            # hashing to the same digest — useful for idempotency checks.
            canonical = json.dumps(
                {"tool": tool_name, "args": arguments}, sort_keys=True, separators=(",", ":")
            )
            digest = hashlib.sha256(canonical.encode()).hexdigest()
            # 8 hex chars (32 bits) is enough entropy for non-security
            # uniqueness; the timestamp suffix disambiguates simultaneous calls.
            correlation_id = f"auto_{digest[:8]}_{int(datetime.now().timestamp())}"

        with self._lock:
            self._calls[correlation_id] = {
                "tool_name": tool_name,
                "arguments": arguments,
                "started_at": datetime.now().isoformat(),
                "status": "in_progress",
            }
            # `move_to_end` keeps the OrderedDict in LRU order; the
            # eviction loop pops from the front.
            self._calls.move_to_end(correlation_id)
            self._maybe_evict_unlocked()
        return correlation_id

    def complete_call(self, correlation_id: str, result: Any, error: str | None = None) -> None:
        """Mark a call as completed (or failed) and record its outcome.

        No-op if `correlation_id` is unknown (e.g. evicted already).
        Triggers a periodic stale-record cleanup.
        """
        with self._lock:
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
            # Cleanup is amortized: every Nth call, scan for stale records.
            if self._op_count % CLEANUP_EVERY_N == 0:
                self._cleanup_stale_unlocked()

    def get_call_status(self, correlation_id: str) -> dict[str, Any] | None:
        """Return the record for `correlation_id`, or `None` if unknown."""
        with self._lock:
            return self._calls.get(correlation_id)

    def cleanup_old_calls(self) -> int:
        """Force a stale-record cleanup; returns the number removed."""
        with self._lock:
            return self._cleanup_stale_unlocked()

    def _maybe_evict_unlocked(self) -> None:
        """LRU-evict from the front until under the size cap. Caller holds the lock."""
        while len(self._calls) > self.max_calls:
            # `last=False` pops the FRONT (oldest) — that's the LRU end.
            self._calls.popitem(last=False)

    def _cleanup_stale_unlocked(self) -> int:
        """Remove records older than `max_age_seconds`. Caller holds the lock."""
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


# Module-level singleton — the standard handle for the rest of the package.
correlation_manager = ToolCallCorrelation()
