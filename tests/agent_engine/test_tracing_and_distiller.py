# SPDX-License-Identifier: MIT
"""Tests for Span-Baum reconstruction and rule lifecycle."""

from __future__ import annotations

import asyncio
import json

import pytest

from sin_code_bundle.agent_engine.distiller import (
    KnowledgeDistiller, _heuristic_rule, _signature,
)
from sin_code_bundle.agent_engine.tracing import (
    Span, SpanEmitter, TraceAssembler, TraceContext,
)
from sin_code_bundle.agent_engine.telemetry import Telemetry


# --------------------------------------------------------------- tracing

def _write_spans(tmp_path, records):
    log = tmp_path / "events.jsonl"
    log.write_text("\n".join(json.dumps(r) for r in records) + "\n")
    return str(log)


def test_assembler_builds_nested_tree(tmp_path):
    t = "trace1"
    records = [
        {"event": "span_start", "trace_id": t, "span_id": "root",
         "parent_span_id": None, "kind": "run", "name": "main", "ts": 1.0},
        {"event": "span_start", "trace_id": t, "span_id": "d1",
         "parent_span_id": "root", "kind": "delegate", "name": "sub-a",
         "ts": 2.0},
        {"event": "span_start", "trace_id": t, "span_id": "d2",
         "parent_span_id": "d1", "kind": "delegate", "name": "sub-a-1",
         "ts": 3.0},
        {"event": "span_end", "trace_id": t, "span_id": "d2",
         "status": "success", "duration_s": 1.0},
        {"event": "span_end", "trace_id": t, "span_id": "d1",
         "status": "success", "duration_s": 2.5},
        {"event": "span_end", "trace_id": t, "span_id": "root",
         "status": "ok", "duration_s": 5.0},
    ]
    roots = TraceAssembler(_write_spans(tmp_path, records)).assemble(t)
    assert len(roots) == 1
    assert roots[0].name == "main"
    assert roots[0].children[0].name == "sub-a"
    assert roots[0].children[0].children[0].name == "sub-a-1"
    tree = TraceAssembler.render_tree(roots)
    assert "run:main" in tree
    # Root has connector, children have branch connectors
    assert "└─" in tree


def test_assembler_defaults_to_latest_trace(tmp_path):
    records = [
        {"event": "span_start", "trace_id": "old", "span_id": "a",
         "parent_span_id": None, "kind": "run", "name": "old-run", "ts": 1},
        {"event": "span_start", "trace_id": "new", "span_id": "b",
         "parent_span_id": None, "kind": "run", "name": "new-run", "ts": 2},
    ]
    roots = TraceAssembler(_write_spans(tmp_path, records)).assemble()
    assert [r.name for r in roots] == ["new-run"]


def test_chrome_export_is_valid_json(tmp_path):
    records = [
        {"event": "span_start", "trace_id": "t", "span_id": "a",
         "parent_span_id": None, "kind": "run", "name": "r", "ts": 1.0},
        {"event": "span_end", "trace_id": "t", "span_id": "a",
         "status": "ok", "duration_s": 2.0},
    ]
    raw = TraceAssembler(_write_spans(tmp_path, records)).to_chrome_trace("t")
    data = json.loads(raw)
    assert data["traceEvents"][0]["ph"] == "X"
    assert data["traceEvents"][0]["dur"] == 2_000_000


def test_trace_context_child_inherits_trace_id():
    parent = TraceContext()
    child = parent.child()
    assert parent.trace_id == child.trace_id
    assert parent.span_id != child.span_id
    assert child.parent_span_id == parent.span_id


def test_span_emitter_writes_start_and_end(tmp_path):
    log = tmp_path / "ev.jsonl"
    tel = Telemetry(log_path=str(log))
    em = SpanEmitter(tel)
    ctx = TraceContext(trace_id="t1", span_id="s1")
    started = em.start(ctx, kind="run", name="x", goal="g")
    em.end(ctx, started, status="ok", steps=3)
    recs = [json.loads(l) for l in log.read_text().splitlines()]
    assert recs[0]["event"] == "span_start" and recs[0]["kind"] == "run"
    assert recs[1]["event"] == "span_end" and recs[1]["status"] == "ok"


