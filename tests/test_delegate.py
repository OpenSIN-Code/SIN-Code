# SPDX-License-Identifier: MIT
"""Core invariants: DAG validation, content-addressed ids, scheduler
resume/skip semantics, redaction, plan-file resolution."""

from __future__ import annotations

import asyncio
import json

import pytest

from sin_delegate.ledger import Ledger
from sin_delegate.models import Plan, Task, TaskOutcome, TaskState
from sin_delegate.planfile import plan_from_dict
from sin_delegate.runner import redact
from sin_delegate.scheduler import Scheduler, critical_path_priority


def _plan(tmp_path, tasks):
    return Plan(goal="test", tasks=tuple(tasks), repo=str(tmp_path))


def test_task_id_is_content_addressed():
    a = Task(title="x", instructions="do x").finalize()
    b = Task(title="x", instructions="do x").finalize()
    c = Task(title="x", instructions="do y").finalize()
    assert a.id == b.id and a.id != c.id


def test_cycle_detection(tmp_path):
    a = Task(title="a", instructions="a", id="A", deps=("B",))
    b = Task(title="b", instructions="b", id="B", deps=("A",))
    with pytest.raises(ValueError, match="cycle"):
        _plan(tmp_path, [a, b]).validate()


def test_critical_path_priority(tmp_path):
    a = Task(title="a", instructions="a", id="A")
    b = Task(title="b", instructions="b", id="B", deps=("A",))
    c = Task(title="c", instructions="c", id="C", deps=("B",))
    d = Task(title="d", instructions="d", id="D")
    prio = critical_path_priority(_plan(tmp_path, [a, b, c, d]))
    assert prio["A"] == 3 and prio["D"] == 1


def test_scheduler_skips_downstream_of_failure(tmp_path):
    a = Task(title="a", instructions="a", id="A")
    b = Task(title="b", instructions="b", id="B", deps=("A",))
    plan = _plan(tmp_path, [a, b])
    ledger = Ledger(tmp_path / "ledger.db")
    async def executor(task):
        return TaskOutcome(task.id,
                           TaskState.FAILED if task.id == "A" else TaskState.DONE,
                           error="boom" if task.id == "A" else "")
    outcomes = asyncio.run(Scheduler(plan, ledger, executor).run())
    assert outcomes["A"].state == TaskState.FAILED
    assert outcomes["B"].state == TaskState.SKIPPED


def test_scheduler_resume_skips_done(tmp_path):
    a = Task(title="a", instructions="a", id="A")
    plan = _plan(tmp_path, [a])
    ledger = Ledger(tmp_path / "ledger.db")
    ledger.register_run(plan.id, plan.goal, "{}")
    ledger.emit(plan.id, "A", "state:done")
    calls = []
    async def executor(task):
        calls.append(task.id)
        return TaskOutcome(task.id, TaskState.DONE)
    outcomes = asyncio.run(Scheduler(plan, ledger, executor).run())
    assert outcomes["A"].state == TaskState.DONE
    assert calls == []


def test_redaction():
    leak = "API_KEY=sk-abcdefghijklmnop1234 and token: ghp_aaaaaaaaaaaaaaaa"
    out = redact(leak)
    assert "sk-abcdefghijklmnop1234" not in out
    assert "[REDACTED]" in out


def test_planfile_resolves_human_keys(tmp_path):
    plan = plan_from_dict({
        "goal": "g",
        "tasks": [
            {"key": "one", "title": "first", "instructions": "i1"},
            {"key": "two", "title": "second", "instructions": "i2",
             "deps": ["one"]},
        ],
    }, repo=str(tmp_path))
    t1 = next(t for t in plan.tasks if t.title == "first")
    t2 = next(t for t in plan.tasks if t.title == "second")
    assert t2.deps == (t1.id,)


def test_ledger_roundtrip(tmp_path):
    from sin_delegate.ledger import Ledger
    ledger = Ledger(tmp_path / "l.db")
    ledger.register_run("p1", "goal", '{"x":1}')
    ledger.emit("p1", "T1", "attempt", {"n": 1})
    ledger.emit("p1", "T1", "state:running")
    ledger.emit("p1", "T1", "verdict", {"passed": True, "gates": {}})
    ledger.emit("p1", "T1", "state:done", {"seconds": 12.5, "error": ""})

    states = ledger.task_states("p1")
    assert states["T1"] == TaskState.DONE
    assert ledger.attempts("p1", "T1") == 1
    assert len(ledger.history("p1")) == 4
    assert ledger.load_plan_json("p1") == '{"x":1}'

    runs = ledger.list_runs()
    assert any(r["plan_id"] == "p1" for r in runs)


def test_dag_validation_unknown_dep(tmp_path):
    a = Task(title="a", instructions="a", id="A", deps=("GHOST",))
    with pytest.raises(ValueError, match="unknown task"):
        _plan(tmp_path, [a]).validate()


def test_run_result_ok_predicate():
    from sin_delegate.models import RunResult
    from sin_delegate.models import TaskOutcome
    r = RunResult("p1", "g", {
        "A": TaskOutcome("A", TaskState.DONE),
        "B": TaskOutcome("B", TaskState.SKIPPED),
    }, 1.0, 2.0)
    assert r.ok
    r2 = RunResult("p1", "g", {
        "A": TaskOutcome("A", TaskState.FAILED),
    }, 1.0, 2.0)
    assert not r2.ok
    r3 = RunResult("p1", "g", {}, 1.0, 2.0)
    assert not r3.ok
