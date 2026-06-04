"""Berechnet Risiko-Scores fuer Changes."""
from __future__ import annotations

from .ast_diff import Change


class RiskScorer:
    WEIGHT = {
        "signature_changed": 3.0,
        "removed": 2.0,
        "decorators_changed": 2.0,
        "body_changed": 1.0,
        "docstring_changed": 0.2,
        "added": 0.5,
    }

    def score(self, changes: list[Change]) -> dict:
        total = sum(self.WEIGHT.get(c.change_type, 0) for c in changes)
        if total < 1:
            risk = "low"
        elif total < 5:
            risk = "medium"
        else:
            risk = "high"
        breakdown: dict[str, int] = {}
        for c in changes:
            breakdown[c.change_type] = breakdown.get(c.change_type, 0) + 1
        return {
            "score": round(total, 2),
            "risk": risk,
            "change_count": len(changes),
            "breakdown": breakdown,
        }
