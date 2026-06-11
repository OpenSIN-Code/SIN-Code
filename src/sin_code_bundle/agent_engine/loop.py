# SPDX-License-Identifier: MIT
"""AgentLoop — the top-level plan -> execute -> verify -> repair orchestrator."""

from __future__ import annotations

import time
from typing import Any, Awaitable, Callable

from .executor import Executor
from .memory_bridge import MemoryBridge
from .planner import Planner
from .router import ToolRouter
from .telemetry import Telemetry
from .types import AgentTask, Plan, StepResult, Verdict
from .verifier import Verifier

RepairFactory = Callable[[AgentTask, Verdict], Awaitable[list[dict[str, Any]]]]


class AgentLoop:
    def __init__(
        self,
        router: ToolRouter,
        verifier: Verifier,
        *,
        telemetry: Telemetry | None = None,
        memory: MemoryBridge | None = None,
        repair_factory: RepairFactory | None = None,
    ) -> None:
        self.router = router
        self.verifier = verifier
        self.telemetry = telemetry or Telemetry()
        self.memory = memory or MemoryBridge()
        self.planner = Planner()
        self.repair_factory = repair_factory

    async def run(
        self, task: AgentTask, step_specs: list[dict[str, Any]]
    ) -> dict[str, Any]:
        t0 = time.monotonic()
        lessons: list[str] = []

        prior = self.memory.recall_similar(task.goal)
        if prior:
            self.telemetry.emit("recall", hits=len(prior))

        plan = self.planner.build(task, step_specs)
        self.telemetry.emit("plan_built", task_id=task.task_id,
                            steps=len(plan.steps))

        executor = Executor(self.router, self.telemetry)
        results: dict[str, StepResult] = {}
        verdict = Verdict(kind=VerdictKind.FAIL_BUDGET)

        try:
            for round_no in range(task.max_repair_rounds + 1):
                results.update(await executor.run(task, plan, self.planner))
                verdict = await self.verifier.verify()
                self.telemetry.emit("verdict", round=round_no,
                                    kind=verdict.kind.value, ok=verdict.ok)
                if verdict.ok:
                    break
                lessons.append(
                    f"round {round_no}: {verdict.kind.value} — "
                    f"{(verdict.repair_hint or verdict.detail)[:300]}"
                )
                if round_no >= task.max_repair_rounds or not self.repair_factory:
                    break
                repair_specs = await self.repair_factory(task, verdict)
                if not repair_specs:
                    break
                plan = self.planner.build(task, repair_specs)
                self.telemetry.emit("repair_plan", round=round_no,
                                    steps=len(plan.steps))
        finally:
            executor.cleanup(task.repo_root)

        outcome = "success" if verdict.ok else f"failed:{verdict.kind.value}"
        elapsed = round(time.monotonic() - t0, 1)
        self.memory.remember_run(
            task_id=task.task_id,
            goal=task.goal,
            outcome=outcome,
            repair_rounds=len(lessons),
            lessons=lessons,
            plan_json=plan.to_json(),
            elapsed_s=elapsed,
        )

        report = {
            "task_id": task.task_id,
            "outcome": outcome,
            "verdict": verdict.kind.value if verdict.kind else "unknown",
            "elapsed_s": round(time.monotonic() - t0, 1),
            "steps_total": len(results),
            "steps_ok": sum(1 for r in results.values() if r.ok),
            "lessons": lessons,
            "router_stats": self.router.stats(),
            "telemetry": self.telemetry.summary(),
        }
        self.telemetry.emit("run_complete", **{
            k: v for k, v in report.items() if k != "router_stats"
        })
        return report
