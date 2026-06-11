# SPDX-License-Identifier: MIT
"""LLM-based Goal → validated Plan JSON.

Three stages:
1. REPO-RECON: deterministic (git ls-files, language/marker detection)
2. DECOMPOSE:  LLM produces a draft against a strict JSON schema
3. CRITIQUE:   second LLM pass reviews the plan (write conflicts,
               missing deps, risk under-statement) and repairs

Backend-agnostic: uses the same runner mechanism as the sub-agents.
"""

from __future__ import annotations

import json
import re
from collections import Counter
from pathlib import Path
from typing import Any

from . import memory
from .models import AgentSpec, Plan, Task
from .planfile import plan_from_dict
from .runner import runner_for

_LANG_BY_EXT = {
    ".py": "python", ".ts": "typescript", ".tsx": "typescript",
    ".js": "javascript", ".go": "go", ".rs": "rust", ".java": "java",
    ".rb": "ruby", ".php": "php", ".cs": "csharp", ".vue": "vue",
}

_MARKERS = {
    "pyproject.toml": "python project (pyproject)",
    "package.json": "node project",
    "go.mod": "go module",
    "Cargo.toml": "rust crate",
    "next.config.mjs": "Next.js app",
    "next.config.js": "Next.js app",
    "vite.config.ts": "Vite app",
    "docker-compose.yml": "docker compose",
    ".github/workflows": "GitHub Actions CI",
}


def recon(repo: str | Path, max_files: int = 400) -> dict:
    """Deterministic repo facts. No LLM, no guessing."""
    root = Path(repo).resolve()
    if not root.exists():
        return {"languages": {}, "markers": [], "has_tests": False,
                "top_dirs": [], "file_sample": []}
    try:
        out = subprocess_run_git_ls_files(root)
    except Exception:
        out = ""
    files = [l for l in out.splitlines() if l][:max_files * 4]
    if not files:
        for p in root.rglob("*"):
            if p.is_file():
                try:
                    rel = p.relative_to(root).as_posix()
                except ValueError:
                    rel = str(p)
                files.append(rel)
                if len(files) >= max_files * 4:
                    break

    langs = Counter(
        _LANG_BY_EXT[Path(f).suffix] for f in files
        if Path(f).suffix in _LANG_BY_EXT
    )
    markers = [desc for marker, desc in _MARKERS.items()
               if (root / marker).exists()]
    has_tests = any(
        re.search(r"(^|/)(tests?|__tests__|spec)/", f) or
        re.search(r"(test_.*\.py|.*\.test\.[jt]sx?)$", f)
        for f in files
    )
    try:
        top_dirs = Counter(f.split("/")[0] for f in files if "/" in f)
        top = [d for d, _ in top_dirs.most_common(12)]
    except Exception:
        top = []
    return {
        "languages": dict(langs.most_common(3)),
        "markers": markers,
        "has_tests": has_tests,
        "top_dirs": top,
        "file_sample": files[:max_files],
    }


def subprocess_run_git_ls_files(root: Path) -> str:
    import subprocess
    return subprocess.run(
        ["git", "-C", str(root), "ls-files"],
        capture_output=True, text=True, timeout=30,
    ).stdout


_DRAFT_PROMPT = """\
You are a planning agent. Produce a step-spec DAG as a JSON array.
No prose, no markdown fences, JSON only.

Each step: {"step_id": str, "title": str, "tool": str, "args": dict,
"deps": [str], "estimated_cost": number, "isolated": bool}}

Available tools: sin_search, sin_read, sin_edit, sin_write, sin_bash,
sin_delegate.

Hard rules:
- Max 8 steps. Each atomic and independently verifiable.
- Exploration (sin_search/sin_read) BEFORE any edit step.
- Two steps writing the same file MUST have a deps edge.
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
2. Every sin_edit preceded (via deps) by a sin_read/sin_search?
3. No two steps write the same file without a deps edge?
4. No step does more than one logical change?
5. step_ids unique, deps reference existing, no cycles?

You may REORDER, ADD DEPS, SPLIT steps, ADD one missing verification step.
You must NOT add new feature work or change the goal.

GOAL: {goal}
PLAN:
{plan}
"""


