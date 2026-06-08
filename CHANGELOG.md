# Changelog

All notable changes to the SIN-Code unified binary will be documented in this file.

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
