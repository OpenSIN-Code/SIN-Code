# test_hooks.py

Tests for the automatic hook installation system.

## What is tested

- `install_opencode_hooks()` creates correct hook files with proper permissions
- `uninstall_opencode_hooks()` removes existing hooks
- `list_opencode_hooks()` returns only existing hooks
- Hook templates contain defensive `command -v sin-brain` checks
- Hook templates clean up temp files after reading

## Files that touch this

- `hooks.py` — the module under test
- `cli.py` — `hooks_install`, `hooks_uninstall`, `hooks_list` commands

## Running

```bash
cd /Users/jeremy/dev/SIN-Code-Bundle
pytest tests/test_hooks.py -v
```
