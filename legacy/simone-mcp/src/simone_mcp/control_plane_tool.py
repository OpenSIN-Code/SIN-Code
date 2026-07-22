"""Single MCP adapter for Simone's durable control plane."""

from __future__ import annotations

from typing import Any

from .control_plane import ControlPlaneStore

TOOL_NAME = "sin_simone_mcp_control_plane"

OPERATIONS = [
    "task.create",
    "task.get",
    "task.list",
    "task.transition",
    "event.append",
    "artifact.attach",
    "evidence.attach",
    "decision.record",
    "research.add",
    "research.update",
    "research.next",
    "summary.get",
]

CONTROL_PLANE_TOOL = {
    "name": TOOL_NAME,
    "title": "Simone Control Plane",
    "description": (
        "Manage durable workflow tasks, evidence, research, "
        "decisions, artifacts, and append-only events."
    ),
    "inputSchema": {
        "$schema": "https://json-schema.org/draft/2020-12/schema",
        "type": "object",
        "additionalProperties": False,
        "required": ["operation", "payload"],
        "properties": {
            "operation": {
                "type": "string",
                "enum": OPERATIONS,
            },
            "payload": {
                "type": "object",
            },
        },
    },
    "outputSchema": {
        "$schema": "https://json-schema.org/draft/2020-12/schema",
        "type": "object",
        "required": ["ok"],
        "properties": {
            "ok": {"type": "boolean"},
            "operation": {"type": "string"},
            "result": {},
            "error": {"type": "string"},
        },
    },
    "annotations": {
        "readOnlyHint": False,
        "destructiveHint": False,
        "idempotentHint": False,
        "openWorldHint": False,
    },
    "execution": {
        "taskSupport": "forbidden",
    },
}


def execute_control_plane(
    payload: dict[str, Any],
) -> dict[str, Any]:
    operation = str(
        payload.get("operation") or ""
    )
    arguments = payload.get("payload")

    if operation not in OPERATIONS:
        return {
            "ok": False,
            "operation": operation,
            "error": "unknown_control_plane_operation",
        }

    if not isinstance(arguments, dict):
        return {
            "ok": False,
            "operation": operation,
            "error": "payload_must_be_object",
        }

    store = ControlPlaneStore()

    try:
        if operation == "task.create":
            result = store.create_task(**arguments)

        elif operation == "task.get":
            result = store.get_task(**arguments)

        elif operation == "task.list":
            result = store.list_tasks(**arguments)

        elif operation == "task.transition":
            result = store.transition_task(**arguments)

        elif operation == "event.append":
            result = store.append_event(**arguments)

        elif operation == "artifact.attach":
            result = store.attach_artifact(**arguments)

        elif operation == "evidence.attach":
            result = store.attach_evidence(**arguments)

        elif operation == "decision.record":
            result = store.record_decision(**arguments)

        elif operation == "research.add":
            result = store.add_research_question(**arguments)

        elif operation == "research.update":
            result = store.update_research_question(**arguments)

        elif operation == "research.next":
            result = store.next_research_questions(**arguments)

        elif operation == "summary.get":
            result = store.summary(**arguments)

        else:
            raise AssertionError(
                f"unhandled operation: {operation}"
            )

        return {
            "ok": True,
            "operation": operation,
            "result": result,
        }

    except Exception as error:
        return {
            "ok": False,
            "operation": operation,
            "error": str(error),
        }
