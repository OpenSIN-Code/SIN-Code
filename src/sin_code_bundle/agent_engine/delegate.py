# SPDX-License-Identifier: MIT
"""Sub-Agent Delegation v2 — supervised, idempotent, deadline-safe recursion.

SOTA principles applied:
  Structured concurrency — every child is a lease with heartbeat; if the
                          parent scope dies, all children are cooperatively
                          cancelled (asyncio.CancelledError propagates
                          through AgentLoop -> Executor -> worktree cleanup).

  Global child limit     — depth alone does not stop fork-bombs (breadth!).
                          A process-wide semaphore caps the TOTAL number of
                          concurrent sub-agents across all depths and paths.

  Idempotency cache      — sub-goals are fingerprinted by (goal, steps, tree).
                          An identical sub-goal on the same workspace state
                          returns the cached result; repair loops never pay
                          twice for the same delegation.

  Adaptive budgets       — instead of a fixed 50% fraction, the allocator
                          learns from MemoryBridge: goal classes that
                          historically succeed fast get less; chronic
                          classes get more (clamped to parent remainder).

  Deadline propagation   — the parent's deadline is a hard wall-clock
                          point, not a relative timer. Children inherit
                          min(their budget, parent_remainder - safety_margin).
                          asyncio.timeout enforces this cooperatively.

  Result contract        — DelegateResult is validated against a strict
                          schema before it reaches the parent. A misbehaving
                          child cannot flood the parent context with garbage.
"""

from __future__ import annotations

import asyncio
import hashlib
import json
import subprocess
import time
import uuid
from dataclasses import dataclass, field
from typing import Any

from .builtin_tools import register_builtin_tools
from .memory_bridge import MemoryBridge
from .router import ToolRouter
from .telemetry import Telemetry
from .types import AgentTask
from .verifier import Verifier

# --------------------------------------------------------------- contracts

RESULT_SCHEMA: dict[str, Any] = {
    "outcome": str,
    "verdict": str,
    "elapsed_s": (int, float),
    "steps_ok": int,
    "steps_total": int,
    "lessons": list,
    "depth": int,
    "delegation_id": str,
    "cached": bool,
}


def validate_result(result: dict[str, Any]) -> dict[str, Any]:
    clean: dict[str, Any] = {}
    for key, expected in RESULT_SCHEMA.items():
        if key not in result:
            raise ValueError(f"delegate result missing key {key!r}")
        value = result[key]
        if not isinstance(value, expected):
            raise ValueError(
                f"delegate result key {key!r}: expected {expected}, "
                f"got {type(value).__name__}"
            )
        clean[key] = value
    clean["lessons"] = [str(x)[:300] for x in clean["lessons"][:5]]
    return clean


# ----------------------------------------------------------------- context

@dataclass(slots=True)
class DelegationContext:
    """Path-local inheritance context (depth, wall-clock deadline)."""

    depth: int = 0
    max_depth: int = 3
    deadline_wall: float = field(
        default_factory=lambda: time.monotonic() + 1800.0
    )
    safety_margin_s: float = 15.0
    min_budget_s: float = 60.0

    def remaining_s(self) -> float:
        return max(0.0, self.deadline_wall - time.monotonic())

    def can_delegate(self) -> tuple[bool, str]:
        if self.depth >= self.max_depth:
            return False, f"max delegation depth {self.max_depth} reached"
        if self.remaining_s() - self.safety_margin_s < self.min_budget_s:
            return False, (
                f"insufficient budget for sub-agent "
                f"({self.remaining_s():.0f}s remaining, "
                f"need >= {self.min_budget_s + self.safety_margin_s:.0f}s)"
            )
        return True, ""

    def child(self, granted_budget_s: float) -> "DelegationContext":
        budget = min(granted_budget_s,
                     self.remaining_s() - self.safety_margin_s)
        return DelegationContext(
            depth=self.depth + 1,
            max_depth=self.max_depth,
            deadline_wall=time.monotonic() + max(0.0, budget),
            safety_margin_s=self.safety_margin_s,
            min_budget_s=self.min_budget_s,
        )


# --------------------------------------------------------- budget allocator

