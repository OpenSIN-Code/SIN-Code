# SPDX-License-Identifier: MIT
"""Escalation protocol: structured decision requests with typed options.

An escalation is a complete decision packet: context, evidence, and a
finite set of TYPED OPTIONS with explicit consequences. The decider
(human or parent-agent) chooses an option_id — never free text.

Persisted in the ledger (kind='escalation:*') — resumable: open
escalations survive crashes.
resolve() is idempotent: double resolutions of the same escalation
are ignored (first-writer-wins, tracked in the ledger).
"""

from __future__ import annotations

import uuid
from dataclasses import dataclass, field
from enum import Enum
from typing import Any

from .ledger import Ledger


class EscalationKind(str, Enum):
    GATE_FAILURE = "gate_failure"
    MERGE_CONFLICT = "merge_conflict"
    BUDGET_EXHAUSTED = "budget_exhausted"
    AGENT_ERROR = "agent_error"


class ActionType(str, Enum):
    RETRY_WITH_GUIDANCE = "retry_with_guidance"
    ACCEPT_BRANCH = "accept_branch"
    MANUAL_MERGE = "manual_merge"
    DROP_TASK = "drop_task"
    ABORT_PLAN = "abort_plan"


@dataclass(frozen=True)
class Option:
    id: str
    action: ActionType
    label: str
    consequence: str
    requires_input: bool = False


@dataclass
class Escalation:
    id: str
    plan_id: str
    task_id: str
    task_title: str
    kind: EscalationKind
    summary: str
    evidence: dict
    options: list
    branch: str = ""
    worktree: str = ""

    def to_dict(self) -> dict:
        return {
            "id": self.id, "plan_id": self.plan_id,
            "task_id": self.task_id, "task_title": self.task_title,
            "kind": self.kind.value, "summary": self.summary,
            "evidence": self.evidence, "branch": self.branch,
            "worktree": self.worktree,
            "options": [{
                "id": o.id, "action": o.action.value, "label": o.label,
                "consequence": o.consequence,
                "requires_input": o.requires_input,
            } for o in self.options],
        }


def _options_for(kind: EscalationKind, branch: str) -> list:
    common_drop = Option(
        "drop", ActionType.DROP_TASK, "Task verwerfen",
        "Task wird SKIPPED; alle abhängigen Tasks werden ebenfalls "
        "übersprungen. Der Branch bleibt zur Inspektion erhalten.")
    common_abort = Option(
        "abort", ActionType.ABORT_PLAN, "Gesamten Plan abbrechen",
        "Alle laufenden Tasks werden kooperativ beendet. Bereits gemergte "
        "Tasks bleiben gemerged (kein globaler Rollback).")
    if kind == EscalationKind.GATE_FAILURE:
        return [
            Option("retry", ActionType.RETRY_WITH_GUIDANCE,
                   "Erneut versuchen mit Korrekturhinweis",
                   "Der Sub-Agent erhält deinen Hinweis + das Gate-Verdict "
                   "und versucht es einmal erneut (frisches Budget-Lease).",
                   requires_input=True),
            Option("accept", ActionType.ACCEPT_BRANCH,
                   "Ergebnis trotz Gate-Failure akzeptieren",
                   f"Branch {branch} wird OHNE bestandene Gates gemerged. "
                   "Das Verdict wird als 'overridden' im Ledger vermerkt — "
                   "du übernimmst die Verantwortung."),
            common_drop, common_abort,
        ]
    if kind == EscalationKind.MERGE_CONFLICT:
        return [
            Option("manual", ActionType.MANUAL_MERGE,
                   "Konflikt manuell auflösen",
                   f"Du löst den Rebase-Konflikt auf Branch {branch} selbst "
                   "und meldest danach 'resolved' — der Task gilt als DONE, "
                   "Downstream-Tasks laufen weiter."),
            Option("retry", ActionType.RETRY_WITH_GUIDANCE,
                   "Neu implementieren auf aktuellem Stand",
                   "Worktree wird auf den aktuellen Base-Stand neu erzeugt; "
                   "der Agent implementiert gegen die neue Basis.",
                   requires_input=True),
            common_drop, common_abort,
        ]
    if kind == EscalationKind.BUDGET_EXHAUSTED:
        return [
            Option("retry", ActionType.RETRY_WITH_GUIDANCE,
                   "Mit zusätzlichem Budget erneut versuchen",
                   "Der Task erhält ein frisches Lease außerhalb des "
                   "ursprünglichen Global-Budgets.", requires_input=False),
            common_drop, common_abort,
        ]
    return [
        Option("retry", ActionType.RETRY_WITH_GUIDANCE,
               "Mit anderem Backend erneut versuchen",
               "Die Policy wählt das nächstbeste Backend für die "
               "task_class; der Hinweis wird injiziert.",
               requires_input=True),
        common_drop, common_abort,
    ]


