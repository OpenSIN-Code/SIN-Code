# SPDX-License-Identifier: MIT
"""DAG scheduler: asyncio, critical-path-first, budget-governed, resumable.

Why better than a naive topo-sort loop:
- Tasks become READY the moment their last dependency finishes (no waves
  that block on the slowest task of a level).
- Priority = critical-path length (longest downstream chain first), which
  provably minimizes makespan for uniform task costs.
- Circuit breaker: if >50% of finished tasks failed, the run halts early
  instead of burning budget on a doomed plan.
- Resume: states are folded from the ledger; DONE tasks are never redone.
"""

from __future__ import annotations

import asyncio
import time
from typing import Awaitable, Callable

from .ledger import Ledger
from .models import Plan, Task, TaskOutcome, TaskState

TaskExecutor = Callable[[Task], Awaitable[TaskOutcome]]


def critical_path_priority(plan: Plan) -> dict[str, int]:
    """Longest path from each task to any sink (higher = schedule earlier)."""
    children: dict[str, list[str]] = {t.id: [] for t in plan.tasks}
    for t in plan.tasks:
        for d in t.deps:
            children[d].append(t.id)
    memo: dict[str, int] = {}

    def depth(tid: str) -> int:
        if tid in memo:
            return memo[tid]
        memo[tid] = 1 + max((depth(c) for c in children[tid]), default=0)
        return memo[tid]

    return {t.id: depth(t.id) for t in plan.tasks}


class Scheduler:
    def __init__(self, plan: Plan, ledger: Ledger,
                 executor: TaskExecutor,
                 max_parallel: int = 4,
                 failure_threshold: float = 0.5) -> None:
        plan.validate()
        self.plan = plan
        self.ledger = ledger
        self.executor = executor
        self.sem = asyncio.Semaphore(max_parallel)
        self.failure_threshold = failure_threshold
        self.tasks = {t.id: t for t in plan.tasks}
        self.priority = critical_path_priority(plan)
        self.outcomes: dict[str, TaskOutcome] = {}
        self._cancelled = False

    def cancel(self) -> None:
        self._cancelled = True

    def _resume_states(self) -> dict[str, TaskState]:
        persisted = self.ledger.task_states(self.plan.id)
        states: dict[str, TaskState] = {}
        for tid in self.tasks:
            prev = persisted.get(tid)
            states[tid] = (TaskState.DONE if prev == TaskState.DONE
                          else TaskState.PENDING)
        return states

    def _circuit_open(self, states: dict[str, TaskState]) -> bool:
        finished = [s for s in states.values()
                    if s in (TaskState.DONE, TaskState.FAILED,
                             TaskState.ESCALATED)]
        failed = [s for s in finished if s != TaskState.DONE]
        return (len(finished) >= 2
                and len(failed) / len(finished) > self.failure_threshold)

    async def run(self) -> dict[str, TaskOutcome]:
        states = self._resume_states()
        for tid, st in states.items():
            if st == TaskState.DONE:
                self.outcomes[tid] = TaskOutcome(tid, TaskState.DONE)
                self.ledger.emit(self.plan.id, tid, "resume:skip-done")

        running: dict[asyncio.Task, str] = {}

        def ready() -> list[str]:
            out = []
            for tid, t in self.tasks.items():
                if states[tid] != TaskState.PENDING:
                    continue
                if all(states[d] == TaskState.DONE for d in t.deps):
                    out.append(tid)
            out.sort(key=lambda x: -self.priority[x])
            return out

        def doomed() -> list[str]:
            bad = {tid for tid, s in states.items()
                   if s in (TaskState.FAILED, TaskState.SKIPPED,
                            TaskState.ESCALATED, TaskState.CANCELLED)}
            out = []
            for tid, t in self.tasks.items():
                if states[tid] == TaskState.PENDING and any(
                        d in bad for d in t.deps):
                    out.append(tid)
            return out

        while True:
            for tid in doomed():
                states[tid] = TaskState.SKIPPED
                self.outcomes[tid] = TaskOutcome(
                    tid, TaskState.SKIPPED,
                    error="upstream dependency failed")
                self.ledger.emit(self.plan.id, tid, "state:skipped")

            if self._cancelled or self._circuit_open(states):
                for fut in running:
                    fut.cancel()
                for tid, st in states.items():
                    if st in (TaskState.PENDING, TaskState.RUNNING):
                        states[tid] = TaskState.CANCELLED
                        self.outcomes[tid] = TaskOutcome(
                            tid, TaskState.CANCELLED)
                        self.ledger.emit(self.plan.id, tid, "state:cancelled")
                break

            for tid in ready():
                states[tid] = TaskState.RUNNING
                self.ledger.emit(self.plan.id, tid, "state:running")
                fut = asyncio.ensure_future(self._execute(self.tasks[tid]))
                running[fut] = tid

            if not running:
                break

            done, _ = await asyncio.wait(
                running.keys(), return_when=asyncio.FIRST_COMPLETED)
            for fut in done:
                tid = running.pop(fut)
                try:
                    outcome = fut.result()
                except asyncio.CancelledError:
                    outcome = TaskOutcome(tid, TaskState.CANCELLED)
                except Exception as e:
                    outcome = TaskOutcome(
                        tid, TaskState.FAILED, error=str(e))
                states[tid] = outcome.state
                self.outcomes[tid] = outcome
                self.ledger.emit(
                    self.plan.id, tid, f"state:{outcome.state.value}",
                    {"error": outcome.error,
                     "seconds": outcome.seconds,
                     "attempts": outcome.attempts})
        return self.outcomes

    async def _execute(self, task: Task) -> TaskOutcome:
        async with self.sem:
            start = time.monotonic()
            base_attempts = self.ledger.attempts(self.plan.id, task.id)
            last_error = ""
            for attempt in range(task.budget.max_retries + 1):
                if self._cancelled:
                    return TaskOutcome(task.id, TaskState.CANCELLED)
                self.ledger.emit(
                    self.plan.id, task.id, "attempt",
                    {"n": base_attempts + attempt + 1})
                outcome = await self.executor(task)
                outcome.attempts = base_attempts + attempt + 1
                outcome.seconds = time.monotonic() - start
                if outcome.state == TaskState.DONE:
                    return outcome
                last_error = outcome.error
                if outcome.state == TaskState.ESCALATED:
                    return outcome
                await asyncio.sleep(task.budget.retry_delay(attempt))
            return TaskOutcome(
                task.id, TaskState.FAILED,
                attempts=base_attempts + task.budget.max_retries + 1,
                seconds=time.monotonic() - start,
                error=last_error or "exhausted retries",
            )
