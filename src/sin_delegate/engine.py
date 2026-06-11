# SPDX-License-Identifier: MIT
"""The Delegator: glues plans to worktrees, runners, verification, ledger, memory."""

from __future__ import annotations

import asyncio
import json
import time
from dataclasses import replace

from . import memory
from .ledger import Ledger
from .models import (Plan, Risk, RunResult, Task, TaskOutcome, TaskState)
from .runner import EchoRunner, runner_for
from .scheduler import Scheduler
from .worktree import GitError, WorktreeManager


class Delegator:
    def __init__(self, plan: Plan, ledger: Ledger | None = None,
                 max_parallel: int = 4, dry_run: bool = False,
                 keep_worktrees: bool = False) -> None:
        self.plan = plan
        self.ledger = ledger or Ledger()
        self.max_parallel = max_parallel
        self.dry_run = dry_run
        self.keep_worktrees = keep_worktrees
        self.wtm = None if dry_run else WorktreeManager(
            plan.repo, plan.base_branch)
        self._pitfalls = memory.recall_pitfalls(plan.goal)
        self._retry_context: dict[str, str] = {}

    async def run(self) -> RunResult:
        started = time.time()
        self.ledger.register_run(self.plan.id, self.plan.goal,
                                 self._plan_json())
        scheduler = Scheduler(self.plan, self.ledger, self._execute_task,
                              max_parallel=self.max_parallel)
        outcomes = await scheduler.run()
        result = RunResult(self.plan.id, self.plan.goal, outcomes,
                           started, time.time())
        if result.ok:
            memory.remember_decision(
                self.plan.goal,
                f"plan {self.plan.id} succeeded: "
                f"{sum(o.state == TaskState.DONE for o in outcomes.values())} "
                f"tasks merged")
        return result

    def run_sync(self) -> RunResult:
        return asyncio.run(self.run())

    async def _execute_task(self, task: Task) -> TaskOutcome:
        if self.dry_run:
            await EchoRunner().run(task, ".", 5)
            return TaskOutcome(task.id, TaskState.DONE,
                               worktree="(dry-run)")

        wt = self.wtm.create(self.plan.id, task.id)
        outcome = TaskOutcome(
            task.id, TaskState.RUNNING,
            worktree=str(wt.path), branch=wt.branch,
        )
        try:
            enriched = self._enrich(task)
            runner = runner_for(enriched.agent)
            res = await runner.run(enriched, str(wt.path),
                                   timeout=task.budget.max_seconds)
            self.ledger.emit(
                self.plan.id, task.id, "agent:finished",
                {"exit": res.exit_code,
                 "tail": res.output[-2000:]})
            if not res.ok:
                outcome.state = TaskState.FAILED
                outcome.error = f"agent exited {res.exit_code}"
                self._learn(task, outcome.error)
                return outcome

            if not wt.commit_all(
                    f"sin-delegate: {task.title} [{task.id}]"):
                outcome.state = TaskState.FAILED
                outcome.error = "agent finished but changed nothing"
                self._learn(task, outcome.error)
                return outcome

            from .verify import verify
            outcome.state = TaskState.VERIFYING
            self.ledger.emit(self.plan.id, task.id, "state:verifying")
            verdict = verify(task, wt)
            outcome.verdict = verdict
            self.ledger.emit(
                self.plan.id, task.id, "verdict",
                {"passed": verdict.passed, "gates": verdict.gates})

            if not verdict.passed:
                self._learn(task, verdict.summary)
                if task.risk == Risk.HIGH:
                    outcome.state = TaskState.ESCALATED
                    outcome.error = (f"HIGH-risk task failed gates: "
                                     f"{verdict.summary}")
                else:
                    self._retry_context[task.id] = self._verdict_feedback(verdict)
                    outcome.state = TaskState.FAILED
                    outcome.error = verdict.summary
                return outcome

            outcome.state = TaskState.MERGING
            self.ledger.emit(self.plan.id, task.id, "state:merging")
            try:
                snapshot = wt.merge_back()
                self.ledger.emit(
                    self.plan.id, task.id, "merged",
                    {"snapshot": snapshot})
            except GitError as e:
                outcome.state = TaskState.ESCALATED
                outcome.error = str(e)
                self._learn(task, f"merge conflict: {e}")
                return outcome

            outcome.state = TaskState.DONE
            return outcome
        finally:
            if outcome.state == TaskState.DONE and not self.keep_worktrees:
                wt.destroy()

    def _enrich(self, task: Task) -> Task:
        extra: list[str] = []
        if self._pitfalls:
            extra.append(
                "Known pitfalls from past runs (avoid these):\n- "
                + "\n- ".join(self._pitfalls))
        if task.id in self._retry_context:
            extra.append(self._retry_context[task.id])
        if not extra:
            return task
        instructions = task.instructions + "\n\n" + "\n\n".join(extra)
        return replace(task, instructions=instructions)

    @staticmethod
    def _verdict_feedback(verdict) -> str:
        failed = {n: r["detail"] for n, r in verdict.gates.items()
                  if not r["ok"]}
        return ("Your previous attempt FAILED verification. Fix exactly this:"
                "\n" + json.dumps(failed, indent=2))

    def _learn(self, task: Task, detail: str) -> None:
        memory.remember_pitfall(self.plan.goal, task.title, detail)

    def _plan_json(self) -> str:
        return json.dumps({
            "goal": self.plan.goal,
            "repo": self.plan.repo,
            "base_branch": self.plan.base_branch,
            "tasks": [{
                "id": t.id, "title": t.title,
                "instructions": t.instructions,
                "deps": list(t.deps),
                "files_hint": list(t.files_hint),
                "risk": t.risk.value, "verify": list(t.verify),
                "backend": t.agent.backend, "model": t.agent.model,
            } for t in self.plan.tasks],
        }, indent=2)


def delegate(goal: str, tasks: list, repo: str = ".",
             base_branch: str = "main", max_parallel: int = 4,
             dry_run: bool = False) -> RunResult:
    """One-shot convenience API."""
    plan = Plan(goal=goal, tasks=tuple(t.finalize() for t in tasks),
                repo=repo, base_branch=base_branch)
    return Delegator(plan, max_parallel=max_parallel,
                     dry_run=dry_run).run_sync()
