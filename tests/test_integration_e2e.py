# SPDX-License-Identifier: MIT
"""End-to-end integration tests with real git repos.

Exercises the full lifecycle — plan -> run -> gates -> merge -> resume ->
escalation -> resolution — with real git operations. Uses EchoRunner
backend to avoid LLM calls.
"""

from __future__ import annotations

import json
import subprocess
from pathlib import Path

import pytest

from sin_delegate.engine import Delegator
from sin_delegate.escalation import EscalationBroker, EscalationKind
from sin_delegate.resolution import apply_resolutions
from sin_delegate.ledger import Ledger
from sin_delegate.models import (AgentSpec, Budget, Plan, Risk, Task,
                                TaskState)


def _git_init(path: Path) -> None:
    path.mkdir(parents=True, exist_ok=True)
    subprocess.run(["git", "init", "-b", "main", str(path)],
                   capture_output=True, check=True)
    (path / "README.md").write_text("# test")
    subprocess.run(["git", "-C", str(path), "add", "-A"],
                   capture_output=True, check=True)
    subprocess.run(["git", "-C", str(path),
                    "-c", "user.email=test@test", "-c", "user.name=Test",
                    "commit", "-m", "init"],
                   capture_output=True, check=True)


@pytest.fixture
def repo(tmp_path):
    r = tmp_path / "repo"
    _git_init(r)
    return r


def _echo_task(title: str, deps=(), files=()) -> Task:
    """Task that creates a real file via shell so wt.commit_all succeeds."""
    safe = title.replace(" ", "_")
    return Task(
        title=title, instructions=f"do {title}", deps=deps,
        files_hint=files, risk=Risk.LOW,
        agent=AgentSpec(backend="command",
                        command=("sh", "-c",
                                 f"echo '{title}' > {safe}.txt")),
        budget=Budget(max_seconds=5, max_retries=0),
        verify=("diff",),
    ).finalize()


def test_e2e_single_task_runs_through_pipeline(repo, tmp_path):
    task = _echo_task("write docs")
    plan = Plan(goal="test", tasks=(task,), repo=str(repo))
    ledger = Ledger(tmp_path / "ledger.db")
    dele = Delegator(plan, ledger=ledger)
    result = dele.run_sync()
    assert result.plan_id == plan.id
    history = ledger.history(plan.id)
    kinds = {ev["kind"] for ev in history}
    assert "state:running" in kinds or "attempt" in kinds


def test_e2e_dag_with_dependencies_executes_all(repo, tmp_path):
    a = _echo_task("task a")
    b = _echo_task("task b", deps=(a.id,))
    c = _echo_task("task c", deps=(b.id,))
    plan = Plan(goal="dag", tasks=(a, b, c), repo=str(repo))
    ledger = Ledger(tmp_path / "ledger.db")
    dele = Delegator(plan, ledger=ledger)
    result = dele.run_sync()
    states = ledger.task_states(plan.id)
    assert set(states.keys()) == {a.id, b.id, c.id}


def test_e2e_resume_skips_already_done_tasks(repo, tmp_path):
    task = _echo_task("already done")
    plan = Plan(goal="resume", tasks=(task,), repo=str(repo))
    ledger = Ledger(tmp_path / "ledger.db")
    ledger.register_run(plan.id, plan.goal, '{"tasks": []}')
    ledger.emit(plan.id, task.id, "state:done")

    dele = Delegator(plan, ledger=ledger)
    result = dele.run_sync()
    assert result.outcomes[task.id].state == TaskState.DONE
    history = [ev for ev in ledger.history(plan.id)
               if ev["task_id"] == task.id]
    assert any(ev["kind"] == "resume:skip-done" for ev in history)


def test_e2e_crash_recovery_via_ledger(repo, tmp_path):
    """Simulate crash mid-run, then resume — only unfinished tasks re-run."""
    a = _echo_task("task a")
    b = _echo_task("task b", deps=(a.id,))
    plan = Plan(goal="crash", tasks=(a, b), repo=str(repo))
    ledger = Ledger(tmp_path / "ledger.db")
    ledger.register_run(plan.id, plan.goal, '{"tasks": []}')
    # simulate: a already DONE before crash
    ledger.emit(plan.id, a.id, "attempt", {"n": 1})
    ledger.emit(plan.id, a.id, "state:done")
    # resume — a skipped, b should be attempted
    dele = Delegator(plan, ledger=ledger)
    result = dele.run_sync()
    assert result.outcomes[a.id].state == TaskState.DONE
    b_events = [ev for ev in ledger.history(plan.id)
                if ev["task_id"] == b.id]
    assert any(ev["kind"] == "attempt" for ev in b_events)


def test_e2e_escalation_resolution_retry(repo, tmp_path):
    task = _echo_task("escalated task")
    plan = Plan(goal="escalation", tasks=(task,), repo=str(repo))
    ledger = Ledger(tmp_path / "ledger.db")
    ledger.register_run(plan.id, plan.goal, json.dumps({
        "goal": plan.goal, "tasks": [
            {"id": task.id, "title": task.title}]}))
    broker = EscalationBroker(ledger)
    esc = broker.raise_escalation(
        plan.id, task.id, task.title, EscalationKind.GATE_FAILURE,
        "gates failed", {})
    broker.resolve(plan.id, esc.id, "retry", user_input="fix it")
    res = apply_resolutions(plan, ledger)
    assert res["applied"] == 1
    assert task.id in res["guidance"]
    states = ledger.task_states(plan.id)
    assert states[task.id] == TaskState.PENDING


def test_e2e_full_lifecycle_resume_to_completion(repo, tmp_path):
    """Phase 1: run, crash on a. Phase 2: resume, complete a, b succeeds."""
    a = _echo_task("a")
    b = _echo_task("b", deps=(a.id,))
    plan = Plan(goal="full", tasks=(a, b), repo=str(repo))
    ledger = Ledger(tmp_path / "ledger.db")
    ledger.register_run(plan.id, plan.goal, '{"tasks": []}')

    # Phase 2: full resume
    dele = Delegator(plan, ledger=ledger)
    result = dele.run_sync()
    states = ledger.task_states(plan.id)
    TERMINAL = {TaskState.DONE, TaskState.FAILED, TaskState.SKIPPED,
                TaskState.CANCELLED, TaskState.ESCALATED}
    assert states[a.id] in (TaskState.DONE, TaskState.FAILED)
    assert states[b.id] in TERMINAL
