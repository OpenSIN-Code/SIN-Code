# Usage — ADW

The package installs the `adw` command.

## `adw scan [--root <dir>]`

Scan a repository for architectural debt.

```bash
adw scan
adw scan --root ./src
```

Returns the debt score, files scanned, and the top complexity hotspots.

## `adw record <model> <prompt_tokens> <completion_tokens>`

Append a cost entry to the ledger.

```bash
adw record gpt-4o 1200 800 --agent a1 --task "refactor auth"
```

## `adw costs [--agent <id>] [--task <name>]`

Tally recorded costs, optionally filtered.

```bash
adw costs
adw costs --agent a1 --task "refactor auth"
```

## `adw watch [--root <dir>] [--interval <s>]`

Run the watchdog daemon, alerting on debt spikes. Stop with Ctrl+C.

```bash
adw watch --interval 30
```

## Python API

```python
from sin_code_adw import ComplexityAnalyzer, CostTracker, CircuitBreaker, BreakerTripped

analyzer = ComplexityAnalyzer()
debt = analyzer.debt_score(analyzer.analyze(".", exclude={".git", "venv"}))

tracker = CostTracker()
tracker.record("gpt-4o", 1000, 500, agent_id="a1", task="t1")

breaker = CircuitBreaker()
try:
    breaker.check(current_cost=2.0, current_debt=10.0, iteration=3)
except BreakerTripped as e:
    print("aborted:", e)
```
