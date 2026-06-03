# SPDX-License-Identifier: MIT
"""Memory/Hindsight Integration for SIN-Code v2.

Docs: memory.doc.md

Persistent memory layer with a best-effort SCKG backend, an optional
Honcho behavioral-memory backend, and a durable SQLite fallback.
Inspired by retain/recall/reflect patterns from Hindsight / Letta.
Tree-sitter, SCKG, and Honcho are *optional* imports; SQLite is the
durable source of truth.
"""

from __future__ import annotations

import json
import sqlite3
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional


# ── HonchoBackend: Behavioral Memory (Optional) ─────────────────────
class HonchoBackend:
    """Optional Honcho behavioral-memory backend.

    Lazily initializes a Honcho client. If the package is not installed
    or the server is unreachable, ``is_available()`` returns False and
    all methods become safe no-ops (return ``None`` / empty list).

    Usage:
        backend = HonchoBackend(workspace_id="sin-bundle")
        if backend.is_available():
            backend.retain_message(
                peer_name="coding-agent",
                content="User prefers TypeScript",
                role="user",
            )
            insights = backend.chat(
                "coding-agent",
                "What does the user prefer?",
            )
    """

    def __init__(
        self,
        workspace_id: str = "sin-bundle",
        base_url: str = "http://localhost:8000",
        timeout: float = 1.0,
    ) -> None:
        # Short timeout: we never want a Honcho outage to block retain/recall.
        # fast-fail: agents shouldn't wait 8s for an unreachable Honcho server.
        self.workspace_id = workspace_id
        self.base_url = base_url
        self.timeout = timeout
        self._honcho: Optional[Any] = None
        self._available: bool = False
        # _init_attempted caches the import + connectivity probe so we don't
        # retry on every method call (cheap operation, but still wasteful).
        self._init_attempted: bool = False
        self._init_error: Optional[str] = None

    def _try_init(self) -> None:
        """Lazily initialize Honcho. Caches the result of the attempt.

        Called once per backend lifetime; the result is memoized via
        ``_init_attempted`` so subsequent ``is_available()`` / method
        calls skip the import + connectivity probe entirely.
        """
        if self._init_attempted:
            return
        self._init_attempted = True
        try:
            from honcho import Honcho  # type: ignore[import-not-found]

            # Initialize with explicit short timeout + 0 retries to fail fast.
            # max_retries=0: first try only, no exponential backoff for status checks.
            self._honcho = Honcho(
                workspace_id=self.workspace_id,
                base_url=self.base_url,
                timeout=self.timeout,
                max_retries=0,
            )
            # Cheap connectivity check — fail fast if server is unreachable.
            # Listing peers is a no-arg call that exercises the full HTTP path.
            list(self._honcho.peers())
            self._available = True
        except Exception as e:  # broad: import error, network, API mismatch
            self._init_error = str(e)
            self._available = False
            self._honcho = None

    def is_available(self) -> bool:
        """Check whether Honcho is reachable. Caches the result."""
        if not self._init_attempted:
            self._try_init()
        return self._available

    def get_status(self) -> Dict[str, Any]:
        """Return a status dict for diagnostics / ``get_stats`` output.

        Triggers lazy init if it hasn't run yet, so a single call to
        ``get_status()`` is sufficient to surface connection errors
        without separately calling ``is_available()`` first.

        Returns:
            Dict with keys: ``available`` (bool), ``workspace_id`` (str),
            ``base_url`` (str), ``error`` (str | None — set when the
            last init attempt failed).
        """
        if not self._init_attempted:
            self._try_init()
        return {
            "available": self._available,
            "workspace_id": self.workspace_id,
            "base_url": self.base_url,
            "error": self._init_error,
        }

    def get_or_create_peer(self, name: str) -> Optional[Any]:
        """Get or create a Honcho peer. Returns ``None`` if unavailable."""
        if not self.is_available() or self._honcho is None:
            return None
        try:
            return self._honcho.peer(name)
        except Exception:
            return None

    def get_or_create_session(self, name: str) -> Optional[Any]:
        """Get or create a Honcho session. Returns ``None`` if unavailable."""
        if not self.is_available() or self._honcho is None:
            return None
        try:
            return self._honcho.session(name)
        except Exception:
            return None

    def retain_message(
        self,
        peer_name: str,
        content: str,
        role: str = "user",
        session_name: Optional[str] = None,
        metadata: Optional[Dict[str, Any]] = None,
    ) -> Optional[Dict[str, Any]]:
        """Store a message in Honcho. Returns a small result dict or ``None``.

        ``session_name`` is optional; when provided, the message is also
        added to that session (and the peer is added to it). Errors are
        swallowed and surfaced in the returned dict.
        """
        if not self.is_available() or self._honcho is None:
            return None
        try:
            peer = self.get_or_create_peer(peer_name)
            if peer is None:
                return None
            message = peer.message(content, role=role, metadata=metadata or {})
            if session_name:
                session = self.get_or_create_session(session_name)
                if session is not None:
                    try:
                        session.add_peers([peer_name])
                    except Exception:
                        # Peer may already be a member — non-fatal.
                        pass
                    try:
                        session.add_messages([message])
                    except Exception:
                        # Session attach is best-effort.
                        pass
            return {
                "success": True,
                "peer": peer_name,
                "session": session_name,
                "stored_in": "Honcho",
            }
        except Exception as e:
            return {"success": False, "error": str(e)}

    def get_session_context(
        self, session_name: str
    ) -> Optional[Dict[str, Any]]:
        """Return the behavioral context for a session, or ``None``."""
        if not self.is_available() or self._honcho is None:
            return None
        try:
            session = self.get_or_create_session(session_name)
            if session is None:
                return None
            ctx = session.context()
            if ctx is None:
                return None
            if hasattr(ctx, "model_dump"):
                return ctx.model_dump()
            return dict(ctx)
        except Exception as e:
            return {"error": str(e)}

    def chat(
        self,
        peer_name: str,
        query: str,
        reasoning_level: str = "low",
    ) -> Optional[str]:
        """Ask a Honcho peer a question (dialectic). Returns ``str | None``."""
        if not self.is_available() or self._honcho is None:
            return None
        try:
            peer = self.get_or_create_peer(peer_name)
            if peer is None:
                return None
            return peer.chat(query)
        except Exception:
            return None

    def search(
        self,
        query: str,
        peer_name: Optional[str] = None,
    ) -> List[Dict[str, Any]]:
        """Semantic search across Honcho memory. Empty list on failure."""
        if not self.is_available() or self._honcho is None:
            return []
        try:
            if peer_name:
                peer = self.get_or_create_peer(peer_name)
                if peer is None:
                    return []
                results = peer.search(query)
            else:
                results = self._honcho.search(query)
            if hasattr(results, "__iter__"):
                return [{"content": str(r)} for r in results]
            return []
        except Exception:
            return []


