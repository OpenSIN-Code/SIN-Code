# SPDX-License-Identifier: MIT
"""Verification gates — diff screen, tests, architecture.

Graceful degradation: a missing optional tool yields ok=True with
detail="skipped (<reason>)" — it never blocks, but the verdict is honest
about what was actually checked.
"""

from __future__ import annotations

import re
import shutil
import subprocess
from pathlib import Path
from typing import Callable

from .models import Task, Verdict
from .worktree import Worktree

Gate = Callable[[Task, Worktree], tuple[bool, str]]

_FORBIDDEN_DIFF = [
    (re.compile(r"(?i)^\+.*\b(api[_-]?key|secret|password)\s*=\s*['\"][^'\"]{8,}"),
     "hardcoded secret introduced"),
    (re.compile(r"^\+.*\beval\s*\("), "eval() introduced"),
    (re.compile(r"^\+.*\bexec\s*\("), "exec() introduced"),
]


def gate_diff(task: Task, wt: Worktree) -> tuple[bool, str]:
    """Cheap, always-on static screen of the produced diff."""
    diff = wt.diff()
    if not diff.strip():
        return False, "agent produced an empty diff"
    for pat, why in _FORBIDDEN_DIFF:
        for line in diff.splitlines():
            if pat.search(line):
                return False, f"forbidden pattern: {why}"
    changed = wt.diff_stat().splitlines()
    return True, f"{max(len(changed) - 1, 0)} file(s) changed, no forbidden patterns"


def _run(cmd: list[str], cwd: str, timeout: int = 300) -> tuple[int, str]:
    try:
        p = subprocess.run(
            cmd, cwd=cwd, capture_output=True, text=True, timeout=timeout)
        return p.returncode, (p.stdout + p.stderr)[-5000:]
    except subprocess.TimeoutExpired:
        return -1, "timeout"
    except FileNotFoundError:
        return -2, "tool not installed"


def gate_tests(task: Task, wt: Worktree) -> tuple[bool, str]:
    """Project-aware test gate: pytest > go test > npm test, first match wins."""
    cwd = str(wt.path)
    has_pyproject = (wt.path / "pyproject.toml").exists()
    has_pytest_files = bool(list(wt.path.glob("**/test_*.py")))
    if has_pyproject or has_pytest_files:
        code, out = _run(["python", "-m", "pytest", "-x", "-q"], cwd)
        if code == -2:
            return True, "skipped (pytest not installed)"
        if code == 5:
            return True, "skipped (no tests collected)"
        if code == 0:
            return True, "tests passed"
        return False, out.splitlines()[-1] if out else f"exit {code}"
    if (wt.path / "go.mod").exists():
        code, out = _run(["go", "test", "./..."], cwd)
        if code == -2:
            return True, "skipped (go missing)"
        if code == 0:
            return True, "go tests passed"
        return False, out[-500:]
    if (wt.path / "package.json").exists():
        code, out = _run(["npm", "test", "--silent"], cwd, timeout=600)
        if code == -2:
            return True, "skipped (npm missing)"
        if code == 0:
            return True, "npm tests passed"
        return False, out[-500:]
    return True, "skipped (no recognized test setup)"


def gate_architecture(task: Task, wt: Worktree) -> tuple[bool, str]:
    """Delegate to the SIN-Code ADW pre-flight if the `sin` CLI is available."""
    if not shutil.which("sin"):
        return True, "skipped (sin CLI not installed)"
    code, out = _run(["sin", "debt", "."], str(wt.path), timeout=120)
    if code in (-1, -2):
        return True, f"skipped ({out})"
    if code == 0:
        return True, "architecture clean"
    return False, out.splitlines()[-1] if out else f"exit {code}"


_GATES: dict[str, Gate] = {
    "diff": gate_diff,
    "tests": gate_tests,
    "architecture": gate_architecture,
}


def register_gate(name: str, fn: Gate) -> None:
    _GATES[name] = fn


def verify(task: Task, wt: Worktree) -> Verdict:
    results: dict[str, dict] = {}
    all_ok = True
    for name in task.verify:
        gate = _GATES.get(name)
        if gate is None:
            results[name] = {"ok": True, "detail": "skipped (unknown gate)"}
            continue
        try:
            ok, detail = gate(task, wt)
        except Exception as e:
            ok, detail = False, f"gate crashed: {e}"
        results[name] = {"ok": ok, "detail": detail}
        all_ok = all_ok and ok
    failed = [n for n, r in results.items() if not r["ok"]]
    summary = ("all gates passed" if all_ok
               else f"failed gates: {', '.join(failed)}")
    return Verdict(passed=all_ok, gates=results, summary=summary)
