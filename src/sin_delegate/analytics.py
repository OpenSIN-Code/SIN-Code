# SPDX-License-Identifier: MIT
"""Cross-Run-Analytics: learns from the ledger which backends/models
work for which task classes. Wilson-scored, EMA-smoothed, no ML.
"""

from __future__ import annotations

import json
import math
from collections import defaultdict
from dataclasses import dataclass
from pathlib import PurePosixPath

from .ledger import Ledger
from .models import Task


def task_class(risk: str, files: list, verify: list) -> str:
    """Deterministic bucket, e.g. 'high:py:arch+diff+tests'."""
    exts = [PurePosixPath(f).suffix.lstrip(".")
            for f in files if PurePosixPath(f).suffix]
    dominant = (max(set(exts), key=exts.count) if exts else "any")
    vprofile = "+".join(sorted(
        {"tests": "tests", "architecture": "arch", "diff": "diff"}.get(v, v)
        for v in verify
        if v in ("tests", "architecture", "diff")
    )) or "none"
    return f"{risk}:{dominant}:{vprofile}"


def task_class_of(task) -> str:
    return task_class(task.risk.value, list(task.files_hint),
                      list(task.verify))


def wilson_lower(successes: int, trials: int, z: float = 1.96) -> float:
    """Lower bound of the Wilson confidence interval — robust for small n."""
    if trials == 0:
        return 0.0
    p = successes / trials
    denom = 1 + z * z / trials
    centre = p + z * z / (2 * trials)
    margin = z * math.sqrt(
        (p * (1 - p) + z * z / (4 * trials)) / trials)
    return max(0.0, (centre - margin) / denom)


@dataclass
class BackendStats:
    backend: str
    model: str
    task_class: str
    trials: int = 0
    passes: int = 0
    ema_seconds: float = 0.0
    ema_attempts: float = 0.0

    EMA_ALPHA = 0.3

    @property
    def score(self) -> float:
        """Wilson lower bound, with a small speed penalty."""
        base = wilson_lower(self.passes, self.trials)
        speed_penalty = min(self.ema_seconds / 3600.0, 0.15)
        return base - speed_penalty

    def observe(self, passed: bool, seconds: float, attempts: int) -> None:
        self.trials += 1
        self.passes += int(passed)
        a = self.EMA_ALPHA
        if self.trials == 1:
            self.ema_seconds = seconds
            self.ema_attempts = attempts
        else:
            self.ema_seconds = a * seconds + (1 - a) * self.ema_seconds
            self.ema_attempts = a * attempts + (1 - a) * self.ema_attempts


class Analytics:
    """Folds ALL ledger runs into BackendStats tables."""

    def __init__(self, ledger: Ledger | None = None) -> None:
        self.ledger = ledger or Ledger()
        self._stats: dict[tuple[str, str, str], BackendStats] = {}
        self._fold()

    def _fold(self) -> None:
        for run in self.ledger.list_runs(limit=1000):
            plan_id = run["plan_id"]
            raw = self.ledger.load_plan_json(plan_id)
            if not raw:
                continue
            try:
                plan = json.loads(raw)
            except json.JSONDecodeError:
                continue
            meta = {t["id"]: t for t in plan.get("tasks", [])}
            self._fold_run(plan_id, meta)

    def _fold_run(self, plan_id: str, meta: dict) -> None:
        per_task: dict = defaultdict(
            lambda: {"passed": None, "seconds": 0.0, "attempts": 0})
        for ev in self.ledger.history(plan_id):
            tid = ev["task_id"]
            if tid not in meta:
                continue
            rec = per_task[tid]
            if ev["kind"] == "attempt":
                rec["attempts"] += 1
            elif ev["kind"] == "verdict":
                rec["passed"] = bool(ev["payload"].get("passed"))
            elif (ev["kind"].startswith("state:")
                  and ev["payload"].get("seconds")):
                rec["seconds"] = float(ev["payload"]["seconds"])
        for tid, rec in per_task.items():
            if rec["passed"] is None:
                continue
            m = meta[tid]
            cls = task_class(m.get("risk", "medium"),
                             m.get("files_hint", m.get("files", [])),
                             m.get("verify", []))
            key = (m.get("backend", "opencode"),
                   m.get("model", ""), cls)
            st = self._stats.setdefault(
                key, BackendStats(key[0], key[1], cls))
            st.observe(rec["passed"], rec["seconds"],
                       max(rec["attempts"], 1))

    def best_backend(self, cls: str,
                     candidates: list | None = None,
                     min_trials: int = 3
                     ) -> tuple[str, str] | None:
        rows = [s for (b, m, c), s in self._stats.items()
                if c == cls and s.trials >= min_trials
                and (candidates is None or (b, m) in candidates)]
        if not rows:
            return None
        best = max(rows, key=lambda s: s.score)
        return (best.backend, best.model)

    def expected_seconds(self, cls: str, default: float = 600.0) -> float:
        rows = [s for (_, _, c), s in self._stats.items() if c == cls]
        if not rows:
            return default
        total = sum(s.trials for s in rows)
        if total == 0:
            return default
        return sum(s.ema_seconds * s.trials for s in rows) / total

    def table(self) -> list[dict]:
        return sorted(
            ({"backend": s.backend,
              "model": s.model or "(default)",
              "task_class": s.task_class,
              "trials": s.trials,
              "pass_rate": (round(s.passes / s.trials, 2)
                            if s.trials else 0),
              "wilson_score": round(s.score, 3),
              "ema_seconds": round(s.ema_seconds, 1),
              "ema_attempts": round(s.ema_attempts, 1)}
             for s in self._stats.values()),
            key=lambda r: (r["task_class"], -r["wilson_score"]))
