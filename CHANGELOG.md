# Changelog

All notable changes to the SIN-Code unified binary will be documented in this file.

## [v3.9.0] - 2026-06-13
### Added
- **GitHub CLI bridge** (`internal/ghbridge/`): bridged external (NEVER vendored) for the official `gh` CLI. 3-tier verb policy enforced in code: read-only (allow) | mutating (ask) | forbidden (hard-blocked). 3 MCP tools: `gh_query` (allow), `gh_execute` (ask), `gh_health` (allow). Enables the SIN-Code contributing workflow "issue first" to be executed by the agent itself.
- New subcommand: `gh` (setup/doctor/run/surface/serve). 35 → 36.
- Permission-Defaults: `gh_query`/`gh_health` → allow, `gh_execute` → ask.
### Security
- Defense in depth: `gh_query` re-validates with `Classify` and rejects mutations even if caller picked wrong tool.
- Fail-closed: unknown verbs/groups → `TierForbidden`, never reach runner.
- `gh api`, `gh auth`, `gh secret`, `gh config`, `gh alias`, `gh extension`, `gh codespace`, `gh fork`, `gh sync`, `gh archive/unarchive/transfer`, `gh ssh-key`, `gh gpg-key` are hard-blocked.
### Mandate Compliance
- M1 n8n-CI only ✓
- M2 CGo-free, stdlib-only ✓
- M3 Verification-Gate passed: build OK, vet OK, race OK
- M4 3-tier policy matches permission engine ✓
- M5 Module path correct ✓
- M7 Race-clean ✓

## [v3.8.0] - 2026-06-13

### Added
- **Vane bridge** (`internal/vane/`): HTTP-Bridge zur ItzCrazyKns/Vane (MIT) self-hosted AI-answering-engine mit zitierten Quellen. stdlib-only, stdio MCP server (2 tools: `vane_research`, `vane_health`), graceful degradation → websearch fallback. Closes #62.
- **Stack consolidation** (`internal/stack/`): unified `sin-code stack install|doctor` über superpowers + dox + vane. Idempotent, --json output, graceful degradation pro layer. Closes #62.
- New subcommands: `vane` (setup/doctor/search/config/serve), `stack` (install/doctor). 33 → 35.
- New MCP servers in `.sin-code/mcp.json`: `vane` (2 tools), plus pre-existing `superpowers` (3 tools) and `dox` (0 tools, protocol-block based).

### Mandate Compliance
- M1 n8n-CI only ✓
- M2 CGo-free, stdlib-only ✓
- M3 Verification-Gate: PoC + Oracle (commit-time) ✓
- M4 Permission-Defaults updated, ecosystem-sync green ✓
- M5 Module path correct ✓
- M7 Race-clean (tested with -race -count=1) ✓

## [3.7.0] - 2026-06-12

### Added
- **`sin-code superpowers`** — integration of obra/superpowers (MIT)
  methodology skills into the SIN-Code agent. Skills (TDD,
  systematic-debugging, subagent-driven-development, verification-before-
  completion, writing-plans, brainstorming, requesting-code-review,
  finishing-a-development-branch, using-git-worktrees) are cloned from
  upstream, pinned to a reviewed commit SHA (supply-chain lock), overlaid
  with SIN-Code tool mappings (M6: sin_* tools over naive builtins), and
  served as MCP tools (`superpowers_list_skills`, `superpowers_find_skill`,
  `superpowers_use_skill`).
- **Review-before-trust update flow:** `sin-code superpowers update`
  shows the upstream skill diff first; applies + re-pins only with
  `--yes` (skill content flows into agent context — must be reviewed
  like a dependency bump).
- **Full YAML frontmatter parser:** handles plain values, quoted strings,
  folded block scalars (>–), literal block scalars (|–), and indented
  continuations — all forms used by upstream superpowers.
- **AGENTS.md auto-injection:** `sin-code superpowers init` adds a
  Superpowers prompt block (bounded by `<!-- SUPERPOWERS:BEGIN/END -->`)
  making skill usage a mandatory agent workflow.
- **Defense-in-depth:** skills are NOT destructive (overlay on top of
  upstream files), idempotent (re-install = no-op), and pinned (no
  automatic `git pull` of new content into agent context).

## [3.6.0] - 2026-06-12

### Added
- **Swarm mode** — `sin-code swarm -p <prompt> --agents <n1,n2,n3>`. N agent
  profiles race the same prompt headless; first verified solution wins.
  Per-agent isolated sessions. Cancellation via parent context.
  Mandate M4 holds (headless ask->deny).
- **Self-extending agent** — `sin_bootstrap_skill` tool. Agent writes
  Python MCP servers from natural-language specs, smoke-tests them,
  and registers in `.sin-code/mcp.json`. Defense-in-depth: permission
  policy "ask" + env gate `SIN_ALLOW_BOOTSTRAP=1` for headless use.
