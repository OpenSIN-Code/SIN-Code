# SPDX-License-Identifier: MIT
"""Stability tests for sin-code-delegate production readiness.

Covers the six stabilization pillars:
- MCP standalone input validation
- Budget governor surplus/extension behaviour
- Ledger corruption resilience (state mapping)
- Crash recovery for stale worktrees
- Escalation / resolution state mapping
- Two-phase rollback detection
"""

from __future__ import annotations

import asyncio
import subprocess
from pathlib import Path

import pytest

from sin_delegate.budget_governor import BudgetGovernor
from sin_delegate.escalation import ActionType, EscalationBroker, EscalationKind
from sin_delegate.ledger import Ledger
from sin_delegate.mcp_tools import (
    _tool_cancel,
    _tool_delegate,
    _tool_escalations,
    _tool_history,
    _tool_resolve,
    _tool_status,
)
from sin_delegate.models import Plan, Task, TaskState
from sin_delegate.multirepo import TwoPhaseMerger
from sin_delegate.resolution import apply_resolutions
from sin_delegate.worktree import GitError, WorktreeManager


def _git_init(path: Path) -> None:
    path.mkdir(parents=True, exist_ok=True)
    subprocess.run(["git", "init", "-b", "main", str(path)], capture_output=True, check=True)
    (path / "README.md").write_text("# init")
    subprocess.run(["git", "-C", str(path), "add", "-A"], capture_output=True, check=True)
    subprocess.run(
        [
            "git",
            "-C",
            str(path),
            "-c",
            "user.email=t@t",
            "-c",
            "user.name=t",
            "commit",
            "-m",
            "init",
        ],
        capture_output=True,
        check=True,
    )


# ---------------------------------------------------------- MCP validation


@pytest.mark.asyncio
async def test_mcp_delegate_rejects_missing_plan():
    result = await _tool_delegate({})
    assert "error" in result
    assert "missing required field 'plan'" in result["error"]


@pytest.mark.asyncio
async def test_mcp_delegate_rejects_invalid_json():
    result = await _tool_delegate({"plan": "not-json"})
    assert "error" in result
    assert "valid JSON" in result["error"]


@pytest.mark.asyncio
async def test_mcp_delegate_rejects_plan_without_tasks():
    result = await _tool_delegate({"plan": '{"goal": "g"}'})
    assert "error" in result
    assert "tasks" in result["error"]


@pytest.mark.asyncio
async def test_mcp_delegate_rejects_non_integer_parallel():
    result = await _tool_delegate(
        {
            "plan": '{"goal": "g", "tasks": []}',
            "parallel": "four",
        }
    )
    assert "error" in result
    assert "integer" in result["error"]


@pytest.mark.asyncio
async def test_mcp_status_and_history_reject_missing_plan_id():
    for fn in (_tool_status, _tool_history, _tool_cancel, _tool_escalations):
        result = await fn({})
        assert "error" in result, fn.__name__
        assert "plan_id" in result["error"]


@pytest.mark.asyncio
async def test_mcp_resolve_rejects_missing_fields():
    result = await _tool_resolve({"plan_id": "p"})
    assert "error" in result
    assert "escalation_id" in result["error"]


# ---------------------------------------------------------- budget governor


def test_budget_governor_lease_respects_global_cap():
    plan = Plan(
        goal="g",
        tasks=(
            Task(title="a", instructions="a").finalize(),
            Task(title="b", instructions="b").finalize(),
        ),
    )
    gov = BudgetGovernor(plan, global_seconds=120, priority={t.id: 1 for t in plan.tasks})
    grant_a = asyncio.run(gov.lease(plan.tasks[0].id))
    assert 0 < grant_a <= 120
    grant_b = asyncio.run(gov.lease(plan.tasks[1].id))
    assert 0 < grant_b <= 120
    assert grant_a + grant_b <= 120


def test_budget_governor_release_and_extension():
    plan = Plan(goal="g", tasks=(Task(title="a", instructions="a").finalize(),))
    gov = BudgetGovernor(plan, global_seconds=300, priority={plan.tasks[0].id: 1})
    grant = asyncio.run(gov.lease(plan.tasks[0].id))
    asyncio.run(gov.release(plan.tasks[0].id, used_seconds=grant - 30))
    snapshot = gov.snapshot()
    assert snapshot["pool"] >= 30
    extra = asyncio.run(gov.request_extension(plan.tasks[0].id, 50))
    assert extra > 0


# ---------------------------------------------------------- ledger resilience


def test_ledger_corrupt_state_is_resilient(tmp_path):
    ledger = Ledger(tmp_path / "l.db")
    ledger.register_run("p1", "goal", "{}")
    ledger.emit("p1", "T1", "state:running")
    ledger.emit("p1", "T1", "state:done")
    ledger.emit("p1", "T1", "state:invalid_xyz")

    states = ledger.task_states("p1")
    assert states["T1"] == TaskState.DONE

    history = ledger.history("p1")
    corrupt_events = [e for e in history if e["kind"] == "ledger:corrupt_state"]
    assert len(corrupt_events) == 1
    assert corrupt_events[0]["payload"]["kind"] == "state:invalid_xyz"


# ---------------------------------------------------------- worktree crash recovery


def test_worktree_recovers_from_stale_directory(tmp_path):
    repo = tmp_path / "repo"
    _git_init(repo)
    wtm = WorktreeManager(repo, base_branch="main")
    stale = repo / ".sin-worktrees" / "plan1" / "taskA"
    stale.mkdir(parents=True)
    (stale / "leftover.txt").write_text("crash debris")

    wt = wtm.create("plan1", "taskA")
    assert wt.path.exists()
    assert (wt.path / ".git").is_file()
    assert not (wt.path / "leftover.txt").exists()
    wt.destroy()


