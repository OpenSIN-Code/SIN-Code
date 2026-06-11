# SPDX-License-Identifier: MIT
"""Plan Synthesizer — Goal -> validated step-spec DAG with critique pass."""

from __future__ import annotations

import json
import re
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Awaitable, Callable

from .memory_bridge import MemoryBridge
from .planner import Planner
from .types import AgentTask

CompleteFn = Callable[[str], Awaitable[str]]

_JSON_BLOCK = re.compile(r"\[[\s\S]*\]")
_ALLOWED_TOOLS = {"sin_search", "sin_read", "sin_edit",
                  "sin_write", "sin_bash", "sin_delegate"}
_MAX_STEPS = 24


def _get_default_distiller():
    """Lazy import to avoid a hard dependency in tests that don't need it."""
    try:
        from .distiller import KnowledgeDistiller
        return KnowledgeDistiller()
    except Exception:
        return None


_KnowledgeDistiller = None  # type alias for forward ref


@dataclass(slots=True)
class RepoSurvey:
    languages: dict[str, int] = field(default_factory=dict)
    test_runner: str | None = None
    lint_tool: str | None = None
    package_files: list[str] = field(default_factory=list)
    top_dirs: list[str] = field(default_factory=list)

    def to_prompt_block(self) -> str:
        return json.dumps({
            "languages": self.languages,
            "test_runner": self.test_runner,
            "lint_tool": self.lint_tool,
            "package_files": self.package_files,
            "top_dirs": self.top_dirs,
        }, ensure_ascii=False)


_EXT_LANG = {".py": "python", ".ts": "typescript", ".tsx": "typescript",
             ".js": "javascript", ".rs": "rust", ".go": "go", ".java": "java"}


def survey_repo(repo_root: str, *, max_files: int = 4000) -> RepoSurvey:
    root = Path(repo_root)
    s = RepoSurvey()
    seen = 0
    if not root.exists():
        return s
    for f in root.rglob("*"):
        if seen >= max_files:
            break
        parts = f.parts
        if ".git" in parts or "node_modules" in parts or ".venv" in parts:
            continue
        if f.is_file():
            seen += 1
            lang = _EXT_LANG.get(f.suffix)
            if lang:
                s.languages[lang] = s.languages.get(lang, 0) + 1
    for name in ("pyproject.toml", "package.json", "Cargo.toml", "go.mod"):
        if (root / name).exists():
            s.package_files.append(name)
    if (root / "pyproject.toml").exists():
        text = (root / "pyproject.toml").read_text(encoding="utf-8",
                                                   errors="replace")
        if "pytest" in text:
            s.test_runner = "pytest"
        if "ruff" in text:
            s.lint_tool = "ruff"
    if (root / "package.json").exists():
        try:
            pkg = json.loads((root / "package.json").read_text(encoding="utf-8"))
            scripts = pkg.get("scripts", {})
            if "test" in scripts:
                s.test_runner = f"npm test ({scripts['test'][:60]})"
            if "lint" in scripts:
                s.lint_tool = f"npm run lint ({scripts['lint'][:60]})"
        except json.JSONDecodeError:
            pass
    try:
        s.top_dirs = sorted(
            d.name for d in root.iterdir()
            if d.is_dir() and d.name not in {".git", "node_modules", ".venv"}
        )[:20]
    except OSError:
        pass
    return s


_DRAFT_PROMPT = """\
You are a planning agent. Produce a step-spec DAG as a JSON array.
No prose, no markdown fences, JSON only.

Each step: {{"step_id": str, "title": str, "tool": str, "args": dict,
"deps": [str], "estimated_cost": number, "isolated": bool}}

Available tools:
- sin_search: {{"pattern": regex, "glob": str}} — locate code FIRST
- sin_read:   {{"path": str, "start": int, "limit": int}} — read before edit
- sin_edit:   {{"path": str, "old": exact-anchor, "new": replacement}}
- sin_write:  {{"path": str, "content": str}} — new files only
- sin_bash:   {{"cmd": str}}
- sin_delegate: {{"goal": str, "steps": [...]}}

Hard rules:
- Max {max_steps} steps. Each step atomic and independently verifiable.
- Exploration (sin_search/sin_read) BEFORE any edit step.
- Two steps writing the same file MUST have a dependency edge.
- Final step MUST be a verification (sin_bash test run).
- estimated_cost: 1 for reads, 3 for edits, 5+ for test runs.

REPO FACTS: {survey}
LESSONS FROM PRIOR SIMILAR RUNS: {lessons}
CONSTRAINTS: {constraints}

GOAL: {goal}
"""

