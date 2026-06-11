# SPDX-License-Identifier: MIT
"""Live observability: poll-based StatusBoard + Markdown reports.

Safe to run from another terminal / process while the run is active —
the board reads only the ledger.
"""

from __future__ import annotations

import json
import sys
import time
from collections import deque
from dataclasses import dataclass, field

from .ledger import Ledger
from .models import TaskState

_GLYPH = {
    TaskState.PENDING: "·", TaskState.READY: "○",
    TaskState.RUNNING: "▶", TaskState.VERIFYING: "▣",
    TaskState.MERGING: "⇡", TaskState.DONE: "✓",
    TaskState.FAILED: "✗", TaskState.SKIPPED: "⤼",
    TaskState.CANCELLED: "⊘", TaskState.ESCALATED: "‼",
}

_TERMINAL = {TaskState.DONE, TaskState.FAILED, TaskState.SKIPPED,
             TaskState.CANCELLED, TaskState.ESCALATED}


@dataclass
class StatusBoard:
    def __init__(self, plan_id: str, ledger: Ledger | None = None,
                 interval: float = 1.0, stream=sys.stderr) -> None:
        self.plan_id = plan_id
        self.ledger = ledger or Ledger()
        self.interval = interval
        self.stream = stream
        self._lines_drawn = 0

    def _titles(self) -> dict:
        raw = self.ledger.load_plan_json(self.plan_id)
        if not raw:
            return {}
        try:
            data = json.loads(raw)
            return {t["id"]: t["title"] for t in data.get("tasks", [])}
        except Exception:
            return {}

    def _render_once(self, titles: dict) -> bool:
        states = self.ledger.task_states(self.plan_id)
        if not states:
            return False
        if self._lines_drawn and self.stream.isatty():
            self.stream.write(
                f"\x1b[{self._lines_drawn}F\x1b[J")
        lines = []
        for tid in sorted(states, key=lambda t: titles.get(t, t)):
            st = states[tid]
            title = titles.get(tid, tid)[:52]
            lines.append(f"  {_GLYPH.get(st, '?')} "
                         f"{st.value:<10} {title}")
        done = sum(1 for s in states.values() if s in _TERMINAL)
        lines.append(f"  {done}/{len(states)} terminal")
        self.stream.write("\n".join(lines) + "\n")
        self.stream.flush()
        self._lines_drawn = len(lines)
        return all(s in _TERMINAL for s in states.values())

    def watch(self, timeout: float = 7200) -> None:
        titles = self._titles()
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            if self._render_once(titles):
                return
            time.sleep(self.interval)


def report(plan_id: str, ledger: Ledger | None = None) -> str:
    """Markdown summary: for PR descriptions or the parent agent."""
    ledger = ledger or Ledger()
    states = ledger.task_states(plan_id)
    events = ledger.history(plan_id)
    raw = ledger.load_plan_json(plan_id)
    plan = json.loads(raw) if raw else {"goal": "?", "tasks": []}
    titles = {t["id"]: t["title"] for t in plan.get("tasks", [])}

    per_task: dict = {tid: {"attempts": 0, "seconds": 0.0,
                            "verdict": None, "error": ""}
                      for tid in states}
    for ev in events:
        tid = ev["task_id"]
        if tid not in per_task:
            continue
        if ev["kind"] == "attempt":
            per_task[tid]["attempts"] += 1
        elif ev["kind"] == "verdict":
            per_task[tid]["verdict"] = ev["payload"]
        elif (ev["kind"].startswith("state:")
              and ev["payload"].get("seconds")):
            per_task[tid]["seconds"] = ev["payload"]["seconds"]
            per_task[tid]["error"] = ev["payload"].get("error", "")

    merged = [t for t, s in states.items() if s == TaskState.DONE]
    escalated = [t for t, s in states.items()
                 if s == TaskState.ESCALATED]
    failed = [t for t, s in states.items()
              if s in (TaskState.FAILED, TaskState.SKIPPED)]

    out = [f"## Delegation Report — `{plan_id}`",
           f"**Goal:** {plan.get('goal', '?')}",
           "",
           f"| | count |", f"|---|---|",
           f"| merged | {len(merged)} |",
           f"| escalated | {len(escalated)} |",
           f"| failed/skipped | {len(failed)} |",
           ""]

    def section(name: str, ids: list) -> None:
        if not ids:
            return
        out.append(f"### {name}")
        for tid in ids:
            info = per_task[tid]
            line = (f"- **{titles.get(tid, tid)}** — "
                    f"{info['attempts']} attempt(s), "
                    f"{info['seconds']:.0f}s")
            if info["error"]:
                line += f" — `{info['error'][:120]}`"
            out.append(line)
            v = info["verdict"]
            if v:
                gates = ", ".join(
                    f"{g}:{'ok' if r['ok'] else 'FAIL'}"
                    for g, r in v.get("gates", {}).items())
                out.append(f"  - gates: {gates}")
        out.append("")

    section("Merged", merged)
    section("Escalated (needs human decision)", escalated)
    section("Failed / Skipped", failed)
    return "\n".join(out)
