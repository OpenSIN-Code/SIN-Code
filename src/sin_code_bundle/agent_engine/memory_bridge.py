# SPDX-License-Identifier: MIT
"""Bridge to sin-brain persistent memory (SQLite + FTS5, stdlib only)."""

from __future__ import annotations

import json
import sqlite3
import time
from pathlib import Path
from typing import Any

_SCHEMA = """
CREATE TABLE IF NOT EXISTS agent_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ts REAL NOT NULL,
    task_id TEXT NOT NULL,
    goal TEXT NOT NULL,
    outcome TEXT NOT NULL,
    repair_rounds INTEGER NOT NULL DEFAULT 0,
    lessons TEXT NOT NULL DEFAULT '[]',
    plan_json TEXT NOT NULL DEFAULT '{}'
);
CREATE VIRTUAL TABLE IF NOT EXISTS agent_runs_fts USING fts5(
    goal, lessons, content='agent_runs', content_rowid='id'
);
CREATE TRIGGER IF NOT EXISTS agent_runs_ai AFTER INSERT ON agent_runs BEGIN
    INSERT INTO agent_runs_fts(rowid, goal, lessons)
    VALUES (new.id, new.goal, new.lessons);
END;
"""


class MemoryBridge:
    def __init__(self, db_path: str | None = None) -> None:
        self.db_path = Path(db_path or Path.home() / ".sin" / "agent-memory.db")
        self.db_path.parent.mkdir(parents=True, exist_ok=True)
        with self._conn() as con:
            con.executescript(_SCHEMA)

    def _conn(self) -> sqlite3.Connection:
        con = sqlite3.connect(self.db_path, timeout=10)
        con.row_factory = sqlite3.Row
        return con

    def remember_run(self, *, task_id: str, goal: str, outcome: str,
                    repair_rounds: int, lessons: list[str],
                    plan_json: str) -> None:
        with self._conn() as con:
            con.execute(
                "INSERT INTO agent_runs "
                "(ts, task_id, goal, outcome, repair_rounds, lessons, plan_json) "
                "VALUES (?, ?, ?, ?, ?, ?, ?)",
                (time.time(), task_id, goal, outcome, repair_rounds,
                 json.dumps(lessons, ensure_ascii=False), plan_json),
            )

    def recall_similar(self, goal: str, limit: int = 5) -> list[dict[str, Any]]:
        terms = " OR ".join(
            w for w in "".join(
                c if c.isalnum() or c.isspace() else " " for c in goal
            ).split() if len(w) > 2
        )
        if not terms:
            return []
        with self._conn() as con:
            rows = con.execute(
                "SELECT r.goal, r.outcome, r.repair_rounds, r.lessons "
                "FROM agent_runs_fts f JOIN agent_runs r ON r.id = f.rowid "
                "WHERE agent_runs_fts MATCH ? ORDER BY rank LIMIT ?",
                (terms, limit),
            ).fetchall()
        return [
            {
                "goal": r["goal"],
                "outcome": r["outcome"],
                "repair_rounds": r["repair_rounds"],
                "lessons": json.loads(r["lessons"]),
            }
            for r in rows
        ]
