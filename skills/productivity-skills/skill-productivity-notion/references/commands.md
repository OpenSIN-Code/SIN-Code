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
- `vibe-notion page create --parent <pid> --title "T"` — create (returns full object)
- `vibe-notion page update <page-id> --title "T"` — rename
- `vibe-notion page update <page-id> --replace-content --markdown "## New\n\nContent"` — replace page content
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
- `vibe-notion database view-list <id>` — list all views for a database
- `vibe-notion database view-get <view-id>` — get view configuration
- `vibe-notion database view-add <id>` — add a new view
- `vibe-notion database view-update <view-id>` — update view properties
- `vibe-notion database view-delete <view-id>` — delete a view

## block
- `vibe-notion block get <block-id>` — block content
- `vibe-notion block children <block-id>` — child blocks (NOT `list-children`)
- `vibe-notion block append <block-id> --markdown "## Title\n\nText"` — append
- `vibe-notion block append <block-id> --content '<json>'` — append raw JSON blocks
- `vibe-notion block upload <parent-id> --file <path>` — upload file as block
- `vibe-notion block move <block-id> --parent <new-parent>` — move block
- `vibe-notion block update <block-id> --content '<json>'` — update block content
- `vibe-notion block delete <block-id>` — delete

## comment
- `vibe-notion comment list --page <page-id>` — list comments on page
- `vibe-notion comment list --block <block-id>` — list comments on block
- `vibe-notion comment create --page <page-id> "Comment text"` — add (text is positional)
- `vibe-notion comment get <comment-id>` — get single comment

## user
- `vibe-notion user me` — current identity
- `vibe-notion user get <user-id>` — user by id

## table (simple tables, not databases)
- `vibe-notion table create <parent-id>` — create simple table
- `vibe-notion table add-row <table-id>` — add row
- `vibe-notion table update-cell <table-id>` — update cell
- `vibe-notion table delete-row <table-id>` — delete row

## raw-cli (escape hatch)
- `vibe-notion raw-cli ['pages', 'move', '<page-id>', '--parent', '<new-parent>']` — move page
- `vibe-notion raw-cli ['databases', 'query', '<id>', '--filter', '<json>']` — query with filters

## Bot mode (vibe-notionbot)
- Requires `NOTION_TOKEN` env var (secret_xxx from integration settings)
- Same commands, but acts as bot identity
- Only sees pages explicitly connected to the integration
