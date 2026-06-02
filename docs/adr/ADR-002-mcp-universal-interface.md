# ADR-002: MCP (Model Context Protocol) as Universal Tool Interface

## Status

Accepted

## Context

Tools need to be callable by AI agents (OpenCode, Claude, Codex). We needed a standard protocol.

## Decision

Use MCP (JSON-RPC 2.0 over stdio) as the universal interface:
- All 7 Go tools expose `--mcp` flag
- All 8 Python subsystems expose `mcp_server.py` modules
- Bundle exposes `sin serve` (FastMCP)
- SIN-Brain exposes memory tools via MCP

## Consequences

- Positive: Single integration pattern for all agents
- Positive: Standardized request/response format
- Negative: JSON serialization overhead
- Negative: Requires MCP library support
