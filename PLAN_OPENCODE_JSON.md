# PLAN: opencode.json Restoration & Tool Registration

**Ziel:** `~/.config/opencode/opencode.json` korrekt konfigurieren und alle 7 SIN-Code Tools als MCP registrieren.

**Status:** ✅ DONE (opencode.json valid, 7 tools registered)
**Aufwand:** ~30 Minuten

> **NOTE (2026-06-02)**: This plan originally used the deprecated `mcpServers`
> key with `type: "stdio"` and `command: "/path"` + `args: ["--mcp"]`. This was
> WRONG. opencode 1.15.11+ uses the legacy `mcp` key, with:
> - `type: "local"` (NOT "stdio")
> - `command: [array]` with `--mcp` INSIDE the array (NOT in `args`)
> - `enabled: true` (required)
>
> The corrected config is below.

---

## Correct opencode.json Format (opencode 1.15.11+)

```json
{
  "mcp": {
    "sin-discover": {
      "type": "local",
      "command": ["/Users/jeremy/.local/bin/discover", "--mcp"],
      "enabled": true,
      "description": "Discover files in a directory with relevance scoring"
    },
    "sin-execute": {
      "type": "local",
      "command": ["/Users/jeremy/.local/bin/execute", "--mcp"],
      "enabled": true,
      "description": "Execute shell commands safely with secret redaction"
    },
    "sin-map": {
      "type": "local",
      "command": ["/Users/jeremy/.local/bin/map", "--mcp"],
      "enabled": true,
      "description": "Map code architecture with dependency graphs"
    },
    "sin-grasp": {
      "type": "local",
      "command": ["/Users/jeremy/.local/bin/grasp", "--mcp"],
      "enabled": true,
      "description": "Deep code understanding for individual files"
    },
    "sin-scout": {
      "type": "local",
      "command": ["/Users/jeremy/.local/bin/scout", "--mcp"],
      "enabled": true,
      "description": "Search code with regex, semantic, and symbol search"
    },
    "sin-harvest": {
      "type": "local",
      "command": ["/Users/jeremy/.local/bin/harvest", "--mcp"],
      "enabled": true,
      "description": "Fetch URLs with caching and structure extraction"
    },
    "sin-orchestrate": {
      "type": "local",
      "command": ["/Users/jeremy/.local/bin/orchestrate", "--mcp"],
      "enabled": true,
      "description": "Manage tasks with dependencies and rollback plans"
    }
  }
}
```

---

## Why the Original Config Was Wrong

| Original (WRONG)          | Correct (opencode 1.15.11+) | Why                                   |
| ------------------------- | --------------------------- | ------------------------------------- |
| `"mcpServers": {...}`    | `"mcp": {...}`              | `mcpServers` is unrecognized key      |
| `"type": "stdio"`         | `"type": "local"`           | Schema only accepts `local` or `remote` |
| `"command": "/path"`      | `"command": ["/path", "--mcp"]` | Must be array; --mcp inside array  |
| `"args": ["--mcp"]`       | (no args — merge into command) | --mcp belongs in command array     |
| (missing) `"enabled": true` | `"enabled": true`         | Required field                        |

**Error produced by wrong config:**
`ConfigInvalidError: Unrecognized key: mcpServers`

---

## Validation

After implementation:
- [x] `~/.config/opencode/opencode.json` exists
- [x] JSON is valid
- [x] All 7 tools are registered under `mcp` (not `mcpServers`)
- [x] opencode recognizes the tools (verified via `opencode debug config`)
- [x] Tools can be called via MCP stdio transport

**Verification command:**
```bash
# Verify config is valid (no mcpServers, has mcp)
python3 -c "
import json
d = json.load(open('/Users/jeremy/.config/opencode/opencode.json'))
assert 'mcp' in d
assert 'mcpServers' not in d
print('✅ opencode.json valid')
"

# Verify each tool responds to MCP initialize
for tool in discover execute map grasp scout harvest orchestrate; do
    echo "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{}}" | \
        /Users/jeremy/.local/bin/$tool --mcp | head -1
done
```

---

## Related Files

- `Infra-SIN-OpenCode-Stack/scripts/sync-opencode-config.sh` — syncs `mcp` key (NOT `mcpServers`)
- `Infra-SIN-OpenCode-Stack/scripts/opencode-auto-sync.sh` — syncs `mcp` key
- `Infra-SIN-OpenCode-Stack/scripts/sync-local-opencode-configs.mjs` — syncs `mcp` key
- `Infra-SIN-OpenCode-Stack/opencode-config-install.sh` — initializes `mcp: {}` (NOT `mcpServers`)

---

**Last corrected:** 2026-06-02
