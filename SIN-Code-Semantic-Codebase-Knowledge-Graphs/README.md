# SIN-Code Semantic Codebase Knowledge Graphs (SCKG)

> A real dependency graph for AI coding agents — Tree-sitter parsing, Git-history
> intent, and blast-radius impact analysis instead of a dumb vector store.

[![Python](https://img.shields.io/badge/python-3.11%2B-blue)](https://www.python.org/)
[![License: MIT](https://img.shields.io/badge/license-MIT-green)](./LICENSE)

Part of the [SIN-Code](https://github.com/OpenSIN-Code) agent-engineering stack.

## Why

Vector-database RAG retrieves text that *looks* similar. It does not know that
changing `parse()` will break 14 call sites. SCKG builds an actual
`MultiDiGraph` of symbols, calls, file-containment and Git-commit intents, so an
agent can ask structural questions: *what calls this? what breaks if I change
it? where are the architectural hubs?*

## Features

- **Tree-sitter AST parsing** for Python, JavaScript and TypeScript.
- **Call / containment edges** resolved across the repository.
- **Git-history intent extraction** — commits classified as refactor / feature /
  fix / docs and linked to the files they touched.
- **Blast-radius impact analysis** with a normalized risk score.
- **Architecture overview** — top hubs by out-degree, total nodes/edges.
- **Persistent graph** stored as JSON under `.sin/`.
- **CLI** (`sckg`) and **MCP server** for agent integration.
- **Graceful degradation** — a missing language grammar is skipped with a
  warning, never a crash.

## Quickstart

```bash
pip install -e .
sckg build            # parse the current repo into .sin/knowledge.graph
sckg arch             # show hubs and totals
sckg find parse       # locate a symbol by name
sckg impact "src/x.py:function:parse"   # blast radius
```

## Documentation

- [INSTALL.md](./INSTALL.md) — installation and verification
- [docs/USAGE.md](./docs/USAGE.md) — CLI commands and MCP tools
- [docs/CONFIGURATION.md](./docs/CONFIGURATION.md) — `config.yaml` reference
- [CONTRIBUTING.md](./CONTRIBUTING.md) — development workflow
- [CHANGELOG.md](./CHANGELOG.md) — release notes

## MCP integration

```yaml
# ~/.config/opencode/config.yaml
mcpServers:
  sckg:
    command: sckg
    args: [serve]
```

Exposed tools: `find_symbol`, `impact_analysis`, `architecture_overview`,
`downstream_deps`.

## License

MIT — see [LICENSE](./LICENSE).
