from __future__ import annotations

import json
from pathlib import Path

import pytest

from simone_mcp.control_plane import (
    ConflictError,
    ControlPlaneStore,
)


@pytest.fixture
def store(tmp_path: Path) -> ControlPlaneStore:
    return ControlPlaneStore(
        tmp_path / "control-plane.db"
    )


def create_task(
    store: ControlPlaneStore,
    tmp_path: Path,
) -> dict:
    repository = tmp_path / "repo"
    repository.mkdir(exist_ok=True)

    return store.create_task(
        repository_root=str(repository),
        base_sha="a" * 40,
        role="implementer",
        objective="Implement durable workflow state",
        specification={
            "allowed_paths": [
                "src/",
                "tests/",
            ],
            "steps": [
                {
                    "id": "S01",
                    "instruction": "Implement storage",
                }
            ],
            "acceptance_criteria": [
                {
                    "id": "AC01",
                    "text": "State survives restart",
                }
            ],
        },
    )


def test_task_survives_store_restart(
    tmp_path: Path,
) -> None:
    database = tmp_path / "control-plane.db"
    first = ControlPlaneStore(database)

    task = create_task(first, tmp_path)

    second = ControlPlaneStore(database)
    loaded = second.get_task(
        task["id"],
        include_events=True,
    )

    assert loaded["id"] == task["id"]
    assert loaded["status"] == "created"
    assert loaded["events"][0]["event_type"] == "task.created"


def test_event_chain_is_linked(
    store: ControlPlaneStore,
    tmp_path: Path,
) -> None:
    task = create_task(store, tmp_path)

    store.append_event(
        task_id=task["id"],
        event_type="checkpoint.received",
        actor="worker",
        payload={
            "checkpoint": "ack",
        },
    )

    events = store.list_events(task["id"])

    assert len(events) == 2
    assert events[1]["previous_hash"] == events[0]["event_hash"]


def test_invalid_task_transition_is_rejected(
    store: ControlPlaneStore,
    tmp_path: Path,
) -> None:
    task = create_task(store, tmp_path)

    with pytest.raises(ConflictError):
        store.transition_task(
            task_id=task["id"],
            target_status="completed",
            actor="codex",
        )


def test_valid_task_lifecycle(
    store: ControlPlaneStore,
    tmp_path: Path,
) -> None:
    task = create_task(store, tmp_path)

    transitions = [
        "planned",
        "dispatched",
        "active",
        "verifying",
        "reviewing",
        "completed",
    ]

    for status in transitions:
        updated = store.transition_task(
            task_id=task["id"],
            target_status=status,
            actor="codex",
        )

        assert updated["status"] == status


def test_research_child_waits_for_parent(
    store: ControlPlaneStore,
    tmp_path: Path,
) -> None:
    task = create_task(store, tmp_path)

    parent = store.add_research_question(
        task_id=task["id"],
        question="How is task state persisted?",
        priority=100,
    )

    child = store.add_research_question(
        task_id=task["id"],
        parent_id=parent["id"],
        question="Which SQLite durability mode is required?",
        priority=90,
    )

    available = store.next_research_questions(
        task_id=task["id"],
    )

    assert [item["id"] for item in available] == [
        parent["id"]
    ]

    store.update_research_question(
        question_id=parent["id"],
        target_status="active",
    )

    store.update_research_question(
        question_id=parent["id"],
        target_status="answered",
        answer="Use a separate authoritative SQLite store.",
    )

    available = store.next_research_questions(
        task_id=task["id"],
    )

    assert child["id"] in {
        item["id"]
        for item in available
    }


def test_decision_requires_existing_evidence(
    store: ControlPlaneStore,
    tmp_path: Path,
) -> None:
    task = create_task(store, tmp_path)

    with pytest.raises(ConflictError):
        store.record_decision(
            task_id=task["id"],
            title="Use SQLite",
            decision="Use WAL SQLite",
            rationale="Local and durable",
            evidence_ids=["EVD-NOT-FOUND"],
        )


def test_artifact_hash_is_verified(
    store: ControlPlaneStore,
    tmp_path: Path,
) -> None:
    task = create_task(store, tmp_path)

    artifact = tmp_path / "report.json"
    artifact.write_text(
        json.dumps({"ok": True}),
        encoding="utf-8",
    )

    attached = store.attach_artifact(
        task_id=task["id"],
        kind="worker-report",
        path=str(artifact),
    )

    assert attached["size_bytes"] > 0
    assert len(attached["sha256"]) == 64
