# SPDX-License-Identifier: MIT
"""MultiRepoDelegator: orchestrates tasks across repo boundaries.

Reuses the entire existing stack — Scheduler, Runner, Gates, Budget
Governor, Escalations — and only changes two things:
  a) Worktree mapping per task by repo (instead of global)
  b) Merges are STAGED, not immediate; the global commit point is
     AFTER the scheduler run (Two-Phase).

Additionally: contracts are extracted after each agent run and injected
into the prompts of dependent tasks in other repos.
"""

from __future__ import annotations

import asyncio
import json
import time
from dataclasses import replace

from . import memory
from .escalation import EscalationBroker, EscalationKind
from .ledger import Ledger
from .models import (Risk, RunResult, Task, TaskOutcome, TaskState)
from .multirepo import (CONTRACT_INSTRUCTIONS, ContractStore, MergeUnit,
                        MultiRepoPlan, TwoPhaseMerger, extract_contract)
from .runner import runner_for
from .scheduler import Scheduler
from .verify import verify
from .worktree import GitError, WorktreeManager


def _topo_order(plan) -> list:
    """Deterministic topological order of task ids."""
    tasks = {t.id: t for t in plan.tasks}
    order: list = []
    seen: set = set()

    def visit(tid: str) -> None:
        if tid in seen:
            return
        seen.add(tid)
        for d in sorted(tasks[tid].deps):
            visit(d)
        order.append(tid)

    for tid in sorted(tasks):
        visit(tid)
    return order


