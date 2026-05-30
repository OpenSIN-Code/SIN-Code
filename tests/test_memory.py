"""Tests for the SIN-Brain memory adapter (BR-1, Issue #14).

These run without `sin_brain` installed: we simulate presence/absence by
injecting fake modules into sys.modules, so the bundle's graceful-degradation
contract is verified in isolation.
"""

from __future__ import annotations

import json
import sys
import types

import pytest

from sin_code_bundle import memory


@pytest.fixture
def fake_sin_brain(monkeypatch):
    """Inject a fake `sin_brain` + `sin_brain.mcp_tools` into sys.modules."""
    pkg = types.ModuleType("sin_brain")

    def stats():
        return {"db_path": "/tmp/sin-brain.db", "tiers": {"core": 3, "recall": 42}}

    pkg.stats = stats

    tools = types.ModuleType("sin_brain.mcp_tools")
    tools.recall = lambda query, scope, k: json.dumps({"hits": [query, scope, k]})
    tools.remember = lambda content, kind, ttl_days, scope: json.dumps({"id": "m1", "kind": kind})
    tools.forget = lambda id: json.dumps({"forgot": id})
    tools.pin = lambda id: json.dumps({"pinned": id})
    tools.link_evidence = lambda entity, verdict, source: json.dumps(
        {"entity": entity, "verdict": verdict, "source": source}
    )
    pkg.mcp_tools = tools

    monkeypatch.setitem(sys.modules, "sin_brain", pkg)
    monkeypatch.setitem(sys.modules, "sin_brain.mcp_tools", tools)
    # importlib.util.find_spec relies on a real spec; give the module one.
    pkg.__spec__ = types.SimpleNamespace(name="sin_brain")
    return pkg


class FakeMCP:
    """Minimal stand-in for FastMCP capturing registered tool names."""

    def __init__(self):
        self.registered: list[str] = []

    def tool(self):
        def deco(fn):
            self.registered.append(fn.__name__)
            return fn

        return deco


# --------------------------- graceful degradation --------------------------- #
def test_detect_env_absent():
    env = memory.detect_env()
    assert env.available is False
    assert env.tiers == {}


def test_operations_raise_when_absent():
    with pytest.raises(memory.MemoryUnavailable):
        memory.recall("anything")
    with pytest.raises(memory.MemoryUnavailable):
        memory.forget("x")


def test_register_tools_noop_when_absent():
    mcp = FakeMCP()
    assert memory.register_tools(mcp) == []
    assert mcp.registered == []


# ----------------------------- with sin-brain ------------------------------ #
def test_detect_env_present(fake_sin_brain):
    env = memory.detect_env()
    assert env.available is True
    assert env.db_path == "/tmp/sin-brain.db"
    assert env.tiers["recall"] == 42


def test_recall_passthrough(fake_sin_brain):
    out = json.loads(memory.recall("login bug", scope="archival", k=3))
    assert out["hits"] == ["login bug", "archival", 3]


def test_remember_validates_kind(fake_sin_brain):
    with pytest.raises(ValueError):
        memory.remember("x", kind="bogus")
    out = json.loads(memory.remember("use RS256", kind="decision"))
    assert out["kind"] == "decision"


def test_link_evidence_validates_source(fake_sin_brain):
    with pytest.raises(ValueError):
        memory.link_evidence("mod.fn", "pass", source="bogus")
    out = json.loads(memory.link_evidence("mod.fn", "pass", source="oracle"))
    assert out["source"] == "oracle"


def test_register_tools_wires_all_five(fake_sin_brain):
    mcp = FakeMCP()
    names = memory.register_tools(mcp)
    assert set(names) == set(memory.TOOL_NAMES)
    assert len(mcp.registered) == 5
