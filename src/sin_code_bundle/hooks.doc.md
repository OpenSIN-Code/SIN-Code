# hooks.py

Generate `.opencode/hooks/pre-command.sh` and `post-command.sh` for automatic
SIN-Brain calls before/after every opencode command.

## Dependencies

- `sin-brain` CLI (optional — hooks are no-ops when absent)
- `jq` (optional — used for pretty-printing recalled memories)

## What it does

1. **pre-command.sh**: Runs before every opencode command. Calls `sin-brain
   recall` with the current task context (from `$OPENCODE_TASK` or `$PWD`)
   and exports the result into `$SIN_BRAIN_CONTEXT`.

2. **post-command.sh**: Runs after every opencode command. Reads
   `/tmp/last_task_result.txt` (if the agent wrote one) and stores it via
   `sin-brain remember` as an episodic memory.

## Files that touch this

- `cli.py` — `hooks_install`, `hooks_uninstall`, `hooks_list` commands
- `skills.py` — manual skill compilation (separate from automatic hooks)

## Usage

```python
from sin_code_bundle.hooks import install_opencode_hooks

paths = install_opencode_hooks(pre_command=True, post_command=True)
```

## Known caveats

- opencode does not expose the task prompt to hooks via env. The hook falls
  back to `$PWD` basename, so agents running in CI should set `OPENCODE_TASK`.
- The hooks are shell scripts (not Python), so the defensive `command -v`
  pattern is used to avoid errors when `sin-brain` is not installed.
- Both hooks clean up temp files after execution to avoid stale data.
