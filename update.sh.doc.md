# update.sh — In-Place Updater (CoDocs companion)

What this file does: Refreshes an existing SIN-Code Tool Suite installation
without going through a full uninstall+reinstall cycle. Pulls the bundle repo,
upgrades the Python bundle and 8 subsystems via `pip install --upgrade`,
rebuilds the 7 Go tools (with optional mtime-bypass via `--force-rebuild`),
and re-registers MCP servers in opencode.json.

Docs: update.sh (source of truth — this file is the "what and why")

---

## What it does (stage by stage)

1. **Refresh bundle repo** — `git pull --ff-only` in `$BUNDLE_DIR` (or in
   `$REPOS_DIR/SIN-Code-Bundle` as fallback). Skipped if the script is in
   a non-git checkout. Use `--skip-pull` to test local changes.

2. **Upgrade sin-code-bundle** — `pip install -e .[mcp,dev] --upgrade`
   (or `uv pip install --python <venv> -e .[mcp,dev] --upgrade` when uv is
   available). Always uses editable install mode.

3. **Upgrade 8 Python subsystems** — for each of the 8 sibling repos
   (sckg, ibd, poc, efsm, adw, oracle, orchestration, review-interface):
   - Skipped if `$REPOS_DIR/<repo>` doesn't exist
   - Skipped if `<repo>/pyproject.toml` is missing
   - Otherwise: `pip install -e <path> --upgrade` (or `uv pip install`)

4. **Rebuild 7 Go tools** — for each of `discover`, `execute`, `map`,
   `grasp`, `scout`, `harvest`, `orchestrate`:
   - Skipped if `$REPOS_DIR/SIN-Code-X-Tool` doesn't exist
   - Skipped if `cmd/<binary>/` is missing in the repo
   - Mtime-aware: skips rebuild if binary is newer than newest `.go` source
     (use `--force-rebuild` to bypass)
   - Otherwise: `go build -trimpath -ldflags='-s -w' -o $BIN_DIR/<bin> ./cmd/<bin>`

5. **Re-register MCP servers in opencode.json** — adds any missing entries
   under the `mcp` block. Idempotent: existing keys are untouched. Always
   writes a timestamped `.bak-<YYYYmmdd-HHMMSS>-update` backup before mutating.
   - `sin-discover`, `sin-execute`, ..., `sin-orchestrate` (7 Go tool entries)
   - `sin-code-bundle` (the unified `sin serve` MCP server)
   - gitnexus and simone-mcp are NOT re-registered by update.sh — they
     belong to install.sh's external-bridges flow. To register them too,
     run `bash install.sh --skip-go --skip-external=false` after update.sh.

6. **Verify final state** — runs `sin status` (prefers `$BUNDLE_DIR/.venv/bin/sin`,
   falls back to PATH). A non-zero exit is downgraded to a warning because
   partial installs (e.g. some subsystems not yet pulled) are still useful.

---

## Relationship to install.sh

`update.sh` is a *subset* of `install.sh` — it re-runs the same commands
but skips platform/prereq detection and external-bridge setup. Use the
following table to choose between the two:

| Use case | install.sh | update.sh |
|---|---|---|
| Bootstrap a brand new machine | YES | NO |
| First-time install (fresh clone) | YES | NO |
| Daily/weekly refresh | NO | YES |
| After a bundle `git pull` of breaking changes | NO | YES |
| Switch Python environment (uv venv → system) | YES (recreates) | NO |
| After `go` version upgrade | NO | YES (`--force-rebuild`) |
| Recover from broken install | YES (`--force`) | NO (install.sh is safer) |

When in doubt: run `update.sh` first (fast), then `install.sh --force` if
something is still wrong (rebuilds everything from scratch).

---

## Flags

| Flag | Behaviour |
|------|-----------|
| `--help` / `-h` | Show usage text, exit 2 |
| `--dry-run` | Print all actions, skip all mutations |
| `--verbose` | Echo every command via printf logging |
| `--force-rebuild` | Force `go build` even if mtime says "up to date" |
| `--skip-go` | Skip Go tool build (only refresh Python + MCP) |
| `--skip-external` | Skip gitnexus + simone-mcp checks (no-op for update.sh, kept for flag symmetry) |
| `--skip-pull` | Don't `git pull` the bundle repo (test local changes) |
| `--subsystems-dir=PATH` | Override `SIN_CODE_REPOS_DIR` for subsystem discovery |

---

## Idempotency guarantees

- **`git pull --ff-only`**: refuses to merge if local commits exist (use
  `git pull --rebase` or `git stash` manually if your branch has diverged).
  Errors are downgraded to warnings — update.sh continues with the current
  working tree.
- **`pip install --upgrade`**: pip itself is idempotent (no-op if version
  unchanged).
- **`go build`**: mtime-aware (skips if binary is newer than source). With
  `--force-rebuild`, every tool is rebuilt unconditionally.
- **opencode.json patch**: existing keys are untouched; only missing keys
  are added.

---

## Environment overrides

| Variable | Purpose | Default |
|----------|---------|---------|
| `SIN_CODE_BIN_DIR` | Go binary install dir | `~/.local/bin` |
| `SIN_CODE_REPOS_DIR` | Parent dir of the 7 Go tool repos + 8 subsystems | `~/dev` |
| `SIN_CODE_OPENCODE_CONFIG` | Path to opencode.json | `~/.config/opencode/opencode.json` |

---

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success (every step completed; skips are fine) |
| 1 | Unrecoverable error (e.g. go build failure, opencode.json patch failure) |
| 2 | `--help` requested |

---

## Common workflows

### Routine weekly refresh

```bash
cd ~/dev/SIN-Code-Bundle
bash update.sh
```

### After a Go tool got a breaking change

```bash
bash update.sh --force-rebuild
```

### Test local bundle changes without committing

```bash
# Make changes to src/, then:
bash update.sh --skip-pull --dry-run   # preview
bash update.sh --skip-pull             # apply
```

### Bootstrap a new Python venv

```bash
cd ~/dev/SIN-Code-Bundle
uv venv .venv
source .venv/bin/activate
bash update.sh   # uv will pick up the venv automatically
```

### CI use (cron / GitHub Action)

```bash
bash update.sh --force-rebuild         # always rebuild Go in CI
```

---

## Known caveats

- **`uv pip install --upgrade` may fail** if the previous install was
  performed with a different Python interpreter (e.g. you switched from
  system pip to uv venv). Workaround: `pip uninstall -y sin-code-bundle`
  first, then `bash update.sh`. Or use `bash install.sh --force` for a
  clean slate.
- **Go tool rebuilds touch the mtime of `$BIN_DIR/<binary>`**. If you
  have other tools depending on the exact binary timestamp (rare),
  use `--skip-go`.
- **`update.sh` does not refresh gitnexus / simone-mcp** because those
  are external bridges (npm install / git clone). If you need to refresh
  them too, run `bash install.sh --skip-go` after `update.sh`.
- **The `sin-code-bundle` MCP entry is added under the key
  `mcp.sin-code-bundle`** (matches install.sh's existing key). If a
  previous install registered it under a different key (e.g. `sin-bundle`),
  it will not be deduplicated — clean it up manually.