class AdaptiveBudgetAllocator:
    """Learns per goal class (first word of goal, normalized) from the
    MemoryBridge how much budget sub-goals of that class really need.

    Heuristic: p75 of historical durations of successful runs * 1.5 buffer,
    clamped to [min_s, fraction * parent_remainder]. Without history: a
    fraction of the parent remainder (conservative default)."""

    def __init__(self, memory: MemoryBridge, *,
                 default_fraction: float = 0.5,
                 min_s: float = 60.0) -> None:
        self.memory = memory
        self.default_fraction = default_fraction
        self.min_s = min_s

    @staticmethod
    def goal_class(goal: str) -> str:
        first = (goal.split() or ["misc"])[0].lower()
        return "".join(c for c in first if c.isalnum()) or "misc"

    def grant(self, goal: str, parent_remaining_s: float) -> float:
        cap = parent_remaining_s * self.default_fraction
        hits = self.memory.recall_similar(goal, limit=10)
        durations = sorted(
            float(h.get("elapsed_s", 0.0)) for h in hits
            if h.get("outcome") == "success" and h.get("elapsed_s")
        )
        if len(durations) >= 3:
            p75 = durations[int(0.75 * (len(durations) - 1))]
            estimate = p75 * 1.5
            return max(self.min_s, min(estimate, cap))
        return max(self.min_s, cap)


# ------------------------------------------------------- idempotency cache

def _tree_hash(repo_root: str) -> str:
    if not repo_root or not __import__("pathlib").Path(repo_root).exists():
        return ""
    stash = subprocess.run(
        ["git", "-C", repo_root, "stash", "create"],
        capture_output=True, text=True, timeout=60,
    ).stdout.strip()
    if stash:
        return stash
    return subprocess.run(
        ["git", "-C", repo_root, "rev-parse", "HEAD^{tree}"],
        capture_output=True, text=True, timeout=60,
    ).stdout.strip()


class DelegationCache:
    """In-memory cache with TTL. Key = (goal, steps, workspace tree).
    A workspace change invalidates the key — never stale hits."""

    def __init__(self, ttl_s: float = 3600.0) -> None:
        self.ttl_s = ttl_s
        self._cache: dict[str, tuple[float, dict[str, Any]]] = {}

    @staticmethod
    def fingerprint(goal: str, steps: list[dict[str, Any]],
                    tree: str) -> str:
        raw = json.dumps({"g": goal, "s": steps, "t": tree}, sort_keys=True)
        return hashlib.sha256(raw.encode()).hexdigest()[:24]

    def get(self, key: str) -> dict[str, Any] | None:
        entry = self._cache.get(key)
        if entry is None:
            return None
        ts, result = entry
        if time.monotonic() - ts > self.ttl_s:
            del self._cache[key]
            return None
        return result

    def put(self, key: str, result: dict[str, Any]) -> None:
        if result.get("outcome") == "success":
            self._cache[key] = (time.monotonic(), result)


# -------------------------------------------------------------- supervisor

@dataclass(slots=True)
class _Lease:
    delegation_id: str
    goal: str
    depth: int
    started_at: float = field(default_factory=time.monotonic)
    last_heartbeat: float = field(default_factory=time.monotonic)
    task: asyncio.Task[Any] | None = None


class DelegationSupervisor:
    """Process-wide supervisor over ALL sub-agents.

    - global_limit: hard semaphore across all depths/paths (anti-breadth).
    - Leases with heartbeat: a child that doesn't pulse for longer than
      heartbeat_timeout_s is considered stalled and gets cancelled.
    - cancel_all(): structured teardown of the entire delegation tree.
    """

    def __init__(self, *, global_limit: int = 8,
                 heartbeat_timeout_s: float = 300.0,
                 telemetry: Telemetry | None = None) -> None:
        self._sem = asyncio.Semaphore(global_limit)
        self.global_limit = global_limit
        self.heartbeat_timeout_s = heartbeat_timeout_s
        self.telemetry = telemetry
        self._leases: dict[str, _Lease] = {}
        self._reaper: asyncio.Task[None] | None = None

    def heartbeat(self, delegation_id: str) -> None:
        lease = self._leases.get(delegation_id)
        if lease:
            lease.last_heartbeat = time.monotonic()

    def active(self) -> list[dict[str, Any]]:
        return [
            {"delegation_id": l.delegation_id, "goal": l.goal[:80],
             "depth": l.depth,
             "age_s": round(time.monotonic() - l.started_at, 1)}
            for l in self._leases.values()
        ]

    async def _reap_stalled(self) -> None:
        while self._leases:
            await asyncio.sleep(min(30.0, self.heartbeat_timeout_s / 4))
            now = time.monotonic()
            for lease in list(self._leases.values()):
                if now - lease.last_heartbeat > self.heartbeat_timeout_s:
                    if self.telemetry:
                        self.telemetry.emit(
                            "delegate_reaped",
                            delegation_id=lease.delegation_id,
                            goal=lease.goal[:80],
                            stalled_s=round(now - lease.last_heartbeat, 1),
                        )
                    if lease.task and not lease.task.done():
                        lease.task.cancel()
                    self._leases.pop(lease.delegation_id, None)
        self._reaper = None

    def cancel_all(self) -> int:
        n = 0
        for lease in list(self._leases.values()):
            if lease.task and not lease.task.done():
                lease.task.cancel()
                n += 1
        self._leases.clear()
        return n

    async def supervise(self, *, delegation_id: str, goal: str, depth: int,
                        coro) -> Any:
        async with self._sem:
            lease = _Lease(delegation_id=delegation_id, goal=goal,
                           depth=depth)
            task = asyncio.ensure_future(coro)
            lease.task = task
            self._leases[delegation_id] = lease
            if self._reaper is None or self._reaper.done():
                self._reaper = asyncio.ensure_future(self._reap_stalled())
            try:
                return await task
            finally:
                self._leases.pop(delegation_id, None)


