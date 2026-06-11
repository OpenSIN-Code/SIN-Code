# SPDX-License-Identifier: MIT
"""Tests for Delegation v2: contract, cache, allocator, supervisor, deadlines."""

from __future__ import annotations

import asyncio
import time

import pytest

from sin_code_bundle.agent_engine.delegate import (
    AdaptiveBudgetAllocator, DelegationCache, DelegationContext,
    DelegationSupervisor, validate_result,
)


def _result(**overrides):
    base = {
        "outcome": "success", "verdict": "pass", "elapsed_s": 1.0,
        "steps_ok": 2, "steps_total": 2, "lessons": [],
        "depth": 1, "delegation_id": "abc", "cached": False,
    }
    return {**base, **overrides}


# ------------------------------------------------------------- contract

def test_contract_strips_unknown_keys_and_caps_lessons():
    raw = _result(lessons=[f"lesson-{i}" * 100 for i in range(20)])
    raw["malicious_context_dump"] = "x" * 100_000
    clean = validate_result(raw)
    assert "malicious_context_dump" not in clean
    assert len(clean["lessons"]) == 5
    assert all(len(l) <= 300 for l in clean["lessons"])


def test_contract_rejects_wrong_types():
    with pytest.raises(ValueError, match="steps_ok"):
        validate_result(_result(steps_ok="two"))
    with pytest.raises(ValueError, match="missing"):
        bad = _result()
        del bad["verdict"]
        validate_result(bad)


# ---------------------------------------------------------------- cache

def test_cache_only_stores_successes_and_respects_tree():
    cache = DelegationCache()
    k1 = DelegationCache.fingerprint("g", [], "tree-A")
    k2 = DelegationCache.fingerprint("g", [], "tree-B")
    assert k1 != k2

    cache.put(k1, _result(outcome="failed:tests"))
    assert cache.get(k1) is None
    cache.put(k1, _result())
    assert cache.get(k1)["outcome"] == "success"


def test_cache_ttl_expiry(monkeypatch):
    cache = DelegationCache(ttl_s=10)
    key = DelegationCache.fingerprint("g", [], "t")
    cache.put(key, _result())
    real = time.monotonic()
    monkeypatch.setattr(time, "monotonic", lambda: real + 11)
    assert cache.get(key) is None


# ------------------------------------------------------------ allocator

class _FakeMemory:
    def __init__(self, hits):
        self._hits = hits

    def recall_similar(self, goal, limit=10):
        return self._hits


def test_allocator_uses_history_p75():
    hits = [{"outcome": "success", "elapsed_s": s}
            for s in (10, 20, 30, 40, 100)]
    alloc = AdaptiveBudgetAllocator(_FakeMemory(hits), min_s=5)
    grant = alloc.grant("refactor auth", parent_remaining_s=1000)
    # p75 von [10,20,30,40,100] = 40 -> *1.5 = 60, weit unter cap (500)
    assert 55 <= grant <= 65


def test_allocator_falls_back_to_fraction_without_history():
    alloc = AdaptiveBudgetAllocator(_FakeMemory([]), default_fraction=0.5)
    assert alloc.grant("anything", parent_remaining_s=800) == 400


def test_allocator_never_exceeds_parent_cap():
    hits = [{"outcome": "success", "elapsed_s": 5000} for _ in range(5)]
    alloc = AdaptiveBudgetAllocator(_FakeMemory(hits), default_fraction=0.5)
    assert alloc.grant("huge", parent_remaining_s=600) <= 300


def test_goal_class_normalization():
    assert AdaptiveBudgetAllocator.goal_class("Refactor The Auth Module") == "refactor"
    assert AdaptiveBudgetAllocator.goal_class("123 numbers in name") == "123"
    assert AdaptiveBudgetAllocator.goal_class("") == "misc"


# -------------------------------------------------------------- context

def test_child_deadline_never_exceeds_parent():
    parent = DelegationContext(deadline_wall=time.monotonic() + 200,
                               safety_margin_s=20)
    child = parent.child(granted_budget_s=10_000)
    assert child.remaining_s() <= parent.remaining_s() - 19


def test_can_delegate_blocks_when_budget_too_low():
    ctx = DelegationContext(deadline_wall=time.monotonic() + 50,
                            safety_margin_s=20, min_budget_s=60)
    ok, reason = ctx.can_delegate()
    assert not ok and "budget" in reason


def test_can_delegate_blocks_at_max_depth():
    ctx = DelegationContext(depth=3, max_depth=3,
                            deadline_wall=time.monotonic() + 1000)
    ok, reason = ctx.can_delegate()
    assert not ok and "depth" in reason


# ------------------------------------------------------------ supervisor

def test_global_limit_caps_breadth_not_just_depth():
    async def scenario():
        sup = DelegationSupervisor(global_limit=2)
        running = {"now": 0, "peak": 0}

        async def child(i):
            running["now"] += 1
            running["peak"] = max(running["peak"], running["now"])
            await asyncio.sleep(0.05)
            running["now"] -= 1
            return i

        results = await asyncio.gather(*[
            sup.supervise(delegation_id=f"d{i}", goal="g", depth=1,
                          coro=child(i))
            for i in range(6)
        ])
        assert sorted(results) == list(range(6))
        assert running["peak"] <= 2

    asyncio.run(scenario())


def test_cancel_all_kills_tree():
    async def scenario():
        sup = DelegationSupervisor(global_limit=4)

        async def hang():
            await asyncio.sleep(60)

        tasks = [
            asyncio.ensure_future(sup.supervise(
                delegation_id=f"d{i}", goal="g", depth=1, coro=hang()))
            for i in range(3)
        ]
        await asyncio.sleep(0.05)
        assert len(sup.active()) == 3
        assert sup.cancel_all() == 3
        results = await asyncio.gather(*tasks, return_exceptions=True)
        assert all(isinstance(r, asyncio.CancelledError) for r in results)

    asyncio.run(scenario())
