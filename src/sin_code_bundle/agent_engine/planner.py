# SPDX-License-Identifier: MIT
"""Dependency-aware DAG planner with critical-path prioritization.

Two jobs:
1. Build a Plan from structured step specs.
2. Compute the critical path so the executor always works on the steps
   that unblock the most downstream work first.
"""

from __future__ import annotations

from typing import Any

from .types import AgentTask, Plan, Step, StepState


class Planner:
    def build(self, task: AgentTask, step_specs: list[dict[str, Any]]) -> Plan:
        plan = Plan(task_id=task.task_id)
        for spec in step_specs:
            plan.add(Step(
                step_id=spec["step_id"],
                title=spec.get("title", spec["step_id"]),
                tool=spec["tool"],
                args=spec.get("args", {}),
                deps=list(spec.get("deps", [])),
                estimated_cost=float(spec.get("estimated_cost", 1.0)),
                isolated=bool(spec.get("isolated", False)),
                max_attempts=int(spec.get("max_attempts", 3)),
            ))
        plan.validate()
        return plan

    def critical_path_weights(self, plan: Plan) -> dict[str, float]:
        dependents: dict[str, list[str]] = {sid: [] for sid in plan.steps}
        indeg: dict[str, int] = {sid: 0 for sid in plan.steps}
        for s in plan.steps.values():
            for d in s.deps:
                dependents[d].append(s.step_id)
                indeg[s.step_id] += 1

        order: list[str] = []
        queue = [sid for sid, deg in indeg.items() if deg == 0]
        while queue:
            sid = queue.pop()
            order.append(sid)
            for child in dependents[sid]:
                indeg[child] -= 1
                if indeg[child] == 0:
                    queue.append(child)

        weights: dict[str, float] = {}
        for sid in reversed(order):
            step = plan.steps[sid]
            child_max = max((weights[c] for c in dependents[sid]), default=0.0)
            weights[sid] = step.estimated_cost + child_max
        return weights

    def ready_steps(self, plan: Plan) -> list[Step]:
        weights = self.critical_path_weights(plan)
        ready: list[Step] = []
        for s in plan.steps.values():
            if s.state is not StepState.PENDING:
                continue
            if all(plan.steps[d].state is StepState.SUCCEEDED for d in s.deps):
                ready.append(s)
        ready.sort(key=lambda s: weights[s.step_id], reverse=True)
        return ready

    def propagate_failure(self, plan: Plan, failed_id: str) -> list[str]:
        dependents: dict[str, list[str]] = {sid: [] for sid in plan.steps}
        for s in plan.steps.values():
            for d in s.deps:
                dependents[d].append(s.step_id)
        skipped: list[str] = []
        stack = list(dependents[failed_id])
        while stack:
            sid = stack.pop()
            step = plan.steps[sid]
            if step.state in (StepState.PENDING, StepState.READY):
                step.state = StepState.SKIPPED
                skipped.append(sid)
                stack.extend(dependents[sid])
        return skipped