# ----------------------------------------------------------- tool factory

def make_delegate_tool(
    parent_ctx: DelegationContext,
    telemetry: Telemetry,
    *,
    supervisor: DelegationSupervisor | None = None,
    cache: DelegationCache | None = None,
    allocator: AdaptiveBudgetAllocator | None = None,
    policy_wrap=None,
):
    supervisor = supervisor or DelegationSupervisor(telemetry=telemetry)
    cache = cache or DelegationCache()
    allocator = allocator or AdaptiveBudgetAllocator(MemoryBridge())

    async def sin_delegate(
        *,
        goal: str,
        steps: list[dict[str, Any]],
        cwd: str,
        constraints: list[str] | None = None,
        parallel: int = 2,
        repair_rounds: int = 2,
    ) -> dict[str, Any]:
        ok, reason = parent_ctx.can_delegate()
        if not ok:
            raise RuntimeError(f"delegation refused: {reason}")

        tree = await asyncio.to_thread(_tree_hash, cwd)
        key = DelegationCache.fingerprint(goal, steps, tree)
        cached = cache.get(key)
        if cached is not None:
            telemetry.emit("delegate_cache_hit", goal=goal[:120],
                           fingerprint=key)
            return {**cached, "cached": True}

        delegation_id = uuid.uuid4().hex[:10]
        granted = allocator.grant(goal, parent_ctx.remaining_s())
        child_ctx = parent_ctx.child(granted)
        telemetry.emit(
            "delegate_start", delegation_id=delegation_id,
            goal=goal[:120], depth=child_ctx.depth,
            granted_budget_s=round(child_ctx.remaining_s(), 1),
            goal_class=AdaptiveBudgetAllocator.goal_class(goal),
        )

        async def child_run() -> dict[str, Any]:
            child_router = register_builtin_tools(ToolRouter())
            if policy_wrap is not None:
                policy_wrap(child_router)
            child_router.register(
                "sin_delegate",
                make_delegate_tool(child_ctx, telemetry,
                                   supervisor=supervisor, cache=cache,
                                   allocator=allocator,
                                   policy_wrap=policy_wrap),
            )
            from .loop import AgentLoop
            child_loop = AgentLoop(
                child_router, Verifier(cwd, telemetry),
                telemetry=telemetry, memory=MemoryBridge(),
            )
            task = AgentTask(
                goal=goal, repo_root=cwd,
                constraints=list(constraints or []),
                max_parallelism=parallel,
                budget_seconds=child_ctx.remaining_s(),
                max_repair_rounds=repair_rounds,
            )
            supervisor.heartbeat(delegation_id)
            report = await child_loop.run(task, steps)
            supervisor.heartbeat(delegation_id)
            return report

        try:
            async with asyncio.timeout(child_ctx.remaining_s()
                                       + parent_ctx.safety_margin_s):
                report = await supervisor.supervise(
                    delegation_id=delegation_id, goal=goal,
                    depth=child_ctx.depth, coro=child_run(),
                )
        except (asyncio.TimeoutError, asyncio.CancelledError) as err:
            telemetry.emit("delegate_aborted", delegation_id=delegation_id,
                           kind=type(err).__name__)
            raise RuntimeError(
                f"sub-agent {delegation_id} aborted "
                f"({type(err).__name__}) — parent deadline protected"
            ) from err

        result = validate_result({
            "outcome": report["outcome"],
            "verdict": report["verdict"],
            "elapsed_s": report["elapsed_s"],
            "steps_ok": report["steps_ok"],
            "steps_total": report["steps_total"],
            "lessons": report["lessons"],
            "depth": child_ctx.depth,
            "delegation_id": delegation_id,
            "cached": False,
        })
        cache.put(key, result)
        telemetry.emit("delegate_done", delegation_id=delegation_id,
                       goal=goal[:120], depth=child_ctx.depth,
                       outcome=result["outcome"])
        return result

    return sin_delegate
