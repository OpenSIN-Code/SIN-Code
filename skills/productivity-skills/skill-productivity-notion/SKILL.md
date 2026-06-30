---
name: skill-productivity-notion
description: >
  Full Notion access via the vibe-notion MCP bridge (tools prefixed notion__).
  Use when the user wants to read, search, or edit Notion: pages, databases,
  blocks, comments, users, workspaces. Triggers on "Notion", "page", "database",
  "roadmap", "sprint", "wiki", "comment in Notion", "Notion durchsuchen",
  "Notion Seite", "Notion Datenbank".
license: MIT
compatibility:
  - sin-code
metadata:
  category: productivity
  lifecycle: bundled
  sources:
    - vibe-notion (npm CLI, https://github.com/vibe-notion/vibe-notion)
---

# vibe-notion — Full Notion Access for SIN-Code Agents

Notion is reachable through MCP tools prefixed `notion__`. Never fabricate Notion
data — always call a tool and use the real result.

## Two modes

- **Act-as-user** (default): `vibe-notion` CLI reads `token_v2` from the browser/
  desktop session. Edits appear under the user's name. Sees everything the user sees.
- **Bot mode** (`VIBE_NOTION_AS_BOT=1` + `NOTION_TOKEN`): acts as a bot integration.
  Only sees pages explicitly connected to the integration.

## Order of operations

1. Unsure of auth -> `notion__notion_read_auth_status`
2. Find workspace -> `notion__notion_read_workspaces` (note the id)
3. Search -> `notion__notion_read_search` (query + workspace_id)
4. Read -> `notion__notion_read_page` / `notion__notion_read_block_children` /
   `notion__notion_read_database_rows`
5. Write only on explicit request. Write tools are gated (`ask` -> headless = deny
   unless --yolo).

## Verification (M3)

After any write, re-read the affected page/block with a `notion__notion_read_*`
tool and confirm the change before reporting success.

## Available tools (17)

### Read tools (auto-allowed)
- `notion__notion_read_auth_status` — check auth state
- `notion__notion_read_workspaces` — list all workspaces
- `notion__notion_read_resolve` — resolve URL/page-id to workspace-id
- `notion__notion_read_search` — full-text search (requires workspace_id)
- `notion__notion_read_page` — get page metadata + properties
- `notion__notion_read_database_schema` — get database schema/properties
- `notion__notion_read_database_rows` — query database rows
- `notion__notion_read_block_children` — list child blocks of a page/block
- `notion__notion_read_comments` — list comments on a page/block
- `notion__notion_read_me` — current Notion user identity

### Write tools (gated: ask)
- `notion__notion_write_create_page` — create a new page
- `notion__notion_write_update_page` — update page title
- `notion__notion_write_archive_page` — archive (soft-delete) a page
- `notion__notion_write_append_block` — append markdown content as blocks
- `notion__notion_write_delete_block` — delete a block
- `notion__notion_write_create_comment` — add a comment

### Escape hatch (gated: ask)
- `notion__notion_raw_cli` — run arbitrary vibe-notion subcommand

## Full CLI reference
See `references/commands.md` for the complete vibe-notion CLI command list.
