# SPDX-License-Identifier: MIT
"""Resolution loop: turn decided escalations into machine actions.

apply_resolutions(plan) runs BEFORE a resume and translates every open
resolution into ledger-state + plan-mutation:

  RETRY_WITH_GUIDANCE -> state=pending, guidance injected on next _enrich,
                         worktree recreated from current base
  ACCEPT_BRANCH       -> branch merged via merge saga, state=done with
                         verdict_overridden
  MANUAL_MERGE        -> state=done without merge (human did it);
                         worktree/branch cleaned up
  DROP_TASK           -> state=skipped (downstream auto-skipped via doomed())
  ABORT_PLAN          -> all non-terminal tasks -> CANCELLED
"""

from __future__ import annotations

from .escalation import ActionType, EscalationBroker
from .ledger import Ledger
from .models import Plan, TaskState
from .worktree import GitError, WorktreeManager


def apply_resolutions(plan: Plan, ledger: Ledger | None = None) -> dict:
    """Returns {applied, guidance, aborted}."""
    ledger = ledger or Ledger()
    broker = EscalationBroker(ledger)
    guidance: dict[str, str] = {}
    applied = 0
    aborted = False

    for res in broker.pending_resolutions(plan.id):
        action = ActionType(res["action"])
        task_id = res["task_id"]
        eid = res["escalation_id"]

        if action == ActionType.RETRY_WITH_GUIDANCE:
            broker.mark_applied(plan.id, task_id, eid)
            ledger.emit(plan.id, task_id, "state:pending",
                        {"via": "escalation_retry"})
            if res.get("user_input"):
                guidance[task_id] = (
                    "Ein menschlicher Reviewer hat deinen letzten Versuch "
                    "geprüft und folgende Anweisung gegeben — befolge sie "
                    "exakt:\n" + res["user_input"])
            _recreate_worktree(plan, task_id)

        elif action == ActionType.ACCEPT_BRANCH:
            ok = _merge_branch(plan, task_id, ledger)
            broker.mark_applied(plan.id, task_id, eid)
            ledger.emit(plan.id, task_id,
                        "state:done" if ok else "state:escalated",
                        {"via": "accept_branch",
                         "verdict_overridden": True})

        elif action == ActionType.MANUAL_MERGE:
            broker.mark_applied(plan.id, task_id, eid)
            ledger.emit(plan.id, task_id, "state:done",
                        {"via": "manual_merge"})
            _cleanup_worktree(plan, task_id)

        elif action == ActionType.DROP_TASK:
            broker.mark_applied(plan.id, task_id, eid)
            ledger.emit(plan.id, task_id, "state:skipped",
                        {"via": "drop_task"})

        elif action == ActionType.ABORT_PLAN:
            broker.mark_applied(plan.id, task_id, eid)
            aborted = True
            states = ledger.task_states(plan.id)
            terminal = {TaskState.DONE, TaskState.FAILED, TaskState.SKIPPED,
                        TaskState.CANCELLED}
            for tid in (t.id for t in plan.tasks):
                if states.get(tid) not in terminal:
                    ledger.emit(plan.id, tid, "state:cancelled",
                                {"via": "abort_plan"})

        applied += 1

    return {"applied": applied, "guidance": guidance, "aborted": aborted}


def _recreate_worktree(plan: Plan, task_id: str) -> None:
    try:
        wtm = WorktreeManager(plan.repo, plan.base_branch)
        wt = wtm.create(plan.id, task_id)
        wt.destroy(delete_branch=True)
        wtm.create(plan.id, task_id)
    except GitError:
        pass


def _merge_branch(plan: Plan, task_id: str, ledger: Ledger) -> bool:
    try:
        wtm = WorktreeManager(plan.repo, plan.base_branch)
        wt = wtm.create(plan.id, task_id)
        snapshot = wt.merge_back()
        ledger.emit(plan.id, task_id, "merged",
                    {"snapshot": snapshot, "via": "accept_branch"})
        wt.destroy()
        return True
    except GitError as e:
        ledger.emit(plan.id, task_id, "escalation:merge_retry_failed",
                    {"error": str(e)})
        return False


def _cleanup_worktree(plan: Plan, task_id: str) -> None:
    try:
        wtm = WorktreeManager(plan.repo, plan.base_branch)
        wtm.create(plan.id, task_id).destroy(delete_branch=False)
    except GitError:
        pass
