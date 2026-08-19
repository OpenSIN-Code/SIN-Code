"""Durable Simone workflow control plane.

This database stores authoritative project state:

- domain tasks,
- append-only task events,
- evidence references,
- artifacts,
- architecture decisions,
- research questions.

It is intentionally separate from MCP protocol tasks and hybrid memory.
"""

from __future__ import annotations

import hashlib
import json
import os
import sqlite3
import uuid
from contextlib import contextmanager
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Iterator

ZERO_HASH = "0" * 64

TASK_TRANSITIONS: dict[str, set[str]] = {
    "created": {"planned", "cancelled", "failed"},
    "planned": {"dispatched", "cancelled", "failed"},
    "dispatched": {"active", "blocked", "cancelled", "failed"},
    "active": {"blocked", "verifying", "cancelled", "failed"},
    "blocked": {"active", "cancelled", "failed"},
    "verifying": {"active", "reviewing", "failed"},
    "reviewing": {"active", "completed", "failed"},
    "completed": set(),
    "failed": set(),
    "cancelled": set(),
}

RESEARCH_TRANSITIONS: dict[str, set[str]] = {
    "pending": {"active", "cancelled"},
    "active": {"answered", "blocked", "cancelled"},
    "blocked": {"active", "cancelled"},
    "answered": set(),
    "cancelled": set(),
}

MAX_EXECUTION_PAYLOAD_CHARS = 16_000
FORBIDDEN_EXECUTION_KEYS = {
    "diff",
    "full_diff",
    "raw_diff",
    "log",
    "raw_log",
    "stdout",
    "stderr",
    "terminal_transcript",
    "raw_terminal_transcript",
    "transcript",
}


class ControlPlaneError(RuntimeError):
    """Base error for durable workflow operations."""


class NotFoundError(ControlPlaneError):
    """Requested entity does not exist."""


class ConflictError(ControlPlaneError):
    """Requested mutation violates the state machine."""


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat()


def canonical_json(value: Any) -> str:
    return json.dumps(
        value,
        ensure_ascii=False,
        sort_keys=True,
        separators=(",", ":"),
    )