- **TUI v3.3.1** — `internal/tui/agent_runner.go` (84.6% cov). TUI embeds
  the real agent loop. Skill palette entries execute live instead of
  printing CLI hints. Permission asks render as TUI dialogs (y/N) over
  the AskReply channel.
- **WebUI-v2 backend API** — `internal/apiweb/api.go` (81.5% cov). 6
  HTTP endpoints (sessions CRUD, fork, knowledge, chat-with-SSE) with
  bearer-token auth via `SIN_API_TOKEN` and localhost-only fallback.
  Mounted by `sin-code serve --transport=http`. Chat endpoint streams
  progress as SSE events, final frame is the stable JSON contract
  `{session_id, summary, verified, turns}`.

## [3.5.0] - 2026-06-12

### Added
- `internal/lessons` — persistent knowledge base (SQLite, modernc);
  failed verifications and tool errors accumulate with occurrence
  counts. `lessons.Briefing` injects top repeated lessons before the
  first turn (singletons are noise, repetition is signal).
- `internal/loopbuilder` — shared factory eliminates duplication of
  provider/permission/hooks/gate/mcp/lessons setup across chat/swarm/
  serve (DRY refactor).
- agentloop.Loop gained `Lessons` field; on verify.fail / tool.error
  the lesson is recorded. On Run() start, the briefing is injected
  before the first turn.
- `internal/mcpclient` — `server__tool` namespacing, LoadConfigs with
  mcp.json discovery (merge defaults + user + workspace), registry of
  13 ecosystem servers (12 skills + Symfony-Lens).
- `sin-code mcp list|status|call` — live MCP debugging.
- Chat command suite (chat_cmd.go, chat_mcp.go, chat_tools.go):
  interactive REPL + headless one-shot with stable JSON contract.
- `sin-code sessions list|show|rm` — persistent resumable sessions
  over `~/.local/share/sin-code/sessions.db` (modernc, foreign_keys=ON).
