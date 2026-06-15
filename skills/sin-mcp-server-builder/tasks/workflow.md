# Tasks: Workflow

Docs: ../SKILL.md

## Pre-flight

- [ ] Confirm server name, description, template, and initial tools.

## Execution

- [ ] Task 1: Scaffold project.
  - Acceptance: `mcp_scaffold` returns project path.
  - Verify: Files created per canonical layout.
- [ ] Task 2: Add tools.
  - Acceptance: Tools added with docstrings and type hints.
  - Verify: `mcp_tool_add` used.
- [ ] Task 3: Generate tests.
  - Acceptance: Test file exists for each tool.
  - Verify: `mcp_tool_test` used.
- [ ] Task 4: Validate.
  - Acceptance: `mcp_validate` passes.
  - Verify: No validation errors.
- [ ] Task 5: Audit.
  - Acceptance: `mcp_audit` passes.
  - Verify: CEO audit grade acceptable.
- [ ] Task 6: Publish/register.
  - Acceptance: `mcp_publish` and `mcp_register` succeed.
  - Verify: Server registered in `opencode.json`.

## Post-flight

- [ ] Provide project path and registration status.
- [ ] Offer next steps (test, deploy).
