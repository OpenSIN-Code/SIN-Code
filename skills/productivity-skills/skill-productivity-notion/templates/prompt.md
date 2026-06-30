# Prompt template

When the user asks to interact with Notion, follow this structure:

1. **Identify intent**: read vs search vs write
2. **Check auth**: call `notion__notion_read_auth_status` if unsure
3. **Find workspace**: call `notion__notion_read_workspaces` if workspace_id unknown
4. **Execute**: call the appropriate `notion__*` tool
5. **Present results**: format the JSON response in readable markdown
6. **Verify writes**: re-read the affected resource after any write operation

Never fabricate Notion data. If a tool returns an error, report it honestly.