- Ecosystem consolidation: ECOSYSTEM.md (24 ACTIVE repos + sync rules),
  requirements-ecosystem.txt (8→24 entries), profiles/*.toml
  (fireworks, qwen-relay), docs/HOOKS.md, docs/WEBUI.md,
  docs/mcp.json.example.
- .github/workflows/ecosystem-sync.yml — CI gate preventing drift
  between registry.go, permission_defaults.go, ECOSYSTEM.md,
  requirements-ecosystem.txt.
- Goal-queue + autonomy: persistent SQLite queue, atomic leases,
  cron + file-watch triggers, skill-lifecycle manager.
- 7 new hook events: goal.enqueued/started/verified/exhausted,
  trigger.fired, skill.installed/failed.
- `sin-code daemon --verify-cmd` — autonomous worker (M3+M4 enforced).
- `sin-code goal add|list` and `sin-code skill install|status`.

## [3.4.0] - 2026-06-12

### Added
- Einstein Layer — the agent that learns from mistakes.

## [Unreleased]

### Added
- **LSP integration dependencies** — `sin-code lsp` now documents its gopls
  requirement. Install via `brew install gopls` (macOS) or
  `go install golang.org/x/tools/gopls@latest` (Linux/CI). Without gopls on
  `$PATH`, Go-language LSP commands degrade gracefully to a "gopls not
  detected" message (see `sin-code lsp servers`).
- **Live LSP regression testscript** — `cmd/sin-code/testdata/scripts/lsp_live.txt`
  exercises symbols / hover / definition / references / format against this
  repository. Added so the LSP client can be re-validated whenever `client.go`
  changes.

### Known Issues
- **LSP framing bug** — `internal/lsp/client.go:Client.Call` reads LSP responses
  one line at a time with `bufio.ReadString('\n')`, but gopls v0.20+ emits
  JSON-RPC notifications (e.g. `window/logMessage`, `$/progress`) on the same
  stdout stream. The header parser only recognises `Content-Length:` lines, so
  notification lines desync the reader, and subsequent `io.ReadFull` returns a
  truncated body. Visible as
  `Error: initialize go: unexpected end of JSON input`
  on every `sin-code lsp {symbols,hover,definition,references,format}` call.
  Workaround: pin gopls to v0.16.x or rewrite `Call` to use
  `bufio.Scanner` with a custom split function that tolerates interleaved
  notifications. Tracked in follow-up issue (see `docs/lsp-known-issues.md`).

## [2.5.0] - 2026-06-11

### Added
- **Persistent Incremental Index (Phase 3)** — gob-persisted trigram + symbol
  index at `<root>/.sin-code/index.bin`. Auto-builds on first search,
  stat-based incremental refresh, 8 parallel build workers. New `index`
  subcommand (build/refresh/status/watch/clear) and MCP `sin_index` tool.
  Scout CLI now uses indexed search with 25-37× speedup over full scan.
- **AST tiered structure extraction (Phase 4)** — 3-tier provider (Go go/ast
  exact, structural fallback, tree-sitter opt-in via `-tags treesitter`).
  Default build stays zero-dep. Enables `read --mode outline` with engine
  info, `edit --symbol NAME` for AST-anchored edits, and unified parsing
  across all consumers.
- **Phase 4b — grasp/map/SCKG migrated to parseOutline()** — removed 5
  regex-based per-language extractors in `grasp.go`, replaced with single
  `parseOutline()` call. SCKG `buildGraph` now uses `parseOutline` for all
  languages (no more regex for Python/JS). Map entry-point detection uses
  `isGoEntryPoint()` via AST lookup. Kind normalization helpers
  (`normalizeGraspKind`, `sckgKind`) maintain backward-compatible labels.
- **Phase 5 — Benchmark suite + CI gate** — 18 Go benchmarks across all
  tools with synthetic project trees (`makeTree()`), `benchmark.sh` shell
  runner with pprof profiling (`PPROF=1` mode), `.github/workflows/go-ci.yml`
  with median speedup gate (≥3× indexed vs fullscan on CI runners).
  BenchmarkComparisonTable directly compares fullscan vs indexed sub-bench.

### Changed
- **Go upgraded to 1.25.11** — was 1.24.3 (ADR-008, st-gvc4). go.mod
  updated, CI workflows updated, govulncheck switched from warn-only to
  blocking (Go 1.25 fixed the stdlib false positives that required the
  carve-out). ADR-008 marked as Superseded.
- **Coverage corrected** — the 93.6% claim in v1.0.9 was for the cmd/sin-code
  package only. Full project coverage (including internal/ and all
  sub-packages: plugins, lsp, memory, todo, notifications, orchestrator,
  webui, llm, attachments, tui, tui/chat) is 68.2% as of this release.
  Goal for v2.6.0: raise internal/ coverage to ≥80%.

### Fixed
- **st-pwt5** — `testdata/scripts/plugin_wire.txt` manifest was using
  deprecated v2.3.0 minimal format. Updated to current TOML schema
  (description, provider, timeout, capabilities, populated agents/tools)
  so the test exercises the modern manifest shape, not the deprecated
  one. Added descriptive comment at top of the testscript.
- **CI benchmark gate** — was using integer-only bash arithmetic that crashed
  on float ns/op values, and used `sort -n | head -1` (minimum) which biased
  against the indexed path. Now uses float-safe awk with median calculation
  and a 3× threshold (was 5× — too aggressive for 2-4 core CI runners).
- **Legacy Python CI** — `ci.yml` was red on every Go commit because the
  deprecated Python stack still ran ruff + pytest. Added path filters so
  it only triggers on `**.py` / `pyproject.toml` / etc.

### Closed Issues
- st-gvc4 (govulncheck blocking) — P3
- st-pwt5 (plugin_wire test) — P2
- st-phw1 (plugin hook wiring) — P0 [closed retroactively, fixed in Phase 3/4]
- st-ptm2 (plugin tools → MCP) — P0 [closed retroactively, fixed in Phase 3/4]

## [2.4.0] - 2026-06-08

LSP framing fix, plugin system, multi-agent orchestrator, TUI chat LLM, NIM
model aliases. See commit `63b33f5` for the full list of changes.

## [1.1.0] - 2026-06-07

### Added
- **TUI 2.0** — complete rewrite of `sin-code tui` as a multi-pane command center
  - Session tab bar (top, up to 6 sessions)
  - Collapsible left sidebar (Ctrl+B) with 5 views + 19 subcommands
  - Custom SIN-Code loading animation (rotating half-block halo + ⚡)
  - Bottom footer with view name, agent (Build/Audit/Stats), token stats, cost
  - Command palette (Ctrl+P), subagents popup (Ctrl+X), view switcher
  - 5 themes: default, Dracula, Nord, Solarized, Monokai
  - Multi-view support: Tools, Sessions, EFM, Config, History
- **EFM OrbStack support** — auto-detect `orb` on macOS, `--runtime orb|docker|auto` flag
- **OrbStack mandate** (PRIORITY -5.0) — added to all 3 AGENTS.md files
- **TUI design doc** — `docs/tui-v2-design.md` (1,319 lines, opencode research)

### Changed
- TUI moved to dedicated `cmd/sin-code/tui/` package (~2,900 LOC, 15 files)
- Old monolithic `tui_test.go` + `tui_interactive_test.go` removed (replaced by 61 new tests)

### Architecture
- Bubbletea v1.3.10 (matches go.mod)
- 5 themes via Lipgloss, multi-pane via lipgloss.JoinHorizontal/Vertical

## [1.0.9] - 2026-06-07

### Added
- 448 new tests bringing coverage from 82.7% to 93.6%
- serve_handlers_test.go: all 13 MCP handleXxx functions + runSubcommand (1136 lines)
- execute_extended_test.go: 55+ tests for runCommand, checkSafety, redactSecrets, signal handling
- main_subprocess_test.go: 11 tests for main() symlink routing + checkUpdate
- efm_test.go: expanded from 14 → 44 tests with Docker skip logic
- sbom_test.go: expanded from 16 → 45 tests, CycloneDX + edge cases
- All 12 core/advanced files pushed to 95%+ coverage

### Changed
- sbom.go: fix parseGoModFallback single-require parsing bug
- Coverage increased from 82.7% to 93.6% (+10.9%)
- Total tests: 415 → 863
- Files at 95%+ coverage: 0/20 → 17/20

## [1.0.8] - 2026-06-07

### Added
- 84 new tests bringing coverage from 73.6% to 82.7%
- self_update_test.go: 30 tests with httptest mocks for GitHub API, tar.gz/zip extraction, downloadFile
- security_extended_test.go: 28 tests for tool runners (govulncheck, gosec, bandit, safety, npm audit, secrets-grep, file-permissions)
- main_extended_test.go: 11 tests for checkUpdate stamp logic + symlink routing
- common_test.go: 7 tests for PrintError, lookupStandalone, capitalize
- config_test.go: +12 tests for get/set roundtrip, list, path, init, persist/reload

### Changed
- self-update.go: extract githubAPIURL var for testability (was hardcoded URL)
- Test coverage increased from 73.6% to 82.7% (+9.1%)
- Total tests: 331 → 415

## [1.0.7] - 2026-06-07

### Added
- 200+ new tests (unit + E2E + MCP integration)
- 7 new dedicated test files (ibd, poc, sckg, efm, grasp, map, scout)
- testscript E2E framework (9 CLI tests)
- MCP server stdio integration tests (10 stdio + 9 integration)
- Dependency: rogpeppe/go-internal v1.15.0 for testscript

### Changed
- Test coverage increased from 48.4% to 72.2%
- Documentation: corrected tool counts across AGENTS.md, main.go, serve.go (19 subcommands = 13 MCP + 6 CLI-only)

## [1.0.4] - 2026-06-07

### Added
- `security` subcommand — auto-detects project type (Go/Python/Node/Generic) and runs available security tools
- `config` subcommand — manages sin-code configuration (get, set, list, path, init)
- `self-update` subcommand — checks GitHub releases and installs latest binary with backup/restore
- TUI themes — 5 built-in color schemes (default, Dracula, Nord, Solarized, Monokai)
- TUI arg-input mode — press 'r' and enter arguments for commands that need them
- Daily update availability check — non-blocking, runs once per day when --version is used
- Windows zip extraction in self-update (archive/zip support)

### Changed
- Pipeline: govulncheck non-blocking (Go 1.24.3 stdlib CVEs fixed in Go 1.25)
- TUI status bar shows dynamic hints per command (Enter: --help, r: run, t: theme, q: quit)
- Homebrew formula updated for v1.0.4 with SHA-256 checksums

### Fixed
- Go version compatibility: downgraded to Go 1.24.3 with compatible dependencies
- Release pipeline: multiple hotfixes for Go toolchain, artifact upload, cross-compilation
- GitNexus index rebuilt with 9,997 symbols and 17,832 relationships
- AGENTS.md synced across all 3 repos (SIN-Code-Bundle, Infra-SIN-OpenCode-Stack, ~/.config/opencode)

## [1.0.3] - 2026-06-07

### Added
- `tui` subcommand — interactive Bubbletea menu for all subcommands with fallback

### Fixed
- Pipeline hardened: go vet blocking, govulncheck non-blocking with artifact upload

## [1.0.2] - 2026-06-07

### Added
- 13 core tools in unified Go binary: discover, execute, map, grasp, scout, harvest, orchestrate, ibd, poc, sckg, adw, oracle, efm
- MCP server mode (`serve`) exposing all 13 tools via JSON-RPC 2.0 stdio
- Symlink backwards compatibility (`discover`, `execute`, etc. → `sin-code`)
- 5-platform release pipeline (darwin/linux × amd64/arm64 + windows-amd64)
- Homebrew formula and tap repo (`OpenSIN-Code/homebrew-sin`)

## [1.0.0] - 2026-06-04

### Added
- Initial release of 7 standalone Python tools (discover, execute, map, grasp, scout, harvest, orchestrate)
- CEOAudit grade A+ (100.0/100)
