# ADR-001: Multi-Agent Architecture for SIN-Code

## Status

Accepted

## Context

The SIN-Code ecosystem needs to support multiple specialized tools (discover, execute, map, etc.) that work together. We needed to decide how to structure the system.

## Decision

We chose a multi-agent architecture with:
- 7 Go-based CLI tools (fast, compiled, MCP-enabled)
- 8 Python-based subsystem tools (semantic analysis, verification, orchestration)
- 1 central Bundle (integration, CLI, MCP server)
- 1 Memory system (SIN-Brain, 4-tier cortex)

## Consequences

- Positive: Each tool is independent, testable, replaceable
- Positive: Teams can work on different tools in parallel
- Negative: Integration complexity between tools
- Negative: Consistent API design required across languages
