# Configuration — ADW

ADW is configured through CLI flags, constructor arguments, and a small amount
of runtime state under `.sin/`.

## Circuit breaker

`BreakerConfig` controls abort thresholds:

| Field | Default | Description |
|-------|---------|-------------|
| `max_cost_usd` | 5.0 | Abort when spend since reset exceeds this. |
| `max_iterations` | 20 | Abort when iteration count reaches this. |
| `max_debt_increase` | 20.0 | Abort when debt score rises by this much. |
| `cooldown_seconds` | 300 | Suggested cooldown after a trip. |

```python
from sin_code_adw import CircuitBreaker
from sin_code_adw.circuit_breaker import BreakerConfig

breaker = CircuitBreaker(BreakerConfig(max_cost_usd=2.0, max_iterations=10))
```

## Cost ledger

- Stored as append-only JSONL, default `.sin/costs.jsonl`.
- Override with `CostTracker(log_path="...")`.

### Pricing table

Approximate USD per 1K tokens lives in `cost_tracker.PRICING`. Unknown models
fall back to a default rate. Update this table to match current provider pricing:

```python
from sin_code_adw.cost_tracker import PRICING
PRICING["my-model"] = {"in": 0.001, "out": 0.004}
```

## Debt scoring

- Score combines average cyclomatic complexity, hotspot count, and total lines.
- Levels: `< 20` healthy, `< 50` manageable, `< 80` warning, otherwise critical.
- A "hotspot" is any block with cyclomatic complexity `> 10`.

## Watchdog daemon

`WatchdogDaemon(repo_root, poll_interval)` polls the repo and emits alerts when
the debt score rises more than 20 points over the baseline, or when the circuit
breaker trips.
