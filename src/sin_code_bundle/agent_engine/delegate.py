# SPDX-License-Identifier: MIT
"""Sub-Agent Delegation — Steps that spawn a nested AgentLoop with own budget."""

from __future__ import annotations

import time
from dataclasses import dataclass, field
from typing import Any, Callable

from .builtin_tools import register_builtin_tools
from .memory_bridge import MemoryBridge
from .router import ToolRouter
from .telemetry import Telemetry
from .types import AgentTask
from .verifier import Verifier


@dataclass(slots=True)
class DelegationContext:
    depth: int = 0
    max_depth: int = 3
    budget_deadline: float = field(
        default_factory=lambda: time.monotonic() + 1800.0
    )
    budget_fraction: float = 0.5
    min_budget_s: float = 60.0

    def remaining_s(self) -> float:
        return max(0.0, self.budget_deadline - time.monotonic())

    def child_budget_s(self) -> float:
        return self.remaining_s() * self.budget_fraction

    def can_delegate(self) -> tuple[bool, str]:
        if self.depth >= self.max_depth:
            return False, f"max delegation depth {self.max_depth} reached"
        if self.child_budget_s() < self.min_budget_s:
            return False, (
                f"insufficient budget for sub-agent "
                f"({self.child_budget_s():.0f}s < {self.min_budget_s:.0f}s)"
            )
        return True, ""

    def child(self) -> "DelegationContext":
        return DelegationContext(
            depth=self.depth + 1,
            max_depth=self.max_depth,
            budget_deadline=time.monotonic() + self.child_budget_s(),
            budget_fraction=self.budget_fraction,
            min_budget_s=self.min_budget_s,
        )


def make_delegate_tool(
    parent_ctx: DelegationContext,
    telemetry: Telemetry,
    *,
    policy_wrap: Callable | None = None,
):
    async def sin_delegate(
        *,
        goal: str,
        steps: list[dict[str, Any]],
        cwd: str,
        constraints: list[str] | None = None,
        parallel: int = 2,
        repair_rounds: int = 2,
    ) -> dict[str, Any]:
        ok, reason = parent_ctx.can_delegate()
        if not ok:
            raise RuntimeError(f"delegation refused: {reason}")

        child_ctx = parent_ctx.child()
        telemetry.emit("delegate_start", goal=goal[:120],
                       depth=child_ctx.depth,
                       budget_s=round(child_ctx.remaining_s(), 1))

        child_router = register_builtin_tools(ToolRouter())
        if policy_wrap is not None:
            policy_wrap(child_router)
        child_router.register(
            "sin_delegate",
            make_delegate_tool(child_ctx, telemetry,
                               policy_wrap=policy_wrap),
        )

        from .loop import AgentLoop
        child_verifier = Verifier(cwd, telemetry)
        child_loop = AgentLoop(child_router, child_verifier,
                               telemetry=telemetry, memory=MemoryBridge())
        task = AgentTask(
            goal=goal, repo_root=cwd,
            constraints=list(constraints or []),
            max_parallelism=parallel,
            budget_seconds=child_ctx.remaining_s(),
            max_repair_rounds=repair_rounds,
        )
        report = await child_loop.run(task, steps)

        telemetry.emit("delegate_done", goal=goal[:120],
                       depth=child_ctx.depth,
                       outcome=report["outcome"])
        return {
            "outcome": report["outcome"],
            "verdict": report["verdict"],
            "elapsed_s": report["elapsed_s"],
            "steps_ok": report["steps_ok"],
            "steps_total": report["steps_total"],
            "lessons": report["lessons"][:5],
            "depth": child_ctx.depth,
        }

    return sin_delegate
