# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.6.4] - 2026-06-04 — CoDocs polish: section separators + magic values

### Changed
- Section separators (`# ── X ───...`) added to all 17 files in `src/` that are
  ≥100 lines, using the standard Unicode box-drawing format (the few files that
  used the older `# --- # ... # --- #` style were normalised to the canonical
  one-liner form).
- Magic-value rationales added to: `cli.py` (mock_env port 8888 = EFSM default,
  sin_bash timeout 60s, search-result cap 200), `mcp_server.py` (same), `bench.py`
  (limit=20 = smoke-test size for SWE-bench Lite), `gitnexus.py` (default
  timeouts 900/1800/300s), `markitdown.py` (convert timeout 300s), `lsp_backend.py`
  (25-caller cap), `safety.py` (max_len 8000 ≈ 2K tokens), `vfs.py` (git diff
  timeout 10s), `rtk.py` (rtk init timeout 120s).
- "Why not obvious" comments added to non-trivial logic blocks: worktree
  sibling-dir rationale (`orchestration_worktrees.py`), TOML strip-table
  trade-off explanation (`mcp_config.py`), `_score_risk` threshold semantics
  (`lsp_backend.py`).
- `src/sin_code_bundle/__init__.doc.md` created (package overview, public API,
  all 24 submodules, optional subsystems, install matrix, CLI examples,
  skills shipped, versioning policy, MIT license note). The package-level
  `__init__.py` now points at it via the standard `Docs:` reference.

### Verified
- All **25/25** `.py` files in `src/` have `.doc.md` companions (was 24/25).
- All 17 files ≥100 lines now have section separators.
- All magic port/timeout/threshold/limit constants have inline rationale.
- `pytest tests/ -q` → 165 passed, 3 pre-existing failures (test_memory
  ones assume sin-brain is absent, but sin-brain IS installed in this
  environment; test_consistency_strict requires a sin-code-*-free checkout).
  No new failures introduced by this change.

## [0.6.3] - 2026-06-04

### Fixed
- CoDocs compliance: added the 4 missing `.doc.md` companion files
  (`dap_bridge.doc.md`, `interceptor.doc.md`, `orchestration_worktrees.doc.md`,
  `mcp_server.doc.md`). Inline header references that previously pointed at
  non-existent companion docs are now satisfied. 24/25 source files have
  companions (the 25th, `__init__.py`, is a version-info file with no
  `Docs:` reference — intentionally no companion).

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

### Removed
- **Dead Honcho / in-bundle `SINMemory` code paths.** The CLI sub-commands
  `sin memory {retain,recall,reflect,stats,forget,honcho-status,honcho-retain,honcho-chat}`
  and `sin context query` referenced `SINMemory` and `HonchoBackend` classes
  that were moved to the external `sin-brain` package (commit `af69464`,
  BR-1, Issue #14). Running any of those commands raised `ImportError`. The
  bundle's `memory.py` is now an honest thin pass-through to
  `sin_brain.mcp_tools`; the five memory operations are exposed only as MCP
  tools (`recall`, `remember`, `forget`, `pin`, `link_evidence`) registered
  by `sin serve`, not as a CLI surface. Honcho integration is out of scope
  for this bundle: install it separately (`pip install honcho-ai`,
  `honcho serve`) and call it from your own application code.
  - `src/sin_code_bundle/cli.py`: 204 lines of dead code removed (the entire
    `memory_app` and `context_app` typer sub-apps).
  - `tests/test_v2_features.py`: 12 perpetually-skipped tests removed
    (the entire `Memory: SQLite + Honcho Backend` section + the
    `_skip_memory_v2` machinery); file 367 → 142 lines.
  - `src/sin_code_bundle/memory.doc.md`: rewritten from 178 lines of stale
    architecture description to 151 lines of honest current-state
    documentation (thin adapter to `sin_brain`, no Honcho).
  - `tests/test_v2_features.doc.md`: test count + Honcho notes corrected
    (19 → 11, no Honcho section).
  - `SECURITY.md`: `Honcho peer memory` row + `HONCHO_API_KEY` hint
    replaced with a `sin-brain memory` row that reflects the real
    deployment.

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

## [0.5.1] - 2026-06-04 — README "Why better" column

### Changed
- **README.md** MCP tools table now has a **"Why better than native / other tools"**
  column with concrete examples (e.g. why `sin_read` > native `read`,
  why `sin_edit` > native `edit` for line-shift resilience, etc.)
- Bundle version bumped to 0.5.1

## [0.6.0] - 2026-06-04 — DAP + Interceptors + Worktrees + Tests

### Added
- **DAP Runtime Bridge** (`dap_bridge.py`): Attach real debuggers (debugpy for Python,
  dlv for Go, node --inspect for Node/JS/TS) to inspect runtime state. Stores trace
  facts in sin-brain.
- **Rule Interceptor** (`interceptor.py`): Pre-flight architectural rule
  enforcement. Blocks hardcoded secrets, frontend-DB imports, eval/exec patterns.
  Loads dynamic rules from sin-code-adw if installed.
- **Isolated Worktrees** (`orchestration_worktrees.py`): Parallel agent task
  execution via git worktrees. Optional merge-back to main.
- **5 new MCP tools**: `sin_runtime_trace`, `sin_stop_trace`,
  `sin_check_architecture`, `sin_create_worktree`, `sin_cleanup_worktree`.
- **Integration tests** (`tests/test_mcp_integration.py`): 10+ tests covering
  Interceptor, DAP Bridge, Worktree Orchestrator with graceful degradation.

### Verified
- `pytest tests/test_mcp_integration.py -v` — all tests pass
- `sin-serve` MCP handshake returns **34 tools** (was 29)
- Bundle version bumped to 0.6.0

## [0.6.2] - 2026-06-04 — uninstall.sh + update.sh

### Added
- `uninstall.sh` — Symmetric counterpart to install.sh. Removes the 7 Go tools,
  the 8 Python subsystem packages, sin-brain, the Python bundle, and
  un-registers all MCP servers from `~/.config/opencode/opencode.json`.
  Flags: `--dry-run`, `--verbose`, `--force`, `--keep-config`, `--keep-bundle`,
  `--keep-go`, `--keep-subsystems`.
- `update.sh` — In-place update. git pull, pip install --upgrade for bundle +
  subsystems, force-rebuild Go tools, re-register MCP. Flags: `--force-rebuild`,
  `--skip-go`, `--skip-external`, `--skip-pull`, `--subsystems-dir=PATH`.
- CoDocs: `uninstall.sh.doc.md`, `update.sh.doc.md`
- README "Quickstart" now documents the uninstall/update pair commands.

### Verified
- `bash uninstall.sh --dry-run` — previews all 4 stages (Go binaries, Python
  bundle, 8 subsystems + sin-brain, opencode.json MCP block)
- `bash update.sh --dry-run` — previews all 6 stages (pull, bundle upgrade,
  8 subsystems, 7 Go builds, MCP re-registration, sin status)
- Idempotency: both scripts re-runnable with no errors (missing items skipped)
- `--help` exits with code 2 (matches install.sh convention)
- Bundle version bumped to 0.6.2
