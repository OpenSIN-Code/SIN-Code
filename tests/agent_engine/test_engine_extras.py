# SPDX-License-Identifier: MIT
"""Tests for repair factory, compactor, checkpoint, policy, delegation, insights."""

from __future__ import annotations

import asyncio
import json
import time
from pathlib import Path

import pytest

import sin_code_bundle.agent_engine.checkpoint as cp_mod
from sin_code_bundle.agent_engine.checkpoint import CheckpointStore
from sin_code_bundle.agent_engine.compactor import ContextCompactor
from sin_code_bundle.agent_engine.delegate import DelegationContext
from sin_code_bundle.agent_engine.insights import TelemetryAnalyzer
from sin_code_bundle.agent_engine.policy_sandbox import (
    PolicyRule, PolicySandbox, PolicyViolation,
)
from sin_code_bundle.agent_engine.repair import LLMRepairFactory
from sin_code_bundle.agent_engine.router import ToolRouter
from sin_code_bundle.agent_engine.types import AgentTask, StepState, Verdict, VerdictKind


# --------------------------------------------------------------- repair

def test_deterministic_lint_repair_runs_once():
    factory = LLMRepairFactory(complete=None)
    verdict = Verdict(kind=VerdictKind.FAIL_LINT, detail="E501 line too long")
    plan1 = asyncio.run(factory.build_repair_plan(AgentTask(goal="g",
                                                            repo_root="."),
                                                    verdict))
    assert plan1 and plan1[0]["tool"] == "sin_bash"
    plan2 = asyncio.run(factory.build_repair_plan(AgentTask(goal="g",
                                                            repo_root="."),
                                                    verdict))
    assert plan2 == []


def test_llm_repair_parses_fenced_json():
    async def fake(prompt: str) -> str:
        return '```json\n' + json.dumps([
            {"step_id": "fix", "tool": "sin_edit",
             "args": {"path": "a.py", "old": "x", "new": "y"}},
            {"step_id": "evil", "tool": "rm_rf", "args": {}},
        ]) + "\n```"
    factory = LLMRepairFactory(complete=fake)
    verdict = Verdict(kind=VerdictKind.FAIL_TESTS, detail="assert 1 == 2")
    plan = asyncio.run(factory.build_repair_plan(AgentTask(goal="g",
                                                           repo_root="."),
                                                   verdict))
    assert [s["step_id"] for s in plan] == ["fix"]


# -------------------------------------------------------------- compactor

def test_compactor_stays_under_budget():
    c = ContextCompactor(budget_chars=2000, hot_count=2)
    for i in range(50):
        c.append(f"s{i}", json.dumps({
            "exit_code": 0, "stdout": "x" * 500, "path": f"f{i}.py",
        }))
    assert c.total_size() <= 2000
    rendered = c.render()
    assert "evicted" in rendered
    assert "s49" in rendered  # hottest entry verbatim


def test_compactor_digest_keeps_facts():
    c = ContextCompactor(budget_chars=100_000, hot_count=1)
    c.append("search1", json.dumps({
        "hits": [{"file": "a.py", "line": 1, "text": "def login"}],
        "truncated": False,
    }))
    c.append("recent", "verbatim content")
    rendered = c.render()
    assert "hit_count" in rendered or "a.py" in rendered


# -------------------------------------------------------------- checkpoint

@pytest.fixture()
def store(tmp_path, monkeypatch):
    monkeypatch.setattr(cp_mod, "_tree_hash", lambda repo: "tree-stable")
    return CheckpointStore("task1", str(tmp_path), base_dir=str(tmp_path))


def test_resume_skips_succeeded_steps(store):
    store.record_step("a", StepState.SUCCEEDED)
    store.record_step("b", StepState.FAILED)
    state = store.load_resume_state()
    assert state.resumable and state.completed_steps == {"a"}
    task = AgentTask(goal="g", repo_root=".")
    from sin_code_bundle.agent_engine.planner import Planner
    plan = Planner().build(task, [
        {"step_id": "a", "tool": "sin_bash", "args": {}},
        {"step_id": "b", "tool": "sin_bash", "args": {}, "deps": ["a"]},
    ])
    skipped = CheckpointStore.apply_to_plan(plan, state)
    assert skipped == ["a"]
    assert plan.steps["a"].state is StepState.SUCCEEDED
    ready = Planner().ready_steps(plan)
    assert [s.step_id for s in ready] == ["b"]


def test_resume_refused_on_changed_workspace(store, monkeypatch):
    store.record_step("a", StepState.SUCCEEDED)
    monkeypatch.setattr(cp_mod, "_tree_hash", lambda repo: "tree-DIFFERENT")
    state = store.load_resume_state()
    assert not state.resumable
    assert "workspace changed" in state.reason


def test_resume_refused_after_run_complete(store):
    store.record_step("a", StepState.SUCCEEDED)
    store.record_run_complete("success")
    assert not store.load_resume_state().resumable


def test_journal_survives_torn_last_line(store):
    store.record_step("a", StepState.SUCCEEDED)
    with store.path.open("a") as fh:
        fh.write('{"ts": 1, "step_id": "b", "sta')  # crash mid-write
    assert store.load_resume_state().completed_steps == {"a"}


# ----------------------------------------------------------------- policy

