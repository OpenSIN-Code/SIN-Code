# SPDX-License-Identifier: MIT
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

Docs: bench.doc.md
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
# ── Task + Result Models: SWE-bench compatible dataclasses ────────────────── #
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
    """Per-task, per-arm outcome record produced by :func:`_eval_one`.

    Attributes:
        instance_id: Originating :class:`Task` id.
        arm: Which arm ("control" = SIN tools off, "sin" = SIN tools on).
        resolved: ``True`` iff the patch applied AND every FAIL_TO_PASS test
            now passes. This is the headline "did the agent solve it?" bit.
        duration_s: Wall-clock seconds for clone + agent + apply + test.
        patch_applied: Whether ``git apply`` accepted the agent's diff.
        fail_to_pass_passed: Count of FAIL_TO_PASS tests that now pass.
        fail_to_pass_total: Size of the FAIL_TO_PASS set (or 1 if the task
            has no named tests and we fell back to a single ``test_cmd`` run).
        error: Stringified exception if the harness itself blew up (clone
            failure, timeout, etc.) — separate from "agent produced bad patch".
    """

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
    """Aggregated stats for one arm across all tasks in a benchmark run.

    Attributes:
        arm: "control" or "sin".
        total: Number of tasks attempted in this arm.
        resolved: Number of tasks whose :class:`TaskResult` had ``resolved=True``.
        resolved_rate: ``resolved / total`` (0.0 if ``total == 0``).
        mean_duration_s: Arithmetic mean of per-task durations.
    """

    arm: Arm
    total: int
    resolved: int
    resolved_rate: float
    mean_duration_s: float


@dataclass
class BenchReport:
    """Top-level benchmark output — per-arm summaries plus raw per-task results.

    Attributes:
        arms: Map ``arm_name -> ArmSummary``.
        delta_resolved_rate: ``sin.resolved_rate - control.resolved_rate``
            (i.e. the headline lift in percentage points / 100). Positive
            means SIN tools helped.
        per_task: Full list of :class:`TaskResult` records for both arms,
            preserving execution order, for drill-down analysis.
        started_at: ISO-8601 timestamp of harness start (local time, no TZ).
        finished_at: ISO-8601 timestamp of harness completion.
    """

    arms: dict[str, ArmSummary]
    delta_resolved_rate: float
    per_task: list[TaskResult]
    started_at: str
    finished_at: str

    def to_json(self) -> str:
        """Serialise the full report to a pretty-printed JSON string.

        Nested dataclasses (:class:`ArmSummary`, :class:`TaskResult`) are
        converted with :func:`dataclasses.asdict` so the output is plain
        JSON — safe to write to disk, post over HTTP, or diff between runs.
        """
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
# ── Agent Runner Protocol: pluggable backends (opencode / codex / dry-run) ── #
# --------------------------------------------------------------------------- #
class AgentRunner(Protocol):
    """Produces a unified diff that attempts to solve `task` inside `workdir`.

    `sin_enabled` tells the runner whether to expose the SIN MCP tools to the
    underlying agent. Implementations should return a unified-diff string (may
    be empty if the agent produced no change).
    """

    def run(self, task: Task, workdir: Path, sin_enabled: bool) -> str:
        """Solve ``task`` inside ``workdir`` and return the resulting unified diff.

        Protocol method — see the class docstring for the contract. Concrete
        implementations should leave their edits in ``workdir`` (typically as
        uncommitted changes) and return them as a diff string.
        """
        ...


class DryRunRunner:
    """Zero-cost runner for smoke-testing the harness itself.

    Produces no patch, so every task "fails" — but exercises the full
    clone/apply/test pipeline so you can validate without an LLM.
    """

    def run(self, task: Task, workdir: Path, sin_enabled: bool) -> str:  # noqa: ARG002
        """Return an empty diff regardless of inputs.

        Intentionally ignores ``task`` / ``workdir`` / ``sin_enabled`` — the
        purpose is to keep the harness wired up end-to-end without making any
        LLM calls. Every task will report ``resolved=False`` in both arms.
        """
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
        # 1800s = 30 min — generous enough for slow LLM rollouts but caps
        # runaway agents so a single bad task can't stall the whole sweep.
        self._timeout_s = timeout_s
        self._env_for = env_for

    def run(self, task: Task, workdir: Path, sin_enabled: bool) -> str:
        """Invoke the external agent, then return whatever ``git diff`` shows.

        The agent is expected to mutate files inside ``workdir`` directly;
        we don't parse its stdout. ``SIN_ENFORCE`` is exported into the
        agent's env so MCP servers can gate themselves on it (1 = SIN tools
        available, 0 = control arm, must not be used).

        Returns:
            Unified-diff text of every uncommitted change the agent made.
            Empty string if the agent produced no edits, crashed, or hit
            the timeout (we deliberately swallow non-zero exit codes here
            — a broken agent is a "failed task", not a harness error).
        """
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
# ── Git / Test Plumbing: worktree prep, patch apply, test execution ──────── #
# --------------------------------------------------------------------------- #
def _sh(cmd: list[str], cwd: Path, timeout: int = 600) -> subprocess.CompletedProcess:
    # 600s = 10 min default — fits clone/checkout/test-id runs; callers
    # override (e.g. clone uses 900s, setup_cmds use 1800s).
    return subprocess.run(
        cmd, cwd=cwd, check=False, capture_output=True, text=True, timeout=timeout
    )


