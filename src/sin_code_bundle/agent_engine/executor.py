# SPDX-License-Identifier: MIT
"""Parallel async executor over isolated git worktrees."""

from __future__ import annotations

import asyncio
import shutil
import subprocess
import tempfile
import time
from pathlib import Path
from typing import Callable

from .planner import Planner
from .router import CircuitOpenError, ToolRouter
from .telemetry import Telemetry
from .types import AgentTask, Plan, Step, StepResult, StepState


class Executor:
    def __init__(self, router: ToolRouter, telemetry: Telemetry,
                 on_step_terminal: Callable[[str, StepState], None] | None = None
                 ) -> None:
        self.router = router
        self.telemetry = telemetry
        self.on_step_terminal = on_step_terminal
        self._write_fence = asyncio.Lock()
        self._worktrees: list[str] = []

    def _create_worktree(self, repo_root: str, step_id: str) -> str:
        wt = tempfile.mkdtemp(prefix=f"sin-wt-{step_id}-")
        subprocess.run(
            ["git", "-C", repo_root, "worktree", "add", "--detach", wt],
            check=True, capture_output=True, text=True, timeout=60,
        )
        self._worktrees.append(wt)
        return wt

    def cleanup(self, repo_root: str) -> None:
        for wt in self._worktrees:
            subprocess.run(
                ["git", "-C", repo_root, "worktree", "remove", "--force", wt],
                capture_output=True, text=True, timeout=60,
            )
            shutil.rmtree(wt, ignore_errors=True)
        self._worktrees.clear()

    async def run(self, task: AgentTask, plan: Plan,
                  planner: Planner) -> dict[str, StepResult]:
        results: dict[str, StepResult] = {}
        sem = asyncio.Semaphore(task.max_parallelism)
        in_flight: set[asyncio.Task[None]] = set()
        deadline = time.monotonic() + task.budget_seconds

        async def _notify(state: StepState, sid: str) -> None:
            if self.on_step_terminal:
                await asyncio.to_thread(self.on_step_terminal, sid, state)

        async def run_step(step: Step) -> None:
            async with sem:
                step.state = StepState.RUNNING
                step.attempts += 1
                self.telemetry.emit("step_start", step_id=step.step_id,
                                    tool=step.tool, attempt=step.attempts)
                start = time.monotonic()
                worktree: str | None = None
                try:
                    args = dict(step.args)
                    if step.isolated:
                        worktree = await asyncio.to_thread(
                            self._create_worktree, task.repo_root, step.step_id
                        )
                        args.setdefault("cwd", worktree)
                        output = await self.router.call(step.tool, **args)
                    else:
                        args.setdefault("cwd", task.repo_root)
                        async with self._write_fence:
                            output = await self.router.call(step.tool, **args)
                    step.state = StepState.SUCCEEDED
                    results[step.step_id] = StepResult(
                        step_id=step.step_id, ok=True, output=output,
                        duration_s=time.monotonic() - start,
                        worktree=worktree,
                    )
                    self.telemetry.emit("step_ok", step_id=step.step_id,
                                        duration_s=round(time.monotonic() - start, 3))
                    await _notify(step.state, step.step_id)
                except Exception as err:
                    if step.attempts < step.max_attempts and not isinstance(
                        err, CircuitOpenError
                    ):
                        step.state = StepState.PENDING
                        self.telemetry.emit("step_retry", step_id=step.step_id,
                                            error=str(err)[:500])
                    else:
                        step.state = StepState.FAILED
                        results[step.step_id] = StepResult(
                            step_id=step.step_id, ok=False, error=str(err),
                            duration_s=time.monotonic() - start,
                        )
                        skipped = planner.propagate_failure(plan, step.step_id)
                        self.telemetry.emit("step_fail", step_id=step.step_id,
                                            error=str(err)[:500], skipped=skipped)
                        await _notify(step.state, step.step_id)
                        for s in skipped:
                            await _notify(StepState.SKIPPED, s)

        while True:
            if time.monotonic() > deadline:
                self.telemetry.emit("budget_exhausted", task_id=task.task_id)
                for t in in_flight:
                    t.cancel()
                break

            for step in planner.ready_steps(plan):
                step.state = StepState.READY
                t = asyncio.create_task(run_step(step))
                in_flight.add(t)
                t.add_done_callback(in_flight.discard)

            terminal = {StepState.SUCCEEDED, StepState.FAILED, StepState.SKIPPED}
            if all(s.state in terminal for s in plan.steps.values()):
                break
            if not in_flight:
                pending = [s.step_id for s in plan.steps.values()
                           if s.state not in terminal]
                if pending:
                    self.telemetry.emit("scheduler_stall", pending=pending)
                break
            await asyncio.sleep(0.05)

        if in_flight:
            await asyncio.gather(*in_flight, return_exceptions=True)
        return results
