"""Fasst AST-Changes zu architektonischen Intents zusammen."""
from __future__ import annotations

from collections import defaultdict
from dataclasses import dataclass, field

from .ast_diff import Change


@dataclass
class IntentSummary:
    headline: str
    category: str  # refactor, feature, api_change, cleanup, unknown
    risk: str  # low, medium, high
    affected_symbols: list[str] = field(default_factory=list)
    rationale: str = ""


class IntentSummarizer:
    """Klassifiziert AST-Changes in architektonische Intents."""

    def summarize(self, changes: list[Change]) -> list[IntentSummary]:
        if not changes:
            return []
        groups: dict[str, list[Change]] = defaultdict(list)
        for c in changes:
            groups[c.change_type].append(c)

        intents: list[IntentSummary] = []

        if groups.get("signature_changed"):
            syms = [c.symbol for c in groups["signature_changed"]]
            intents.append(IntentSummary(
                headline=f"API signatures modified: {len(syms)} symbol(s)",
                category="api_change", risk="high", affected_symbols=syms,
                rationale="Changed signatures may break callers. Review required.",
            ))

        added = [c.symbol for c in groups.get("added", [])]
        removed = [c.symbol for c in groups.get("removed", [])]
        if added and removed and len(added) == len(removed):
            intents.append(IntentSummary(
                headline=f"Possible rename/refactor: {len(added)} symbol(s) swapped",
                category="refactor", risk="medium", affected_symbols=added + removed,
                rationale="Equal adds/removals suggests renaming or restructuring.",
            ))
        elif added:
            intents.append(IntentSummary(
                headline=f"New symbols added: {len(added)}",
                category="feature", risk="low", affected_symbols=added,
                rationale="New code was introduced.",
            ))
        elif removed:
            intents.append(IntentSummary(
                headline=f"Symbols removed: {len(removed)}",
                category="cleanup", risk="medium", affected_symbols=removed,
                rationale="Existing code was deleted - verify no callers remain.",
            ))

        if groups.get("body_changed"):
            syms = [c.symbol for c in groups["body_changed"]]
            intents.append(IntentSummary(
                headline=f"Internal logic changed: {len(syms)} symbol(s)",
                category="refactor", risk="low", affected_symbols=syms,
                rationale="Implementation changed but public surface kept intact.",
            ))

        if groups.get("decorators_changed"):
            syms = [c.symbol for c in groups["decorators_changed"]]
            intents.append(IntentSummary(
                headline=f"Cross-cutting concerns changed: {len(syms)} symbol(s)",
                category="api_change", risk="medium", affected_symbols=syms,
                rationale="Decorators often control auth, tracing, caching - high impact.",
            ))

        if not intents:
            intents.append(IntentSummary(
                headline="Cosmetic changes only", category="unknown", risk="low",
                rationale="Docstrings or whitespace adjusted.",
            ))
        return intents
