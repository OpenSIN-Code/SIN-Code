# SPDX-License-Identifier: MIT
"""Tests for the agent engine core (types, planner, router, telemetry, builtin tools, loop)."""

from __future__ import annotations

import asyncio
import json
import time
from pathlib import Path

import pytest

import sin_code_bundle.agent_engine as ae
from sin_code_bundle.agent_engine import (
    AgentLoop, AgentTask, CircuitOpenError, Executor, MemoryBridge,
    Planner, Plan, Step, StepState, Telemetry, ToolRouter, Verdict,
    VerdictKind, Verifier, register_builtin_tools,
)


def make_plan(specs):
    task = AgentTask(goal="t", repo_root=".")
    return task, Planner().build(task, specs)


def test_plan_rejects_cycle():
    with pytest.raises(ValueError, match="cycle"):
        make_plan([
            {"step_id": "a", "tool": "x", "deps": ["b"]},
            {"step_id": "b", "tool": "x", "deps": ["a"]},
        ])


def test_critical_path_ordering():
    _, plan = make_plan([
        {"step_id": "root", "tool": "x", "estimated_cost": 1},
        {"step_id": "long1", "tool": "x", "deps": ["root"], "estimated_cost": 10},
        {"step_id": "long2", "tool": "x", "deps": ["long1"], "estimated_cost": 10},
        {"step_id": "short", "tool": "x", "deps": ["root"], "estimated_cost": 1},
    ])
    w = Planner().critical_path_weights(plan)
    assert w["long1"] > w["short"]
    assert w["root"] > w["long1"]


def test_failure_propagation_skips_dependents():
    _, plan = make_plan([
        {"step_id": "a", "tool": "x"},
        {"step_id": "b", "tool": "x", "deps": ["a"]},
        {"step_id": "c", "tool": "x", "deps": ["b"]},
    ])
    plan.steps["a"].state = StepState.FAILED
    skipped = Planner().propagate_failure(plan, "a")
    assert set(skipped) == {"b", "c"}
    assert plan.steps["c"].state is StepState.SKIPPED


def test_router_circuit_breaker_opens():
    async def boom(**_):
        raise RuntimeError("kaput")

    async def scenario():
        r = ToolRouter(max_retries=1)
        r.register("flaky", boom, failure_threshold=2, cooldown_s=60)
        for _ in range(2):
            with pytest.raises(RuntimeError):
                await r.call("flaky")
        with pytest.raises(CircuitOpenError):
            await r.call("flaky")

    asyncio.run(scenario())


def test_router_success_resets_circuit():
    calls = {"n": 0}

    async def sometimes(**_):
        calls["n"] += 1
        if calls["n"] < 2:
            raise RuntimeError("transient")
        return "ok"

    async def scenario():
        r = ToolRouter(max_retries=3, base_delay_s=0.01, max_delay_s=0.02)
        r.register("t", sometimes)
        assert await r.call("t") == "ok"
        assert r.stats()["t"]["circuit"] == "closed"

    asyncio.run(scenario())


def test_telemetry_emits_to_jsonl(tmp_path):
    log = tmp_path / "events.jsonl"
    t = Telemetry(log_path=str(log))
    t.emit("test_event", x=1)
    t.emit("test_event", x=2)
    lines = log.read_text(encoding="utf-8").splitlines()
    assert len(lines) == 2
    rec = json.loads(lines[0])
    assert rec["event"] == "test_event" and rec["x"] == 1
    assert t.summary()["events"]["test_event"] == 2


def test_memory_bridge_roundtrip(tmp_path):
    db = tmp_path / "mem.db"
    mb = MemoryBridge(db_path=str(db))
    mb.remember_run(task_id="t1", goal="fix the auth bug", outcome="success",
                   repair_rounds=1, lessons=["check token expiry first"],
                   plan_json="{}")
    hits = mb.recall_similar("auth bug fix", limit=3)
    assert len(hits) == 1
    assert hits[0]["outcome"] == "success"
    assert "token expiry" in hits[0]["lessons"][0]


