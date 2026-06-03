"""Memory/Hindsight Integration for SIN-Code v2.

Docs: memory.doc.md

Persistent memory layer with a best-effort SCKG backend and an
in-memory + SQLite fallback. Inspired by retain/recall/reflect
patterns from Hindsight / Letta. Tree-sitter and SCKG are *optional*
imports; SQLite is the durable source of truth.
"""
from __future__ import annotations

import json
import sqlite3
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional


class SINMemory:
    """Memory system with optional SCKG semantic backend.

    Usage:
        mem = SINMemory(Path("/path/to/repo"))
        mem.retain("User prefers TypeScript", tags=["preference"])
        facts = mem.recall("typescript")
    """

    def __init__(
        self,
        repo_root: Optional[Path] = None,
        db_path: Optional[Path] = None,
    ) -> None:
        self.repo_root = repo_root or Path.cwd()
        self.db_path = db_path or (self.repo_root / ".sin_memory.db")
        self._sckg: Optional[Any] = None
        # SQLite is always-on (durable, no extra deps)
        self._init_db()
        # SCKG is best-effort (semantic layer, optional dep)
        self._try_init_sckg()

    def _init_db(self) -> None:
        """Initialize SQLite store (always works, even if SCKG is missing)."""
        self.db_path.parent.mkdir(parents=True, exist_ok=True)
        with sqlite3.connect(str(self.db_path)) as conn:
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS memories (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    fact TEXT NOT NULL,
                    context TEXT,
                    tags TEXT,
                    created_at TEXT NOT NULL
                )
                """
            )
            conn.execute(
                """
                CREATE INDEX IF NOT EXISTS idx_memories_tags
                ON memories(tags)
                """
            )

    def _try_init_sckg(self) -> None:
        """Try to load SCKG as semantic backend (best-effort)."""
        try:
            from sin_code_sckg import KnowledgeGraph  # type: ignore

            self._sckg = KnowledgeGraph(str(self.repo_root))
        except ImportError:
            self._sckg = None
        except Exception:
            # SCKG init can fail for any reason (corrupt index, missing
            # model, etc.) — degrade gracefully to SQLite-only.
            self._sckg = None

    def retain(
        self,
        fact: str,
        context: Optional[Dict[str, Any]] = None,
        tags: Optional[List[str]] = None,
    ) -> Dict[str, Any]:
        """Store a fact in memory.

        Always persists to SQLite. Also adds to SCKG if available
        (as a ``memory`` node so it's queryable via the graph).
        """
        context = context or {}
        tags = tags or []
        created_at = datetime.now(timezone.utc).isoformat()

        with sqlite3.connect(str(self.db_path)) as conn:
            cur = conn.execute(
                "INSERT INTO memories (fact, context, tags, created_at) "
                "VALUES (?, ?, ?, ?)",
                (
                    fact,
                    json.dumps(context),
                    ",".join(tags),
                    created_at,
                ),
            )
            mem_id = cur.lastrowid

        stored_in = "SQLite"
        # Best-effort SCKG storage — SQLite is the source of truth
        if self._sckg is not None:
            try:
                self._sckg.add_node(
                    node_id=f"memory:{mem_id}",
                    node_type="memory",
                    name=fact[:100],
                    properties={"context": context, "tags": tags},
                )
                stored_in = "SQLite + SCKG"
            except Exception:
                # SCKG storage is best-effort; never fail a retain
                pass

        return {
            "success": True,
            "id": mem_id,
            "stored_in": stored_in,
            "created_at": created_at,
        }

    def recall(
        self,
        query: str,
        limit: int = 10,
        tags: Optional[List[str]] = None,
    ) -> List[Dict[str, Any]]:
        """Search memory for facts matching ``query``.

        Uses LIKE-based search on SQLite. Tags are AND-combined.
        Results are ordered newest-first.
        """
        with sqlite3.connect(str(self.db_path)) as conn:
            sql = (
                "SELECT id, fact, context, tags, created_at "
                "FROM memories WHERE fact LIKE ?"
            )
            params: List[Any] = [f"%{query}%"]
            if tags:
                for tag in tags:
                    sql += " AND tags LIKE ?"
                    params.append(f"%{tag}%")
            sql += " ORDER BY created_at DESC LIMIT ?"
            params.append(limit)
            rows = conn.execute(sql, params).fetchall()

        return [
            {
                "id": row[0],
                "fact": row[1],
                "context": json.loads(row[2] or "{}"),
                "tags": row[3].split(",") if row[3] else [],
                "created_at": row[4],
            }
            for row in rows
        ]

    def reflect(
        self,
        query: str,
        context: Optional[Dict[str, Any]] = None,
    ) -> Dict[str, Any]:
        """Synthesize an answer from memory.

        Simple implementation: recall relevant facts and concatenate.
        For higher confidence, pair with an LLM synthesizer (out of scope).
        """
        facts = self.recall(query, limit=5)
        if not facts:
            return {
                "answer": "No relevant memories found.",
                "sources": [],
                "confidence": 0.0,
            }
        answer = "\n\n".join(f"- {f['fact']}" for f in facts)
        return {
            "answer": answer,
            "sources": [f["id"] for f in facts],
            "confidence": 0.5,
            "note": "Use a stronger synthesizer (LLM) for higher confidence",
        }

    def forget(self, memory_id: int) -> bool:
        """Remove a fact from memory by ID. Returns True if a row was deleted."""
        with sqlite3.connect(str(self.db_path)) as conn:
            cur = conn.execute("DELETE FROM memories WHERE id = ?", (memory_id,))
            return cur.rowcount > 0

    def get_stats(self) -> Dict[str, Any]:
        """Get memory statistics: total count, distinct tags, backend."""
        with sqlite3.connect(str(self.db_path)) as conn:
            total = conn.execute("SELECT COUNT(*) FROM memories").fetchone()[0]
            tag_rows = conn.execute(
                "SELECT tags FROM memories WHERE tags != ''"
            ).fetchall()
        all_tags: set[str] = set()
        for row in tag_rows:
            for t in row[0].split(","):
                if t:
                    all_tags.add(t)
        return {
            "total_facts": total,
            "tags": sorted(all_tags),
            "backend": "SQLite + SCKG" if self._sckg else "SQLite only",
        }


__all__ = ["SINMemory"]
