# SPDX-License-Identifier: MIT
"""Adaptive Budget Governor: intelligently distributes a global time budget.

Three mechanisms that beat static per-task budgets:
1. INFORMED SEEDING    initial budgets from Analytics-EMA per task_class
                        (with 2x safety factor), capped by global budget
2. SURPLUS RECYCLING   tasks finishing faster than budgeted return their
                        surplus to a pool; running and upcoming tasks can
                        draw from it (weighted by critical-path priority)
3. DEADLINE PRESSURE   as the global budget approaches its end, new grants
                        shrink proportionally (graceful degradation, not
                        hard abort mid-merge)

Async-safe via asyncio.Lock. Pure logic, no I/O — fully unit-testable.
"""

from __future__ import annotations

import asyncio
import time
from dataclasses import dataclass, field

from .analytics import Analytics, task_class_of
from .models import Plan, Task


@dataclass
class Allocation:
    task_id: str
    granted: float
    started: float = 0.0
    extended: float = 0.0


@dataclass
class BudgetGovernor:
    plan: Plan
    global_seconds: float
    priority: dict[str, int]
    analytics: Analytics | None = None
    safety_factor: float = 2.0
    min_grant: float = 60.0

    _pool: float = field(init=False, default=0.0)
    _allocs: dict[str, Allocation] = field(init=False,
                                          default_factory=dict)
    _deadline: float = field(init=False, default=0.0)
    _lock: asyncio.Lock = field(init=False,
                                default_factory=asyncio.Lock)

    def __post_init__(self) -> None:
        self._deadline = time.monotonic() + self.global_seconds
        self._seed()

    def _estimate(self, task: Task) -> float:
        if self.analytics is None:
            return task.budget.max_seconds
        est = self.analytics.expected_seconds(
            task_class_of(task), default=task.budget.max_seconds)
        return min(est * self.safety_factor, task.budget.max_seconds)

    def _seed(self) -> None:
        estimates = {t.id: max(self._estimate(t), self.min_grant)
                     for t in self.plan.tasks}
        total = sum(estimates.values())
        scale = (min(1.0, self.global_seconds / total) if total else 1.0)
        for tid, est in estimates.items():
            self._allocs[tid] = Allocation(
                tid, granted=max(est * scale, self.min_grant))
        self._pool = max(0.0, self.global_seconds - sum(
            a.granted for a in self._allocs.values()))

    def remaining_global(self) -> float:
        return max(0.0, self._deadline - time.monotonic())

    def _pressure(self) -> float:
        """1.0 = relaxed, -> 0.0 as deadline approaches."""
        return min(1.0, self.remaining_global()
                   / max(self.global_seconds, 1.0) * 2.0)

    async def lease(self, task_id: str) -> float:
        async with self._lock:
            alloc = self._allocs[task_id]
            alloc.started = time.monotonic()
            grant = alloc.granted * self._pressure()
            grant = max(grant,
                        min(self.min_grant, self.remaining_global()))
            return min(grant, self.remaining_global())

    async def release(self, task_id: str, used_seconds: float) -> None:
        async with self._lock:
            alloc = self._allocs[task_id]
            surplus = (alloc.granted + alloc.extended) - used_seconds
            if surplus > 0:
                self._pool += surplus

    async def request_extension(self, task_id: str,
                                seconds: float) -> float:
        async with self._lock:
            if self._pool <= 0:
                return 0.0
            max_prio = max(self.priority.values()) or 1
            weight = self.priority.get(task_id, 1) / max_prio
            grant = min(seconds * weight, self._pool,
                        self.remaining_global())
            if grant < 5.0:
                return 0.0
            self._pool -= grant
            self._allocs[task_id].extended += grant
            return grant

    def snapshot(self) -> dict:
        return {
            "pool": round(self._pool, 1),
            "remaining_global": round(self.remaining_global(), 1),
            "pressure": round(self._pressure(), 2),
            "allocations": {
                tid: {"granted": round(a.granted, 1),
                      "extended": round(a.extended, 1)}
                for tid, a in self._allocs.items()},
        }