_CRITIQUE_PROMPT = """\
You are a plan reviewer. Check the DAG against this checklist and output
the CORRECTED JSON array (or identical if correct). JSON only.

Checklist:
1. Final step is a verification?
2. Every sin_edit preceded (via deps) by a sin_read/sin_search of same area?
3. No two steps write the same file without a dependency edge?
4. No step does more than one logical change?
5. step_ids unique, deps reference existing step_ids, no cycles?

You may REORDER, ADD DEPS, SPLIT steps, ADD one missing verification step.
You must NOT add new feature work or change the goal.

GOAL: {goal}
PLAN:
{plan}
"""


class PlanSynthesizer:
    def __init__(self, complete: CompleteFn | None = None, *,
                 memory: MemoryBridge | None = None,
                 distiller: "KnowledgeDistiller | None" = None,
                 critique: bool = True) -> None:
        self.complete = complete
        self.memory = memory or MemoryBridge()
        self.distiller = distiller or _get_default_distiller()
        self.critique = critique
        self.planner = Planner()

    async def synthesize(self, task: AgentTask) -> list[dict[str, Any]]:
        if self.complete is None:
            raise RuntimeError(
                "PlanSynthesizer requires a complete() callable — "
                "no LLM configured, refusing to hallucinate a plan."
            )

        lessons = self.memory.recall_similar(task.goal, limit=5)
        lesson_text = "; ".join(
            line for hit in lessons for line in hit["lessons"]
        )[:1500] or "none"

        survey = survey_repo(task.repo_root)

        standing = self.distiller.render_constraints()
        constraints_block = "; ".join(task.constraints) or "none"
        if standing:
            constraints_block = f"{constraints_block}\n{standing}"

        draft_raw = await self.complete(_DRAFT_PROMPT.format(
            max_steps=_MAX_STEPS,
            survey=survey.to_prompt_block(),
            lessons=lesson_text,
            constraints=constraints_block,
            goal=task.goal,
        ))
        specs = self._parse_and_validate(task, draft_raw)

        if self.critique and specs:
            critiqued_raw = await self.complete(_CRITIQUE_PROMPT.format(
                goal=task.goal,
                plan=json.dumps(specs, ensure_ascii=False, indent=2),
            ))
            critiqued = self._parse_and_validate(task, critiqued_raw,
                                                 fallback=specs)
            specs = critiqued

        if not specs:
            raise ValueError("synthesizer produced no valid plan")
        return specs

    def _parse_and_validate(
        self, task: AgentTask, raw: str,
        fallback: list[dict[str, Any]] | None = None,
    ) -> list[dict[str, Any]]:
        match = _JSON_BLOCK.search(raw)
        if not match:
            return fallback or []
        try:
            specs = json.loads(match.group(0))
        except json.JSONDecodeError:
            return fallback or []
        if not isinstance(specs, list):
            return fallback or []

        cleaned: list[dict[str, Any]] = []
        seen_ids: set[str] = set()
        for spec in specs[:_MAX_STEPS]:
            if not isinstance(spec, dict):
                continue
            sid = spec.get("step_id")
            if (not isinstance(sid, str) or sid in seen_ids
                    or spec.get("tool") not in _ALLOWED_TOOLS
                    or not isinstance(spec.get("args"), dict)):
                continue
            seen_ids.add(sid)
            cleaned.append({
                "step_id": sid,
                "title": str(spec.get("title", sid))[:120],
                "tool": spec["tool"],
                "args": spec["args"],
                "deps": [d for d in spec.get("deps", [])
                         if isinstance(d, str)],
                "estimated_cost": float(spec.get("estimated_cost", 1.0)),
                "isolated": bool(spec.get("isolated", False)),
            })
        ids = {s["step_id"] for s in cleaned}
        for s in cleaned:
            s["deps"] = [d for d in s["deps"] if d in ids]
        try:
            self.planner.build(task, cleaned)
        except ValueError:
            return fallback or []
        return cleaned
