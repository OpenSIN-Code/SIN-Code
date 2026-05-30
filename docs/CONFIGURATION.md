# Configuration — SIN-Code Bundle

The bundle itself is thin: it orchestrates the subsystems, so most configuration
lives in each subsystem's own docs. This page covers what the bundle controls.

## Subsystem availability

The bundle treats each subsystem as an **optional dependency**. It imports them
lazily and reports availability via `sin status`. Install only what you need:

```bash
pip install -e ../SIN-Code-Semantic-Codebase-Knowledge-Graphs   # impact, bootstrap graph
pip install -e ../SIN-Code-Intent-Based-Diffing                 # review, semantic_diff
pip install -e ../SIN-Code-Proof-of-Correctness                 # verify
pip install -e ../SIN-Code-Architectural-Debt-Watchdogs         # debt, baselines, ledger
pip install -e ../SIN-Code-Verification-Oracle                  # verification oracle
pip install -e .                                                # the bundle
```

If a command needs a subsystem that is not installed, `sin` prints a clear
message naming the missing package rather than raising an import error.

## Runtime artifacts (`.sin/`)

`sin bootstrap` writes into `.sin/` inside the target repository:

| Path | Written by | Contents |
|------|-----------|----------|
| `.sin/knowledge.graph` | SCKG | Persisted knowledge graph (JSON). |
| `.sin/baseline.json` | ADW | Complexity baseline at bootstrap time. |
| `.sin/costs.jsonl` | ADW | Append-only cost ledger. |

Delete any of these to reset that piece of state.

## MCP server

`sin serve` requires the optional `mcp` dependency:

```bash
pip install -e ".[mcp]"
```

Only tools whose backing subsystem is installed are registered.

## Per-subsystem configuration

See each subsystem's `docs/CONFIGURATION.md`:

- SCKG — `config.yaml` (repo root, excludes, languages, graph storage).
- IBD — stateless; risk weights overridable in code.
- POC — stateless; `--max-examples` flag.
- EFSM — per-task context; Docker vs. subprocess backend.
- ADW — `BreakerConfig`, pricing table, ledger path.
- Oracle — CLI flags, eval suite JSON, optional external tools.
