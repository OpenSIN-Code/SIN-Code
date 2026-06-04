import tempfile
from pathlib import Path

import pytest
from sin_code_adw.circuit_breaker import (
    BreakerConfig,
    BreakerTripped,
    CircuitBreaker,
)
from sin_code_adw.complexity import ComplexityAnalyzer
from sin_code_adw.cost_tracker import CostTracker
from sin_code_adw.daemon import WatchdogDaemon


def test_complexity_analyze_and_score():
    analyzer = ComplexityAnalyzer()
    with tempfile.TemporaryDirectory() as d:
        Path(d, "m.py").write_text("def simple(x):\n    return x + 1\n")
        reports = analyzer.analyze(d)
        assert len(reports) == 1
        score = analyzer.debt_score(reports)
        assert "level" in score
        assert score["level"] in ("healthy", "manageable", "warning", "critical")


def test_debt_score_empty():
    analyzer = ComplexityAnalyzer()
    assert analyzer.debt_score([])["level"] == "none"


def test_cost_tracker_records_and_totals():
    with tempfile.TemporaryDirectory() as d:
        tracker = CostTracker(log_path=str(Path(d, "costs.jsonl")))
        entry = tracker.record("gpt-4o", 1000, 500, agent_id="a1", task="t1")
        assert entry.cost_usd > 0
        total = tracker.total_for(agent_id="a1")
        assert total["entries"] == 1
        assert total["total_usd"] > 0


def test_cost_tracker_unknown_model_uses_default():
    with tempfile.TemporaryDirectory() as d:
        tracker = CostTracker(log_path=str(Path(d, "costs.jsonl")))
        entry = tracker.record("unknown-model", 1000, 1000)
        assert entry.cost_usd == pytest.approx(0.005 + 0.015)


def test_circuit_breaker_trips_on_iterations():
    breaker = CircuitBreaker(BreakerConfig(max_iterations=3))
    with pytest.raises(BreakerTripped):
        breaker.check(iteration=3)


def test_circuit_breaker_trips_on_cost():
    breaker = CircuitBreaker(BreakerConfig(max_cost_usd=1.0))
    breaker.reset(start_cost=0.0)
    with pytest.raises(BreakerTripped):
        breaker.check(current_cost=2.0)


def test_circuit_breaker_guard_catches_trip():
    breaker = CircuitBreaker(BreakerConfig(max_iterations=0))

    def work():
        breaker.check(iteration=5)
        return "should-not-reach"

    result = breaker.guard(work, lambda: 0.0, lambda: 0.0)
    assert result["aborted"] is True


def test_daemon_instantiates():
    wd = WatchdogDaemon(".", poll_interval=1)
    assert wd.alerts == []
    assert wd._running is False
