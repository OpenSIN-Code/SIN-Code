# SPDX-License-Identifier: MIT
"""Event-sourced run ledger on SQLite (WAL).

Every state transition is an immutable event. Current state is a fold over
events — which makes runs crash-safe, resumable, and fully auditable.
"""

from __future__ import annotations

import json
import sqlite3
import time
from contextlib import contextmanager
from pathlib import Path
from typing import Any, Iterator

from .models import TaskState

_SCHEMA = """
CREATE TABLE IF NOT EXISTS runs (
    plan_id    TEXT PRIMARY KEY,
    goal       TEXT NOT NULL,
    plan_json  TEXT NOT NULL,
    created_at REAL NOT NULL
);
CREATE TABLE IF NOT EXISTS events (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    plan_id TEXT NOT NULL,
    task_id TEXT NOT NULL,
    kind    TEXT NOT NULL,
    payload TEXT NOT NULL DEFAULT '{}',
    ts      REAL NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_events_plan ON events(plan_id, task_id, seq);
"""


class Ledger:
    def __init__(self, path: str | Path = "~/.sin-code/delegate/ledger.db") -> None:
        self.path = Path(path).expanduser()
        self.path.parent.mkdir(parents=True, exist_ok=True)
        with self._conn() as db:
            db.executescript(_SCHEMA)

    @contextmanager
    def _conn(self) -> Iterator[sqlite3.Connection]:
        db = sqlite3.connect(self.path, timeout=30)
        db.execute("PRAGMA journal_mode=WAL")
        try:
            yield db
            db.commit()
        finally:
            db.close()

    def register_run(self, plan_id: str, goal: str, plan_json: str) -> None:
        with self._conn() as db:
            db.execute(
                "INSERT OR IGNORE INTO runs(plan_id, goal, plan_json, "
                "created_at) VALUES (?, ?, ?, ?)",
                (plan_id, goal, plan_json, time.time()))

    def emit(self, plan_id: str, task_id: str, kind: str,
             payload: dict[str, Any] | None = None) -> None:
        with self._conn() as db:
            db.execute(
                "INSERT INTO events(plan_id, task_id, kind, payload, ts) "
                "VALUES (?, ?, ?, ?, ?)",
                (plan_id, task_id, kind, json.dumps(payload or {}),
                 time.time()))

    def task_states(self, plan_id: str) -> dict[str, TaskState]:
        states: dict[str, TaskState] = {}
        with self._conn() as db:
            rows = db.execute(
                "SELECT task_id, kind FROM events WHERE plan_id=? "
                "ORDER BY seq",
                (plan_id,)).fetchall()
        for task_id, kind in rows:
            if kind.startswith("state:"):
                states[task_id] = TaskState(kind.split(":", 1)[1])
        return states

    def attempts(self, plan_id: str, task_id: str) -> int:
        with self._conn() as db:
            (n,) = db.execute(
                "SELECT COUNT(*) FROM events WHERE plan_id=? AND "
                "task_id=? AND kind='attempt'",
                (plan_id, task_id)).fetchone()
        return int(n)

    def history(self, plan_id: str) -> list[dict[str, Any]]:
        with self._conn() as db:
            rows = db.execute(
                "SELECT seq, task_id, kind, payload, ts FROM events "
                "WHERE plan_id=? ORDER BY seq",
                (plan_id,)).fetchall()
        return [
            {"seq": s, "task_id": t, "kind": k,
             "payload": json.loads(p), "ts": ts}
            for s, t, k, p, ts in rows
        ]

    def load_plan_json(self, plan_id: str) -> str | None:
        with self._conn() as db:
            row = db.execute(
                "SELECT plan_json FROM runs WHERE plan_id=?",
                (plan_id,)).fetchone()
        return row[0] if row else None

    def list_runs(self, limit: int = 20) -> list[dict[str, Any]]:
        with self._conn() as db:
            rows = db.execute(
                "SELECT plan_id, goal, created_at FROM runs "
                "ORDER BY created_at DESC LIMIT ?",
                (limit,)).fetchall()
        return [
            {"plan_id": p, "goal": g, "created_at": c}
            for p, g, c in rows
        ]
