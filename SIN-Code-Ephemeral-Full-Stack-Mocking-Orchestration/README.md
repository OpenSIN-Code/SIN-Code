# SIN-Code Ephemeral Full-Stack Mocking Orchestration (EFSM)

> Spin up a complete, isolated test environment per agent task — stateful API
> mocks, an optional sandboxed runner, and one-command teardown.

[![Python](https://img.shields.io/badge/python-3.11%2B-blue)](https://www.python.org/)
[![License: MIT](https://img.shields.io/badge/license-MIT-green)](./LICENSE)

Part of the [SIN-Code](https://github.com/OpenSIN-Code) agent-engineering stack.

## Why

Agents that touch external services (payments, third-party APIs, a database)
either hit real systems (dangerous, flaky, costly) or rely on brittle, stateless
stubs. EFSM builds a disposable full-stack environment: **stateful** mocks that
remember what you POSTed, wired into a runner, so tests exercise realistic
behavior and then vanish.

## Features

- **Stateful mock server** (FastAPI) — POST creates, GET returns what you
  created; per-resource in-memory state and scripted scenarios.
- **Orchestrator** — configures mocks + env vars from a task description
  (`external_apis`, `requires_db`, `test_command`).
- **Sandboxed execution** — Docker when available, with an automatic
  **subprocess fallback** so it still runs without Docker.
- **In-process dispatch** for fast unit testing of mocks (no network needed).
- **CLI** (`efsm`) to set up an environment and run tests in one shot.

## Quickstart

```bash
pip install -e .
efsm setup mytask --api stripe --api github --db --test-cmd "pytest"
```

## Documentation

- [INSTALL.md](./INSTALL.md)
- [docs/USAGE.md](./docs/USAGE.md)
- [docs/CONFIGURATION.md](./docs/CONFIGURATION.md)
- [CONTRIBUTING.md](./CONTRIBUTING.md)
- [CHANGELOG.md](./CHANGELOG.md)

## Note on isolation

Docker gives the strongest isolation. When Docker is not present, EFSM falls back
to a resource-limited subprocess runner — convenient for local/CI use, but not a
security boundary for untrusted code.

## License

MIT — see [LICENSE](./LICENSE).
