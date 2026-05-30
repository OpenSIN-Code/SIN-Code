"""SWE-bench-style A/B evaluation harness for the SIN-Code Bundle.

Goal: produce an objective, reproducible number that answers
"do the SIN tools (impact / semantic_diff / verify / oracle) actually improve
an agent's pass-rate?"

Design
------
- Loads a task set (SWE-bench Lite subset by default, or a local JSONL file).
- Runs each task twice through a pluggable agent runner:
    * arm "control"  -> SIN tools DISABLED  (SIN_ENFORCE=0)
    * arm "sin"      -> SIN tools ENABLED   (SIN_ENFORCE=1)
- Applies the produced patch in an isolated git worktree and runs the task's
  FAIL_TO_PASS / PASS_TO_PASS tests.
- Reports resolved-rate per arm, the delta, and a per-task breakdown.

The harness is intentionally runner-agnostic: you wire in opencode / codex /
hermes via a small AgentRunner. A DryRunRunner is included so `sin bench`
works end-to-end without any LLM credits.
"""

from __future__ import annotations

import json
import statistics
import subprocess
import tempfile
import time
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Callable, Iterable, Literal, Optional, Protocol

Arm = Literal["control", "sin"]


# --------------------------------------------------------------------------- #
# Task + result models
# --------------------------------------------------------------------------- #
@dataclass(frozen=True)
class Task:
    """One benchmark instance (SWE-bench compatible subset of fields)."""

    instance_id: str
    repo: str
    base_commit: str
    problem_statement: str
    fail_to_pass: list[str] = field(default_factory=list)
    pass_to_pass: list[str] = field(default_factory=list)
    setup_cmds: list[str] = field(default_factory=list)
    test_cmd: str = "pytest -q"


@dataclass
class TaskResult:
    instance_id: str
    arm: Arm
    resolved: bool
    duration_s: float
    patch_applied: bool
    fail_to_pass_passed: int
    fail_to_pass_total: int
    error: Optional[str] = None


@dataclass
class ArmSummary:
    arm: Arm
    total: int
    resolved: int
    resolved_rate: float
    mean_duration_s: float


@dataclass
class BenchReport:
    arms: dict[str, ArmSummary]
    delta_resolved_rate: float
    per_task: list[TaskResult]
    started_at: str
    finished_at: str

    def to_json(self) -> str:
        return json.dumps(
            {
                "arms": {k: asdict(v) for k, v in self.arms.items()},
                "delta_resolved_rate": self.delta_resolved_rate,
                "per_task": [asdict(r) for r in self.per_task],
                "started_at": self.started_at,
                "finished_at": self.finished_at,
            },
            indent=2,
        )


# --------------------------------------------------------------------------- #
# Agent runner protocol
# --------------------------------------------------------------------------- #
class AgentRunner(Protocol):
    """Produces a unified diff that attempts to solve `task` inside `workdir`.

    `sin_enabled` tells the runner whether to expose the SIN MCP tools to the
    underlying agent. Implementations should return a unified-diff string (may
    be empty if the agent produced no change).
    """

    def run(self, task: Task, workdir: Path, sin_enabled: bool) -> str: ...


class DryRunRunner:
    """Zero-cost runner for smoke-testing the harness itself.

    Produces no patch, so every task "fails" — but exercises the full
    clone/apply/test pipeline so you can validate without an LLM.
    """

    def run(self, task: Task, workdir: Path, sin_enabled: bool) -> str:  # noqa: ARG002
        return ""


class CommandRunner:
    """Runs an external agent CLI and captures the diff it leaves in the repo.

    Example wiring for opencode:
        CommandRunner(
            build_cmd=lambda task, sin: [
                "opencode", "run",
                "-m", task.problem_statement,
            ],
        )
    """

    def __init__(
        self,
        build_cmd: Callable[[Task, bool], list[str]],
        timeout_s: int = 1800,
        env_for: Optional[Callable[[Task, bool], dict[str, str]]] = None,
    ) -> None:
        self._build_cmd = build_cmd
        self._timeout_s = timeout_s
        self._env_for = env_for

    def run(self, task: Task, workdir: Path, sin_enabled: bool) -> str:
        import os

        cmd = self._build_cmd(task, sin_enabled)
        env = {**os.environ}
        if self._env_for:
            env.update(self._env_for(task, sin_enabled))
        env["SIN_ENFORCE"] = "1" if sin_enabled else "0"

        subprocess.run(
            cmd,
            cwd=workdir,
            env=env,
            timeout=self._timeout_s,
            check=False,
            capture_output=True,
            text=True,
        )
        diff = subprocess.run(
            ["git", "diff"],
            cwd=workdir,
            check=False,
            capture_output=True,
            text=True,
        )
        return diff.stdout


