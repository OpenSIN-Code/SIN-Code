# SIN-Code Architectural Debt Watchdogs (ADW)

> The meta-guardian of the stack: quantify technical debt, track agent API
> costs, and trip a circuit breaker before a runaway agent does damage.

[![Python](https://img.shields.io/badge/python-3.11%2B-blue)](https://www.python.org/)
[![License: MIT](https://img.shields.io/badge/license-MIT-green)](./LICENSE)

Part of the [SIN-Code](https://github.com/OpenSIN-Code) agent-engineering stack.

## Why

Autonomous agents can quietly rack up complexity and API spend, or loop forever.
ADW gives you measurable guardrails: a debt score that rises when code rots, a
running cost ledger per agent/task, and a circuit breaker that aborts when cost,
iterations, or debt cross your thresholds.

## Features

- **Complexity analyzer** (built on [radon](https://radon.readthedocs.io/)) —
  cyclomatic complexity, maintainability index, and hotspot detection.
- **Debt score** (0–100) with levels: healthy / manageable / warning / critical.
- **Cost tracker** — append-only JSONL ledger of token usage and USD cost per
  model, agent, and task.
- **Circuit breaker** — abort on max cost, max iterations, or debt increase.
- **Watchdog daemon** — background polling that alerts on debt spikes.
- **CLI** (`adw`) for scanning, recording costs, and watching.

## Quickstart

```bash
pip install -e .
adw scan                         # architectural debt of the current repo
adw record gpt-4o 1200 800 --agent a1 --task "refactor"
adw costs --agent a1             # tally spend
adw watch --interval 30          # run the background watchdog
```

## Documentation

- [INSTALL.md](./INSTALL.md)
- [docs/USAGE.md](./docs/USAGE.md)
- [docs/CONFIGURATION.md](./docs/CONFIGURATION.md)
- [CONTRIBUTING.md](./CONTRIBUTING.md)
- [CHANGELOG.md](./CHANGELOG.md)

## License

MIT — see [LICENSE](./LICENSE).
