# opencode-zod-patch.js - Zod Compatibility Sandbox

## Purpose

Prevents the critical `TypeError: undefined is not an object (evaluating 'n._zod.def')`
crash that occurs when third-party plugins (e.g., `claude-mem`, `oh-my-openagent`) load
Zod v4 into the same scope as the OpenCode core, which uses internal Zod v3 APIs.

## What it does

1. **Patches `_def` property** - Adds missing property for Zod v4 compatibility
2. **Patches `_zod` internal accessor** - Maps v4 internal structure to v3 expectations
3. **Hooks into `require()`** - Dynamically patches Zod when loaded by plugins
4. **Prevents crashes** - Returns safe fallbacks instead of throwing

## Dependencies

- `zod` - The library being patched (optional, can be loaded later)
- `module` - Node.js module system for require() hooking

## Usage

```bash
# Preload before starting OpenCode
node -r ./opencode-zod-patch.js ./node_modules/.bin/opencode

# Or set in NODE_OPTIONS
export NODE_OPTIONS="-r /path/to/opencode-zod-patch.js"
opencode

# Or require at start of your script
require('./opencode-zod-patch.js');
const opencode = require('opencode');
```

## How it works

OpenCode's `toJsonSchema` function accesses `schema._zod.def` internally.
Zod v4 changed this structure, causing undefined access when plugins load v4.

This patch:
1. Adds `_def` getter if missing (Zod v4)
2. Adds `_zod` getter that returns `{def: this._def || {}}`
3. Hooks `require()` to catch dynamically loaded Zod modules

## Integration with Workflow

1. **Must be loaded before any plugins** that might load Zod
2. **Recommended**: Add to `NODE_OPTIONS` environment variable
3. **Used by**: `opencode-safe-start.sh` automatically preloads this
4. **Works with**: `claude-mem`, `oh-my-openagent`, and any other Zod-v4 plugin

## Key Features

- **Zero-config** - Works automatically when loaded
- **Idempotent** - Can be loaded multiple times safely
- **Non-destructive** - Doesn't break existing Zod v3 installations
- **Dynamic hooking** - Catches plugins that load Zod later
- **Fail-safe** - Returns empty objects instead of crashing

## Known Caveats

- Must be loaded BEFORE the problematic plugins
- Only patches the specific crash point (`_zod.def`)
- May mask other Zod v4 incompatibilities
- Requires Node.js environment (not browser)

## Related Files

- `opencode-safe-start.sh` - Automatically preloads this patch
- `opencode-cleanup-hook.sh` - Cleans up after runs
- `teammate-adapter.js` - Distributes tasks to agents

## Security Note

This patches internal Zod APIs. While safe for compatibility, always verify
Zod behavior after patching, especially for validation in production.