def test_policy_deny_beats_allow():
    sandbox = PolicySandbox(rules=[
        PolicyRule(action="allow", tool="sin_bash", arg="cmd", pattern="^git"),
        PolicyRule(action="deny", tool="sin_bash", arg="cmd",
                   pattern="push --force", reason="no force push"),
    ])
    allowed, reason = sandbox.decide(
        "sin_bash", {"cmd": "git push --force origin main"})
    assert not allowed and "force" in reason


def test_policy_default_deny():
    sandbox = PolicySandbox(default="deny", rules=[
        PolicyRule(action="allow", tool="sin_bash", arg="cmd",
                   pattern="^pytest"),
    ])
    assert sandbox.decide("sin_bash", {"cmd": "pytest -x"})[0]
    assert not sandbox.decide("sin_bash", {"cmd": "rm -rf /"})[0]
    assert not sandbox.decide("sin_write", {"path": "x.py"})[0]


def test_policy_wrap_blocks_router_call(tmp_path):
    async def echo(**k):
        return k

    async def scenario():
        router = ToolRouter()
        router.register("sin_bash", echo)
        sandbox = PolicySandbox(
            rules=[PolicyRule(action="deny", tool="sin_bash", arg="cmd",
                              pattern="rm -rf", reason="destructive")],
            audit_path=tmp_path / "audit.jsonl",
        )
        sandbox.wrap(router)
        assert await router.call("sin_bash", cmd="ls") == {"cmd": "ls"}
        with pytest.raises(PolicyViolation):
            await router.call("sin_bash", cmd="rm -rf /tmp/x")
        audit = (tmp_path / "audit.jsonl").read_text()
        assert "destructive" in audit
    asyncio.run(scenario())


def test_policy_dry_run_logs_but_allows(tmp_path):
    async def echo(**k):
        return "ran"

    async def scenario():
        router = ToolRouter()
        router.register("sin_bash", echo)
        sandbox = PolicySandbox(
            rules=[PolicyRule(action="deny", tool="sin_bash", arg="cmd",
                              pattern="rm -rf")],
            dry_run=True, audit_path=tmp_path / "audit.jsonl",
        )
        sandbox.wrap(router)
        assert await router.call("sin_bash", cmd="rm -rf /tmp/x") == "ran"
        rec = json.loads(
            (tmp_path / "audit.jsonl").read_text().splitlines()[0])
        assert rec["dry_run"] is True
    asyncio.run(scenario())


# --------------------------------------------------------------- delegation

def test_depth_limit_blocks_fork_bombs():
    ctx = DelegationContext(max_depth=2, budget_deadline=time.monotonic() + 1000)
    child = ctx.child()
    grandchild = child.child()
    assert ctx.can_delegate()[0]
    assert child.can_delegate()[0]
    ok, reason = grandchild.can_delegate()
    assert not ok and "depth" in reason


def test_budget_shrinks_per_generation():
    ctx = DelegationContext(budget_deadline=time.monotonic() + 1000,
                            budget_fraction=0.5)
    child = ctx.child()
    assert child.remaining_s() <= ctx.remaining_s() * 0.5 + 1


def test_tiny_budget_refuses_delegation():
    ctx = DelegationContext(budget_deadline=time.monotonic() + 100,
                            budget_fraction=0.5, min_budget_s=60)
    ok, reason = ctx.can_delegate()
    assert not ok and "budget" in reason


# --------------------------------------------------------------- insights

def _write_events(tmp_path, events):
    log = tmp_path / "events.jsonl"
    log.write_text("\n".join(json.dumps(e) for e in events) + "\n")
    return str(log)


def test_tool_health_flags_chronically_failing_tool(tmp_path):
    events = []
    for i in range(10):
        events.append({"event": "step_start", "step_id": f"e{i}",
                       "tool": "sin_edit"})
        if i < 5:
            events.append({"event": "step_fail", "step_id": f"e{i}"})
    analyzer = TelemetryAnalyzer(_write_events(tmp_path, events))
    crits = [i for i in analyzer.analyze() if i.severity == "critical"]
    assert any("sin_edit" in i.finding for i in crits)


def test_repair_hotspot_detects_lint_dominance(tmp_path):
    events = [{"event": "verdict", "ok": False, "kind": "fail_lint"}
              for _ in range(5)]
    events.append({"event": "verdict", "ok": False, "kind": "fail_tests"})
    analyzer = TelemetryAnalyzer(_write_events(tmp_path, events))
    warns = [i for i in analyzer.analyze() if i.category == "repair_hotspots"]
    assert warns and "fail_lint" in warns[0].finding
    assert "ruff" in warns[0].recommendation


def test_budget_exhaustion_is_critical(tmp_path):
    events = (
        [{"event": "budget_exhausted"} for _ in range(2)]
        + [{"event": "run_complete"} for _ in range(4)]
    )
    analyzer = TelemetryAnalyzer(_write_events(tmp_path, events))
    crits = [i for i in analyzer.analyze()
             if i.severity == "critical" and i.category == "stalls"]
    assert crits and "budget" in crits[0].finding


def test_prompt_rendering_includes_warns(tmp_path):
    events = [{"event": "delegate_done", "outcome": "success"}
              for _ in range(5)]
    analyzer = TelemetryAnalyzer(_write_events(tmp_path, events))
    results = analyzer.analyze()
    block = analyzer.render_for_prompt(results)
    assert "no systemic issues" in block or "[warn]" in block
