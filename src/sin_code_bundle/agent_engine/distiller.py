# SPDX-License-Identifier: MIT
"""Knowledge-Distiller — Lessons + Insights -> Standing-Constraints.

Closes the self-improvement loop: lessons from individual runs
(MemoryBridge) and systemic insights (TelemetryAnalyzer) are raw
material. The distiller periodically condenses them into a small,
curated rule set (Standing-Constraints) that the PlanSynthesizer
injects into every new plan.

Rule lifecycle (evidence-based, never eternal):
    candidate  — distilled from >= min_evidence similar lessons
    active     — promoted by an LLM pass (or heuristically when --no-llm)
                 to a precise, general, actionable rule
    retired    — decay: if a rule isn't supported by new evidence over N
                 runs, its score drops; below the threshold it is deleted.
                 No rule-graveyard.

Deduplication uses a normalized signature (verdict-kind + keywords),
capped at max_active rules — the constraint budget in the prompt stays
constant, regardless of how much history accumulates.
"""

from __future__ import annotations

import json
import re
import sqlite3
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Awaitable, Callable

CompleteFn = Callable[[str], Awaitable[str]]

_SCHEMA = """
CREATE TABLE IF NOT EXISTS standing_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    signature TEXT NOT NULL UNIQUE,
    rule TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'candidate',
    evidence_count INTEGER NOT NULL DEFAULT 1,
    score REAL NOT NULL DEFAULT 1.0,
    created_ts REAL NOT NULL,
    last_evidence_ts REAL NOT NULL
);
"""

_STOPWORDS = frozenset(
    "the a an of to in for and or with on is are was were be been round "
    "this that it its from at by as not no".split()
)

_KIND_RX = re.compile(r"fail_[a-z]+")

_DISTILL_PROMPT = """\
You are a knowledge distiller for a coding agent. Below are raw failure
lessons that share a pattern. Write ONE precise, general, actionable rule
the agent should always follow to avoid this failure class.

Requirements:
- One sentence, imperative mood, max 160 characters.
- General (no file names, no project-specific identifiers).
- Output the rule text only — no quotes, no prose, no numbering.

LESSONS:
{lessons}
"""


def _signature(lesson: str) -> str:
    """Normalized signature: verdict-kind + top keywords."""
    match = _KIND_RX.search(lesson)
    kind_str = match.group(0) if match else "generic"
    words = sorted({
        w for w in re.findall(r"[a-z]{4,}", lesson.lower())
        if w not in _STOPWORDS and not w.startswith("fail")
    })[:4]
    return f"{kind_str}:{'-'.join(words)}"


def _heuristic_rule(lessons: list[str]) -> str:
    """Fallback without LLM: known classes get canonical rules."""
    joined = " ".join(lessons).lower()
    if "fail_lint" in joined:
        return ("Run the lint autofix step before the verification step "
                "in every plan.")
    if "fail_tests" in joined:
        return ("Read the affected test file before editing the code "
                "under test.")
    if "fail_semantic" in joined and "delet" in joined:
        return ("Keep diffs small; split large changes into delegated "
                "sub-tasks instead of bulk deletions.")
    if "fail_architecture" in joined:
        return ("Check architecture rules before editing module "
                "boundaries or imports.")
    return f"Avoid repeating: {lessons[0][:140]}"


@dataclass(slots=True)
class StandingRule:
    signature: str
    rule: str
    state: str
    evidence_count: int
    score: float


