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
| [GitNexus](https://github.com/abhigyanpatwari/GitNexus) | Upstream code knowledge graph — bridged, mandatory graph context for agents |

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
| `sin preflight [root]` | Ensure GitNexus graph context is fresh before agents code. |
| `sin gitnexus setup` | Wire GitNexus MCP into OpenCode / Codex / Hermes. |
| `sin gitnexus index\|status\|doctor\|context\|impact\|ai-context` | GitNexus graph operations. |
| `sin serve` | Unified MCP server across available subsystems. |

## GitNexus: mandatory graph context

Coder agents should never edit a repo "blind". The bundle bridges
[GitNexus](https://github.com/abhigyanpatwari/GitNexus) (kept as the upstream
original, **not** vendored — it is PolyForm-Noncommercial while the bundle is
MIT) and makes its code knowledge graph available to every agent:

```bash
sin gitnexus setup   # wire OpenCode + Codex + Hermes to the GitNexus MCP server
sin preflight        # auto-build/refresh the graph before any agent task
```

Requires Node.js >= 18 (`npx`). See [docs/GITNEXUS.md](./docs/GITNEXUS.md).

## Documentation

- [INSTALL.md](./INSTALL.md)
- [docs/USAGE.md](./docs/USAGE.md)
- [docs/CONFIGURATION.md](./docs/CONFIGURATION.md)
- [docs/GITNEXUS.md](./docs/GITNEXUS.md)
- [CONTRIBUTING.md](./CONTRIBUTING.md)
- [CHANGELOG.md](./CHANGELOG.md)

## License

MIT — see [LICENSE](./LICENSE).
