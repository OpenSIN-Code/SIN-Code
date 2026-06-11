# SPDX-License-Identifier: MIT
"""Core data model. Everything is a frozen-ish dataclass or an explicit enum.

Task identity is CONTENT-ADDRESSED: id = blake2b(goal + spec + parent-ids).
Re-running the same plan resumes instead of redoing (exactly-once semantics
on top of the event ledger).
"""

from __future__ import annotations

import hashlib
import json
import time
from dataclasses import dataclass, field
from enum import Enum
from typing import Any


class TaskState(str, Enum):
    PENDING = "pending"
    READY = "ready"
    RUNNING = "running"
    VERIFYING = "verifying"
    MERGING = "merging"
    DONE = "done"
    FAILED = "failed"
    SKIPPED = "skipped"
    CANCELLED = "cancelled"
    ESCALATED = "escalated"


class Risk(str, Enum):
    LOW = "low"
    MEDIUM = "medium"
    HIGH = "high"


@dataclass(frozen=True)
class Budget:
    max_seconds: float = 600.0
    max_retries: int = 2
    max_tokens: int = 200_000
    backoff_base: float = 2.0

    def retry_delay(self, attempt: int) -> float:
        return min(self.backoff_base ** attempt, 60.0)


@dataclass(frozen=True)
class AgentSpec:
    backend: str = "opencode"
    model: str = ""
    command: tuple[str, ...] = ()
    system_hint: str = ""
    env: dict[str, str] = field(default_factory=dict)


@dataclass(frozen=True)
class Task:
    title: str
    instructions: str
    deps: tuple[str, ...] = ()
    files_hint: tuple[str, ...] = ()
    risk: Risk = Risk.MEDIUM
    budget: Budget = field(default_factory=Budget)
    agent: AgentSpec = field(default_factory=AgentSpec)
    verify: tuple[str, ...] = ("diff", "tests", "architecture")
    id: str = ""

    def finalize(self) -> "Task":
        if self.id:
            return self
        payload = json.dumps({
            "title": self.title,
            "instructions": self.instructions,
            "deps": sorted(self.deps),
            "files": sorted(self.files_hint),
            "backend": self.agent.backend,
        }, sort_keys=True).encode()
        tid = hashlib.blake2b(payload, digest_size=8).hexdigest()
        return Task(**{**{f: getattr(self, f)
                           for f in self.__dataclass_fields__},
                       "id": tid})


@dataclass(frozen=True)
class Plan:
    goal: str
    tasks: tuple[Task, ...]
    repo: str = "."
    base_branch: str = "main"
    created_at: float = field(default_factory=time.time)

    @property
    def id(self) -> str:
        return hashlib.blake2b(
            (self.goal + "".join(t.id for t in self.tasks)).encode(),
            digest_size=8).hexdigest()

    def validate(self) -> None:
        ids = {t.id for t in self.tasks}
        if len(ids) != len(self.tasks):
            raise ValueError("duplicate task ids in plan")
        for t in self.tasks:
            for d in t.deps:
                if d not in ids:
                    raise ValueError(
                        f"task {t.id} depends on unknown task {d}")
        graph = {t.id: set(t.deps) for t in self.tasks}
        WHITE, GRAY, BLACK = 0, 1, 2
        color = {tid: WHITE for tid in graph}

        def visit(node: str) -> None:
            if color[node] == GRAY:
                raise ValueError(
                    f"dependency cycle involving task {node}")
            if color[node] == BLACK:
                return
            color[node] = GRAY
            for dep in graph[node]:
                visit(dep)
            color[node] = BLACK

        for tid in graph:
            visit(tid)


@dataclass(frozen=True)
class Verdict:
    passed: bool
    gates: dict[str, Any] = field(default_factory=dict)
    summary: str = ""


@dataclass
class TaskOutcome:
    task_id: str
    state: TaskState
    attempts: int = 0
    worktree: str = ""
    branch: str = ""
    verdict: Verdict | None = None
    error: str = ""
    seconds: float = 0.0


@dataclass
class RunResult:
    plan_id: str
    goal: str
    outcomes: dict[str, TaskOutcome]
    started_at: float
    finished_at: float

    @property
    def ok(self) -> bool:
        return all(
            o.state in (TaskState.DONE, TaskState.SKIPPED)
            for o in self.outcomes.values()
        ) and any(o.state == TaskState.DONE for o in self.outcomes.values())

    def to_json(self) -> str:
        def enc(obj: Any) -> Any:
            if isinstance(obj, Enum):
                return obj.value
            if hasattr(obj, "__dataclass_fields__"):
                return {k: enc(v) for k, v in obj.__dict__.items()}
            if isinstance(obj, dict):
                return {k: enc(v) for k, v in obj.items()}
            if isinstance(obj, (list, tuple)):
                return [enc(v) for v in obj]
            return obj

        return json.dumps(enc(self.__dict__), indent=2)