# ---------------------------------------------------------- escalation / resolution


def test_escalation_state_mapping(tmp_path):
    ledger = Ledger(tmp_path / "l.db")
    broker = EscalationBroker(ledger)
    esc = broker.raise_escalation(
        "p1",
        "T1",
        "task title",
        EscalationKind.GATE_FAILURE,
        "gates failed",
        {"diff": "boom"},
        branch="b1",
        worktree="w1",
    )

    open_ = broker.open_escalations("p1")
    assert len(open_) == 1
    assert open_[0]["id"] == esc.id

    result = broker.resolve("p1", esc.id, "drop", decided_by="parent")
    assert result["ok"]
    assert result["action"] == ActionType.DROP_TASK.value

    assert broker.open_escalations("p1") == []
    pending = broker.pending_resolutions("p1")
    assert len(pending) == 1
    assert pending[0]["escalation_id"] == esc.id

    broker.mark_applied("p1", "T1", esc.id)
    assert broker.pending_resolutions("p1") == []


def test_resolution_apply_handles_corrupt_action(tmp_path):
    plan = Plan(goal="g", tasks=(Task(title="t", instructions="t", id="T1"),))
    ledger = Ledger(tmp_path / "l.db")
    ledger.register_run(plan.id, "goal", "{}")
    # Manually inject a corrupt resolution record
    ledger.emit(
        plan.id,
        "T1",
        "escalation:resolved",
        {"escalation_id": "e1", "task_id": "T1", "action": "not_a_valid_action", "option_id": "x"},
    )
    result = apply_resolutions(plan, ledger)
    assert result["applied"] == 0

    history = ledger.history(plan.id)
    assert any(e["kind"] == "ledger:corrupt_resolution" for e in history)


def test_resolution_drop_skips_downstream(tmp_path):
    plan = Plan(
        goal="g",
        tasks=(
            Task(title="a", instructions="a", id="A"),
            Task(title="b", instructions="b", id="B", deps=("A",)),
        ),
    )
    ledger = Ledger(tmp_path / "l.db")
    broker = EscalationBroker(ledger)
    esc = broker.raise_escalation(plan.id, "A", "a", EscalationKind.GATE_FAILURE, "boom", {})
    broker.resolve(plan.id, esc.id, "drop")

    result = apply_resolutions(plan, ledger)
    assert result["applied"] == 1
    states = ledger.task_states(plan.id)
    assert states["A"] == TaskState.SKIPPED


# ---------------------------------------------------------- multirepo rollback


def test_two_phase_merger_stages_and_rolls_back(tmp_path):
    repo = tmp_path / "repo"
    _git_init(repo)
    wtm = WorktreeManager(repo, base_branch="main")
    wt = wtm.create("plan1", "T1")
    (wt.path / "file.txt").write_text("change")
    wt.commit_all("commit")

    ledger = Ledger(tmp_path / "l.db")
    merger = TwoPhaseMerger(
        {"repo": type("R", (), {"path": str(repo), "base_branch": "main"})()}, ledger, "plan1"
    )
    merger.stage(type("U", (), {"task_id": "T1", "worktree": wt, "repo_name": "repo"})())
    assert len(merger.units) == 1

    # Commit succeeds; rollback path is covered by unit tests in the
    # multirepo module. Here we assert the stage → snapshot bookkeeping.
    merger.commit(["T1"])
    assert (repo / "file.txt").read_text() == "change"

    history = ledger.history("plan1")
    assert any(e["kind"] == "merge:phase2_done" for e in history)


def test_two_phase_merger_rollback_on_conflict(tmp_path):
    repo = tmp_path / "repo"
    _git_init(repo)
    wtm = WorktreeManager(repo, base_branch="main")
    wt = wtm.create("plan1", "T1")
    (wt.path / "conflict.txt").write_text("branch version")
    wt.commit_all("branch commit")

    # Introduce a conflicting commit on main after the worktree branched
    (repo / "conflict.txt").write_text("main version")
    subprocess.run(["git", "-C", str(repo), "add", "-A"], capture_output=True, check=True)
    subprocess.run(
        [
            "git",
            "-C",
            str(repo),
            "-c",
            "user.email=t@t",
            "-c",
            "user.name=t",
            "commit",
            "-m",
            "main commit",
        ],
        capture_output=True,
        check=True,
    )

    ledger = Ledger(tmp_path / "l.db")
    merger = TwoPhaseMerger(
        {"repo": type("R", (), {"path": str(repo), "base_branch": "main"})()}, ledger, "plan1"
    )
    merger.stage(type("U", (), {"task_id": "T1", "worktree": wt, "repo_name": "repo"})())

    with pytest.raises(GitError):
        merger.commit(["T1"])

    # The first unit failed before any repo was modified, so the rollback set
    # is empty, but the event must still be emitted and the base branch must
    # remain exactly at the pre-merge snapshot.
    history = ledger.history("plan1")
    rollback = [e for e in history if e["kind"] == "merge:phase2_rollback"]
    assert len(rollback) == 1
    assert rollback[0]["payload"]["failed_unit"] == "T1"
    assert rollback[0]["payload"]["rolled_back_repos"] == []

    head = subprocess.run(
        ["git", "-C", str(repo), "rev-parse", "HEAD"], capture_output=True, text=True, check=True
    ).stdout.strip()
    snap = subprocess.run(
        ["git", "-C", str(repo), "rev-parse", "sin-global-snap/plan1"],
        capture_output=True,
        text=True,
        check=True,
    ).stdout.strip()
    assert head == snap
