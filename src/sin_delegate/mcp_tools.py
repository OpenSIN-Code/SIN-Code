# SPDX-License-Identifier: MIT
"""MCP tool surface — registers 6 tools on the unified sin-serve server.

register(add_tool) follows the SIN-Code Bundle plugin contract.
"""

from __future__ import annotations

import json
from typing import Callable

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
    plan_arg = args.get("plan")
    if not plan_arg:
        return {"error": "missing required field 'plan'"}
    try:
        data = json.loads(plan_arg) if isinstance(plan_arg, str) else plan_arg
    except json.JSONDecodeError as e:
        return {"error": f"plan is not valid JSON: {e}"}
    if not isinstance(data, dict) or "tasks" not in data:
        return {"error": "plan must be an object with a 'tasks' list"}
    try:
        parallel = int(args.get("parallel", 4))
    except (TypeError, ValueError) as e:
        return {"error": f"'parallel' must be an integer: {e}"}
    try:
        plan = plan_from_dict(
            {"goal": data.get("goal", "mcp-task"),
             "base_branch": data.get("base_branch", "main"),
             "tasks": data["tasks"]},
            repo=args.get("repo", "."))
    except Exception as e:
        return {"error": f"invalid plan: {type(e).__name__}: {e}"}
    dele = Delegator(plan, max_parallel=parallel,
                     dry_run=bool(args.get("dry_run", False)))
    result = await dele.run()
    return json.loads(result.to_json())


def _plan_id(args: dict) -> str | dict:
    pid = args.get("plan_id")
    if not pid:
        return {"error": "missing required field 'plan_id'"}
    return pid


async def _tool_status(args: dict) -> dict:
    pid = _plan_id(args)
    if isinstance(pid, dict):
        return pid
    states = Ledger().task_states(pid)
    return {"plan_id": pid,
            "states": {k: v.value for k, v in states.items()}}


async def _tool_history(args: dict) -> dict:
    pid = _plan_id(args)
    if isinstance(pid, dict):
        return pid
    return {"plan_id": pid, "events": Ledger().history(pid)}


async def _tool_cancel(args: dict) -> dict:
    pid = _plan_id(args)
    if isinstance(pid, dict):
        return pid
    Ledger().emit(pid, "*", "cancel:requested")
    return {"plan_id": pid, "cancelled": True}


async def _tool_escalations(args: dict) -> dict:
    pid = _plan_id(args)
    if isinstance(pid, dict):
        return pid
    return {"plan_id": pid,
            "escalations": EscalationBroker().open_escalations(pid)}


async def _tool_resolve(args: dict) -> dict:
    pid = _plan_id(args)
    if isinstance(pid, dict):
        return pid
    for field in ("escalation_id", "option_id"):
        if not args.get(field):
            return {"error": f"missing required field '{field}'"}
    return EscalationBroker().resolve(
        pid, args["escalation_id"], args["option_id"],
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