class KnowledgeDistiller:
    def __init__(self, db_path: str | None = None, *,
                 complete: CompleteFn | None = None,
                 min_evidence: int = 3,
                 max_active: int = 12,
                 decay: float = 0.85,
                 retire_below: float = 0.3) -> None:
        self.db_path = Path(
            db_path or Path.home() / ".sin" / "agent-memory.db")
        self.db_path.parent.mkdir(parents=True, exist_ok=True)
        self.complete = complete
        self.min_evidence = min_evidence
        self.max_active = max_active
        self.decay = decay
        self.retire_below = retire_below
        with self._conn() as con:
            con.executescript(_SCHEMA)

    def _conn(self) -> sqlite3.Connection:
        con = sqlite3.connect(self.db_path, timeout=10)
        con.row_factory = sqlite3.Row
        return con

    async def distill(self, raw_lessons: list[str]) -> dict[str, Any]:
        now = time.time()
        clusters: dict[str, list[str]] = {}
        for lesson in raw_lessons:
            clusters.setdefault(_signature(lesson), []).append(lesson)

        promoted, reinforced = [], []
        with self._conn() as con:
            con.execute("UPDATE standing_rules SET score = score * ?",
                        (self.decay,))

            for sig, lessons in clusters.items():
                row = con.execute(
                    "SELECT * FROM standing_rules WHERE signature = ?",
                    (sig,),
                ).fetchone()
                if row is None:
                    initial_state = ('active' if len(lessons)
                                     >= self.min_evidence else 'candidate')
                    con.execute(
                        "INSERT INTO standing_rules (signature, rule, "
                        "state, evidence_count, score, created_ts, "
                        "last_evidence_ts) VALUES (?, ?, ?, ?, 1.0, ?, ?)",
                        (sig, _heuristic_rule(lessons), initial_state,
                         len(lessons), now, now),
                    )
                    if initial_state == 'active':
                        promoted.append(sig)
                    continue
                new_count = row["evidence_count"] + len(lessons)
                con.execute(
                    "UPDATE standing_rules SET evidence_count = ?, "
                    "score = score + ?, last_evidence_ts = ? "
                    "WHERE signature = ?",
                    (new_count, float(len(lessons)), now, sig),
                )
                reinforced.append(sig)

                if (row["state"] == "candidate"
                        and new_count >= self.min_evidence):
                    rule_text = _heuristic_rule(lessons)
                    if self.complete is not None:
                        try:
                            raw = await self.complete(_DISTILL_PROMPT.format(
                                lessons="\n".join(
                                    f"- {l}" for l in lessons[:8]),
                            ))
                            candidate = raw.strip().splitlines()[0][:160]
                            if 20 <= len(candidate) <= 160:
                                rule_text = candidate
                        except Exception:
                            pass
                    con.execute(
                        "UPDATE standing_rules SET state = 'active', "
                        "rule = ? WHERE signature = ?",
                        (rule_text, sig),
                    )
                    promoted.append(sig)

            retired_rows = con.execute(
                "SELECT signature FROM standing_rules WHERE score < ?",
                (self.retire_below,),
            ).fetchall()
            con.execute("DELETE FROM standing_rules WHERE score < ?",
                        (self.retire_below,))
            con.execute(
                "DELETE FROM standing_rules WHERE state = 'active' AND id "
                "NOT IN (SELECT id FROM standing_rules "
                "WHERE state='active' ORDER BY score DESC LIMIT ?)",
                (self.max_active,),
            )

        return {
            "clusters": len(clusters),
            "promoted": promoted,
            "reinforced": reinforced,
            "retired": [r["signature"] for r in retired_rows],
        }

    def active_rules(self) -> list[StandingRule]:
        with self._conn() as con:
            rows = con.execute(
                "SELECT * FROM standing_rules WHERE state = 'active' "
                "ORDER BY score DESC LIMIT ?",
                (self.max_active,),
            ).fetchall()
        return [StandingRule(
            signature=r["signature"], rule=r["rule"], state=r["state"],
            evidence_count=r["evidence_count"], score=r["score"],
        ) for r in rows]

    def render_constraints(self, *, max_chars: int = 1000) -> str:
        """Prompt-Block for the PlanSynthesizer."""
        rules = self.active_rules()
        if not rules:
            return ""
        lines = [f"- {r.rule}" for r in rules]
        return ("STANDING RULES (distilled from past failures — obey):\n"
                + "\n".join(lines))[:max_chars]

    def harvest_lessons(self, *, since_s: float = 7 * 86400) -> list[str]:
        """Raw lessons of the last period from agent_runs."""
        cutoff = time.time() - since_s
        with self._conn() as con:
            rows = con.execute(
                "SELECT lessons FROM agent_runs WHERE ts > ? "
                "AND lessons != '[]'",
                (cutoff,),
            ).fetchall()
        out: list[str] = []
        for r in rows:
            try:
                out.extend(str(x) for x in json.loads(r["lessons"]))
            except json.JSONDecodeError:
                continue
        return out
