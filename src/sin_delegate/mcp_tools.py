# SPDX-License-Identifier: MIT
"""MCP tool surface — registers 4 tools on the unified sin-serve server.

register(add_tool) follows the SIN-Code Bundle plugin contract.
"""

from __future__ import annotations

import json
from typing import Any, Callable

from .engine import Delegator
from .escalation import EscalationBroker
from .ledger import Ledger
from .planfile import plan_from_dict


_PLAN_SCHEMA = {
    "type": "object",
    "required": ["plan"],
    "properties": {
        "plan": {"type": "object",
                 "description": "Plan: {goal, base_branch?, tasks:[{...}]}"},
        "repo": {"type": "string", "default": "."},
        "parallel": {"type": "integer", "default": 4},
        "dry_run": {"type": "boolean", "default": False},
    },
}


async def _tool_delegate(args: dict) -> dict:
    data = json.loads(args["plan"]) if isinstance(args["plan"], str) \
        else args["plan"]
    if isinstance(data, dict) and "tasks" in data:
        specs = data["tasks"]
    else:
        specs = data
    plan = plan_from_dict(
        {"goal": data.get("goal", "mcp-task"),
         "base_branch": data.get("base_branch", "main"),
         "tasks": specs},
        repo=args.get("repo", "."))
    dele = Delegator(plan, max_parallel=int(args.get("parallel", 4)),
                     dry_run=bool(args.get("dry_run", False)))
    result = dele.run_sync()
    return json.loads(result.to_json())


async def _tool_status(args: dict) -> dict:
    states = Ledger().task_states(args["plan_id"])
    return {"plan_id": args["plan_id"],
            "states": {k: v.value for k, v in states.items()}}


async def _tool_history(args: dict) -> dict:
    return {"plan_id": args["plan_id"],
            "events": Ledger().history(args["plan_id"])}


async def _tool_cancel(args: dict) -> dict:
    Ledger().emit(args["plan_id"], "*", "cancel:requested")
    return {"plan_id": args["plan_id"], "cancelled": True}


async def _tool_escalations(args: dict) -> dict:
    return {"plan_id": args["plan_id"],
            "escalations": EscalationBroker().open_escalations(
                args["plan_id"])}


async def _tool_resolve(args: dict) -> dict:
    return EscalationBroker().resolve(
        args["plan_id"], args["escalation_id"], args["option_id"],
        user_input=args.get("input", ""), decided_by="parent_agent")


def register(add_tool: Callable) -> None:
    add_tool(
        name="sin_delegate",
        description=(
            "Delegate a goal to parallel, budget-governed sub-agents. Tasks "
            "run in isolated git worktrees, pass verification gates (diff "
            "screen, tests, architecture) and merge back atomically. "
            "Resumable: re-submitting an identical plan skips DONE tasks."),
        schema=_PLAN_SCHEMA,
        handler=_tool_delegate,
    )
    add_tool(
        name="sin_delegate_status",
        description="Current state of every task in a delegation run.",
        schema={"type": "object", "required": ["plan_id"],
                "properties": {"plan_id": {"type": "string"}}},
        handler=_tool_status,
    )
    add_tool(
        name="sin_delegate_history",
        description="Full audit event log of a delegation run.",
        schema={"type": "object", "required": ["plan_id"],
                "properties": {"plan_id": {"type": "string"}}},
        handler=_tool_history,
    )
    add_tool(
        name="sin_delegate_cancel",
        description="Cooperatively cancel a running delegation.",
        schema={"type": "object", "required": ["plan_id"],
                "properties": {"plan_id": {"type": "string"}}},
        handler=_tool_cancel,
    )
    add_tool(
        name="sin_delegate_escalations",
        description=(
            "Open decision requests of a delegation run. Each escalation "
            "contains full evidence and a finite set of typed options."),
        schema={"type": "object", "required": ["plan_id"],
                "properties": {"plan_id": {"type": "string"}}},
        handler=_tool_escalations,
    )
    add_tool(
        name="sin_delegate_resolve",
        description=(
            "Answer an escalation by choosing an option_id. Options of "
            "type retry_with_guidance require 'input'."),
        schema={"type": "object",
                "required": ["plan_id", "escalation_id", "option_id"],
                "properties": {
                    "plan_id": {"type": "string"},
                    "escalation_id": {"type": "string"},
                    "option_id": {"type": "string"},
                    "input": {"type": "string", "default": ""}}},
        handler=_tool_resolve,
    )
