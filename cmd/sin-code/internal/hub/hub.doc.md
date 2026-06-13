# Tool Catalog Hub (`internal/hub`)

Docs: `hub.go`

## What
A read-only, catalog-style helper that documents every sin-code subcommand and
relevant MCP skill surface. It is the landing page for `sin-code hub` and the
source of truth for the `HUB_CATEGORIES` table used by the TUI and chat agents.

## Why
With 36+ subcommands and 44+ MCP tools, onboarding and discovery become hard.
The hub provides a single, searchable, categorized view without adding new
runtime dependencies.

## Files
- `hub.go` — catalog data, search, and formatting
- `hub_test.go` — catalog completeness, search, and formatting tests
- `hub_cmd.go` — cobra CLI binding (in `cmd/sin-code/`)

## Usage
```bash
sin-code hub              # print full categorized catalog
sin-code hub list         # flat list of all tools
sin-code hub search auth  # search by name/short/description
sin-code hub info gh      # detailed info for one tool
```

## Maintenance rule
When you add a new subcommand, add an entry to `DefaultCatalog()` in
`hub.go` and update the category counts in the README/AGENTS if they change.

## Known caveats
- The catalog is static. Dynamic runtime tools (MCP servers that appear only
  when their launcher is present) are not enumerated here; use `sin-code mcp status`.
- Tool examples are illustrative; run `sin-code <tool> --help` for the full flag set.
