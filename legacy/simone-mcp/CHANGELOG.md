# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial SIN-Code-Bundle integration (ceo-audit workflow v3)
- OpenCode MCP server registration under `OpenSIN-Code/Simone-MCP`
- Repository-level `SIN_GITHUB_FALLBACK_TOKEN` secret for the App commenter fallback
- Simone MCP server — LSP-grade code analysis tools via MCP/JSON-RPC
- `sin_simone_mcp_symbol_search`, `sin_simone_mcp_find_references`, `sin_simone_mcp_project_overview`
- `sin_simone_mcp_structural_edit`, `sin_simone_mcp_memory_query`, `sin_simone_mcp_health`
- FastAPI + Python 3.12+ backend with Docker and `install.sh`
- `agent.json` card, A2A-CARD.md, and full `docs/` set
- `landing-worker` and `clients/` reference implementations

### Security
- All commits verified via `git-immortal-commit` (annotated tags)
- MIT license, CONTRIBUTING.md, CONFIGURATION.md, INSTALL.md provided

