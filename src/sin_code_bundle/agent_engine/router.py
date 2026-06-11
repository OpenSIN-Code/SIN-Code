# SPDX-License-Identifier: MIT
"""Tool router with per-tool circuit breakers and decorrelated-jitter backoff."""

from __future__ import annotations

import asyncio
import random
import time
from dataclasses import dataclass, field
from typing import Any, Awaitable, Callable

ToolFn = Callable[..., Awaitable[Any]]


class CircuitOpenError(RuntimeError):
    """Raised when a tool's circuit breaker is open."""


@dataclass(slots=True)
class _Circuit:
    failure_threshold: int = 5
    cooldown_s: float = 30.0
    consecutive_failures: int = 0
    opened_at: float | None = None
    half_open: bool = False

    @property
    def state(self) -> str:
        if self.opened_at is None:
            return "closed"
        if time.monotonic() - self.opened_at >= self.cooldown_s:
            return "half_open"
        return "open"

    def record_success(self) -> None:
        self.consecutive_failures = 0
        self.opened_at = None
        self.half_open = False

    def record_failure(self) -> None:
        self.consecutive_failures += 1
        if self.consecutive_failures >= self.failure_threshold:
            self.opened_at = time.monotonic()


@dataclass
class ToolRouter:
    max_retries: int = 3
    base_delay_s: float = 0.5
    max_delay_s: float = 20.0
    _tools: dict[str, ToolFn] = field(default_factory=dict)
    _circuits: dict[str, _Circuit] = field(default_factory=dict)
    _stats: dict[str, dict[str, int]] = field(default_factory=dict)

    def register(self, name: str, fn: ToolFn, *,
                 failure_threshold: int = 5,
                 cooldown_s: float = 30.0) -> None:
        self._tools[name] = fn
        self._circuits[name] = _Circuit(
            failure_threshold=failure_threshold, cooldown_s=cooldown_s
        )
        self._stats[name] = {"calls": 0, "failures": 0, "retries": 0}

    def stats(self) -> dict[str, dict[str, Any]]:
        return {
            name: {**self._stats[name], "circuit": self._circuits[name].state}
            for name in self._tools
        }

    async def call(self, name: str, **kwargs: Any) -> Any:
        if name not in self._tools:
            raise KeyError(f"unknown tool: {name!r}")
        circuit = self._circuits[name]
        state = circuit.state
        if state == "open":
            raise CircuitOpenError(
                f"circuit open for tool {name!r} "
                f"({circuit.consecutive_failures} consecutive failures)"
            )

        self._stats[name]["calls"] += 1
        delay = self.base_delay_s
        last_err: Exception | None = None

        attempts = 1 if state == "half_open" else self.max_retries
        for attempt in range(attempts):
            try:
                result = await self._tools[name](**kwargs)
                circuit.record_success()
                return result
            except Exception as err:
                last_err = err
                self._stats[name]["failures"] += 1
                circuit.record_failure()
                if circuit.state == "open" or attempt == attempts - 1:
                    break
                delay = min(
                    self.max_delay_s,
                    random.uniform(self.base_delay_s, delay * 3),
                )
                self._stats[name]["retries"] += 1
                await asyncio.sleep(delay)

        raise RuntimeError(
            f"tool {name!r} failed after retries: {last_err}"
        ) from last_err