# --------------------------------------------------------------------------- #
# Git / test plumbing
# --------------------------------------------------------------------------- #
def _sh(cmd: list[str], cwd: Path, timeout: int = 600) -> subprocess.CompletedProcess:
    return subprocess.run(
        cmd, cwd=cwd, check=False, capture_output=True, text=True, timeout=timeout
    )


def _prepare_worktree(task: Task, root: Path) -> Path:
    work = root / task.instance_id.replace("/", "__")
    work.mkdir(parents=True, exist_ok=True)
    url = f"https://github.com/{task.repo}.git"
    _sh(["git", "clone", "--quiet", url, "."], cwd=work, timeout=900)
    _sh(["git", "checkout", "--quiet", task.base_commit], cwd=work)
    for cmd in task.setup_cmds:
        _sh(["bash", "-lc", cmd], cwd=work, timeout=1800)
    return work


def _apply_patch(diff: str, work: Path) -> bool:
    if not diff.strip():
        return False
    patch = work / ".sin_patch.diff"
    patch.write_text(diff, encoding="utf-8")
    res = _sh(["git", "apply", "--whitespace=nowarn", str(patch)], cwd=work)
    return res.returncode == 0


def _run_named_tests(work: Path, task: Task) -> tuple[int, int]:
    if not task.fail_to_pass:
        res = _sh(["bash", "-lc", task.test_cmd], cwd=work, timeout=1800)
        return (1, 1) if res.returncode == 0 else (0, 1)

    passed = 0
    for test_id in task.fail_to_pass:
        res = _sh(
            ["bash", "-lc", f"{task.test_cmd} {test_id}"],
            cwd=work,
            timeout=900,
        )
        if res.returncode == 0:
            passed += 1
    return passed, len(task.fail_to_pass)


# --------------------------------------------------------------------------- #
# Core eval loop
# --------------------------------------------------------------------------- #
def _eval_one(task: Task, arm: Arm, runner: AgentRunner, root: Path) -> TaskResult:
    start = time.time()
    try:
        work = _prepare_worktree(task, root)
        diff = runner.run(task, work, sin_enabled=(arm == "sin"))
        applied = _apply_patch(diff, work)
        passed, total = (0, len(task.fail_to_pass) or 1)
        if applied:
            passed, total = _run_named_tests(work, task)
        resolved = applied and passed == total and total > 0
        return TaskResult(
            instance_id=task.instance_id,
            arm=arm,
            resolved=resolved,
            duration_s=round(time.time() - start, 2),
            patch_applied=applied,
            fail_to_pass_passed=passed,
            fail_to_pass_total=total,
        )
    except Exception as exc:  # noqa: BLE001
        return TaskResult(
            instance_id=task.instance_id,
            arm=arm,
            resolved=False,
            duration_s=round(time.time() - start, 2),
            patch_applied=False,
            fail_to_pass_passed=0,
            fail_to_pass_total=len(task.fail_to_pass) or 1,
            error=str(exc),
        )


def _summarize(arm: Arm, results: list[TaskResult]) -> ArmSummary:
    subset = [r for r in results if r.arm == arm]
    total = len(subset)
    resolved = sum(1 for r in subset if r.resolved)
    rate = (resolved / total) if total else 0.0
    mean_dur = statistics.mean([r.duration_s for r in subset]) if subset else 0.0
    return ArmSummary(
        arm=arm,
        total=total,
        resolved=resolved,
        resolved_rate=round(rate, 4),
        mean_duration_s=round(mean_dur, 2),
    )


