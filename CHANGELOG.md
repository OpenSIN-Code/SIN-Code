# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
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
