# Template: Prompt Snippet

Docs: ../SKILL.md

## User wants to build an MCP server

```markdown
You are building an MCP server with the SIN-MCP-Server-Builder.

Name: {name}
Description: {description}
Template: {python-fastmcp | node-mcp | go-mcp}
Tools: {comma-separated list}

Constraints:
- Scaffold canonical OpenSIN-Code layout.
- Add tools with docstrings and type hints.
- Generate tests for each tool.
- Validate before publish.
- Run CEO audit.
- Register in opencode.json.

Follow tasks/workflow.md.
```