def run_benchmark(
    tasks: Iterable[Task],
    runner: AgentRunner,
    arms: tuple[Arm, ...] = ("control", "sin"),
    workspace: Optional[Path] = None,
) -> BenchReport:
    started = time.strftime("%Y-%m-%dT%H:%M:%S")
    tasks = list(tasks)
    results: list[TaskResult] = []

    with tempfile.TemporaryDirectory(prefix="sin-bench-") as tmp:
        root = Path(workspace) if workspace else Path(tmp)
        root.mkdir(parents=True, exist_ok=True)
        for arm in arms:
            for task in tasks:
                results.append(_eval_one(task, arm, runner, root / arm))

    summaries = {arm: _summarize(arm, results) for arm in arms}
    delta = 0.0
    if "sin" in summaries and "control" in summaries:
        delta = round(summaries["sin"].resolved_rate - summaries["control"].resolved_rate, 4)
    return BenchReport(
        arms=summaries,
        delta_resolved_rate=delta,
        per_task=results,
        started_at=started,
        finished_at=time.strftime("%Y-%m-%dT%H:%M:%S"),
    )


# --------------------------------------------------------------------------- #
# Task loading
# --------------------------------------------------------------------------- #
def load_tasks_jsonl(path: Path, limit: Optional[int] = None) -> list[Task]:
    """Load tasks from a JSONL file (SWE-bench compatible field names)."""
    tasks: list[Task] = []
    for line in path.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line:
            continue
        d = json.loads(line)
        tasks.append(
            Task(
                instance_id=d["instance_id"],
                repo=d["repo"],
                base_commit=d["base_commit"],
                problem_statement=d.get("problem_statement", ""),
                fail_to_pass=d.get("FAIL_TO_PASS", d.get("fail_to_pass", [])),
                pass_to_pass=d.get("PASS_TO_PASS", d.get("pass_to_pass", [])),
                setup_cmds=d.get("setup_cmds", []),
                test_cmd=d.get("test_cmd", "pytest -q"),
            )
        )
        if limit and len(tasks) >= limit:
            break
    return tasks


def load_swebench_lite(limit: Optional[int] = 20) -> list[Task]:
    """Load SWE-bench Lite via `datasets` if available; else raise a clear error."""
    try:
        from datasets import load_dataset  # type: ignore
    except ImportError as exc:
        raise RuntimeError(
            "SWE-bench Lite requires the 'datasets' package. "
            "Install with: pip install 'sin-code-bundle[bench]', "
            "or pass --tasks <file.jsonl>."
        ) from exc

    ds = load_dataset("princeton-nlp/SWE-bench_Lite", split="test")
    tasks: list[Task] = []
    for row in ds:
        tasks.append(
            Task(
                instance_id=row["instance_id"],
                repo=row["repo"],
                base_commit=row["base_commit"],
                problem_statement=row["problem_statement"],
                fail_to_pass=json.loads(row["FAIL_TO_PASS"])
                if isinstance(row["FAIL_TO_PASS"], str)
                else row["FAIL_TO_PASS"],
                pass_to_pass=json.loads(row["PASS_TO_PASS"])
                if isinstance(row["PASS_TO_PASS"], str)
                else row["PASS_TO_PASS"],
            )
        )
        if limit and len(tasks) >= limit:
            break
    return tasks


# --------------------------------------------------------------------------- #
# Pretty printing
# --------------------------------------------------------------------------- #
def format_report(report: BenchReport) -> str:
    lines = ["", "SIN-Code Bench — A/B resolved-rate", "=" * 40]
    for arm, s in report.arms.items():
        lines.append(
            f"  {arm:<8} {s.resolved}/{s.total} resolved "
            f"({s.resolved_rate * 100:5.1f}%)  mean {s.mean_duration_s}s"
        )
    sign = "+" if report.delta_resolved_rate >= 0 else ""
    lines.append("-" * 40)
    lines.append(
        f"  SIN delta: {sign}{report.delta_resolved_rate * 100:.1f} pp (percentage points)"
    )
    lines.append("=" * 40)
    return "\n".join(lines)
