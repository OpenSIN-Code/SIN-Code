# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Performance monitoring** across all 16 SIN-Code repos:
  - 600x speedup for Discover 500-1000 file projects (parallel analysis + content cache)
  - 1200x speedup for SCKG 10000+ node queries (pre-built adjacency indexes)
  - Extension filter optimization via string suffix matching
- **Test stabilization** for all 7 Go tools + 8 Python subsystems:
  - Fixed zsh compatibility in Execute tests
  - Fixed secret redaction patterns (secret_key, private_key, bearer)
  - Fixed macOS /private symlink handling in project root detection
  - Fixed JSON parsing tests for nested objects
  - Fixed process group timeout tests
  - All 472+ tests passing across all repos

### Changed
- Version bump to 0.3.6 to align with Go tool releases

## [0.1.0] - 2026-05-30

### Added
- Initial public release.
- Core library modules, CLI entry point, and test suite.
- Graceful degradation when optional external tools are unavailable.

### Notes
- This is an early release of the SIN-Code agent-engineering stack. APIs may
  still change before 1.0.0.
