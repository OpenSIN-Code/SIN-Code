# opencode-safe-start.sh - Safe Process Bootstrapper

## Purpose

Fixes the `{env:VAR}` substitution bug in `opencode.jsonc` that causes crashes
when API keys contain special characters (especially `$`) or when certain MCP servers
(like Tavily) fail to load their configuration.

## What it does

1. **Reads** `opencode.jsonc` from `~/.config/opencode/`
2. **Parses** all `{env:VARIABLE}` placeholders
3. **Substitutes** with actual environment variable values
4. **Escapes** special characters (`$`, `"`, `\`) for safe JSON
5. **Creates** temporary sanitized config file
6. **Starts** OpenCode with the patched config
7. **Cleans up** temporary file on exit

## Dependencies

- `bash` - Shell interpreter
- `node` - Node.js for JSON parsing (more reliable than sed/awk)
- `opencode` - The OpenCode CLI

## Usage

```bash
# Basic usage (passes all args to opencode)
./opencode-safe-start.sh

# With arguments
./opencode-safe-start.sh --verbose --mode agent

# Custom config path
OPENCODE_CONFIG=/custom/path/opencode.jsonc ./opencode-safe-start.sh
```

## Environment Variables

- `OPENCODE_CONFIG` - Path to opencode.jsonc (default: `~/.config/opencode/opencode.jsonc`)
- All variables referenced in `{env:...}` placeholders

## How it works

The standard OpenCode parser has issues:
1. **Fails on `$` in API keys** - Dollar signs trigger shell expansion
2. **Inconsistent with Tavily** - Specific MCP server parsing issue
3. **No proper escaping** - JSON special characters break parsing

This wrapper:
1. Uses Node.js to parse JSONC (removes comments, handles JSON)
2. Replaces `{env:VAR}` with properly escaped values
3. Escapes `$` as `\$` (critical fix!)
4. Escapes `"` as `\"` and `\` as `\\`
5. Creates temp file, runs opencode, cleans up on exit

## Integration with Workflow

1. **Pre-flight** - Run before any OpenCode session
2. **Auto-loads** `opencode-zod-patch.js` via Node.js `-r` flag
3. **Validates** that critical API keys are set
4. **Protected** - Temporary config is cleaned up even on crash

## Key Features

- **Shell-safe** - Handles `$` characters in API keys
- **JSONC parsing** - Removes comments, handles JSON
- **Escape handling** - Proper escaping of all special chars
- **Auto-cleanup** - Temp file removed on exit (trap EXIT)
- **Warning system** - Alerts when API keys are missing
- **Node.js based** - More reliable than sed/awk for JSON

## Known Caveats

- Requires Node.js in PATH
- Creates temporary file (cleaned up automatically)
- Slightly slower than direct start (parsing overhead)
- Only handles `{env:VAR}` format, not other substitution patterns

## Security Note

- Temporary config file contains API keys in plaintext
- File is cleaned up immediately on exit
- Consider file permissions in multi-user environments

## Related Files

- `opencode-zod-patch.js` - Automatically preloaded by this script
- `opencode-cleanup-hook.sh` - Cleans up after sessions
- `teammate-adapter.js` - Swarm coordination

## Exit Codes

- `0` - Success
- `1` - Config file not found
- `2` - Node.js not available
- `3` - OpenCode not found
