# SIN-Code Bundle

> One CLI and one MCP server that orchestrate the entire SIN-Code
> agent-engineering stack.

[![Python](https://img.shields.io/badge/python-3.11%2B-blue)](https://www.python.org/)
[![License: MIT](https://img.shields.io/badge/license-MIT-green)](./LICENSE)

Part of the [SIN-Code](https://github.com/OpenSIN-Code) agent-engineering stack.

## What is SIN-Code?

A set of state-of-the-art tools that give AI coding agents the signals they
actually lack — structural knowledge, semantic diffs, correctness proofs,
ephemeral test environments, debt/cost guardrails, and an independent
verification oracle.

| Repo | Role |
|------|------|
| [SCKG](https://github.com/OpenSIN-Code/SIN-Code-Semantic-Codebase-Knowledge-Graphs) | Semantic codebase knowledge graph |
| [IBD](https://github.com/OpenSIN-Code/SIN-Code-Intent-Based-Diffing) | Intent-based semantic diffing |
| [POC](https://github.com/OpenSIN-Code/SIN-Code-Proof-of-Correctness) | Lightweight proof of correctness |
| [EFSM](https://github.com/OpenSIN-Code/SIN-Code-Ephemeral-Full-Stack-Mocking-Orchestration) | Ephemeral full-stack mocking |
| [ADW](https://github.com/OpenSIN-Code/SIN-Code-Architectural-Debt-Watchdogs) | Architectural debt & cost watchdog |
| [Oracle](https://github.com/OpenSIN-Code/SIN-Code-Verification-Oracle) | Independent verification oracle |
| CoDocs | Co-located docs standard (`.doc.md` companions) — built into the bundle |

## What the Bundle does

- Provides a single `sin` CLI over all subsystems.
- Exposes a unified MCP server so one entry serves agents all tools.
- **Degrades gracefully**: each subsystem is an optional dependency. The bundle
  detects which are installed (`sin status`) and only wires up what's available.

## Quickstart

```bash
# Install the subsystems you want, then the bundle:
pip install -e ../SIN-Code-Semantic-Codebase-Knowledge-Graphs
pip install -e ../SIN-Code-Verification-Oracle
pip install -e .

sin status            # show which subsystems are available
sin bootstrap .       # initialize available subsystems for a repo
sin serve             # unified MCP server
```

## Commands

| Command | Description |
|---------|-------------|
| `sin status` | Show which subsystems are installed/available. |
| `sin bootstrap [repo]` | Initialize available subsystems (graph, baselines, ledger). |
| `sin review <a> <b>` | Semantic review of a change (IBD). |
| `sin verify <module> <fn>` | Proof-of-correctness for a function (POC). |
| `sin debt [root]` | Architectural debt overview (ADW). |
| `sin codocs check [root]` | Validate co-located `.doc.md` references (built-in). |
| `sin codocs list [root]` | List all CoDocs references and whether they resolve. |
| `sin codocs install-skill` | Install the CoDocs agent skill (Hermes / OpenCode). |
| `sin serve` | Unified MCP server across available subsystems. |

## Documentation

- [INSTALL.md](./INSTALL.md)
- [docs/USAGE.md](./docs/USAGE.md)
- [docs/CONFIGURATION.md](./docs/CONFIGURATION.md)
- [docs/CODOCS.md](./docs/CODOCS.md)
- [CONTRIBUTING.md](./CONTRIBUTING.md)
- [CHANGELOG.md](./CHANGELOG.md)

## License

MIT — see [LICENSE](./LICENSE).
