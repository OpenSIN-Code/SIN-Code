# SPDX-License-Identifier: MIT
"""Live dashboard — JSONL telemetry tail with ANSI rendering (no curses)."""

from __future__ import annotations

import json
import os
import sys
import time
from collections import deque
from dataclasses import dataclass, field
from pathlib import Path

_CLEAR = "\x1b[2J\x1b[H"
_DIM = "\x1b[2m"
_BOLD = "\x1b[1m"
_GREEN = "\x1b[32m"
_RED = "\x1b[31m"
_YELLOW = "\x1b[33m"
_CYAN = "\x1b[36m"
_RESET = "\x1b[0m"

_STATE_STYLE = {
    "running": (_CYAN, "RUN "),
    "ok": (_GREEN, "OK  "),
    "fail": (_RED, "FAIL"),
    "retry": (_YELLOW, "RTRY"),
    "skip": (_DIM, "SKIP"),
}


@dataclass(slots=True)
class _StepView:
    step_id: str
    tool: str = "?"
    state: str = "running"
    attempts: int = 1
    started: float = field(default_factory=time.monotonic)
    duration_s: float | None = None


@dataclass
class DashboardState:
    steps: dict[str, _StepView] = field(default_factory=dict)
    verdicts: deque[str] = field(default_factory=lambda: deque(maxlen=6))
    swarm: dict[str, str] = field(default_factory=dict)
    events: int = 0
    task_id: str = "?"

    def apply(self, rec: dict) -> None:
        self.events += 1
        ev = rec.get("event", "")
        sid = rec.get("step_id", "")
        if ev == "plan_built":
            self.task_id = rec.get("task_id", "?")
        elif ev == "step_start":
            self.steps[sid] = _StepView(
                step_id=sid, tool=rec.get("tool", "?"),
                attempts=int(rec.get("attempt", 1)),
            )
        elif ev == "step_ok" and sid in self.steps:
            v = self.steps[sid]
            v.state, v.duration_s = "ok", rec.get("duration_s")
        elif ev == "step_retry" and sid in self.steps:
            self.steps[sid].state = "retry"
        elif ev == "step_fail" and sid in self.steps:
            self.steps[sid].state = "fail"
            for skipped in rec.get("skipped", []):
                self.steps.setdefault(
                    skipped, _StepView(step_id=skipped)
                ).state = "skip"
        elif ev == "verdict":
            mark = "PASS" if rec.get("ok") else str(rec.get("kind", "?")).upper()
            self.verdicts.append(f"round {rec.get('round', '?')}: {mark}")
        elif ev == "swarm_member_done":
            member = rec.get("member", "?")
            self.swarm[member] = "ok" if rec.get("ok") else "fail"
        elif ev == "swarm_merged":
            self.swarm[rec.get("member", "?")] = "merged"
        elif ev == "swarm_merge_conflict":
            self.swarm[rec.get("member", "?")] = "conflict"
        elif ev == "swarm_merge_reverted":
            self.swarm[rec.get("member", "?")] = "reverted"

    def render(self, *, color: bool = True) -> str:
        def style(code: str, text: str) -> str:
            return f"{code}{text}{_RESET}" if color else text

        lines = [
            style(_BOLD, f"sin agent watch  task={self.task_id}  "
                         f"events={self.events}"),
            "",
            style(_BOLD, f"{'STATE':<6}{'STEP':<28}{'TOOL':<14}"
                         f"{'TRY':<5}{'TIME':<8}"),
        ]
        for v in list(self.steps.values())[-30:]:
            code, label = _STATE_STYLE.get(v.state, (_DIM, v.state[:4]))
            dur = (v.duration_s if v.duration_s is not None
                   else time.monotonic() - v.started)
            lines.append(
                style(code, f"{label:<6}")
                + f"{v.step_id[:26]:<28}{v.tool[:12]:<14}"
                + f"{v.attempts:<5}{dur:>6.1f}s"
            )
        if self.verdicts:
            lines += ["", style(_BOLD, "VERDICTS")]
            lines += [f"  {v}" for v in self.verdicts]
        if self.swarm:
            lines += ["", style(_BOLD, "SWARM")]
            for member, status in self.swarm.items():
                code = {"ok": _GREEN, "merged": _GREEN, "fail": _RED,
                        "conflict": _YELLOW, "reverted": _YELLOW}.get(
                    status, _CYAN)
                lines.append(f"  {member:<20}" + style(code, status))
        return "\n".join(lines)


def watch(log_path: str | None = None, *, refresh_s: float = 0.5,
          once: bool = False) -> None:
    path = Path(log_path or os.environ.get("SIN_AGENT_LOG", "")
                or Path.home() / ".sin" / "agent-events.jsonl")
    state = DashboardState()
    is_tty = sys.stdout.isatty()
    pos = 0
    try:
        while True:
            if path.exists():
                with path.open("r", encoding="utf-8") as fh:
                    fh.seek(pos)
                    for line in fh:
                        try:
                            state.apply(json.loads(line))
                        except json.JSONDecodeError:
                            continue
                    pos = fh.tell()
            if is_tty:
                sys.stdout.write(_CLEAR + state.render(color=True) + "\n")
            else:
                sys.stdout.write(state.render(color=False) + "\n---\n")
            sys.stdout.flush()
            if once:
                return
            time.sleep(refresh_s)
    except KeyboardInterrupt:
        sys.stdout.write("\n")
