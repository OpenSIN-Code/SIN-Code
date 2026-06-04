"""Background-Daemon, der alle Watchdogs vereint."""

from __future__ import annotations

import json
import threading
import time

from .circuit_breaker import BreakerConfig, BreakerTripped, CircuitBreaker
from .complexity import ComplexityAnalyzer
from .cost_tracker import CostTracker


class WatchdogDaemon:
    """Laeuft im Hintergrund und eskaliert bei Problemen."""

    DEFAULT_EXCLUDE = {"venv", ".venv", "node_modules", ".git", "__pycache__"}

    def __init__(self, repo_root: str = ".", poll_interval: int = 30):
        self.repo_root = repo_root
        self.poll_interval = poll_interval
        self.analyzer = ComplexityAnalyzer()
        self.cost = CostTracker()
        self.breaker = CircuitBreaker(BreakerConfig())
        self._running = False
        self.alerts: list[dict] = []
        self._thread: threading.Thread | None = None

    def start(self) -> threading.Thread:
        self._running = True
        self._thread = threading.Thread(target=self._loop, daemon=True)
        self._thread.start()
        return self._thread

    def stop(self) -> None:
        self._running = False

    def _loop(self) -> None:
        baseline = self.analyzer.debt_score(
            self.analyzer.analyze(self.repo_root, exclude=self.DEFAULT_EXCLUDE)
        )
        while self._running:
            try:
                reports = self.analyzer.analyze(self.repo_root, exclude=self.DEFAULT_EXCLUDE)
                current = self.analyzer.debt_score(reports)
                total_cost = self.cost.total_for()["total_usd"]
                self.breaker.check(current_cost=total_cost, current_debt=current["score"])
                if current["score"] > baseline["score"] + 20:
                    self._alert("debt_spike", current)
            except BreakerTripped as exc:
                self._alert("breaker_tripped", {"reason": str(exc)})
                break
            except Exception as exc:
                self._alert("error", {"reason": str(exc)})
            time.sleep(self.poll_interval)

    def _alert(self, kind: str, data: dict) -> None:
        entry = {"kind": kind, "timestamp": time.time(), "data": data}
        self.alerts.append(entry)
        print(f"[ADW ALERT] {kind}: {json.dumps(data)}")
