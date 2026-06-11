# SPDX-License-Identifier: MIT
"""Telemetry Analyzer — distills event history into prioritized insights."""

from __future__ import annotations

import json
import os
import re
from collections import Counter
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

_STEP_PREFIX = re.compile(r"^([a-zA-Z]+)")


@dataclass(slots=True)
class Insight:
    severity: str
    category: str
    finding: str
    recommendation: str
    evidence: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        return {
            "severity": self.severity,
            "category": self.category,
            "finding": self.finding,
            "recommendation": self.recommendation,
            "evidence": self.evidence,
        }


class TelemetryAnalyzer:
    def __init__(self, log_path: str | None = None) -> None:
        self.log_path = Path(
            log_path or os.environ.get("SIN_AGENT_LOG", "")
            or Path.home() / ".sin" / "agent-events.jsonl"
        )

    def _events(self) -> list[dict[str, Any]]:
        if not self.log_path.exists():
            return []
        events: list[dict[str, Any]] = []
        for line in self.log_path.read_text(encoding="utf-8").splitlines():
            try:
                events.append(json.loads(line))
            except json.JSONDecodeError:
                continue
        return events

    def analyze(self) -> list[Insight]:
        events = self._events()
        if not events:
            return [Insight("info", "general", "no telemetry recorded yet",
                            "run some agent tasks first")]
        insights: list[Insight] = []
        insights += self._tool_health(events)
        insights += self._repair_hotspots(events)
        insights += self._step_patterns(events)
        insights += self._stalls(events)
        insights += self._delegation(events)
        order = {"critical": 0, "warn": 1, "info": 2}
        insights.sort(key=lambda i: order[i.severity])
        return insights

    def _tool_health(self, events: list[dict]) -> list[Insight]:
        starts: Counter[str] = Counter()
        fails: Counter[str] = Counter()
        retries: Counter[str] = Counter()
        tool_of_step: dict[str, str] = {}
        for e in events:
            ev = e.get("event")
            if ev == "step_start":
                tool = e.get("tool", "?")
                starts[tool] += 1
                tool_of_step[e.get("step_id", "")] = tool
            elif ev == "step_fail":
                fails[tool_of_step.get(e.get("step_id", ""), "?")] += 1
            elif ev == "step_retry":
                retries[tool_of_step.get(e.get("step_id", ""), "?")] += 1

        out: list[Insight] = []
        for tool, n in starts.items():
            if n < 5:
                continue
            fail_rate = fails[tool] / n
            retry_rate = retries[tool] / n
            if fail_rate > 0.3:
                out.append(Insight(
                    "critical", "tool_health",
                    f"tool {tool!r} fails {fail_rate:.0%} of the time "
                    f"({fails[tool]}/{n})",
                    f"inspect {tool!r} arguments in failing steps; consider "
                    "a lower failure_threshold so its circuit opens earlier",
                    {"starts": n, "fails": fails[tool],
                     "fail_rate": round(fail_rate, 2)},
                ))
            elif retry_rate > 0.5:
                out.append(Insight(
                    "warn", "tool_health",
                    f"tool {tool!r} retries {retry_rate:.0%} of calls",
                    "raise base_delay_s for this tool or serialize its "
                    "steps via dependency edges",
                    {"starts": n, "retries": retries[tool]},
                ))
        return out

    def _repair_hotspots(self, events: list[dict]) -> list[Insight]:
        kinds = Counter(
            e.get("kind", "?") for e in events
            if e.get("event") == "verdict" and not e.get("ok")
        )
        total = sum(kinds.values())
        out: list[Insight] = []
        if not total:
            return out
        top, n = kinds.most_common(1)[0]
        share = n / total
        if share > 0.5 and total >= 4:
            fix = {
                "fail_lint": "add a 'ruff check . --fix' step BEFORE the "
                             "verification step in every plan",
                "fail_tests": "plans skip exploration — enforce sin_read of "
                              "the test file before each edit step",
                "fail_architecture": "feed ADW rules into the synthesizer "
                                     "prompt as hard constraints",
                "fail_semantic": "plans produce oversized diffs — split "
                                 "goals into smaller delegated sub-tasks",
            }.get(top, "investigate this failure class manually")
            out.append(Insight(
                "warn", "repair_hotspots",
                f"{share:.0%} of all verification failures are {top!r} "
                f"({n}/{total})",
                fix, {"distribution": dict(kinds)},
            ))
        return out

    def _step_patterns(self, events: list[dict]) -> list[Insight]:
        starts: Counter[str] = Counter()
        fails: Counter[str] = Counter()
        for e in events:
            sid = e.get("step_id", "")
            m = _STEP_PREFIX.match(sid)
            if not m:
                continue
            prefix = m.group(1).lower()
            if e.get("event") == "step_start":
                starts[prefix] += 1
            elif e.get("event") == "step_fail":
                fails[prefix] += 1
        out: list[Insight] = []
        for prefix, n in starts.items():
            if n >= 8 and fails[prefix] / n > 0.4:
                out.append(Insight(
                    "warn", "step_patterns",
                    f"steps prefixed {prefix!r}* fail "
                    f"{fails[prefix] / n:.0%} of the time",
                    f"review how {prefix!r} steps are planned — they likely "
                    "need finer-grained exploration dependencies",
                    {"starts": n, "fails": fails[prefix]},
                ))
        return out

    def _stalls(self, events: list[dict]) -> list[Insight]:
        stalls = sum(1 for e in events
                     if e.get("event") == "scheduler_stall")
        exhausted = sum(1 for e in events
                        if e.get("event") == "budget_exhausted")
        runs = max(1, sum(1 for e in events
                          if e.get("event") == "run_complete"))
        out: list[Insight] = []
        if exhausted / runs > 0.25:
            out.append(Insight(
                "critical", "stalls",
                f"{exhausted}/{runs} runs exhausted their budget",
                "raise --budget, or shrink plans via sin_delegate so "
                "long-running test steps get isolated child budgets",
                {"budget_exhausted": exhausted, "runs": runs},
            ))
        if stalls:
            out.append(Insight(
                "warn", "stalls",
                f"{stalls} scheduler stalls (steps pending, none ready)",
                "check plans for dependency chains on steps that can fail "
                "permanently — add fallback paths or reduce fan-in",
                {"stalls": stalls},
            ))
        return out

    def _delegation(self, events: list[dict]) -> list[Insight]:
        done = [e for e in events if e.get("event") == "delegate_done"]
        if len(done) < 3:
            return []
        ok = sum(1 for e in done if e.get("outcome") == "success")
        rate = ok / len(done)
        if rate < 0.5:
            return [Insight(
                "warn", "delegation",
                f"sub-agents succeed only {rate:.0%} of the time "
                f"({ok}/{len(done)})",
                "child budgets may be too small — raise budget_fraction or "
                "delegate smaller goals",
                {"delegations": len(done), "successes": ok},
            )]
        return [Insight(
            "info", "delegation",
            f"sub-agents succeed {rate:.0%} of the time — delegation pays off",
            "consider delegating more long-running verification work",
            {"delegations": len(done), "successes": ok},
        )]

    def render_for_prompt(self, insights: list[Insight],
                          *, max_chars: int = 1200) -> str:
        lines = [
            f"[{i.severity}] {i.finding} => {i.recommendation}"
            for i in insights if i.severity != "info"
        ]
        return "\n".join(lines)[:max_chars] or "no systemic issues detected"