class MultiRepoDelegator:
    def __init__(self, mrp: MultiRepoPlan, ledger: Ledger | None = None,
                 max_parallel: int = 4) -> None:
        mrp.validate()
        self.mrp = mrp
        self.plan = mrp.plan
        self.ledger = ledger or Ledger()
        self.max_parallel = max_parallel
        self.wtms = {name: WorktreeManager(ref.path, ref.base_branch)
                     for name, ref in mrp.repos.items()}
        self.contracts = ContractStore(self.ledger, self.plan.id)
        self.merger = TwoPhaseMerger(mrp.repos, self.ledger, self.plan.id)
        self.broker = EscalationBroker(self.ledger)
        self._titles = {t.id: t.title for t in self.plan.tasks}
        self._pitfalls = memory.recall_pitfalls(self.plan.goal)
        self._retry_context: dict = {}

    async def run(self) -> RunResult:
        started = time.time()
        self.ledger.register_run(self.plan.id, self.plan.goal,
                                 self._plan_json())
        scheduler = Scheduler(self.plan, self.ledger, self._execute_task,
                              max_parallel=self.max_parallel)
        outcomes = await scheduler.run()

        all_verified = all(
            o.state in (TaskState.DONE, TaskState.SKIPPED)
            for o in outcomes.values())
        if all_verified and self.merger.units:
            try:
                self.merger.commit(_topo_order(self.plan))
                for unit in self.merger.units:
                    unit.worktree.destroy()
            except GitError as e:
                for unit in self.merger.units:
                    esc = self.broker.raise_escalation(
                        self.plan.id, unit.task_id,
                        self._titles[unit.task_id],
                        EscalationKind.MERGE_CONFLICT,
                        summary=f"two-phase commit aborted: {e}",
                        evidence={"repo": unit.repo_name},
                        branch=unit.worktree.branch,
                        worktree=str(unit.worktree.path))
                    outcomes[unit.task_id] = TaskOutcome(
                        unit.task_id, TaskState.ESCALATED,
                        error=f"escalation {esc.id}: {e}",
                        branch=unit.worktree.branch)
                    self.ledger.emit(
                        self.plan.id, unit.task_id,
                        "state:escalated", {"error": str(e)})
        elif self.merger.units:
            self.ledger.emit(
                self.plan.id, "*", "merge:phase2_withheld",
                {"reason": "not all tasks verified",
                 "staged": len(self.merger.units)})

        return RunResult(self.plan.id, self.plan.goal, outcomes,
                         started, time.time())

    def run_sync(self) -> RunResult:
        return asyncio.run(self.run())

    async def _execute_task(self, task: Task) -> TaskOutcome:
        repo_name = self.mrp.task_repo[task.id]
        wt = self.wtms[repo_name].create(self.plan.id, task.id)
        outcome = TaskOutcome(
            task.id, TaskState.RUNNING,
            worktree=str(wt.path), branch=wt.branch,
        )
        enriched = self._enrich(task)
        runner = runner_for(enriched.agent)
        res = await runner.run(enriched, str(wt.path),
                               timeout=task.budget.max_seconds)
        self.ledger.emit(
            self.plan.id, task.id, "agent:finished",
            {"repo": repo_name, "exit": res.exit_code,
             "tail": res.output[-2000:]})
        if not res.ok:
            outcome.state = TaskState.FAILED
            outcome.error = f"agent exited {res.exit_code}"
            return outcome

        contract = extract_contract(res.output)
        if contract:
            self.contracts.publish(task.id, contract)

        if not wt.commit_all(
                f"sin-delegate: {task.title} [{task.id}]"):
            outcome.state = TaskState.FAILED
            outcome.error = "agent finished but changed nothing"
            return outcome

        outcome.state = TaskState.VERIFYING
        self.ledger.emit(self.plan.id, task.id, "state:verifying")
        verdict = verify(task, wt)
        outcome.verdict = verdict
        self.ledger.emit(
            self.plan.id, task.id, "verdict",
            {"passed": verdict.passed, "gates": verdict.gates})

        if not verdict.passed:
            memory.remember_pitfall(
                self.plan.goal, task.title, verdict.summary)
            if task.risk == Risk.HIGH:
                esc = self.broker.raise_escalation(
                    self.plan.id, task.id, task.title,
                    EscalationKind.GATE_FAILURE,
                    summary=f"HIGH-risk task failed gates: "
                            f"{verdict.summary}",
                    evidence={"gates": verdict.gates,
                              "diff_stat": wt.diff_stat(),
                              "repo": repo_name},
                    branch=wt.branch, worktree=str(wt.path))
                outcome.state = TaskState.ESCALATED
                outcome.error = f"escalation {esc.id}"
            else:
                self._retry_context[task.id] = (
                    "Dein letzter Versuch riss die Gates:\n"
                    + verdict.summary)
                outcome.state = TaskState.FAILED
                outcome.error = verdict.summary
            return outcome

        # NO merge here — only stage (Phase 1 ends with verification)
        self.merger.stage(MergeUnit(task.id, wt, repo_name))
        outcome.state = TaskState.DONE
        return outcome

    def _enrich(self, task: Task) -> Task:
        extra = [CONTRACT_INSTRUCTIONS]
        contracts = self.contracts.render(task.deps, self._titles)
        if contracts:
            extra.append(contracts)
        if self._pitfalls:
            extra.append(
                "Bekannte Pitfalls (vermeiden):\n- "
                + "\n- ".join(self._pitfalls))
        if task.id in self._retry_context:
            extra.append(self._retry_context[task.id])
        return replace(
            task,
            instructions=task.instructions + "\n\n"
            + "\n\n".join(extra))

    def _plan_json(self) -> str:
        return json.dumps({
            "goal": self.plan.goal,
            "multi_repo": True,
            "repos": {n: {"path": r.path, "base_branch": r.base_branch}
                      for n, r in self.mrp.repos.items()},
            "tasks": [{
                "id": t.id, "title": t.title,
                "instructions": t.instructions,
                "deps": list(t.deps),
                "files_hint": list(t.files_hint),
                "risk": t.risk.value, "verify": list(t.verify),
                "backend": t.agent.backend, "model": t.agent.model,
                "repo": self.mrp.task_repo[t.id],
            } for t in self.plan.tasks],
        }, indent=2)