class EscalationBroker:
    def __init__(self, ledger: Ledger | None = None) -> None:
        self.ledger = ledger or Ledger()

    def raise_escalation(self, plan_id: str, task_id: str,
                         task_title: str, kind: EscalationKind,
                         summary: str, evidence: dict,
                         branch: str = "",
                         worktree: str = "") -> Escalation:
        esc = Escalation(
            id=uuid.uuid4().hex[:12], plan_id=plan_id, task_id=task_id,
            task_title=task_title, kind=kind, summary=summary,
            evidence=evidence, branch=branch, worktree=worktree,
            options=_options_for(kind, branch),
        )
        self.ledger.emit(plan_id, task_id, "escalation:raised",
                         esc.to_dict())
        return esc

    def open_escalations(self, plan_id: str) -> list:
        raised: dict = {}
        resolved: set = set()
        for ev in self.ledger.history(plan_id):
            if ev["kind"] == "escalation:raised":
                raised[ev["payload"]["id"]] = ev["payload"]
            elif ev["kind"] == "escalation:resolved":
                resolved.add(ev["payload"]["escalation_id"])
        return [e for eid, e in raised.items() if eid not in resolved]

    def resolve(self, plan_id: str, escalation_id: str, option_id: str,
                user_input: str = "",
                decided_by: str = "human") -> dict:
        open_ = {e["id"]: e for e in self.open_escalations(plan_id)}
        esc = open_.get(escalation_id)
        if esc is None:
            return {"ok": False,
                    "error": f"escalation {escalation_id} not open "
                             f"(unknown or already resolved)"}
        option = next((o for o in esc["options"] if o["id"] == option_id),
                      None)
        if option is None:
            valid = [o["id"] for o in esc["options"]]
            return {"ok": False,
                    "error": f"unknown option {option_id!r}; "
                             f"valid: {valid}"}
        if option["requires_input"] and not user_input.strip():
            return {"ok": False,
                    "error": f"option {option_id!r} requires input "
                             f"(e.g. guidance for the retry)"}
        self.ledger.emit(plan_id, esc["task_id"],
                         "escalation:resolved", {
            "escalation_id": escalation_id,
            "task_id": esc["task_id"],
            "option_id": option_id,
            "action": option["action"],
            "user_input": user_input,
            "decided_by": decided_by,
        })
        return {"ok": True, "action": option["action"],
                "task_id": esc["task_id"], "user_input": user_input}

    def pending_resolutions(self, plan_id: str) -> list:
        resolutions: list = []
        applied: set = set()
        for ev in self.ledger.history(plan_id):
            if ev["kind"] == "escalation:resolved":
                resolutions.append(ev["payload"])
            elif ev["kind"] == "escalation:applied":
                applied.add(ev["payload"]["escalation_id"])
        return [r for r in resolutions
                if r["escalation_id"] not in applied]

    def mark_applied(self, plan_id: str, task_id: str,
                     escalation_id: str) -> None:
        self.ledger.emit(plan_id, task_id, "escalation:applied",
                         {"escalation_id": escalation_id})
