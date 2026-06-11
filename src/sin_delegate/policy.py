# SPDX-License-Identifier: MIT
"""Policy layer: makes delegation self-optimizing.

Before every run, the plan is routed through the policy:
- BACKEND ROUTING: if Analytics has enough data for a task_class, the
  proven best (backend, model) is set unless the plan explicitly pins
  a backend (explicit > learned, always).
- EXPLORATION: with probability epsilon (default 10%), LOW-risk tasks
  get a candidate outside the optimum — otherwise the statistics
  ossify. HIGH risk never explores.
- BUDGET SEEDING: Analytics-EMA replaces default budgets (see Governor).

Deterministic seed per plan_id — reproducible routing decisions.
"""

from __future__ import annotations

import random
from dataclasses import dataclass, replace

from .analytics import Analytics, task_class_of
from .models import AgentSpec, Plan, Risk, Task

DEFAULT_CANDIDATES: list = [
    ("opencode", ""),
    ("claude", ""),
    ("codex", ""),
]


@dataclass
class RoutingDecision:
    task_id: str
    chosen: tuple
    reason: str  # "pinned" | "learned" | "explore" | "default"


@dataclass
class Policy:
    analytics: Analytics
    epsilon: float = 0.10
    candidates: list | None = None
    min_trials: int = 3

    def __post_init__(self) -> None:
        if self.candidates is None:
            self.candidates = list(DEFAULT_CANDIDATES)

    def apply(self, plan: Plan) -> tuple:
        rng = random.Random(plan.id)
        decisions: list = []
        new_tasks: list = []
        for task in plan.tasks:
            choice, reason = self._route(task, rng)
            decisions.append(RoutingDecision(task.id, choice, reason))
            if reason in ("pinned", "default"):
                new_tasks.append(task)
            else:
                backend, model = choice
                new_tasks.append(replace(
                    task, agent=AgentSpec(
                        backend=backend, model=model,
                        command=task.agent.command,
                        system_hint=task.agent.system_hint,
                        env=task.agent.env)))
        return replace(plan, tasks=tuple(new_tasks)), decisions

    def _route(self, task: Task,
               rng: random.Random) -> tuple:
        pinned = (task.agent.backend, task.agent.model)
        if task.agent.model or task.agent.backend == "command":
            return pinned, "pinned"

        cls = task_class_of(task)
        best = self.analytics.best_backend(cls, self.candidates,
                                           self.min_trials)
        if (task.risk == Risk.LOW and best is not None
                and rng.random() < self.epsilon):
            others = [c for c in self.candidates if c != best]
            if others:
                return rng.choice(others), "explore"

        if best is not None:
            return best, "learned"
        return pinned, "default"
