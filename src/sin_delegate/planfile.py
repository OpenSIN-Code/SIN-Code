# SPDX-License-Identifier: MIT
"""Plan file loader: declarative JSON plans → validated Plan objects.

Example plan.json:
{
  "goal": "Add OAuth login",
  "base_branch": "main",
  "tasks": [
    {"key": "schema", "title": "DB schema", "instructions": "...",
     "risk": "high", "verify": ["diff", "tests"]},
    {"key": "api", "title": "API routes", "instructions": "...",
     "deps": ["schema"], "backend": "claude", "model": "claude-sonnet-4-5"}
  ]
}

Human-friendly `key`s are resolved to content-addressed ids automatically
in two passes (first to assign ids, then to rewrite deps).
"""

from __future__ import annotations

import json
from pathlib import Path

from .models import AgentSpec, Budget, Plan, Risk, Task


def load_plan(path: str | Path, repo: str = ".") -> Plan:
    data = json.loads(Path(path).read_text())
    return plan_from_dict(data, repo=repo)


def plan_from_dict(data: dict, repo: str = ".") -> Plan:
    raw_tasks = data.get("tasks", [])
    if not raw_tasks:
        raise ValueError("plan has no tasks")

    # pass 1: build tasks without deps to obtain ids per human key.
    # If a task already has an explicit "id" (multirepo pre-resolved),
    # use it as both the draft and the final id.
    drafts: dict[str, Task] = {}
    for rt in raw_tasks:
        key = rt.get("key") or rt["title"]
        draft = _task_from(rt, deps=())
        if rt.get("id"):
            draft = Task(**{**{f: getattr(draft, f)
                                for f in draft.__dataclass_fields__},
                            "id": rt["id"]})
        drafts[key] = draft

    key_to_final_id = {k: t.id or t.finalize().id
                       for k, t in drafts.items()}

    # pass 2: rebuild with resolved dep ids. If a dep already IS a task
    # id (not a human key), use it directly.
    tasks: list[Task] = []
    for rt in raw_tasks:
        dep_keys = rt.get("deps", [])
        resolved: list = []
        for d in dep_keys:
            if d in key_to_final_id:
                resolved.append(key_to_final_id[d])
            else:
                # assume it's already a task id
                resolved.append(d)
        tasks.append(_task_from(rt, deps=tuple(resolved)).finalize())

    plan = Plan(
        goal=data["goal"],
        tasks=tuple(tasks),
        repo=str(Path(repo).resolve()),
        base_branch=data.get("base_branch", "main"),
    )
    plan.validate()
    return plan


def _task_from(rt: dict, deps: tuple) -> Task:
    budget = Budget(
        max_seconds=float(rt.get("max_seconds", 600)),
        max_retries=int(rt.get("max_retries", 2)),
        max_tokens=int(rt.get("max_tokens", 200_000)),
    )
    agent = AgentSpec(
        backend=rt.get("backend", "opencode"),
        model=rt.get("model", ""),
        command=tuple(rt.get("command", [])),
        system_hint=rt.get("system_hint", ""),
    )
    return Task(
        title=rt["title"],
        instructions=rt.get("instructions", rt["title"]),
        deps=deps,
        files_hint=tuple(rt.get("files", [])),
        risk=Risk(rt.get("risk", "medium")),
        budget=budget,
        agent=agent,
        verify=tuple(rt.get("verify", ["diff", "tests", "architecture"])),
    )