# ── SINMemory: SQLite-Backed Facts ─────────────────────────────────
class SINMemory:
    """Memory system with optional SCKG + Honcho backends.

    Usage:
        mem = SINMemory(Path("/path/to/repo"))
        mem.retain("User prefers TypeScript", tags=["preference"])
        facts = mem.recall("typescript")
    """

    def __init__(
        self,
        repo_root: Optional[Path] = None,
        db_path: Optional[Path] = None,
        honcho_workspace: Optional[str] = None,
        honcho_base_url: Optional[str] = None,
    ) -> None:
        self.repo_root = repo_root or Path.cwd()
        self.db_path = db_path or (self.repo_root / ".sin_memory.db")
        self._sckg: Optional[Any] = None
        # SQLite is always-on (durable, no extra deps).
        # We use connection-per-operation (no pooling) below — SQLite is
        # fine with that for our access pattern, and it sidesteps any
        # thread-safety concerns across agents.
        self._init_db()
        # SCKG is best-effort (semantic layer, optional dep).
        self._try_init_sckg()
        # Honcho backend — graceful if unavailable (lazy import, lazy init).
        self.honcho = HonchoBackend(
            workspace_id=honcho_workspace
            or f"sin-bundle-{self.repo_root.name}",
            base_url=honcho_base_url or "http://localhost:8000",
        )

    def _init_db(self) -> None:
        """Initialize SQLite store (always works, even if SCKG is missing)."""
        self.db_path.parent.mkdir(parents=True, exist_ok=True)
        # Connection-per-operation: open a fresh connection, use it via
        # `with`, let it close. No pooling needed at this scale and this
        # avoids sharing connections across threads (SQLite default).
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

    # ── Public API: retain / recall / reflect / forget ───────────────
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
        # datetime.now(timezone.utc) instead of datetime.utcnow() — the
        # latter is deprecated in Python 3.12+ and produces naive timestamps.
        created_at = datetime.now(timezone.utc).isoformat()

        # Connection-per-operation (see _init_db for rationale).
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
        # Best-effort SCKG storage — SQLite is the source of truth.
        if self._sckg is not None:
            try:
                self._sckg.add_node(
                    node_id=f"memory:{mem_id}",
                    node_type="memory",
                    # fact[:100] keeps node names short in graph visualizations.
                    name=fact[:100],
                    properties={"context": context, "tags": tags},
                )
                stored_in = "SQLite + SCKG"
            except Exception:
                # SCKG storage is best-effort; never fail a retain.
                # add_node can raise on schema mismatch, duplicate ID, etc.
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
        # Connection-per-operation; see _init_db for rationale.
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

        # tags stored as comma-joined string for SQLite portability
        # (SQLite has no native array type; JOIN-by-comma is the simplest
        # portable encoding and supports LIKE-based filtering above).
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

    # ── Public API: Unified Context (SCKG + Honcho + SQLite) ─────────
    def get_context_for_query(self, query: str) -> Dict[str, Any]:
        """Unified context retrieval: SCKG code + Honcho behavioral.

        Returns a dict with:

        - ``query`` — the input echoed back
        - ``code_knowledge`` — SCKG results (or ``None``)
        - ``behavioral_insights`` — Honcho chat response (or ``None``)
        - ``synthesis`` — combined string for LLM prompt injection
        - ``backends`` — which backends are live in this instance

        If neither optional backend is available, the method still
        returns a well-formed dict (with ``synthesis`` empty) so
        callers can rely on the shape.
        """
        result: Dict[str, Any] = {
            "query": query,
            "code_knowledge": None,
            "behavioral_insights": None,
            "synthesis": "",
            "backends": {
                "sqlite": True,
                "sckg": self._sckg is not None,
                "honcho": self.honcho.is_available(),
            },
        }

        # Code knowledge (best-effort)
        if self._sckg is not None:
            try:
                sckg_results = (
                    self._sckg.query(query)
                    if hasattr(self._sckg, "query")
                    else None
                )
                result["code_knowledge"] = {
                    "type": "sckg",
                    "results": sckg_results,
                }
            except Exception:
                # SCKG can be partial — never let it block context assembly.
                pass

        # Behavioral insights (best-effort) — peer.chat can be slow or
        # raise on Honcho server hiccups; we never want it to fail
        # context assembly, so we swallow everything.
        if self.honcho.is_available():
            try:
                peer = self.honcho.get_or_create_peer("coding-agent")
                if peer is not None:
                    insight = peer.chat(
                        f"Based on past interactions, what context is "
                        f"relevant for: {query}"
                    )
                    result["behavioral_insights"] = {
                        "type": "honcho",
                        "insight": insight,
                    }
            except Exception:
                # Honcho errors never fail the call.
                pass

        # Simple synthesis — concatenated hints for LLM prompt injection.
        parts: List[str] = []
        if result["code_knowledge"]:
            parts.append(f"Code: {result['code_knowledge']}")
        if result["behavioral_insights"]:
            insight_text = result["behavioral_insights"].get("insight", "")
            if insight_text:
                parts.append(f"Behavior: {insight_text}")
        result["synthesis"] = (
            "\n".join(parts) if parts else "No context available."
        )
        return result

    def get_stats(self) -> Dict[str, Any]:
        """Get memory statistics: total count, distinct tags, backends."""
        with sqlite3.connect(str(self.db_path)) as conn:
            total = conn.execute("SELECT COUNT(*) FROM memories").fetchone()[0]
            tag_rows = conn.execute(
                "SELECT tags FROM memories WHERE tags != ''"
            ).fetchall()
        # tags stored as comma-joined string for SQLite portability;
        # split here to derive the distinct-tag set (see retain()).
        all_tags: set[str] = set()
        for row in tag_rows:
            for t in row[0].split(","):
                if t:
                    all_tags.add(t)
        return {
            "total_facts": total,
            "tags": sorted(all_tags),
            "backend": "SQLite + SCKG" if self._sckg else "SQLite only",
            "honcho": (
                self.honcho.get_status()
                if self.honcho is not None
                else {"available": False}
            ),
        }


__all__ = ["SINMemory", "HonchoBackend"]