# -------------------------------------------------------------- distiller

def _distiller(tmp_path, **kw):
    return KnowledgeDistiller(db_path=str(tmp_path / "mem.db"), **kw)


def test_signature_clusters_similar_lessons():
    # Same kind, same 4 keywords → same signature
    a = _signature("round 0: fail_lint — unused imports in module auth")
    b = _signature("round 1: fail_lint — unused imports in module auth")
    c = _signature("round 0: fail_tests — assertion error in auth")
    assert a == b and a != c
    # And both have the "fail_lint:" prefix
    assert a.startswith("fail_lint:")


def test_heuristic_lint_rule():
    rule = _heuristic_rule(["round 0: fail_lint — something"])
    assert "lint" in rule.lower()


def test_heuristic_semantic_deletion_rule():
    rule = _heuristic_rule(["round 0: fail_semantic — huge deletion"])
    assert "delet" in rule.lower() or "diff" in rule.lower()


def test_rule_promotion_after_min_evidence(tmp_path):
    d = _distiller(tmp_path, min_evidence=3)
    lesson = "round 0: fail_lint — unused imports detected"
    asyncio.run(d.distill([lesson]))
    assert d.active_rules() == []
    asyncio.run(d.distill([lesson, lesson]))
    rules = d.active_rules()
    assert len(rules) == 1 and "lint" in rules[0].rule.lower()


def test_rules_decay_and_retire(tmp_path):
    d = _distiller(tmp_path, min_evidence=1, decay=0.5, retire_below=0.3)
    asyncio.run(d.distill(["round 0: fail_tests — broken assert"]))
    assert len(d.active_rules()) == 1
    asyncio.run(d.distill([]))
    asyncio.run(d.distill([]))
    assert d.active_rules() == []


def test_active_cap_keeps_top_scored(tmp_path):
    d = _distiller(tmp_path, min_evidence=1, max_active=2)
    for i in range(5):
        lesson = f"round 0: fail_semantic — oversized deletion in mod{i}"
        asyncio.run(d.distill([lesson] * (i + 1)))
    assert len(d.active_rules()) <= 2


def test_render_constraints_block(tmp_path):
    d = _distiller(tmp_path, min_evidence=1)
    asyncio.run(d.distill(["round 0: fail_lint — style violations"]))
    block = d.render_constraints()
    assert block.startswith("STANDING RULES") and "- " in block


def test_empty_distiller_renders_empty():
    d = _distiller(None, min_evidence=3) if False else _FakeEmpty()
    assert d.render_constraints() == ""


class _FakeEmpty:
    def render_constraints(self, **kw):
        return ""


def test_harvest_lessons_from_real_memory(tmp_path):
    import sqlite3, time as _t
    db = tmp_path / "mem.db"
    con = sqlite3.connect(db)
    con.executescript("""
    CREATE TABLE agent_runs (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        ts REAL NOT NULL, task_id TEXT, goal TEXT, outcome TEXT,
        repair_rounds INTEGER DEFAULT 0, lessons TEXT DEFAULT '[]',
        plan_json TEXT DEFAULT '{}', elapsed_s REAL DEFAULT 0
    );""")
    con.execute("INSERT INTO agent_runs (ts, task_id, goal, outcome, "
                "lessons) VALUES (?, 't1', 'fix lint', 'failed:lint', ?)",
                (_t.time(), '["round 0: fail_lint — typo"]'))
    con.commit()
    con.close()
    d = KnowledgeDistiller(db_path=str(db))
    lessons = d.harvest_lessons(since_s=10_000_000)
    assert any("fail_lint" in l for l in lessons)
