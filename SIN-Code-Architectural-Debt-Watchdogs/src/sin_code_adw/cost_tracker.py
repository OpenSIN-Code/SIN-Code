"""Trackt API-Kosten von Agent-Runs."""

from __future__ import annotations

import json
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from pathlib import Path

# Grobe Pricing (USD pro 1K tokens) - Stand 2026
PRICING = {
    "gpt-4o": {"in": 0.0025, "out": 0.01},
    "o3": {"in": 0.002, "out": 0.008},
    "claude-4-opus": {"in": 0.015, "out": 0.075},
    "claude-3.5-sonnet": {"in": 0.003, "out": 0.015},
    "gemini-2.5-pro": {"in": 0.00125, "out": 0.005},
}

DEFAULT_PRICE = {"in": 0.005, "out": 0.015}


@dataclass
class CostEntry:
    timestamp: str
    model: str
    prompt_tokens: int
    completion_tokens: int
    cost_usd: float
    agent_id: str
    task: str


class CostTracker:
    """Persistente Kosten-Verfolgung pro Agent-Task (JSONL)."""

    def __init__(self, log_path: str = ".sin/costs.jsonl"):
        self.log_path = Path(log_path)
        self.log_path.parent.mkdir(parents=True, exist_ok=True)

    def record(
        self,
        model: str,
        prompt_tokens: int,
        completion_tokens: int,
        agent_id: str = "default",
        task: str = "",
    ) -> CostEntry:
        price = PRICING.get(model, DEFAULT_PRICE)
        cost = (prompt_tokens / 1000) * price["in"] + (completion_tokens / 1000) * price["out"]
        entry = CostEntry(
            timestamp=datetime.now(timezone.utc).isoformat(),
            model=model,
            prompt_tokens=prompt_tokens,
            completion_tokens=completion_tokens,
            cost_usd=round(cost, 6),
            agent_id=agent_id,
            task=task,
        )
        with open(self.log_path, "a", encoding="utf-8") as fh:
            fh.write(json.dumps(asdict(entry)) + "\n")
        return entry

    def total_for(self, agent_id: str | None = None, task: str | None = None) -> dict:
        total = 0.0
        entries = 0
        if not self.log_path.exists():
            return {"total_usd": 0.0, "entries": 0}
        with open(self.log_path, encoding="utf-8") as fh:
            for line in fh:
                line = line.strip()
                if not line:
                    continue
                try:
                    rec = json.loads(line)
                except json.JSONDecodeError:
                    continue
                if agent_id and rec.get("agent_id") != agent_id:
                    continue
                if task and rec.get("task") != task:
                    continue
                total += rec.get("cost_usd", 0.0)
                entries += 1
        return {"total_usd": round(total, 4), "entries": entries}
