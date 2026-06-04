# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Operational hardening** (closes #8): production-readiness CI/release tooling.
  - `.github/workflows/ci.yml`: `ruff check` + `ruff format --check` lint gate
    and a `pytest` matrix across Python 3.11/3.12/3.13, plus a non-blocking
    cross-repo consistency job.
  - `.github/workflows/release.yml`: builds sdist+wheel on `v*` tags, verifies a
    clean-env install, attaches artifacts to a GitHub Release, and publishes to
    PyPI via Trusted Publishing.
  - `scripts/check_consistency.py` (WS4): asserts version alignment, subsystem
    import health, and that every `sin mcp-config` client points at the real
    `sin serve` entry point. `--strict` mode for full multi-repo CI.
  - `scripts/dev_install.sh` + `scripts/run_all_tests.sh` (WS5): two-command
    editable bootstrap and aggregated test runner across all 8 sibling repos.
  - Adopted a shared `ruff` config (E/F/I/W) and applied a one-shot mechanical
    format; aligned `__version__` with the packaged `0.2.0`.
- **GitNexus bridge** (`sin_code_bundle.gitnexus`): integrates the upstream
  [GitNexus](https://github.com/abhigyanpatwari/GitNexus) code knowledge graph
  as a mandatory, always-on context source for coder agents. GitNexus is
  invoked via `npx` (not vendored), keeping the bundle MIT-licensed while
  GitNexus stays PolyForm-Noncommercial upstream.
  - `sin gitnexus setup` wires the GitNexus MCP server into OpenCode, Codex,
    and Hermes configs (idempotent, preserves existing config).
  - `sin preflight` auto-builds/refreshes the graph so agents never code blind.
  - `sin gitnexus index|status|doctor|context|impact|ai-context` commands.
  - `gitnexus_context`, `gitnexus_impact`, `gitnexus_ai_context` exposed via
    `sin serve`; GitNexus availability shown in `sin status`.
  - Docs at `docs/GITNEXUS.md`; requires Node.js >= 18.
- **CoDocs** integration, merged from the former
  `SIN-Hermes-Bundles/SIN-Code-CoDocs-Bundle` repo:
  - `sin_code_bundle.codocs` — a robust, stdlib-only validator that replaces the
    original fragile `grep | sed` one-liner.
  - `sin codocs check`, `sin codocs list`, and `sin codocs install-skill` CLI
    commands, plus a `codocs_check` MCP tool exposed via `sin serve`.
  - Packaged agent skill (`data/codocs/SKILL.md`), `docs/CODOCS.md`, and a
    worked example under `examples/codocs/`.

## [0.1.0] - 2026-05-30

### Added
- Initial public release.
- Core library modules, CLI entry point, and test suite.
- Graceful degradation when optional external tools are unavailable.

### Notes
- This is an early release of the SIN-Code agent-engineering stack. APIs may
  still change before 1.0.0.

## [0.3.0] - 2026-06-04 — SOTA MCP Tools

### Added
- **5 core MCP tools** in `sin serve` to REPLACE native opencode read/write/edit/bash/search:
  - `sin_read` — URI-scheme aware (sckg://, poc://, ibd://, adw://, efsm://, oracle://, conflict://) + size-safe file reading with `summarize` mode
  - `sin_write` — atomic write with auto-backup, syntax pre-validation for .py/.ts/.js/.go
  - `sin_edit` — hashline-anchored semantic patching (line-shift resilient)
  - `sin_bash` — safe shell exec with secret-redaction + timeout + structured result
  - `sin_search` — wraps `scout` Go tool (semantic/regex/symbol/usage), Python-regex fallback
- AGENTS.md mandate: `tools.{read,write,edit,bash,search,find,grep,glob,list,webfetch,task} = false` in `~/.config/opencode/opencode.json`
- AGENTS.md SIN-Tools-Only Mandat section (PRIORITY -10.0) in BOTH `~/.config/opencode/AGENTS.md` and Infra repo

### Changed
- `sin serve` MCP server now exposes 10 tools (was 8)
- All 30 OpenSIN-Code repos received `ceo-audit.yml v3` with `SIN_GITHUB_FALLBACK_TOKEN` env

### Verified
- Live test on SIN-Code-Bundle push: A+ (100.0/100), 0 Critical, 0 High
- 10 tools returned by `tools/list` MCP call

## [0.4.0] - 2026-06-04 — ALL SUBSYSTEMS + SIN-BRAIN in [all] extra

### Added
- **`pip install sin-code-bundle[all]`** installs the COMPLETE SOTA agent stack in one command:
  - 8 SIN-Code subsystem packages (sckg, ibd, poc, efsm, adw, oracle, orchestration, review-interface)
  - sin-brain (memory cortex with 5 tools)
  - LSP deps (tree-sitter + 4 per-language parsers: Python/JS/TS/Go)
  - bench, mcp, otel, dev extras
- `pyproject.toml` now has 9 new extras: `sckg`, `ibd`, `poc`, `efsm`, `adw`, `oracle`, `orchestration`, `review`, `memory`
- Tree-sitter switched from `tree-sitter-languages` (no Py3.14 wheel) to direct bindings:
  - `tree-sitter>=0.23` + `tree-sitter-{python,javascript,typescript,go}>=0.23`
- `sin serve` now exposes **24 MCP tools** (was 10):
  - 5 core file-ops: sin_read, sin_write, sin_edit, sin_bash, sin_search
  - 9 subsystem tools: impact, semantic_diff, architectural_debt, verify_tests, prove, mock_env, orchestrate, task_status, semantic_review
  - 5 memory tools: recall_tool, remember_tool, forget_tool, pin_tool, link_evidence_tool
  - 4 gitnexus + 1 markitdown + 1 codocs_check

### Fixed
- LSP dep Python 3.14 compat (tree-sitter-languages workaround)

### Verified
- All 9 subsystem Python packages importable
- `sin serve` exposes 24 tools via `tools/list` MCP handshake
- `pip install -e .[all]` completes successfully

## [0.5.0] - 2026-06-04 — Standalone mcp_server.py + 28 tools + README coverage

### Added
- **`src/sin_code_bundle/mcp_server.py`** — standalone MCP server module
  (in addition to `cli.py::serve`). Invoke via:
  - `python -m sin_code_bundle.mcp_server`
  - `sin-serve` console script
  - `sin serve` (legacy, identical)
- **2 new console scripts**: `sin-serve`, `sin-serve-mcp`
- **4 new MCP tools** (sin serve now exposes **28** instead of 24):
  - `sin_vfs_resolve` — resolve a SIN URI scheme (`sckg://`, `poc://`, etc.) to structured content
  - `sin_vfs_schemes` — list all available URI schemes with descriptions
  - `sin_ast_edit` — tree-sitter AST-based edit with POC verification, falls back to hashline
  - `sin_hashline_validate` — validate a previously-created hashline patch can still be applied
- **`review` tool** from sin-code-review-interface subsystem
- **README**: full tool-coverage table (native → SIN) + 28-tool MCP inventory
  + console script examples + install verification

### Changed
- `pyproject.toml` adds `sin-serve` and `sin-serve-mcp` console scripts
- Bundle version bumped to 0.5.0

### Verified
- `sin-serve` MCP handshake returns **28 tools**
- All 4 new tools (sin_vfs_resolve, sin_vfs_schemes, sin_ast_edit, sin_hashline_validate) functional
- `sin serve` (legacy) still works, returns same 28 tools
