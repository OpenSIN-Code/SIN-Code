# `sin-code` — Unified Go Binary for SIN-Code Tools

**What it does:** `sin-code` is a single Go binary that consolidates 13 specialized SIN-Code analysis and manipulation tools behind a cobra-based CLI. It replaces 13 separate Go binaries with one.

**Created:** v1.0.0 (2026-06-06)

## Why a unified binary?

| Aspect | 13 separate binaries | Unified `sin-code` |
|--------|---------------------|-------------------|
| Install | 13 `go install` commands | 1 `go build` |
| PATH | 13 entries | 1 entry (`sin-code`) |
| Versioning | 13 independent versions | Atomic (single version) |
| Startup | Shell-out overhead per call | Direct in-process call |
| Distribution | 13 releases | 1 release |
| Dependencies | 13 `go.sum` files | 1 `go.sum` |

## Subcommands (13)

### Core analysis (7)
- `discover` — File discovery with relevance scoring, pattern matching, dependency analysis
- `execute` — Safe command execution with timeout, secret redaction, error analysis
- `map` — Architecture analysis with dependency graphs, entry points, hot paths
- `grasp` — Deep code understanding for individual files (structure, deps, usage)
- `scout` — Code search with regex, semantic, symbol, and usage modes
- `harvest` — URL fetching with caching, structure extraction, change detection
- `orchestrate` — Task management with dependencies, parallel execution, rollback

### Advanced tools (6)
- `ibd` — Intent-Based Diffing: compare code changes against stated intent
- `poc` — Proof-of-Correctness: verify code satisfies its specification
- `sckg` — Semantic Codebase Knowledge Graphs: build & query code graph
- `adw` — Architectural Debt Watchdogs: detect god modules, circular deps, coupling
- `oracle` — Verification Oracle: independent verification of claims with evidence
- `efm` — Ephemeral Full-Stack Mocking: spin up disposable test environments

## Installation

```bash
go install github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code@latest
# or
git clone https://github.com/OpenSIN-Code/SIN-Code-Bundle && cd SIN-Code-Bundle
go build -o ~/.local/bin/sin-code ./cmd/sin-code
```

## Usage

```bash
sin-code --help                    # List all 13 subcommands
sin-code discover . -pattern "**/*.py" -format json
sin-code execute -command "npm test" -timeout 60
sin-code sckg . -action build
sin-code ibd -before v1.0 -after HEAD -intent "add retry"
```

## Integration with `sin` Python CLI

The `sin sin-code run <tool> -- <args...>` Python command now routes through the unified `sin-code` binary:

```bash
sin sin-code run sckg -- --action build --format json .
sin sin-code run ibd -- --before v1.0 --after HEAD -intent "fix bug"
```

The `--` separator is required so typer doesn't try to parse the subcommand's flags.

## Architecture

```
cmd/sin-code/
  main.go              # Root cobra command, registers all 13 subcommands
  internal/
    common.go          # Shared utilities (error printing)
    discover.go        # discover subcommand
    execute.go         # execute subcommand
    map.go             # map subcommand
    grasp.go           # grasp subcommand
    scout.go           # scout subcommand
    harvest.go         # harvest subcommand
    orchestrate.go     # orchestrate subcommand
    ibd.go             # ibd subcommand
    poc.go             # poc subcommand
    sckg.go            # sckg subcommand
    adw.go             # adw subcommand
    oracle.go          # oracle subcommand
    efm.go             # efm subcommand
```

## Backwards Compatibility

The 7 standalone tool repos (`SIN-Code-Discover-Tool`, `SIN-Code-Execute-Tool`, `SIN-Code-Map-Tool`, `SIN-Code-Grasp-Tool`, `SIN-Code-Scout-Tool`, `SIN-Code-Harvest-Tool`, `SIN-Code-Orchestrate-Tool`) are still maintained for projects that depend on the standalone binaries. The Python CLI falls back to standalone binaries if `sin-code` is not found.

The 6 advanced tool repos (`SIN-Code-IBD-Tool`, `SIN-Code-PoC-Tool`, `SIN-Code-SCKG-Tool`, `SIN-Code-ADW-Tool`, `SIN-Code-Oracle-Tool`, `SIN-Code-EFM-Tool`) are now **native subcommands** of `sin-code`. The thin Python wrappers (`sin ibd`, `sin poc`, etc.) are deprecated in favor of `sin sin-code run <tool>` or direct `sin-code <tool>` calls.

## Dependencies

- `github.com/spf13/cobra` v1.10.2 — CLI framework
- Go 1.25.0+ (matches SIN-Code-Bundle/go.mod)

## Known Limitations

- The current subcommands are pass-through stubs that delegate to the standalone tool repos. Full in-process implementation of each tool's logic is planned for v1.1.0.
- The `--` separator in Python CLI is awkward but necessary due to typer's flag parsing.
