# Workflow

## 1. Auth check (first contact)
```
notion__notion_read_auth_status
```
If not authenticated, instruct the user to run:
```bash
vibe-notion auth extract --source browser
```

## 2. Discover workspaces
```
notion__notion_read_workspaces
```
Note the workspace IDs for subsequent calls.

## 3. Search
```
notion__notion_read_search(query="Roadmap", workspace_id="<id>")
```

## 4. Read content
- Page: `notion__notion_read_page(page_id="<id>")`
- Block children: `notion__notion_read_block_children(block_id="<id>")`
- Database rows: `notion__notion_read_database_rows(database_id="<id>")`

## 5. Write (only on explicit user request)
- Create page: `notion__notion_write_create_page(title="...", workspace_id="...")`
- Append content: `notion__notion_write_append_block(block_id="...", markdown="...")`
- Create comment: `notion__notion_write_create_comment(target_id="...", text="...")`

## 6. Verify (M3 — mandatory)
After any write, re-read the affected resource and confirm the change:
```
notion__notion_read_page(page_id="<just-created-or-edited-id>")
```
