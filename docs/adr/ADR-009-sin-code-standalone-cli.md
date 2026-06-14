# ADR-009: sin-code as a Standalone Coding CLI

## Status

Accepted

## Context

SIN-Code started as a loose federation of Python tools and external bridges.
Over time the agent-engineering surface consolidated: file I/O, search,
architecture analysis, verification, orchestration, memory, and UI all need to
work from a single, fast, portable binary. The Go rewrite of the core toolchain
created the opportunity to ship one binary that covers the full software
development lifecycle, instead of relying on a fragmented Python toolchain.

## Decision

We promote `sin-code` from a helper utility to a first-class, standalone
coding CLI. It is now distributed as a single Go binary with 32 subcommands
(13 core MCP tools + 19 utility/specialized CLI commands) and is the primary
interface for both humans and AI agents.

### Logical layers

| Layer | Subcommands | Purpose |
|-------|-------------|---------|
| Read / edit | `read`, `write`, `edit` | Atomic writes and hashline/AST-verified surgical edits |
| Analysis / search | `discover`, `scout`, `grasp`, `map`, `adw`, `sckg` | Regex/semantic search, dependency graphs, architecture maps |
| Security / quality | `security`, `sbom`, `oracle`, `poc`, `ibd` | Security scans, SBOMs, proof-of-correctness, intent diffing |
| Multi-agent orchestration | `orchestrator-run`, `orchestrator-plan`, `orchestrator-agents` | Pre-LLM router, planner, and parallel sub-agents |
| Task / memory | `todo`, `notifications`, `memory` | Project-level issue tracker and persistent semantic memory |
| User interfaces | `tui`, `webui`, `serve` | Terminal UI, web dashboard, and MCP server |

### External interfaces

- `sin-code tui` — interactive multi-pane terminal interface (Bubble Tea).
- `sin-code webui` — web-based dashboard.
- `sin-code serve` — MCP server exposing the 13 core tools to editors and other
  CLIs (VS Code, Cursor, opencode cli, codex cli).

## Consequences

- **Positive:** One binary, no Python venv, cross-platform, consistent CLI/MCP UX.
- **Positive:** Faster execution and lower resource usage than the legacy Python stack.
- **Positive:** Native integration with the SIN-Code MCP ecosystem.
- **Negative:** Contributors need the Go toolchain; the legacy Python stack still
  exists for fallback use.
- **Negative:** A single binary is a single point of failure for the whole tool
  suite, raising the bar for reliability and test coverage.

## Open Tasks and Roadmap

### Current open issues

- **st-cov1 — Raise `cmd/sin-code/internal/` test coverage to ≥80%.**
  The root `internal` package currently sits at ~72.7% (EFM tests excluded due
  to Docker/OrbStack dependency). Gaps are concentrated in `internal/lsp`,
  `internal/memory`, `internal/orchestrator`, and `internal/index`.

- **st-bug1 Bug 3 — POC parser weakness.**
  The parent issue `st-bug1` is closed (4/5 dogfooding bugs fixed in v2.5.0),
  but Bug 3 was deferred. The Proof-of-Correctness tool treats natural-language
  words such as "must" or "Spec" as required function names. A structured
  requirement parser is needed.

### SOTA roadmap (medium-term workstreams)

- **WS1 — Compiler/LSP as a primary correctness oracle:** integrate gopls/
  pyright/tsserver behind a stable adapter and feed diagnostics into the Oracle.
- **WS2 — Budget-aware context selection:** let SCKG return a minimal, ranked
  code context fitting a given LLM token budget.
- **WS3 — Behavioral trace diffing:** capture execution traces before/after a
  change and compare behavior, not just AST diffs.
- **WS6 — Incremental graph updates:** file-watcher-based incremental SCKG updates
  instead of full rebuilds.
- **WS7 — Polyglot parity:** bring JS/TS SCKG and IBD support to the same level
  as Python.
- **WS9 — Semantic merge for parallel agents:** symbol-level conflict resolution
  when multiple agents edit the same code.

## References

- `AGENTS.md` — canonical SIN-Code tool-suite rules and current priorities.
- `docs/issues/st-cov1-coverage-80-percent.md` — coverage issue.
- `docs/issues/st-bug1-dogfooding-bugs.md` — dogfooding bugs (Bug 3 deferred).
- `docs/plans/sota-roadmap.md` — full SOTA roadmap with all workstreams.
- `cmd/sin-code/main.go` — unified binary entry point.
