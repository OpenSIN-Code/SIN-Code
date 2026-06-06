# `src/index.js`

## What it does
Entry point for the `{{ name }}` MCP server. Wires up the `@modelcontextprotocol/sdk`
Server with a `StdioServerTransport` and registers every tool generated
by `sin-mcp-server-builder`.

## Dependencies
- `@modelcontextprotocol/sdk` (the official MCP SDK for Node.js)

## Tools exposed
{% for tool in tools -%}
- `{{ tool }}`
{% endfor %}

## Usage
```bash
npm start
```
