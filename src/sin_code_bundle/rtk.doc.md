# rtk.py

RTK (https://github.com/rtk-ai/rtk) bridge — discovers the upstream `rtk`
Rust binary and runs `rtk init` for each supported coder agent. RTK is
NOT an MCP server; it integrates via each agent's own hook / plugin
mechanism, so this bridge is intentionally thin.

## Dependencies

- stdlib: `shutil`, `subprocess`, `dataclasses`
- external: `rtk` binary on `PATH` (Apache-2.0, NOT vendored by the bundle)

## Touched by

- `cli.py` — `sin rtk doctor|setup|gain`
- `install.sh` — `sin rtk setup` is invoked during bundle install

## What it does

1. **`detect_env()`** — locates the `rtk` binary on `PATH`.
2. **`init_args(agent)`** — returns the upstream `rtk init` arguments
   for the agent (mirrors RTK's "Supported AI Tools" matrix).
3. **`setup_agents(agents)`** — runs `rtk init` for each agent. The
   upstream tool rewires the agent's own config (hooks, plugins) so
   subsequent commands go through RTK's filter/compressor.
4. **`gain()`** — returns RTK's token-savings stats as JSON (best-effort).
5. **`doctor()`** — aggregate availability report.

## Important config

- `RTK_BINARY = "rtk"` — the binary name we look for
- `_INIT_ARGS` — the per-agent `rtk init` arguments; keep aligned with
  RTK upstream
- `AGENTS = ("opencode", "codex", "hermes")`

## Usage

```python
from sin_code_bundle import rtk

print(rtk.doctor())         # → {"available": True, "binary": "/usr/local/bin/rtk", ...}
print(rtk.setup_agents())   # → {"opencode": "rtk init -g --opencode", ...}
print(rtk.gain())           # → {"saved_tokens": 12345, "savings_pct": 67.8, ...}
```

## Known caveats

- `rtk init` mutates each agent's config file (e.g.
  `~/.claude/settings.json`). There is no automatic uninstall; users
  must run `rtk init --uninstall` (or restore from backup) themselves.
- `gain()` is best-effort — older RTK versions may not support
  `--format json`, in which case the raw text is returned under
  `{"raw": ...}`.
- The bridge does not vendor or build RTK; users must install it
  separately (`brew install rtk`, `cargo install --git …`, etc.).
