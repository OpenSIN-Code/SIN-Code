"""Circuit Breaker fuer Agent-Loops."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Callable


class BreakerTripped(Exception):
    """Ausnahme wenn ein Limit ueberschritten wird."""


@dataclass
class BreakerConfig:
    max_cost_usd: float = 5.0
    max_iterations: int = 20
    max_debt_increase: float = 20.0
    cooldown_seconds: int = 300


class CircuitBreaker:
    """Bricht Agent-Runs ab, wenn Schwellen ueberschritten werden."""

    def __init__(self, config: BreakerConfig | None = None):
        self.config = config or BreakerConfig()
        self._iterations = 0
        self._start_cost = 0.0
        self._start_debt = 0.0

    def check(
        self,
        current_cost: float = 0.0,
        current_debt: float = 0.0,
        iteration: int | None = None,
    ) -> None:
        if iteration is not None:
            self._iterations = iteration
        if self._iterations >= self.config.max_iterations:
            raise BreakerTripped(f"Max iterations {self.config.max_iterations} reached")
        if current_cost - self._start_cost >= self.config.max_cost_usd:
            raise BreakerTripped(f"Cost limit ${self.config.max_cost_usd} exceeded")
        if current_debt - self._start_debt >= self.config.max_debt_increase:
            raise BreakerTripped(f"Debt increased by {self.config.max_debt_increase} points")

    def reset(self, start_cost: float = 0.0, start_debt: float = 0.0) -> None:
        self._iterations = 0
        self._start_cost = start_cost
        self._start_debt = start_debt

    def guard(
        self,
        fn: Callable[[], Any],
        cost_fn: Callable[[], float],
        debt_fn: Callable[[], float],
    ) -> Any:
        """Fuehrt fn() aus und faengt ein Tripping ab."""
        self.reset(cost_fn(), debt_fn())
        try:
            return fn()
        except BreakerTripped as exc:
            return {"aborted": True, "reason": str(exc)}
