# SPDX-License-Identifier: MIT
"""Analytics + Policy + Budget-Governor + Escalation + Multirepo tests."""

from __future__ import annotations

import asyncio
import json
import subprocess
import time

import pytest

from sin_delegate.analytics import (Analytics, BackendStats,
                                     task_class, task_class_of,
                                     wilson_lower)
from sin_delegate.budget_governor import BudgetGovernor
from sin_delegate.escalation import (ActionType, EscalationBroker,
                                     EscalationKind)
from sin_delegate.resolution import apply_resolutions
from sin_delegate.ledger import Ledger
from sin_delegate.models import (Plan, Risk, Task, TaskState)
from sin_delegate.multirepo import (MergeUnit, TwoPhaseMerger,
                                    extract_contract,
                                    multirepo_plan_from_dict)
from sin_delegate.multirepo_engine import MultiRepoDelegator, _topo_order
from sin_delegate.policy import Policy


# --------------------------------------------------------------- analytics

def test_wilson_punishes_small_samples():
    assert wilson_lower(47, 50) > wilson_lower(1, 1)
    assert wilson_lower(0, 0) == 0.0


def test_backend_stats_ema_smoothing():
    s = BackendStats("opencode", "", "low:py:tests")
    s.observe(True, 100.0, 1)
    s.observe(True, 200.0, 1)
    assert 100.0 < s.ema_seconds < 200.0


def test_task_class_bucketing():
    assert task_class("high", ["a.py", "b.py"],
                      ["diff", "tests"]) == "high:py:diff+tests"
    assert task_class("low", [], []) == "low:any:none"


def test_analytics_folds_ledger(tmp_path):
    ledger = Ledger(tmp_path / "l.db")
    ledger.register_run("p1", "g", json.dumps({
        "goal": "g", "tasks": [
            {"id": "T1", "title": "t1", "backend": "claude",
             "model": "m1", "risk": "high",
             "files_hint": ["a.py"], "verify": ["diff", "tests"]},
        ],
    }))
    ledger.emit("p1", "T1", "attempt", {"n": 1})
    ledger.emit("p1", "T1", "verdict", {"passed": True, "gates": {}})
    ledger.emit("p1", "T1", "state:done", {"seconds": 60.0, "error": ""})

    analytics = Analytics(ledger)
    table = analytics.table()
    assert len(table) == 1
    row = table[0]
    assert row["backend"] == "claude"
    assert row["trials"] == 1
    assert row["pass_rate"] == 1.0
    assert row["wilson_score"] > 0


def test_analytics_best_backend_returns_none_below_threshold(tmp_path):
    ledger = Ledger(tmp_path / "l.db")
    ledger.register_run("p1", "g", json.dumps({
        "goal": "g", "tasks": [
            {"id": "T1", "title": "t1", "backend": "claude", "model": "m1",
             "risk": "low", "files_hint": [], "verify": ["diff"]},
        ],
    }))
    ledger.emit("p1", "T1", "verdict", {"passed": True, "gates": {}})
    ledger.emit("p1", "T1", "state:done", {"seconds": 1, "error": ""})

    analytics = Analytics(ledger)
    # only 1 trial — below min_trials=3 — should return None
    assert analytics.best_backend("low:any:diff",
                                  candidates=[("claude", "m1")]) is None


# --------------------------------------------------------------- policy

def test_policy_respects_pinned_model(tmp_path):
    from sin_delegate.models import AgentSpec
    t = Task(title="x", instructions="x", id="X",
             agent=AgentSpec(backend="claude", model="claude-sonnet-4-5"))
    plan = Plan(goal="g", tasks=(t,), repo=str(tmp_path))
    new_plan, decisions = Policy(Analytics(Ledger(tmp_path / "l.db"))).apply(plan)
    assert decisions[0].reason == "pinned"
    assert new_plan.tasks[0].agent.model == "claude-sonnet-4-5"


def test_policy_never_explores_high_risk(tmp_path):
    t = Task(title="x", instructions="x", id="X", risk=Risk.HIGH)
    plan = Plan(goal="g", tasks=(t,), repo=str(tmp_path))
    pol = Policy(Analytics(Ledger(tmp_path / "l.db")), epsilon=1.0)
    _, decisions = pol.apply(plan)
    assert decisions[0].reason in ("default", "learned")


# --------------------------------------------------------------- budget governor

def test_governor_surplus_recycling(tmp_path):
    t1 = Task(title="a", instructions="a", id="A")
    t2 = Task(title="b", instructions="b", id="B")
    plan = Plan(goal="g", tasks=(t1, t2), repo=str(tmp_path))
    gov = BudgetGovernor(plan=plan, global_seconds=1200,
                         priority={"A": 2, "B": 1})

    async def flow():
        lease_a = await gov.lease("A")
        assert lease_a > 0
        await gov.release("A", used_seconds=lease_a * 0.2)
        pool_before = gov.snapshot()["pool"]
        granted = await gov.request_extension("B", 100.0)
        return pool_before, granted
    pool_before, granted = asyncio.run(flow())
    assert pool_before > 0
    assert 0 < granted <= 100.0


def test_governor_deadline_pressure(tmp_path):
    t = Task(title="a", instructions="a", id="A")
    plan = Plan(goal="g", tasks=(t,), repo=str(tmp_path))
    gov = BudgetGovernor(plan=plan, global_seconds=10_000,
                         priority={"A": 1})
    gov._deadline = time.monotonic() + 100
    lease = asyncio.run(gov.lease("A"))
    assert lease <= 100


# --------------------------------------------------------------- escalation