def _extract_json(text: str) -> dict:
    """Robust: find outermost { } even in noisy output."""
    text = re.sub(r"```(?:json)?", "", text)
    start = text.find("{")
    if start < 0:
        raise ValueError("planner output contains no JSON object")
    depth = 0
    in_string = False
    escape = False
    for i in range(start, len(text)):
        ch = text[i]
        if escape:
            escape = False
            continue
        if ch == "\\":
            escape = True
            continue
        if ch == '"':
            in_string = not in_string
            continue
        if in_string:
            continue
        if ch == "{":
            depth += 1
        elif ch == "}":
            depth -= 1
            if depth == 0:
                return json.loads(text[start:i + 1])
    raise ValueError("unbalanced JSON in planner output")


class Planner:
    def __init__(self, backend: str = "opencode", model: str = "") -> None:
        self.spec = AgentSpec(backend=backend, model=model)

    async def plan(self, goal: str, repo: str = ".",
                   critique: bool = True) -> Plan:
        if self.spec.backend == "echo" or self.spec.model == "_stub_":
            # Deterministic path for tests: synthesize a minimal plan
            t = Task(title="stub", instructions=goal,
                     agent=AgentSpec(backend="echo", model="")).finalize()
            return plan_from_dict(
                {"goal": goal, "tasks": [
                    {"key": t.id, "title": t.title,
                     "instructions": t.instructions}]},
                repo=repo)

        facts = recon(repo)
        pitfalls = memory.recall_pitfalls(goal)
        prompt = "\n\n".join(filter(None, [
            f"Ziel: {goal}",
            "Repo-Fakten (deterministisch ermittelt, NICHT anzweifeln):\n"
            + json.dumps({k: v for k, v in facts.items()
                          if k != "file_sample"}, indent=2),
            "Relevante Dateien (Auszug):\n"
            + "\n".join(facts["file_sample"][:120]),
            ("Bekannte Pitfalls aus früheren Runs:\n- "
             + "\n- ".join(pitfalls)) if pitfalls else "",
            _DRAFT_PROMPT,
        ]))

        try:
            draft = await self._ask(prompt, repo)
        except Exception:
            # Backend unavailable — return a minimal valid plan
            t = Task(title="fallback", instructions=goal,
                     agent=self.spec).finalize()
            return plan_from_dict(
                {"goal": goal, "tasks": [
                    {"key": t.id, "title": t.title,
                     "instructions": t.instructions}]},
                repo=repo)

        if critique and isinstance(draft, dict) and "tasks" in draft:
            try:
                revised = await self._ask(
                    _CRITIQUE_PROMPT.format(
                        goal=goal,
                        plan=json.dumps(draft, indent=2, ensure_ascii=False)),
                    repo)
                if isinstance(revised, dict) and revised.get("tasks"):
                    draft = revised
            except Exception:
                pass

        try:
            return plan_from_dict(draft, repo=repo)
        except Exception:
            t = Task(title="fallback", instructions=goal,
                     agent=self.spec).finalize()
            return plan_from_dict(
                {"goal": goal, "tasks": [
                    {"key": t.id, "title": t.title,
                     "instructions": t.instructions}]},
                repo=repo)

    async def _ask(self, prompt: str, repo: str) -> dict:
        task = Task(title="plan", instructions=prompt,
                    agent=self.spec).finalize()
        res = await runner_for(self.spec).run(task, cwd=repo, timeout=300)
        if not res.ok:
            raise RuntimeError(
                f"planner backend failed: {res.output[-500:]}")
        return _extract_json(res.output)


def plan_sync(goal: str, repo: str = ".", backend: str = "opencode",
              model: str = "", critique: bool = True) -> Plan:
    import asyncio
    return asyncio.run(Planner(backend, model).plan(goal, repo, critique))
