# `main.go`

## What it does
Entry point for the `{{ name }}` MCP server, written in Go. Registers every
generated tool with the `github.com/modelcontextprotocol/go-sdk` and runs
over stdio.

## Dependencies
- `github.com/modelcontextprotocol/go-sdk`

## Tools exposed
{% for tool in tools -%}
- `{{ tool }}`
{% endfor %}

## Usage
```bash
go run main.go
```
