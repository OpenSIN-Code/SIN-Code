# SPDX-License-Identifier: MIT
"""Repair factories — turn a failed Verdict into a new repair plan.

Two tiers, cheapest first:
1. DeterministicRepair — known failure classes get rule-based fixes.
2. LLMRepairFactory — falls back to a model with strict JSON schema.
"""

from __future__ import annotations

import json
import re
from typing import Any, Awaitable, Callable

from .types import AgentTask, Verdict, VerdictKind

CompleteFn = Callable[[str], Awaitable[str]]

_JSON_BLOCK = re.compile(r"\[[\s\S]*\]")
_ALLOWED_TOOLS = {"sin_search", "sin_read", "sin_edit",
                  "sin_write", "sin_bash", "sin_delegate"}

_DETERMINISTIC_FIXES: dict[VerdictKind, list[dict[str, Any]]] = {
    VerdictKind.FAIL_LINT: [
        {
            "step_id": "autofix-lint",
            "title": "Run lint autofix",
            "tool": "sin_bash",
            "args": {"cmd": "ruff check . --fix && ruff format ."},
            "estimated_cost": 1,
        },
    ],
}

_REPAIR_PROMPT = """\
You are a repair agent. A verification gate failed. Produce a MINIMAL repair
plan as a JSON array of step specs. No prose, no markdown fences, JSON only.

Each step spec: {{"step_id": str, "title": str, "tool": str, "args": dict,
"deps": [str], "estimated_cost": number}}

Available tools:
- sin_search: {{"pattern": regex, "glob": str}}
- sin_read:   {{"path": str, "start": int, "limit": int}}
- sin_edit:   {{"path": str, "old": exact-anchor, "new": replacement}}
- sin_write:  {{"path": str, "content": str}}
- sin_bash:   {{"cmd": str}}
- sin_delegate: {{"goal": str, "steps": [...]}}

Rules:
- NEVER weaken tests, disable lint rules, or bypass architecture checks.
- Prefer sin_edit over sin_write.
- End the plan with a verification step that re-runs the failing check.
- Maximum 6 steps.

GOAL: {goal}
CONSTRAINTS: {constraints}
FAILURE KIND: {kind}
REPAIR HINT: {hint}
FAILURE OUTPUT (truncated):
{detail}
"""


class DeterministicRepair:
    def plan_for(self, verdict: Verdict) -> list[dict[str, Any]] | None:
        return _DETERMINISTIC_FIXES.get(verdict.kind)


class LLMRepairFactory:
    def __init__(self, complete: CompleteFn | None = None,
                 *, max_detail_chars: int = 6000) -> None:
        self.complete = complete
        self.deterministic = DeterministicRepair()
        self.max_detail_chars = max_detail_chars
        self._attempted_deterministic: set[VerdictKind] = set()

    async def build_repair_plan(
        self, task: AgentTask, verdict: Verdict
    ) -> list[dict[str, Any]]:
        if verdict.kind not in self._attempted_deterministic:
            fix = self.deterministic.plan_for(verdict)
            if fix:
                self._attempted_deterministic.add(verdict.kind)
                return fix
        if self.complete is None:
            return []
        prompt = _REPAIR_PROMPT.format(
            goal=task.goal,
            constraints="; ".join(task.constraints) or "none",
            kind=verdict.kind.value,
            hint=verdict.repair_hint or "none",
            detail=verdict.detail[-self.max_detail_chars:],
        )
        raw = await self.complete(prompt)
        return self._parse_specs(raw)

    @staticmethod
    def _parse_specs(raw: str) -> list[dict[str, Any]]:
        match = _JSON_BLOCK.search(raw)
        if not match:
            return []
        try:
            specs = json.loads(match.group(0))
        except json.JSONDecodeError:
            return []
        if not isinstance(specs, list):
            return []
        valid: list[dict[str, Any]] = []
        for spec in specs[:6]:
            if (
                isinstance(spec, dict)
                and isinstance(spec.get("step_id"), str)
                and spec.get("tool") in _ALLOWED_TOOLS
                and isinstance(spec.get("args"), dict)
            ):
                valid.append(spec)
        return valid
