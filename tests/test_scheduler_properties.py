# SPDX-License-Identifier: MIT
"""Property-based scheduler tests over randomized DAGs.

Tests the six fundamental invariants for ANY DAG:
  I1  DEPENDENCY ORDER     a task never starts before its deps are terminal
  I2  COMPLETENESS          every task reaches a terminal state
  I3  FAILURE PROPAGATION   downstream of FAILED is SKIPPED, never DONE
  I4  PARALLELISM BOUND     never more than max_parallel concurrent
  I5  RESUME IDEMPOTENCE    re-run skips DONE tasks (full + partial resume)
  I6  PRIORITY SANITY       critical_path_priority is well-defined
"""

from __future__ import annotations

import asyncio
import random

from sin_delegate.ledger import Ledger
from sin_delegate.models import (Budget, Plan, Task, TaskOutcome, TaskState)
from sin_delegate.scheduler import (Scheduler, critical_path_priority)

TERMINAL = {TaskState.DONE, TaskState.FAILED, TaskState.SKIPPED,
            TaskState.CANCELLED, TaskState.ESCALATED}


def random_dag(rng: random.Random, n_tasks: int) -> Plan:
    """Acyclic by construction: edges only i -> j with i < j."""
    tasks: list = []
    ids = [f"T{i:02d}" for i in range(n_tasks)]
    for j in range(n_tasks):
        possible = ids[:j]
        k = rng.randint(0, min(3, len(possible)))
        deps = tuple(rng.sample(possible, k)) if k else ()
        tasks.append(Task(
            title=f"task {j}", instructions=f"do {j}", id=ids[j],
            deps=deps, budget=Budget(max_seconds=5, max_retries=0,
                                     backoff_base=1.0)))
    return Plan(goal="property test", tasks=tuple(tasks), repo=".")


class Probe:
    def __init__(self, rng: random.Random, fail_rate: float = 0.0) -> None:
        self.rng = rng
        self.fail_rate = fail_rate
        self.started_after: dict = {}
        self.executed: list = []
        self.concurrent = 0
        self.max_concurrent = 0
        self._terminal: set = set()

    async def __call__(self, task: Task) -> TaskOutcome:
        self.started_after[task.id] = set(self._terminal)
        self.executed.append(task.id)
        self.concurrent += 1
        self.max_concurrent = max(self.max_concurrent, self.concurrent)
        await asyncio.sleep(self.rng.uniform(0, 0.003))
        self.concurrent -= 1
        failed = self.rng.random() < self.fail_rate
        self._terminal.add(task.id)
        if failed:
            return TaskOutcome(task.id, TaskState.FAILED,
                               error="random failure")
        return TaskOutcome(task.id, TaskState.DONE)


def _run(plan: Plan, ledger: Ledger, probe: Probe,
         max_parallel: int = 4) -> dict:
    sched = Scheduler(plan, ledger, probe, max_parallel=max_parallel)
    return asyncio.run(sched.run())


def test_property_dependency_order_and_completeness(tmp_path):
    for seed in range(25):
        rng = random.Random(seed)
        plan = random_dag(rng, rng.randint(3, 15))
        probe = Probe(rng, fail_rate=0.0)
        outcomes = _run(plan, Ledger(tmp_path / f"l{seed}.db"), probe)
        assert set(outcomes) == {t.id for t in plan.tasks}, f"seed={seed}"
        for tid, o in outcomes.items():
            assert o.state in TERMINAL, f"seed={seed} task={tid} {o.state}"
        deps_of = {t.id: set(t.deps) for t in plan.tasks}
        for tid, terminal_at_start in probe.started_after.items():
            missing = deps_of[tid] - terminal_at_start
            assert not missing, (
                f"seed={seed}: {tid} started before deps {missing}")


def test_property_failure_propagation(tmp_path):
    for seed in range(100, 125):
        rng = random.Random(seed)
        plan = random_dag(rng, rng.randint(4, 12))
        probe = Probe(rng, fail_rate=0.4)
        outcomes = _run(plan, Ledger(tmp_path / f"l{seed}.db"), probe)
        bad = {tid for tid, o in outcomes.items()
               if o.state in (TaskState.FAILED, TaskState.SKIPPED,
                              TaskState.CANCELLED)}
        deps_of = {t.id: set(t.deps) for t in plan.tasks}
        for tid, o in outcomes.items():
            if deps_of[tid] & bad:
                assert o.state != TaskState.DONE, (
                    f"seed={seed}: {tid} DONE despite failed upstream")


def test_property_parallelism_bound(tmp_path):
    for seed in range(200, 215):
        rng = random.Random(seed)
        plan = random_dag(rng, 12)
        limit = rng.randint(1, 4)
        probe = Probe(rng)
        _run(plan, Ledger(tmp_path / f"l{seed}.db"), probe,
             max_parallel=limit)
        assert probe.max_concurrent <= limit, (
            f"seed={seed}: {probe.max_concurrent} > limit {limit}")


def test_property_resume_idempotence(tmp_path):
    for seed in range(300, 315):
        rng = random.Random(seed)
        plan = random_dag(rng, rng.randint(3, 10))
        ledger = Ledger(tmp_path / f"l{seed}.db")
        ledger.register_run(plan.id, plan.goal, "{}")
        probe1 = Probe(random.Random(seed), fail_rate=0.0)
        out1 = _run(plan, ledger, probe1)
        assert all(o.state == TaskState.DONE for o in out1.values())
        probe2 = Probe(random.Random(seed + 1))
        out2 = _run(plan, ledger, probe2)
        assert probe2.executed == [], (
            f"seed={seed}: resume re-executed {probe2.executed}")
        assert all(o.state == TaskState.DONE for o in out2.values())


def test_property_partial_resume_runs_only_unfinished(tmp_path):
    for seed in range(400, 412):
        rng = random.Random(seed)
        plan = random_dag(rng, rng.randint(4, 10))
        ledger = Ledger(tmp_path / f"l{seed}.db")
        ledger.register_run(plan.id, plan.goal, "{}")
        done: set = set()
        for t in plan.tasks:  # topo order (index = topo for this generator)
            if set(t.deps) <= done and rng.random() < 0.5:
                done.add(t.id)
                ledger.emit(plan.id, t.id, "state:done")
        probe = Probe(random.Random(seed), fail_rate=0.0)
        out = _run(plan, ledger, probe)
        assert set(probe.executed) == {t.id for t in plan.tasks} - done, (
            f"seed={seed}")
        assert all(o.state == TaskState.DONE for o in out.values())


def test_property_critical_path_dominates_children(tmp_path):
    for seed in range(500, 520):
        rng = random.Random(seed)
        plan = random_dag(rng, rng.randint(3, 20))
        prio = critical_path_priority(plan)
        for t in plan.tasks:
            for d in t.deps:
                assert prio[d] > prio[t.id], (
                    f"seed={seed}: prio({d})={prio[d]} <= "
                    f"prio({t.id})={prio[t.id]}")