def _prepare_worktree(task: Task, root: Path) -> Path:
    work = root / task.instance_id.replace("/", "__")
    work.mkdir(parents=True, exist_ok=True)
    url = f"https://github.com/{task.repo}.git"
    # 900s clone timeout — large monorepos (django, sympy) routinely
    # need >5 min on a cold network; tighter would flake the harness.
    _sh(["git", "clone", "--quiet", url, "."], cwd=work, timeout=900)
    _sh(["git", "checkout", "--quiet", task.base_commit], cwd=work)
    for cmd in task.setup_cmds:
        # 1800s per setup cmd — pip installs of scientific stacks (scipy,
        # pandas) can be slow when wheels are missing for the platform.
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
        # Fallback path: SWE-bench tasks usually name specific tests, but some
        # in-house tasks just ship a `test_cmd` and rely on its overall exit
        # code (0 = solved, non-zero = not solved).
        res = _sh(["bash", "-lc", task.test_cmd], cwd=work, timeout=1800)
        return (1, 1) if res.returncode == 0 else (0, 1)

    passed = 0
    for test_id in task.fail_to_pass:
        # 900s per single test — pytest selectors on huge repos (django) need
        # collection time even before the test itself runs.
        res = _sh(
            ["bash", "-lc", f"{task.test_cmd} {test_id}"],
            cwd=work,
            timeout=900,
        )
        if res.returncode == 0:
            passed += 1
    return passed, len(task.fail_to_pass)


# --------------------------------------------------------------------------- #
# ── Core Eval Loop: drive runner + scoring per task per arm ──────────────── #
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
    """Run every ``task`` through every ``arm`` and return an aggregated report.

    Each (task, arm) pair gets its own clone under ``workspace / <arm> /
    <task.instance_id>`` so arms can never poison each other's worktree.
    The agent is invoked once per pair via ``runner``; its diff is applied
    and the FAIL_TO_PASS tests are run to score the attempt.

    Args:
        tasks: Iterable of :class:`Task` (consumed once; materialised internally).
        runner: Pluggable :class:`AgentRunner` (e.g. :class:`DryRunRunner`,
            :class:`CommandRunner`).
        arms: Which arms to run. Default ``("control", "sin")`` produces the
            standard A/B delta; pass ``("sin",)`` for a single-arm run.
        workspace: Persistent workspace dir. Pass a real path to keep clones
            on disk for post-mortem inspection; default uses a tempdir
            wiped on return.

    Returns:
        :class:`BenchReport` with per-arm summaries, headline delta, and
        per-task detail.
    """
    started = time.strftime("%Y-%m-%dT%H:%M:%S")
    tasks = list(tasks)
    results: list[TaskResult] = []

    with tempfile.TemporaryDirectory(prefix="sin-bench-") as tmp:
        root = Path(workspace) if workspace else Path(tmp)
        root.mkdir(parents=True, exist_ok=True)
        for arm in arms:
            for task in tasks:
                # Per-arm subdir keeps the two clones strictly isolated —
                # otherwise the second arm would inherit the first arm's
                # leftover patch state.
                results.append(_eval_one(task, arm, runner, root / arm))

    summaries = {arm: _summarize(arm, results) for arm in arms}
    delta = 0.0
    if "sin" in summaries and "control" in summaries:
        delta = round(
            summaries["sin"].resolved_rate - summaries["control"].resolved_rate, 4
        )
    return BenchReport(
        arms=summaries,
        delta_resolved_rate=delta,
        per_task=results,
        started_at=started,
        finished_at=time.strftime("%Y-%m-%dT%H:%M:%S"),
    )


# --------------------------------------------------------------------------- #
# ── Task Loading: JSONL + SWE-bench Lite via datasets ────────────────────── #
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
# ── Pretty Printing: human-readable terminal report ──────────────────────── #
# --------------------------------------------------------------------------- #
def format_report(report: BenchReport) -> str:
    """Render a :class:`BenchReport` as a fixed-width terminal block.

    Used by the ``sin bench`` CLI to print results at the end of a run.
    Layout::

        SIN-Code Bench — A/B resolved-rate
        ========================================
          control  3/20 resolved ( 15.0%)  mean 142.5s
          sin      7/20 resolved ( 35.0%)  mean 187.2s
        ----------------------------------------
          SIN delta: +20.0 pp (percentage points)
        ========================================

    Returns:
        Multi-line string with no trailing newline — caller decides spacing.
    """
    lines = ["", "SIN-Code Bench — A/B resolved-rate", "=" * 40]
    for arm, s in report.arms.items():
        lines.append(
            f"  {arm:<8} {s.resolved}/{s.total} resolved "
            f"({s.resolved_rate * 100:5.1f}%)  mean {s.mean_duration_s}s"
        )
    sign = "+" if report.delta_resolved_rate >= 0 else ""
    lines.append("-" * 40)
    lines.append(
        f"  SIN delta: {sign}{report.delta_resolved_rate * 100:.1f} pp "
        "(percentage points)"
    )
    lines.append("=" * 40)
    return "\n".join(lines)
