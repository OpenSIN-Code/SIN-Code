# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
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

## [0.1.0] - 2026-05-30

### Added
- Initial public release.
- Core library modules, CLI entry point, and test suite.
- Graceful degradation when optional external tools are unavailable.

### Notes
- This is an early release of the SIN-Code agent-engineering stack. APIs may
  still change before 1.0.0.
