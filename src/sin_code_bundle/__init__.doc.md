# `sin_code_bundle` — Unified SOTA Agent-Engineering Stack

This package is the **MIT-licensed Python meta-package** that wires the SIN-Code
stack into a single installable for coder agents (OpenCode / Codex / Hermes).

The bundle itself is **intentionally thin** — most "real" code lives in
upstream tools (the seven SIN-Code Go tools, the `sin-code-*` Python
subsystems, the `sin-brain` memory cortex). The bundle's job is to:

- detect what's installed,
- generate one-liner MCP / AGENTS.md / skill configs for each agent,
- run the `sin` CLI (status, preflight, mcp-config, skills, ceo-audit, …),
- serve the **unified MCP server** (`sin serve` / `python -m
  sin_code_bundle.mcp_server`) that exposes 24 tools in a single endpoint,
- and ship the `sin-codocs` + `ceo-audit` skills so agents can self-document
  and self-audit.

## Public API

| Symbol | Kind | Purpose |
| --- | --- | --- |
| `__version__` | `str` | Bundle version. PEP 440 / semver. Source of truth for `sin status`, the release workflow, and PyPI. |

Everything else in the package is **submodule surface**; import the submodule
you need (`from sin_code_bundle import codocs, memory, hashline, …`).

## Submodules

| Submodule | Doc | What it does |
| --- | --- | --- |
| `agents_md` | `agents_md.doc.md` | Idempotent AGENTS.md generator (SIN-Code block w/ markers) |
| `ast_edit` | `ast_edit.doc.md` | Tree-sitter-based AST edit (Python/JS/TS/Go/Rust) |
| `bench` | `bench.doc.md` | SWE-bench A/B harness (control vs sin arm) |
| `budget` | `budget.doc.md` | MCP output trimmer (cap lists, truncate strings) |
| `cache` | `cache.doc.md` | Content-hashed GraphCache for impact results |
| `cli` | `cli.doc.md` | The `sin` Typer CLI (status, mcp-config, skills, …) |
| `codocs` | `codocs.doc.md` | Co-located docs standard validator (`.doc.md` companions) |
| `dap_bridge` | `dap_bridge.doc.md` | DAP runtime trace (debugpy / dlv / node) |
| `gitnexus` | `gitnexus.doc.md` | Bridge to the `gitnexus` npm graph-context tool |
| `hashline` | `hashline.doc.md` | Content-hash anchored patching (line-shift resilient) |
| `hooks` | `hooks.doc.md` | `~/.opencode/hooks/` installer (pre/post-command) |
| `interceptor` | `interceptor.doc.md` | ADW pre-flight rule engine (no hardcoded secrets, …) |
| `lsp_backend` | `lsp_backend.doc.md` | Multilspy-driven structural impact (LSP + tree-sitter fallback) |
| `lsp_bootstrap` | `lsp_bootstrap.doc.md` | Language server detection + install hints (`sin doctor`) |
| `markitdown` | `markitdown.doc.md` | Bridge to the `markitdown` document-conversion tool |
| `mcp_config` | `mcp_config.doc.md` | OpenCode/Codex/Hermes MCP config generator + merger |
| `mcp_server` | `mcp_server.doc.md` | Unified MCP server (24 tools, stdio) |
| `memory` | `memory.doc.md` | Thin adapter to the external `sin-brain` memory cortex |
| `orchestration_worktrees` | `orchestration_worktrees.doc.md` | Isolated git worktrees for parallel agent tasks |
| `policy` | `policy.doc.md` | Risk-gating + tamper-evident audit log (`AuditLog`) |
| `rtk` | `rtk.doc.md` | Bridge to the `rtk` token-saving Rust proxy |
| `safety` | `safety.doc.md` | Hardened subprocess + prompt-sanitization helpers |
| `skills` | `skills.doc.md` | Compile portable `skills/*.md` into each agent's native format |
| `vfs` | `vfs.doc.md` | URI-scheme resolver (sckg://, poc://, ibd://, adw://, …) |

## External subsystems (optional, install via `[all]`)

| Package | Role |
| --- | --- |
| `sin-code-sckg` | Semantic Codebase Knowledge Graph (SCKG) |
| `sin-code-ibd` | Intent-Based Diffing (semantic diff + risk scoring) |
| `sin-code-adw` | Architectural Debt Watchdog (complexity + cost tracking) |
| `sin-code-oracle` | Independent execution-based verification |
| `sin-code-poc` | Proof of Correctness (property-based) |
| `sin-code-efsm` | Ephemeral Full-Stack Mock environments |
| `sin-code-orchestration` | Multi-agent task orchestration |
| `sin-code-review-interface` | Semantic review UI |
| `sin-brain` | Memory cortex (SQLite + FTS5, 4-tier recall) |

## Install

```bash
pip install sin-code-bundle           # core
pip install 'sin-code-bundle[all]'    # all 9 optional subsystems
pip install 'sin-code-bundle[mcp]'    # MCP server only
pip install 'sin-code-bundle[bench]'  # SWE-bench A/B harness
```

## CLI

```bash
sin status                # what subsystems are installed?
sin preflight .           # guarantee a fresh GitNexus index
sin mcp-config opencode   # print ready-to-paste opencode.json
sin mcp-config opencode --write
sin serve                 # start the unified MCP server (stdio)
sin skills opencode       # compile skills/*.md into .opencode/command/
sin codocs check .        # verify every # Docs: foo.doc.md resolves
sin ceo-audit run .       # 47-gate, 8-axis SOTA audit
```

## Skills shipped in the bundle

- `sin-codocs` — the CoDocs standard, installable into agent skill dirs
- `ceo-audit` — 47-gate SOTA repository audit (security, perf, quality, …)

## Versioning

Follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html):

- MAJOR = breaking API change
- MINOR = backwards-compatible feature
- PATCH = backwards-compatible bugfix / docs / polish

The current version is `__version__` at the top of this file.

## License

MIT — see the project root `LICENSE` file. External tools (GitNexus,
MarkItDown, RTK) are linked at runtime via their own licenses and are
NOT vendored.
