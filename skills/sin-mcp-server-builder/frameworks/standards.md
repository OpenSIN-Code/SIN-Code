# Frameworks: Standards & Constraints

Docs: ../SKILL.md

## Technology Stack

- Templates: `python-fastmcp`, `node-mcp`, `go-mcp`.
- FastMCP >= 0.3.0, jinja2 >= 3.0.0.
- Canonical OpenSIN-Code project layout.

## Standards

- Every project has `pyproject.toml` / `package.json` / `go.mod`, tests, CoDocs, and `ceo-audit.yml`.
- Every tool has type hints and docstrings.
- Every tool has a generated test.
- Pass CEO audit before publish.

## Constraints

- No publishing without validation.
- No audit skip.
- Register in `opencode.json` after install.

## Quality Gates

- Scaffold successful.
- Tests pass.
- Validation passes.
- CEO audit passes.
- Server registered.
