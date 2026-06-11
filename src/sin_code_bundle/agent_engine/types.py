# SPDX-License-Identifier: MIT
"""Core datatypes for the SIN Agent Engine.

Everything is a frozen-ish dataclass with explicit state machines so the
engine is fully introspectable, serializable, and replayable.
"""

from __future__ import annotations

import enum
import hashlib
import json
import time
import uuid
from dataclasses import dataclass, field
from typing import Any


class StepState(enum.Enum):
    PENDING = "pending"
    READY = "ready"
    RUNNING = "running"
    SUCCEEDED = "succeeded"
    FAILED = "failed"
    SKIPPED = "skipped"
    REPAIRING = "repairing"


class VerdictKind(enum.Enum):
    PASS = "pass"
    FAIL_TESTS = "fail_tests"
    FAIL_LINT = "fail_lint"
    FAIL_ARCHITECTURE = "fail_architecture"
    FAIL_SEMANTIC = "fail_semantic"
    FAIL_BUDGET = "fail_budget"


@dataclass(slots=True)
class AgentTask:
    goal: str
    repo_root: str
    constraints: list[str] = field(default_factory=list)
    max_parallelism: int = 4
    max_repair_rounds: int = 3
    budget_seconds: float = 1800.0
    task_id: str = field(default_factory=lambda: uuid.uuid4().hex[:12])
    created_at: float = field(default_factory=time.time)

    def fingerprint(self) -> str:
        raw = json.dumps({"goal": self.goal, "constraints": self.constraints},
                         sort_keys=True)
        return hashlib.sha256(raw.encode()).hexdigest()[:16]


@dataclass(slots=True)
class Step:
    step_id: str
    title: str
    tool: str
    args: dict[str, Any]
    deps: list[str] = field(default_factory=list)
    estimated_cost: float = 1.0
    isolated: bool = False
    state: StepState = StepState.PENDING
    attempts: int = 0
    max_attempts: int = 3

    def to_dict(self) -> dict[str, Any]:
        return {
            "step_id": self.step_id,
            "title": self.title,
            "tool": self.tool,
            "args": self.args,
            "deps": self.deps,
            "estimated_cost": self.estimated_cost,
            "isolated": self.isolated,
            "state": self.state.value,
            "attempts": self.attempts,
        }


@dataclass(slots=True)
class StepResult:
    step_id: str
    ok: bool
    output: Any = None
    error: str | None = None
    duration_s: float = 0.0
    worktree: str | None = None
    artifacts: dict[str, str] = field(default_factory=dict)


@dataclass(slots=True)
class Verdict:
    kind: VerdictKind
    detail: str = ""
    failing_steps: list[str] = field(default_factory=list)
    repair_hint: str | None = None

    @property
    def ok(self) -> bool:
        return self.kind is VerdictKind.PASS


@dataclass(slots=True)
class Plan:
    task_id: str
    steps: dict[str, Step] = field(default_factory=dict)

    def add(self, step: Step) -> None:
        if step.step_id in self.steps:
            raise ValueError(f"duplicate step_id: {step.step_id}")
        self.steps[step.step_id] = step

    def validate(self) -> None:
        for s in self.steps.values():
            for d in s.deps:
                if d not in self.steps:
                    raise ValueError(f"{s.step_id}: unknown dep {d!r}")
        indeg = {sid: 0 for sid in self.steps}
        dependents: dict[str, list[str]] = {sid: [] for sid in self.steps}
        for s in self.steps.values():
            for d in s.deps:
                indeg[s.step_id] += 1
                dependents[d].append(s.step_id)
        queue = [sid for sid, deg in indeg.items() if deg == 0]
        seen = 0
        while queue:
            sid = queue.pop()
            seen += 1
            for child in dependents[sid]:
                indeg[child] -= 1
                if indeg[child] == 0:
                    queue.append(child)
        if seen != len(self.steps):
            raise ValueError("plan contains a dependency cycle")

    def to_json(self) -> str:
        return json.dumps(
            {"task_id": self.task_id,
             "steps": [s.to_dict() for s in self.steps.values()]},
            indent=2,
        )
