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
| [Discover](https://github.com/OpenSIN-Code/SIN-Code-Discover-Tool) | File discovery with pattern matching, relevance scoring, dependency mapping |
| [Execute](https://github.com/OpenSIN-Code/SIN-Code-Execute-Tool) | Safe command execution with secret redaction, timeout, error analysis |
| [Map](https://github.com/OpenSIN-Code/SIN-Code-Map-Tool) | Architecture analysis with module mapping, entry points, hot paths |
| [Grasp](https://github.com/OpenSIN-Code/SIN-Code-Grasp-Tool) | Single-file deep analysis with structure, dependencies, context |
| [Scout](https://github.com/OpenSIN-Code/SIN-Code-Scout-Tool) | Code search with regex, semantic, symbol, usage modes |
| [Harvest](https://github.com/OpenSIN-Code/SIN-Code-Harvest-Tool) | URL/API fetching with caching, structure extraction, auth management |
| [Orchestrate](https://github.com/OpenSIN-Code/SIN-Code-Orchestrate-Tool) | Task management with dependencies, parallel execution, rollback |
| [GitNexus](https://github.com/abhigyanpatwari/GitNexus) | Upstream code knowledge graph — bridged, mandatory graph context for agents |
| [MarkItDown](https://github.com/microsoft/markitdown) | Upstream doc→Markdown converter — bridged, document context for agents |
| [RTK](https://github.com/rtk-ai/rtk) | Upstream token-saving command proxy — bridged, 60-90% fewer command tokens |
| CoDocs | Co-located docs standard (`.doc.md` companions) — built into the bundle |

## What the Bundle does

- Provides a single `sin` CLI over all subsystems.
- Exposes a unified MCP server so one entry serves agents all tools.
- **Auto-detects and installs external MCP servers**: `gitnexus` (graph context)
  and `simone-mcp` (code intelligence) are checked and installed if missing.
- **Checks 8 Python subsystems**: SCKG, IBD, POC, EFSM, ADW, Oracle,
  Orchestration, Review-Interface — reports what's installed and suggests
  `pip install -e` commands for missing ones.
- **Degrades gracefully**: each subsystem is an optional dependency. The bundle
  detects which are installed (`sin status`) and only wires up what's available.

## Quickstart

### One-command full install

```bash
# Bootstraps the entire SIN-Code stack (7 Go tools + Python bundle + MCP config + externals)
bash install.sh
```

This installs all 7 Go tools, the Python bundle in editable mode, auto-detects
and installs **gitnexus** (graph context) and **simone-mcp** (code intelligence),
checks for **SIN-Brain** (docs-only), verifies all 8 Python subsystems, and
registers everything in `~/.config/opencode/opencode.json`.

**Flags:** `--help` `--dry-run` `--verbose` `--force` `--skip-go` `--skip-external`

**Environment overrides:**
```bash
SIN_CODE_BIN_DIR=~/custom-bin SIN_CODE_REPOS_DIR=~/my-repos bash install.sh
```

**Full installation takes ~2–5 minutes** (depending on Go build cache and
whether npm packages need downloading). Re-runs are safe and idempotent.

See `install.sh --help` for full details. The companion docs are at
[`install.sh.doc.md`](./install.sh.doc.md).

### Manual install (step by step)

```bash
# Install the subsystems you want, then the bundle:
pip install -e ../SIN-Code-Semantic-Codebase-Knowledge-Graphs
pip install -e ../SIN-Code-Intent-Based-Diffing
pip install -e ../SIN-Code-Proof-of-Correctness
pip install -e ../SIN-Code-Ephemeral-Full-Stack-Mocking-Orchestration
pip install -e ../SIN-Code-Architectural-Debt-Watchdogs
pip install -e ../SIN-Code-Verification-Oracle
pip install -e ../SIN-Code-Orchestration
pip install -e ../SIN-Code-Review-Interface
pip install -e .

sin status # show which subsystems are available
sin bootstrap . # initialize available subsystems for a repo
sin serve # unified MCP server
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
| `sin markitdown setup\|doctor\|convert` | Wire/convert via MarkItDown (doc→Markdown). |
| `sin rtk setup\|doctor\|gain` | Wire RTK token-saving proxy into agents. |
| `sin codocs check [root]` | Validate co-located `.doc.md` references (built-in). |
| `sin codocs list [root]` | List all CoDocs references and whether they resolve. |
| `sin codocs install-skill` | Install the CoDocs agent skill (Hermes / OpenCode). |
| `sin sin-code run <tool> [args]` | Run a SIN-Code Go tool (discover, execute, map, grasp, scout, harvest, orchestrate). |
| `sin sin-code agents-md` | Generate AGENTS.md with SIN-Code Tool Suite rules. |
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

## More external tools: MarkItDown & RTK

Two more best-in-class upstream tools are bridged the same way (installed and
updated independently, never vendored), so every coder agent gets them:

```bash
sin markitdown setup   # MarkItDown MCP: convert PDF/Office/images to Markdown
sin rtk setup          # RTK: route agent shell commands through a token-saving proxy
```

- **[MarkItDown](https://github.com/microsoft/markitdown)** (MIT) — document
  context. Install: `pip install markitdown-mcp "markitdown[all]"`.
- **[RTK](https://github.com/rtk-ai/rtk)** (Apache-2.0) — 60-90% fewer tokens on
  common commands. Install: `brew install rtk`.

See [docs/EXTERNAL_TOOLS.md](./docs/EXTERNAL_TOOLS.md) for the full matrix.

## SIN-Code Go Tools (v2)

The next-generation SIN-Code tools are Go binaries that replace OpenCode's built-in tools:

```bash
# Install all 7 tools
go install github.com/OpenSIN-Code/SIN-Code-Discover-Tool/cmd/discover@latest
go install github.com/OpenSIN-Code/SIN-Code-Execute-Tool/cmd/execute@latest
go install github.com/OpenSIN-Code/SIN-Code-Map-Tool/cmd/map@latest
go install github.com/OpenSIN-Code/SIN-Code-Grasp-Tool/cmd/grasp@latest
go install github.com/OpenSIN-Code/SIN-Code-Scout-Tool/cmd/scout@latest
go install github.com/OpenSIN-Code/SIN-Code-Harvest-Tool/cmd/harvest@latest
go install github.com/OpenSIN-Code/SIN-Code-Orchestrate-Tool/cmd/orchestrate@latest

# Or via the bundle
sin sin-code run discover --help
sin sin-code run execute --help
sin sin-code run map --help
sin sin-code run grasp --help
sin sin-code run scout --help
sin sin-code run harvest --help
sin sin-code run orchestrate --help

# Generate AGENTS.md for any repo
sin sin-code agents-md --output AGENTS.md
```

| Tool | Purpose | Status |
|------|---------|--------|
| discover | File discovery with pattern matching, relevance scoring, dependency mapping | v0.2.5 ✅ |
| execute | Safe command execution with secret redaction, timeout, error analysis | v0.2.4 ✅ |
| map | Architecture analysis with module mapping, entry points, hot paths | v0.2.5 ✅ |
| grasp | Single-file deep analysis with structure, dependencies, context | v0.2.4 ✅ |
| scout | Code search with regex, semantic, symbol, usage modes | v0.1.5 ✅ |
| harvest | URL/API fetching with caching, structure extraction, auth management | v0.1.4 ✅ |
| orchestrate | Task management with dependencies, parallel execution, rollback | v0.1.6 ✅ |

## Documentation

- [INSTALL.md](./INSTALL.md)
- [docs/USAGE.md](./docs/USAGE.md)
- [docs/CONFIGURATION.md](./docs/CONFIGURATION.md)
- [docs/GITNEXUS.md](./docs/GITNEXUS.md)
- [docs/EXTERNAL_TOOLS.md](./docs/EXTERNAL_TOOLS.md)
- [docs/CODOCS.md](./docs/CODOCS.md)
- [CONTRIBUTING.md](./CONTRIBUTING.md)
- [CHANGELOG.md](./CHANGELOG.md)

## License

MIT — see [LICENSE](./LICENSE).