def sha256_text(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()

    with path.open("rb") as handle:
        while chunk := handle.read(1024 * 1024):
            digest.update(chunk)

    return digest.hexdigest()


def _validate_execution_payload(value: Any, path: str = "payload") -> None:
    if isinstance(value, dict):
        for key, child in value.items():
            normalized = str(key).strip().lower()
            if normalized in FORBIDDEN_EXECUTION_KEYS:
                raise ValueError(
                    f"execution payload must not contain {path}.{key}"
                )
            _validate_execution_payload(child, f"{path}.{key}")
    elif isinstance(value, list):
        for index, child in enumerate(value):
            _validate_execution_payload(child, f"{path}[{index}]")


def _validate_sha256(value: str, field: str) -> str:
    normalized = value.lower().strip()
    if len(normalized) != 64 or any(
        character not in "0123456789abcdef"
        for character in normalized
    ):
        raise ValueError(f"{field} must be a 64-character SHA-256")
    return normalized


def default_database_path() -> Path:
    configured = os.getenv("SIMONE_CONTROL_PLANE_DB")

    if configured:
        return Path(configured).expanduser().resolve()

    return (
        Path.home()
        / ".simone"
        / "control-plane"
        / "control-plane.db"
    )


class ControlPlaneStore:
    def __init__(self, path: Path | None = None) -> None:
        self.path = (path or default_database_path()).resolve()
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self._initialize()

    def _connect(self) -> sqlite3.Connection:
        connection = sqlite3.connect(
            self.path,
            timeout=30,
            isolation_level=None,
        )
        connection.row_factory = sqlite3.Row
        connection.execute("PRAGMA foreign_keys=ON")
        connection.execute("PRAGMA journal_mode=WAL")
        connection.execute("PRAGMA synchronous=FULL")
        connection.execute("PRAGMA busy_timeout=30000")
        return connection

    @contextmanager
    def transaction(self) -> Iterator[sqlite3.Connection]:
        connection = self._connect()

        try:
            connection.execute("BEGIN IMMEDIATE")
            yield connection
            connection.commit()
        except Exception:
            connection.rollback()
            raise
        finally:
            connection.close()

    def _initialize(self) -> None:
        connection = self._connect()

        try:
            connection.executescript(
                """
                CREATE TABLE IF NOT EXISTS projects (
                    id TEXT PRIMARY KEY,
                    root TEXT NOT NULL UNIQUE,
                    created_at TEXT NOT NULL
                );

                CREATE TABLE IF NOT EXISTS tasks (
                    id TEXT PRIMARY KEY,
                    project_id TEXT NOT NULL,
                    task_hash TEXT NOT NULL,
                    base_sha TEXT NOT NULL,
                    role TEXT NOT NULL,
                    objective TEXT NOT NULL,
                    status TEXT NOT NULL,
                    specification_json TEXT NOT NULL,
                    created_at TEXT NOT NULL,
                    updated_at TEXT NOT NULL,
                    FOREIGN KEY(project_id) REFERENCES projects(id)
                );

                CREATE INDEX IF NOT EXISTS idx_tasks_project_status
                    ON tasks(project_id, status);

                CREATE TABLE IF NOT EXISTS task_events (
                    task_id TEXT NOT NULL,
                    sequence INTEGER NOT NULL,
                    event_type TEXT NOT NULL,
                    actor TEXT NOT NULL,
                    payload_json TEXT NOT NULL,
                    previous_hash TEXT NOT NULL,
                    event_hash TEXT NOT NULL,
                    created_at TEXT NOT NULL,
                    PRIMARY KEY(task_id, sequence),
                    UNIQUE(task_id, event_hash),
                    FOREIGN KEY(task_id) REFERENCES tasks(id)
                );

                CREATE TABLE IF NOT EXISTS artifacts (
                    id TEXT PRIMARY KEY,
                    task_id TEXT NOT NULL,
                    kind TEXT NOT NULL,
                    path TEXT NOT NULL,
                    sha256 TEXT NOT NULL,
                    size_bytes INTEGER NOT NULL,
                    metadata_json TEXT NOT NULL,
                    created_at TEXT NOT NULL,
                    FOREIGN KEY(task_id) REFERENCES tasks(id)
                );

                CREATE INDEX IF NOT EXISTS idx_artifacts_task
                    ON artifacts(task_id, kind);

                CREATE TABLE IF NOT EXISTS evidence (
                    id TEXT PRIMARY KEY,
                    task_id TEXT NOT NULL,
                    claim_id TEXT NOT NULL,
                    source TEXT NOT NULL,
                    path TEXT,
                    line_start INTEGER,
                    line_end INTEGER,
                    sha256 TEXT NOT NULL,
                    trust_level TEXT NOT NULL,
                    metadata_json TEXT NOT NULL,
                    created_at TEXT NOT NULL,
                    FOREIGN KEY(task_id) REFERENCES tasks(id)
                );

                CREATE INDEX IF NOT EXISTS idx_evidence_task_claim
                    ON evidence(task_id, claim_id);

                CREATE TABLE IF NOT EXISTS decisions (
                    id TEXT PRIMARY KEY,
                    task_id TEXT NOT NULL,
                    title TEXT NOT NULL,
                    decision TEXT NOT NULL,
                    rationale TEXT NOT NULL,
                    evidence_ids_json TEXT NOT NULL,
                    status TEXT NOT NULL,
                    created_at TEXT NOT NULL,
                    FOREIGN KEY(task_id) REFERENCES tasks(id)
                );

                CREATE INDEX IF NOT EXISTS idx_decisions_task
                    ON decisions(task_id, status);

                CREATE TABLE IF NOT EXISTS research_questions (
                    id TEXT PRIMARY KEY,
                    task_id TEXT NOT NULL,
                    parent_id TEXT,
                    question TEXT NOT NULL,
                    status TEXT NOT NULL,
                    priority INTEGER NOT NULL,
                    answer TEXT,
                    evidence_ids_json TEXT NOT NULL,
                    created_at TEXT NOT NULL,
                    updated_at TEXT NOT NULL,
                    FOREIGN KEY(task_id) REFERENCES tasks(id),
                    FOREIGN KEY(parent_id) REFERENCES research_questions(id)
                );

                CREATE INDEX IF NOT EXISTS idx_research_task_status
                    ON research_questions(task_id, status, priority);

                CREATE TABLE IF NOT EXISTS execution_links (
                    task_id TEXT NOT NULL,
                    source TEXT NOT NULL,
                    external_task_id TEXT NOT NULL,
                    metadata_json TEXT NOT NULL,
                    created_at TEXT NOT NULL,
                    updated_at TEXT NOT NULL,
                    PRIMARY KEY(task_id, source),
                    UNIQUE(source, external_task_id),
                    FOREIGN KEY(task_id) REFERENCES tasks(id)
                );

                CREATE TABLE IF NOT EXISTS execution_event_receipts (
                    task_id TEXT NOT NULL,
                    source TEXT NOT NULL,
                    external_event_id TEXT NOT NULL,
                    external_sequence INTEGER,
                    external_hash TEXT,
                    event_type TEXT NOT NULL,
                    actor TEXT NOT NULL,
                    payload_sha256 TEXT NOT NULL,
                    simone_sequence INTEGER NOT NULL,
                    simone_event_hash TEXT NOT NULL,
                    created_at TEXT NOT NULL,
                    PRIMARY KEY(source, external_event_id),
                    FOREIGN KEY(task_id) REFERENCES tasks(id)
                );

                CREATE UNIQUE INDEX IF NOT EXISTS
                    idx_execution_event_sequence
                    ON execution_event_receipts(
                        task_id,
                        source,
                        external_sequence
                    )
                    WHERE external_sequence IS NOT NULL;

                CREATE INDEX IF NOT EXISTS idx_execution_receipts_task
                    ON execution_event_receipts(task_id, source);
                """
            )
        finally:
            connection.close()

    @staticmethod
    def _project_id(root: Path) -> str:
        return sha256_text(str(root.resolve()))[:24]

    @staticmethod
    def _task_row(row: sqlite3.Row) -> dict[str, Any]:
        result = dict(row)
        result["specification"] = json.loads(
            result.pop("specification_json")
        )
        return result

    def _ensure_task(
        self,
        connection: sqlite3.Connection,
        task_id: str,
    ) -> sqlite3.Row:
        row = connection.execute(
            "SELECT * FROM tasks WHERE id = ?",
            (task_id,),
        ).fetchone()

        if row is None:
            raise NotFoundError(f"task not found: {task_id}")

        return row

    def _append_event(
        self,
        connection: sqlite3.Connection,
        *,
        task_id: str,
        event_type: str,
        actor: str,
        payload: dict[str, Any],
    ) -> dict[str, Any]:
        self._ensure_task(connection, task_id)

        previous = connection.execute(
            """
            SELECT sequence, event_hash
            FROM task_events
            WHERE task_id = ?
            ORDER BY sequence DESC
            LIMIT 1
            """,
            (task_id,),
        ).fetchone()

        sequence = int(previous["sequence"]) + 1 if previous else 1
        previous_hash = (
            str(previous["event_hash"])
            if previous
            else ZERO_HASH
        )
        created_at = utc_now()

        material = {
            "task_id": task_id,
            "sequence": sequence,
            "event_type": event_type,
            "actor": actor,
            "payload": payload,
            "previous_hash": previous_hash,
            "created_at": created_at,
        }

        event_hash = sha256_text(canonical_json(material))

        connection.execute(
            """
            INSERT INTO task_events(
                task_id,
                sequence,
                event_type,
                actor,
                payload_json,
                previous_hash,
                event_hash,
                created_at
            )
            VALUES (?, ?, ?, ?, ?, ?, ?, ?)
            """,
            (
                task_id,
                sequence,
                event_type,
                actor,
                canonical_json(payload),
                previous_hash,
                event_hash,
                created_at,
            ),
        )

        connection.execute(
            "UPDATE tasks SET updated_at = ? WHERE id = ?",
            (created_at, task_id),
        )

        return {
            **material,
            "event_hash": event_hash,
        }

    def create_task(
        self,
        *,
        repository_root: str,
        base_sha: str,
        role: str,
        objective: str,
        specification: dict[str, Any],
        task_id: str | None = None,
    ) -> dict[str, Any]:
        root = Path(repository_root).expanduser().resolve()

        if not objective.strip():
            raise ValueError("objective must not be empty")

        if not base_sha.strip():
            raise ValueError("base_sha must not be empty")

        task_id = task_id or f"TASK-{uuid.uuid4().hex[:12]}"
        now = utc_now()
        project_id = self._project_id(root)

        hash_material = {
            "repository_root": str(root),
            "base_sha": base_sha,
            "role": role,
            "objective": objective,
            "specification": specification,
        }
        task_hash = "sha256:" + sha256_text(
            canonical_json(hash_material)
        )

        with self.transaction() as connection:
            connection.execute(
                """
                INSERT OR IGNORE INTO projects(id, root, created_at)
                VALUES (?, ?, ?)
                """,
                (project_id, str(root), now),
            )

            try:
                connection.execute(
                    """
                    INSERT INTO tasks(
                        id,
                        project_id,
                        task_hash,
                        base_sha,
                        role,
                        objective,
                        status,
                        specification_json,
                        created_at,
                        updated_at
                    )
                    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                    """,
                    (
                        task_id,
                        project_id,
                        task_hash,
                        base_sha,
                        role,
                        objective,
                        "created",
                        canonical_json(specification),
                        now,
                        now,
                    ),
                )
            except sqlite3.IntegrityError as error:
                raise ConflictError(
                    f"task already exists: {task_id}"
                ) from error

            self._append_event(
                connection,
                task_id=task_id,
                event_type="task.created",
                actor="codex",
                payload={
                    "task_hash": task_hash,
                    "base_sha": base_sha,
                    "role": role,
                },
            )

        return self.get_task(task_id)

    def get_task(
        self,
        task_id: str,
        *,
        include_events: bool = False,
    ) -> dict[str, Any]:
        connection = self._connect()

        try:
            row = self._ensure_task(connection, task_id)
            result = self._task_row(row)

            if include_events:
                result["events"] = self.list_events(task_id)

            return result
        finally:
            connection.close()

    def list_tasks(
        self,
        *,
        repository_root: str | None = None,
        status: str | None = None,
        limit: int = 100,
    ) -> list[dict[str, Any]]:
        connection = self._connect()

        try:
            clauses: list[str] = []
            parameters: list[Any] = []

            if repository_root:
                root = str(
                    Path(repository_root)
                    .expanduser()
                    .resolve()
                )
                clauses.append("p.root = ?")
                parameters.append(root)

            if status:
                clauses.append("t.status = ?")
                parameters.append(status)

            where = (
                "WHERE " + " AND ".join(clauses)
                if clauses
                else ""
            )

            parameters.append(max(1, min(limit, 500)))

            rows = connection.execute(
                f"""
                SELECT t.*
                FROM tasks t
                JOIN projects p ON p.id = t.project_id
                {where}
                ORDER BY t.updated_at DESC
                LIMIT ?
                """,
                parameters,
            ).fetchall()

            return [self._task_row(row) for row in rows]
        finally:
            connection.close()

    def transition_task(
        self,
        *,
        task_id: str,
        target_status: str,
        actor: str,
        reason: str = "",
    ) -> dict[str, Any]:
        with self.transaction() as connection:
            row = self._ensure_task(connection, task_id)
            current = str(row["status"])

            allowed = TASK_TRANSITIONS.get(current)

            if allowed is None:
                raise ConflictError(
                    f"unknown current status: {current}"
                )

            if target_status not in allowed:
                raise ConflictError(
                    f"invalid task transition: "
                    f"{current} -> {target_status}"
                )

            now = utc_now()

            connection.execute(
                """
                UPDATE tasks
                SET status = ?, updated_at = ?
                WHERE id = ?
                """,
                (target_status, now, task_id),
            )

            self._append_event(
                connection,
                task_id=task_id,
                event_type="task.transitioned",
                actor=actor,
                payload={
                    "from": current,
                    "to": target_status,
                    "reason": reason,
                },
            )

        return self.get_task(task_id)

    def append_event(
        self,
        *,
        task_id: str,
        event_type: str,
        actor: str,
        payload: dict[str, Any],
    ) -> dict[str, Any]:
        with self.transaction() as connection:
            return self._append_event(
                connection,
                task_id=task_id,
                event_type=event_type,
                actor=actor,
                payload=payload,
            )

    def list_events(
        self,
        task_id: str,
    ) -> list[dict[str, Any]]:
        connection = self._connect()

        try:
            self._ensure_task(connection, task_id)

            rows = connection.execute(
                """
                SELECT *
                FROM task_events
                WHERE task_id = ?
                ORDER BY sequence
                """,
                (task_id,),
            ).fetchall()

            return [
                {
                    "task_id": row["task_id"],
                    "sequence": row["sequence"],
                    "event_type": row["event_type"],
                    "actor": row["actor"],
                    "payload": json.loads(row["payload_json"]),
                    "previous_hash": row["previous_hash"],
                    "event_hash": row["event_hash"],
                    "created_at": row["created_at"],
                }
                for row in rows
            ]
        finally:
            connection.close()

    def bind_execution(
        self,
        *,
        task_id: str,
        source: str,
        external_task_id: str,
        metadata: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        source = source.strip()
        external_task_id = external_task_id.strip()
        if metadata is None:
            metadata = {}
        if not isinstance(metadata, dict):
            raise ValueError("metadata must be an object")

        if not source:
            raise ValueError("source must not be empty")
        if not external_task_id:
            raise ValueError("external_task_id must not be empty")

        _validate_execution_payload(metadata, "metadata")
        if len(canonical_json(metadata)) > MAX_EXECUTION_PAYLOAD_CHARS:
            raise ValueError("execution metadata exceeds size limit")

        now = utc_now()

        with self.transaction() as connection:
            self._ensure_task(connection, task_id)

            external = connection.execute(
                """
                SELECT * FROM execution_links
                WHERE source = ? AND external_task_id = ?
                """,
                (source, external_task_id),
            ).fetchone()
            if external is not None and external["task_id"] != task_id:
                raise ConflictError(
                    "external execution task is already bound to another task"
                )

            existing = connection.execute(
                """
                SELECT * FROM execution_links
                WHERE task_id = ? AND source = ?
                """,
                (task_id, source),
            ).fetchone()
            if existing is not None:
                if existing["external_task_id"] != external_task_id:
                    raise ConflictError(
                        "task already has a different execution binding"
                    )
                return {
                    "task_id": task_id,
                    "source": source,
                    "external_task_id": external_task_id,
                    "metadata": json.loads(existing["metadata_json"]),
                    "created_at": existing["created_at"],
                    "updated_at": existing["updated_at"],
                    "duplicate": True,
                }

            connection.execute(
                """
                INSERT INTO execution_links(
                    task_id, source, external_task_id,
                    metadata_json, created_at, updated_at
                ) VALUES (?, ?, ?, ?, ?, ?)
                """,
                (
                    task_id,
                    source,
                    external_task_id,
                    canonical_json(metadata),
                    now,
                    now,
                ),
            )
            self._append_event(
                connection,
                task_id=task_id,
                event_type="execution.bound",
                actor="controller",
                payload={
                    "source": source,
                    "external_task_id": external_task_id,
                    "metadata": metadata,
                },
            )

        return {
            "task_id": task_id,
            "source": source,
            "external_task_id": external_task_id,
            "metadata": metadata,
            "created_at": now,
            "updated_at": now,
            "duplicate": False,
        }

    def ingest_execution_event(
        self,
        *,
        task_id: str,
        source: str,
        external_event_id: str,
        event_type: str,
        actor: str,
        payload: dict[str, Any],
        external_sequence: int | None = None,
        external_hash: str | None = None,
    ) -> dict[str, Any]:
        source = source.strip()
        external_event_id = external_event_id.strip()
        event_type = event_type.strip()
        actor = actor.strip()
        if not isinstance(payload, dict):
            raise ValueError("payload must be an object")

        if not all((source, external_event_id, event_type, actor)):
            raise ValueError(
                "source, external_event_id, event_type, and actor are required"
            )
        if external_sequence is not None and external_sequence < 1:
            raise ValueError("external_sequence must be >= 1")
        if external_hash is not None:
            external_hash = _validate_sha256(
                external_hash,
                "external_hash",
            )

        _validate_execution_payload(payload)
        serialized = canonical_json(payload)
        if len(serialized) > MAX_EXECUTION_PAYLOAD_CHARS:
            raise ValueError("execution payload exceeds size limit")
        payload_sha256 = sha256_text(serialized)

        with self.transaction() as connection:
            self._ensure_task(connection, task_id)
            binding = connection.execute(
                """
                SELECT external_task_id FROM execution_links
                WHERE task_id = ? AND source = ?
                """,
                (task_id, source),
            ).fetchone()
            if binding is None:
                raise ConflictError(
                    "execution source must be bound before events are ingested"
                )

            existing = connection.execute(
                """
                SELECT * FROM execution_event_receipts
                WHERE source = ? AND external_event_id = ?
                """,
                (source, external_event_id),
            ).fetchone()
            if existing is not None:
                expected = {
                    "task_id": task_id,
                    "event_type": event_type,
                    "actor": actor,
                    "payload_sha256": payload_sha256,
                    "external_sequence": external_sequence,
                    "external_hash": external_hash,
                }
                actual = {key: existing[key] for key in expected}
                if actual != expected:
                    raise ConflictError(
                        "external event replay does not match original receipt"
                    )
                return {
                    "task_id": task_id,
                    "source": source,
                    "external_event_id": external_event_id,
                    "simone_sequence": existing["simone_sequence"],
                    "simone_event_hash": existing["simone_event_hash"],
                    "payload_sha256": payload_sha256,
                    "duplicate": True,
                }

            event = self._append_event(
                connection,
                task_id=task_id,
                event_type=f"execution.{event_type}",
                actor=actor,
                payload={
                    "source": source,
                    "external_task_id": binding["external_task_id"],
                    "external_event_id": external_event_id,
                    "external_sequence": external_sequence,
                    "external_hash": external_hash,
                    "payload_sha256": payload_sha256,
                    "trust_level": "external-untrusted",
                    "data": payload,
                },
            )
            connection.execute(
                """
                INSERT INTO execution_event_receipts(
                    task_id, source, external_event_id,
                    external_sequence, external_hash,
                    event_type, actor, payload_sha256,
                    simone_sequence, simone_event_hash, created_at
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """,
                (
                    task_id,
                    source,
                    external_event_id,
                    external_sequence,
                    external_hash,
                    event_type,
                    actor,
                    payload_sha256,
                    event["sequence"],
                    event["event_hash"],
                    event["created_at"],
                ),
            )

        return {
            "task_id": task_id,
            "source": source,
            "external_event_id": external_event_id,
            "simone_sequence": event["sequence"],
            "simone_event_hash": event["event_hash"],
            "payload_sha256": payload_sha256,
            "duplicate": False,
        }

    def record_execution_artifact(
        self,
        *,
        task_id: str,
        source: str,
        kind: str,
        reference: str,
        sha256: str,
        size_bytes: int,
        metadata: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        source = source.strip()
        kind = kind.strip()
        reference = reference.strip()
        digest = _validate_sha256(sha256, "sha256")
        if metadata is None:
            metadata = {}
        if not isinstance(metadata, dict):
            raise ValueError("metadata must be an object")

        if not source or not kind or not reference:
            raise ValueError("source, kind, and reference are required")
        if size_bytes < 0:
            raise ValueError("size_bytes must be >= 0")
        if len(reference) > 4096:
            raise ValueError("artifact reference is too long")
        _validate_execution_payload(metadata, "metadata")
        if len(canonical_json(metadata)) > MAX_EXECUTION_PAYLOAD_CHARS:
            raise ValueError("artifact metadata exceeds size limit")

        artifact_id = "ART-" + sha256_text(
            f"{task_id}:{source}:{kind}:{digest}"
        )[:20]
        created_at = utc_now()

        with self.transaction() as connection:
            self._ensure_task(connection, task_id)
            binding = connection.execute(
                """
                SELECT external_task_id FROM execution_links
                WHERE task_id = ? AND source = ?
                """,
                (task_id, source),
            ).fetchone()
            if binding is None:
                raise ConflictError(
                    "execution source must be bound before artifacts are recorded"
                )

            existing = connection.execute(
                "SELECT * FROM artifacts WHERE id = ?",
                (artifact_id,),
            ).fetchone()
            if existing is not None:
                expected = {
                    "task_id": task_id,
                    "kind": kind,
                    "path": reference,
                    "sha256": digest,
                    "size_bytes": size_bytes,
                }
                actual = {key: existing[key] for key in expected}
                if actual != expected:
                    raise ConflictError(
                        "execution artifact replay does not match original"
                    )
                return {
                    "id": artifact_id,
                    "task_id": task_id,
                    "kind": kind,
                    "reference": reference,
                    "sha256": digest,
                    "size_bytes": size_bytes,
                    "metadata": json.loads(existing["metadata_json"]),
                    "created_at": existing["created_at"],
                    "duplicate": True,
                }

            stored_metadata = {
                **metadata,
                "execution_source": source,
                "external_task_id": binding["external_task_id"],
                "reference_only": True,
            }
            connection.execute(
                """
                INSERT INTO artifacts(
                    id, task_id, kind, path, sha256,
                    size_bytes, metadata_json, created_at
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
                """,
                (
                    artifact_id,
                    task_id,
                    kind,
                    reference,
                    digest,
                    size_bytes,
                    canonical_json(stored_metadata),
                    created_at,
                ),
            )
            self._append_event(
                connection,
                task_id=task_id,
                event_type="execution.artifact_recorded",
                actor="controller",
                payload={
                    "artifact_id": artifact_id,
                    "source": source,
                    "kind": kind,
                    "reference": reference,
                    "sha256": digest,
                    "size_bytes": size_bytes,
                },
            )

        return {
            "id": artifact_id,
            "task_id": task_id,
            "kind": kind,
            "reference": reference,
            "sha256": digest,
            "size_bytes": size_bytes,
            "metadata": stored_metadata,
            "created_at": created_at,
            "duplicate": False,
        }

    def attach_artifact(
        self,
        *,
        task_id: str,
        kind: str,
        path: str,
        metadata: dict[str, Any] | None = None,
        expected_sha256: str | None = None,
    ) -> dict[str, Any]:
        artifact_path = Path(path).expanduser().resolve()

        if not artifact_path.is_file():
            raise ValueError(
                f"artifact file not found: {artifact_path}"
            )

        digest = sha256_file(artifact_path)

        if expected_sha256 and digest != expected_sha256:
            raise ConflictError(
                "artifact SHA-256 does not match expected value"
            )

        artifact_id = "ART-" + sha256_text(
            f"{task_id}:{kind}:{digest}"
        )[:20]
        size_bytes = artifact_path.stat().st_size
        created_at = utc_now()

        with self.transaction() as connection:
            self._ensure_task(connection, task_id)

            existing = connection.execute(
                "SELECT * FROM artifacts WHERE id = ?",
                (artifact_id,),
            ).fetchone()
            if existing is not None:
                if (
                    existing["task_id"] != task_id
                    or existing["kind"] != kind
                    or existing["sha256"] != digest
                    or existing["size_bytes"] != size_bytes
                ):
                    raise ConflictError(
                        "artifact identifier collision"
                    )
                return {
                    "id": artifact_id,
                    "task_id": task_id,
                    "kind": kind,
                    "path": existing["path"],
                    "sha256": digest,
                    "size_bytes": size_bytes,
                    "metadata": json.loads(existing["metadata_json"]),
                    "created_at": existing["created_at"],
                    "duplicate": True,
                }

            connection.execute(
                """
                INSERT INTO artifacts(
                    id,
                    task_id,
                    kind,
                    path,
                    sha256,
                    size_bytes,
                    metadata_json,
                    created_at
                )
                VALUES (?, ?, ?, ?, ?, ?, ?, ?)
                """,
                (
                    artifact_id,
                    task_id,
                    kind,
                    str(artifact_path),
                    digest,
                    size_bytes,
                    canonical_json(metadata or {}),
                    created_at,
                ),
            )

            self._append_event(
                connection,
                task_id=task_id,
                event_type="artifact.attached",
                actor="controller",
                payload={
                    "artifact_id": artifact_id,
                    "kind": kind,
                    "sha256": digest,
                    "size_bytes": size_bytes,
                },
            )

        return {
            "id": artifact_id,
            "task_id": task_id,
            "kind": kind,
            "path": str(artifact_path),
            "sha256": digest,
            "size_bytes": size_bytes,
            "metadata": metadata or {},
            "created_at": created_at,
            "duplicate": False,
        }

    def attach_evidence(
        self,
        *,
        task_id: str,
        claim_id: str,
        source: str,
        sha256: str,
        trust_level: str,
        path: str | None = None,
        line_start: int | None = None,
        line_end: int | None = None,
        metadata: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        if trust_level not in {
            "controller-verified",
            "repository",
            "external-untrusted",
            "worker-claim",
        }:
            raise ValueError(
                f"invalid trust level: {trust_level}"
            )

        if line_start is not None and line_start < 1:
            raise ValueError("line_start must be >= 1")

        if line_end is not None and line_start is not None:
            if line_end < line_start:
                raise ValueError(
                    "line_end must not be before line_start"
                )

        evidence_id = f"EVD-{uuid.uuid4().hex[:16]}"
        created_at = utc_now()

        with self.transaction() as connection:
            self._ensure_task(connection, task_id)

            connection.execute(
                """
                INSERT INTO evidence(
                    id,
                    task_id,
                    claim_id,
                    source,
                    path,
                    line_start,
                    line_end,
                    sha256,
                    trust_level,
                    metadata_json,
                    created_at
                )
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """,
                (
                    evidence_id,
                    task_id,
                    claim_id,
                    source,
                    path,
                    line_start,
                    line_end,
                    sha256,
                    trust_level,
                    canonical_json(metadata or {}),
                    created_at,
                ),
            )

            self._append_event(
                connection,
                task_id=task_id,
                event_type="evidence.attached",
                actor="controller",
                payload={
                    "evidence_id": evidence_id,
                    "claim_id": claim_id,
                    "source": source,
                    "trust_level": trust_level,
                    "sha256": sha256,
                },
            )

        return {
            "id": evidence_id,
            "task_id": task_id,
            "claim_id": claim_id,
            "source": source,
            "path": path,
            "line_start": line_start,
            "line_end": line_end,
            "sha256": sha256,
            "trust_level": trust_level,
            "metadata": metadata or {},
            "created_at": created_at,
        }

    def record_decision(
        self,
        *,
        task_id: str,
        title: str,
        decision: str,
        rationale: str,
        evidence_ids: list[str],
        status: str = "accepted",
    ) -> dict[str, Any]:
        if status not in {
            "proposed",
            "accepted",
            "rejected",
            "superseded",
        }:
            raise ValueError(f"invalid decision status: {status}")

        decision_id = f"DEC-{uuid.uuid4().hex[:16]}"
        created_at = utc_now()

        with self.transaction() as connection:
            self._ensure_task(connection, task_id)

            for evidence_id in evidence_ids:
                row = connection.execute(
                    """
                    SELECT id
                    FROM evidence
                    WHERE id = ? AND task_id = ?
                    """,
                    (evidence_id, task_id),
                ).fetchone()

                if row is None:
                    raise ConflictError(
                        f"unknown evidence for task: {evidence_id}"
                    )

            connection.execute(
                """
                INSERT INTO decisions(
                    id,
                    task_id,
                    title,
                    decision,
                    rationale,
                    evidence_ids_json,
                    status,
                    created_at
                )
                VALUES (?, ?, ?, ?, ?, ?, ?, ?)
                """,
                (
                    decision_id,
                    task_id,
                    title,
                    decision,
                    rationale,
                    canonical_json(evidence_ids),
                    status,
                    created_at,
                ),
            )

            self._append_event(
                connection,
                task_id=task_id,
                event_type="decision.recorded",
                actor="codex",
                payload={
                    "decision_id": decision_id,
                    "title": title,
                    "status": status,
                    "evidence_ids": evidence_ids,
                },
            )

        return {
            "id": decision_id,
            "task_id": task_id,
            "title": title,
            "decision": decision,
            "rationale": rationale,
            "evidence_ids": evidence_ids,
            "status": status,
            "created_at": created_at,
        }

    def add_research_question(
        self,
        *,
        task_id: str,
        question: str,
        priority: int = 50,
        parent_id: str | None = None,
    ) -> dict[str, Any]:
        if not question.strip():
            raise ValueError("question must not be empty")

        question_id = f"RQ-{uuid.uuid4().hex[:16]}"
        now = utc_now()

        with self.transaction() as connection:
            self._ensure_task(connection, task_id)

            if parent_id:
                parent = connection.execute(
                    """
                    SELECT id
                    FROM research_questions
                    WHERE id = ? AND task_id = ?
                    """,
                    (parent_id, task_id),
                ).fetchone()

                if parent is None:
                    raise ConflictError(
                        f"research parent not found: {parent_id}"
                    )

            connection.execute(
                """
                INSERT INTO research_questions(
                    id,
                    task_id,
                    parent_id,
                    question,
                    status,
                    priority,
                    answer,
                    evidence_ids_json,
                    created_at,
                    updated_at
                )
                VALUES (?, ?, ?, ?, ?, ?, NULL, '[]', ?, ?)
                """,
                (
                    question_id,
                    task_id,
                    parent_id,
                    question,
                    "pending",
                    int(priority),
                    now,
                    now,
                ),
            )

            self._append_event(
                connection,
                task_id=task_id,
                event_type="research.question_added",
                actor="codex",
                payload={
                    "question_id": question_id,
                    "parent_id": parent_id,
                    "priority": priority,
                },
            )

        return {
            "id": question_id,
            "task_id": task_id,
            "parent_id": parent_id,
            "question": question,
            "status": "pending",
            "priority": priority,
            "created_at": now,
            "updated_at": now,
        }

    def update_research_question(
        self,
        *,
        question_id: str,
        target_status: str,
        answer: str | None = None,
        evidence_ids: list[str] | None = None,
    ) -> dict[str, Any]:
        evidence_ids = evidence_ids or []

        with self.transaction() as connection:
            row = connection.execute(
                """
                SELECT *
                FROM research_questions
                WHERE id = ?
                """,
                (question_id,),
            ).fetchone()

            if row is None:
                raise NotFoundError(
                    f"research question not found: {question_id}"
                )

            current = str(row["status"])

            if target_status not in RESEARCH_TRANSITIONS[current]:
                raise ConflictError(
                    f"invalid research transition: "
                    f"{current} -> {target_status}"
                )

            if target_status == "answered" and not answer:
                raise ValueError(
                    "answered research question requires an answer"
                )

            task_id = str(row["task_id"])

            for evidence_id in evidence_ids:
                evidence = connection.execute(
                    """
                    SELECT id
                    FROM evidence
                    WHERE id = ? AND task_id = ?
                    """,
                    (evidence_id, task_id),
                ).fetchone()

                if evidence is None:
                    raise ConflictError(
                        f"unknown evidence: {evidence_id}"
                    )

            now = utc_now()

            connection.execute(
                """
                UPDATE research_questions
                SET
                    status = ?,
                    answer = ?,
                    evidence_ids_json = ?,
                    updated_at = ?
                WHERE id = ?
                """,
                (
                    target_status,
                    answer,
                    canonical_json(evidence_ids),
                    now,
                    question_id,
                ),
            )

            self._append_event(
                connection,
                task_id=task_id,
                event_type="research.question_updated",
                actor="librarian",
                payload={
                    "question_id": question_id,
                    "from": current,
                    "to": target_status,
                    "evidence_ids": evidence_ids,
                },
            )

        return self.get_research_question(question_id)

    def get_research_question(
        self,
        question_id: str,
    ) -> dict[str, Any]:
        connection = self._connect()

        try:
            row = connection.execute(
                """
                SELECT *
                FROM research_questions
                WHERE id = ?
                """,
                (question_id,),
            ).fetchone()

            if row is None:
                raise NotFoundError(
                    f"research question not found: {question_id}"
                )

            result = dict(row)
            result["evidence_ids"] = json.loads(
                result.pop("evidence_ids_json")
            )
            return result
        finally:
            connection.close()

    def next_research_questions(
        self,
        *,
        task_id: str,
        limit: int = 4,
    ) -> list[dict[str, Any]]:
        connection = self._connect()

        try:
            self._ensure_task(connection, task_id)

            rows = connection.execute(
                """
                SELECT child.*
                FROM research_questions child
                LEFT JOIN research_questions parent
                    ON parent.id = child.parent_id
                WHERE
                    child.task_id = ?
                    AND child.status = 'pending'
                    AND (
                        child.parent_id IS NULL
                        OR parent.status = 'answered'
                    )
                ORDER BY
                    child.priority DESC,
                    child.created_at ASC
                LIMIT ?
                """,
                (task_id, max(1, min(limit, 20))),
            ).fetchall()

            return [
                {
                    **dict(row),
                    "evidence_ids": json.loads(
                        row["evidence_ids_json"]
                    ),
                }
                for row in rows
            ]
        finally:
            connection.close()

    def summary(self, task_id: str) -> dict[str, Any]:
        connection = self._connect()

        try:
            task = self._task_row(
                self._ensure_task(connection, task_id)
            )

            counts = {}

            for table in (
                "task_events",
                "artifacts",
                "evidence",
                "decisions",
                "research_questions",
                "execution_links",
                "execution_event_receipts",
            ):
                row = connection.execute(
                    f"""
                    SELECT COUNT(*) AS count
                    FROM {table}
                    WHERE task_id = ?
                    """,
                    (task_id,),
                ).fetchone()

                counts[table] = int(row["count"])

            pending_research = connection.execute(
                """
                SELECT COUNT(*) AS count
                FROM research_questions
                WHERE task_id = ?
                  AND status IN ('pending', 'active', 'blocked')
                """,
                (task_id,),
            ).fetchone()

            return {
                "task": task,
                "counts": counts,
                "pending_research": int(
                    pending_research["count"]
                ),
            }
        finally:
            connection.close()
