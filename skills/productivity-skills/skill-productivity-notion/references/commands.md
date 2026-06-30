# vibe-notion CLI Command Reference

> The MCP bridge wraps these commands. Use `notion__*` tools instead of calling
> the CLI directly. This reference documents what's available behind the bridge.

## auth
- `vibe-notion auth status` — check auth state
- `vibe-notion auth extract --source browser|app|auto` — extract token_v2
- `vibe-notion auth logout` — remove stored credentials

## workspace
- `vibe-notion workspace list` — all workspaces
- `vibe-notion workspace resolve <page-id-or-url>` — workspace id from URL/page

## search
- `vibe-notion search "<query>" --workspace-id <id>` — full-text search

## page
- `vibe-notion page get <page-id>` — page metadata + properties
- `vibe-notion page list --workspace-id <id>` — list pages in workspace
- `vibe-notion page create --workspace-id <id>|--parent <pid> --title "T"` — create
- `vibe-notion page update <page-id> --title "T"` — rename
- `vibe-notion page archive <page-id>` — soft-delete

## database
- `vibe-notion database get <id>` — schema/properties
- `vibe-notion database query <id>` — rows
- `vibe-notion database list --workspace-id <id>` — list databases
- `vibe-notion database create --parent <pid> --title "Name"` — create
- `vibe-notion database add-row <id>` — add row
- `vibe-notion database update-row <row-page-id>` — update row
- `vibe-notion database update <id>` — update schema
- `vibe-notion database delete-property <id>` — delete property

## block
- `vibe-notion block get <block-id>` — block content
- `vibe-notion block list-children <block-id>` — child blocks
- `vibe-notion block append <block-id> --markdown "## Title\n\nText"` — append
- `vibe-notion block update <block-id>` — update
- `vibe-notion block delete <block-id>` — delete

## comment
- `vibe-notion comment list <page-or-block-id>` — list comments
- `vibe-notion comment create <page-or-block-id> --text "Comment"` — add
- `vibe-notion comment get <comment-id>` — get single comment

## user
- `vibe-notion user me` — current identity
- `vibe-notion user get <user-id>` — user by id

## Bot mode (vibe-notionbot)
- Requires `NOTION_TOKEN` env var (secret_xxx from integration settings)
- Same commands, but acts as bot identity
- Only sees pages explicitly connected to the integration