def test_raise_and_list_open_escalations(tmp_path):
    broker = EscalationBroker(Ledger(tmp_path / "l.db"))
    esc = broker.raise_escalation(
        "p1", "T1", "risky", EscalationKind.GATE_FAILURE,
        "gates failed", {"gates": {}}, branch="sin/delegate/p1/T1")
    open_ = broker.open_escalations("p1")
    assert len(open_) == 1
    assert open_[0]["id"] == esc.id
    ids = [o["id"] for o in open_[0]["options"]]
    assert {"retry", "accept", "drop", "abort"} <= set(ids)


def test_resolve_requires_input_for_retry(tmp_path):
    broker = EscalationBroker(Ledger(tmp_path / "l.db"))
    esc = broker.raise_escalation(
        "p1", "T1", "t", EscalationKind.GATE_FAILURE, "x", {})
    assert not broker.resolve("p1", esc.id, "retry")["ok"]
    assert broker.resolve("p1", esc.id, "retry",
                            user_input="fix")["ok"]


def test_resolve_is_idempotent(tmp_path):
    broker = EscalationBroker(Ledger(tmp_path / "l.db"))
    esc = broker.raise_escalation(
        "p1", "T1", "t", EscalationKind.GATE_FAILURE, "x", {})
    assert broker.resolve("p1", esc.id, "drop")["ok"]
    assert not broker.resolve("p1", esc.id, "accept")["ok"]


def test_apply_resolutions_drop_yields_pending_or_skipped(tmp_path):
    ledger = Ledger(tmp_path / "l.db")
    plan = Plan(goal="g", repo=".",
               tasks=(Task(title="t", instructions="x", id="T1"),))
    broker = EscalationBroker(ledger)
    ledger.register_run(plan.id, "g", json.dumps({
        "goal": "g", "tasks": [{"id": "T1", "title": "t"}]}))
    esc = broker.raise_escalation(
        plan.id, "T1", "t", EscalationKind.GATE_FAILURE, "x", {})
    assert broker.resolve(plan.id, esc.id, "drop")["ok"]
    res = apply_resolutions(plan, ledger)
    assert res["applied"] == 1
    assert ledger.task_states(plan.id)["T1"] == TaskState.SKIPPED


# --------------------------------------------------------------- multirepo

def test_extract_contract_last_block_wins():
    out = ('bla <sin-contract>{"v": 1}</sin-contract> mehr '
           '<sin-contract>{"v": 2}</sin-contract> ende')
    assert extract_contract(out) == {"v": 2}


def test_extract_contract_tolerates_garbage():
    assert extract_contract("kein contract hier") is None
    assert extract_contract("<sin-contract>{kaputt}</sin-contract>") is None


def _git_init(path: Path) -> None:
    path.mkdir(parents=True, exist_ok=True)
    subprocess.run(["git", "init", "-b", "main", str(path)],
                   capture_output=True, check=True)
    subprocess.run(["git", "-C", str(path), "add", "-A"],
                   capture_output=True, check=True)
    subprocess.run(["git", "-C", str(path),
                    "-c", "user.email=t@t", "-c", "user.name=t",
                    "commit", "-m", "init", "--allow-empty"],
                   capture_output=True, check=True)


def test_multirepo_plan_parsing_two_repos(tmp_path):
    api = tmp_path / "api"
    web = tmp_path / "web"
    _git_init(api)
    _git_init(web)
    mrp = multirepo_plan_from_dict({
        "goal": "g",
        "repos": {"api": {"path": str(api)},
                  "web": {"path": str(web)}},
        "tasks": [
            {"key": "a", "repo": "api", "title": "endpoint",
             "instructions": "i"},
            {"key": "b", "repo": "web", "title": "client",
             "instructions": "i", "deps": ["a"]},
        ],
    })
    assert set(mrp.task_repo.values()) == {"api", "web"}
    t_client = next(t for t in mrp.plan.tasks if t.title == "client")
    t_ep = next(t for t in mrp.plan.tasks if t.title == "endpoint")
    assert t_client.deps == (t_ep.id,)


def test_multirepo_plan_rejects_unknown_repo(tmp_path):
    api = tmp_path / "api"
    _git_init(api)
    with pytest.raises(ValueError, match="unknown repo"):
        multirepo_plan_from_dict({
            "goal": "g",
            "repos": {"api": {"path": str(api)}},
            "tasks": [{"key": "a", "repo": "ghost", "title": "t",
                       "instructions": "i"}],
        })


def test_topo_order_respects_deps(tmp_path):
    api = tmp_path / "api"
    _git_init(api)
    mrp = multirepo_plan_from_dict({
        "goal": "g",
        "repos": {"api": {"path": str(api)}},
        "tasks": [
            {"title": "c", "instructions": "i",
             "deps": ["b"]},
            {"title": "b", "instructions": "i",
             "deps": ["a"]},
            {"title": "a", "instructions": "i"},
        ],
    })
    order = _topo_order(mrp.plan)
    by_title = {t.id: t.title for t in mrp.plan.tasks}
    titles = [by_title[tid] for tid in order]
    assert titles.index("a") < titles.index("b") < titles.index("c")


# --------------------------------------------------------------- doctor

def test_check_backend_reports_missing():
    from sin_delegate.doctor import check_backend
    c = check_backend("nonexistent-backend-xyz")
    assert not c.ok and "not found" in c.detail


def test_check_backend_skips_command():
    from sin_delegate.doctor import check_backend
    c = check_backend("command")
    assert c.ok


def test_check_repo_rejects_nonexistent(tmp_path):
    from sin_delegate.doctor import check_repo
    c = check_repo(str(tmp_path / "ghost"))
    assert not c.ok and "does not exist" in c.detail


def test_check_ledger_tolerates_missing(tmp_path):
    from sin_delegate.doctor import check_ledger
    c = check_ledger(str(tmp_path / "doesnt-exist.db"))
    assert c.ok and c.level == "info"