def test_builtin_tools_work_in_tempdir(tmp_path):
    cwd = str(tmp_path)
    async def scenario():
        router = register_builtin_tools(ToolRouter())
        # write
        r = await router.call("sin_write", path="a.txt", content="hello\nworld\n",
                              cwd=cwd)
        assert r["bytes"] == 12
        # read
        r = await router.call("sin_read", path="a.txt", cwd=cwd, start=1, limit=10)
        assert "hello" in r["content"]
        # edit (anchored)
        r = await router.call("sin_edit", path="a.txt", old="hello",
                              new="HI", cwd=cwd)
        assert r["replaced"] == 1
        # search
        r = await router.call("sin_search", pattern="world", cwd=cwd, glob="*.txt")
        assert len(r["hits"]) == 1
        # bash
        r = await router.call("sin_bash", cmd="cat a.txt", cwd=cwd)
        assert "HI" in r["stdout"]
        # edit ambiguous fails (router wraps tool errors in RuntimeError)
        await router.call("sin_write", path="b.txt", content="x\nx\n", cwd=cwd)
        with pytest.raises(RuntimeError, match="ambiguous"):
            await router.call("sin_edit", path="b.txt", old="x",
                              new="y", cwd=cwd)
        # edit anchor not found
        with pytest.raises(RuntimeError, match="not found"):
            await router.call("sin_edit", path="a.txt", old="nope",
                              new="x", cwd=cwd)
    asyncio.run(scenario())


def test_agent_task_fingerprint_stable():
    a = AgentTask(goal="g", repo_root=".")
    b = AgentTask(goal="g", repo_root=".")
    c = AgentTask(goal="different", repo_root=".")
    assert a.fingerprint() == b.fingerprint()
    assert a.fingerprint() != c.fingerprint()


def test_step_state_enum_complete():
    states = {s.value for s in StepState}
    assert {"pending", "ready", "running", "succeeded",
            "failed", "skipped", "repairing"} <= states


def test_verdict_kind_pass_property():
    v = Verdict(kind=VerdictKind.PASS)
    assert v.ok
    v2 = Verdict(kind=VerdictKind.FAIL_TESTS)
    assert not v2.ok


def test_executor_runs_and_propagates_failures():
    async def scenario():
        telemetry = Telemetry()
        router = ToolRouter()
        async def ok_tool(**_):
            return "ok"
        async def fail_tool(**_):
            raise RuntimeError("nope")
        router.register("ok", ok_tool)
        router.register("fail", fail_tool)
        executor = Executor(router, telemetry)
        task = AgentTask(goal="g", repo_root=".")
        plan = Planner().build(task, [
            {"step_id": "a", "tool": "ok", "args": {}, "max_attempts": 1},
            {"step_id": "b", "tool": "fail", "args": {}, "deps": ["a"],
             "max_attempts": 1},
            {"step_id": "c", "tool": "ok", "args": {}, "deps": ["b"],
             "max_attempts": 1},
        ])
        results = await executor.run(task, plan, Planner())
        assert results["a"].ok
        assert not results["b"].ok
        assert "c" not in results  # skipped, no result row
        assert plan.steps["c"].state is StepState.SKIPPED
        # pending hook fired
        assert telemetry.counters.get("step_fail", 0) >= 1
    asyncio.run(scenario())


def test_synthesize_critique_fallback_on_garbage():
    from sin_code_bundle.agent_engine.synthesizer import PlanSynthesizer
    calls = {"n": 0}

    async def fake(prompt: str) -> str:
        calls["n"] += 1
        if calls["n"] == 1:
            return json.dumps([
                {"step_id": "a", "tool": "sin_bash",
                 "args": {"cmd": "pytest"}, "deps": []},
            ])
        return "garbage no json"

    s = PlanSynthesizer(complete=fake, critique=True)
    specs = asyncio.run(s.synthesize(AgentTask(goal="x", repo_root=".")))
    assert [x["step_id"] for x in specs] == ["a"]


def test_synthesizer_refuses_without_llm():
    from sin_code_bundle.agent_engine.synthesizer import PlanSynthesizer
    s = PlanSynthesizer(complete=None)
    with pytest.raises(RuntimeError, match="refusing to hallucinate"):
        asyncio.run(s.synthesize(AgentTask(goal="x", repo_root=".")))


def test_dashboard_state_lifecycle():
    from sin_code_bundle.agent_engine.watch import DashboardState
    d = DashboardState()
    d.apply({"event": "plan_built", "task_id": "t1"})
    d.apply({"event": "step_start", "step_id": "s1", "tool": "sin_edit",
             "attempt": 1})
    d.apply({"event": "step_ok", "step_id": "s1", "duration_s": 1.2})
    d.apply({"event": "step_start", "step_id": "s2", "tool": "sin_bash",
             "attempt": 1})
    d.apply({"event": "step_fail", "step_id": "s2", "skipped": ["s3"]})
    d.apply({"event": "verdict", "round": 0, "ok": False, "kind": "fail_tests"})
    d.apply({"event": "swarm_member_done", "member": "be", "ok": True})
    assert d.task_id == "t1"
    assert d.steps["s1"].state == "ok"
    assert d.steps["s2"].state == "fail"
    assert d.steps["s3"].state == "skip"
    assert d.swarm["be"] == "ok"
    rendered = d.render(color=False)
    assert "s1" in rendered and "FAIL" in rendered.upper()
