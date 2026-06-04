# uninstall.sh — Symmetric Uninstaller (CoDocs companion)

What this file does: Removes everything that `install.sh` installed — the 7 Go
tool binaries, the Python bundle, the 8 Python subsystem packages, sin-brain,
and the MCP server registrations in `~/.config/opencode/opencode.json`. Idempotent
and safe to re-run.

Docs: uninstall.sh (source of truth — this file is the "what and why")

---

## What it does (stage by stage)

1. **Confirmation prompt** — refuses to run non-interactively without `--force`.
   Dry-run mode (--dry-run) skips the prompt entirely. Lists exactly what will
   be removed before asking. Always backs up the opencode.json before mutating
   (timestamped `.bak-<YYYYmmdd-HHMMSS>-uninstall`).

2. **Remove 7 Go binaries** — `rm -f` for each of `discover`, `execute`, `map`,
   `grasp`, `scout`, `harvest`, `orchestrate` in `$BIN_DIR` (default
   `~/.local/bin`). Silently skips missing binaries.

3. **Uninstall Python bundle** — `pip uninstall -y sin-code-bundle` (or
   `uv pip uninstall -y [--python <venv> | --system]`). Probes with
   `pip show` first to avoid noise on absent packages.

4. **Uninstall 8 Python subsystems + sin-brain** — same dispatch for
   `sin-code-sckg`, `sin-code-ibd`, `sin-code-poc`, `sin-code-efsm`,
   `sin-code-adw`, `sin-code-oracle`, `sin-code-orchestration`,
   `sin-code-review-interface`, and `sin-brain`.

5. **Strip MCP entries** — uses `python3` to safely remove the keys
   `sin-discover, sin-execute, sin-map, sin-grasp, sin-scout, sin-harvest,
   sin-orchestrate, gitnexus, sin-simone-mcp, sin-code-bundle` from the `mcp`
   block in `~/.config/opencode/opencode.json`. Other keys (custom agents,
   non-SIN MCP servers) are preserved untouched.

6. **Final summary** — prints a coloured block showing what was removed, what
   was kept, and the `DRY RUN` / `LIVE` mode.

---

## Relationship to `install.sh`

Every component removed by `uninstall.sh` corresponds 1:1 to a component
installed by `install.sh`:

| install.sh step | uninstall.sh step | What |
|---|---|---|
| Step 3 (Python bundle) | Step 2/4 | `sin-code-bundle` |
| Step 4 (8 Python subsystems) | Step 3/4 | 8 sin-code-* packages + sin-brain |
| Step 5 (7 Go tools build) | Step 1/4 | 7 binaries in `~/.local/bin` |
| Step 11 (opencode.json patch) | Step 4/4 | 10 MCP keys stripped |
| — | (informational) | sin-code-bundle CLI console script |

What install.sh installs but uninstall.sh does NOT touch (intentional):

- The 7 Go tool source repos under `$REPOS_DIR` (they were `git clone`d
  separately, not installed by install.sh). To re-install, re-clone.
- The Python venv at `$BUNDLE_DIR/.venv` (created by the user, not by
  install.sh). Install.sh uses it if present, but doesn't own it.
- Backups at `$OPENCODE_CONFIG.bak-*` (kept for forensic recovery).
- Non-SIN MCP servers registered by the user in opencode.json.
- Custom agents, providers, or other config in opencode.json.

---

## Flags

| Flag | Behaviour |
|------|-----------|
| `--help` / `-h` | Show usage text, exit 2 |
| `--dry-run` | Print all actions, skip all mutations, skip confirmation prompt |
| `--verbose` | Echo every command via `printf` logging |
| `--force` / `--yes` | Skip the "Continue? [y/N]" prompt (required for CI) |
| `--keep-go` | Don't touch the 7 Go tool binaries |
| `--keep-bundle` | Don't uninstall the `sin-code-bundle` Python package |
| `--keep-subsystems` | Don't uninstall the 8 Python subsystems + sin-brain |
| `--keep-config` | Don't strip sin-* MCP entries from opencode.json |

`--keep-*` flags are combinable: `bash uninstall.sh --force --keep-bundle
--keep-go` will leave the Python bundle and Go binaries in place while still
removing the subsystems and the opencode.json MCP block.

---

## Idempotency guarantees

- **Go binaries**: `rm -f` (no error if missing).
- **Python packages**: `pip show <pkg>` probes first; absent packages are
  silently skipped (counted as "already absent" in the summary).
- **MCP keys**: `python3` JSON edit only removes keys that are present;
  missing keys are silently skipped.
- **Re-running `bash uninstall.sh` is always safe** — it will report
  "0 removed, N already absent" the second time.

---

## Environment overrides

| Variable | Purpose | Default |
|----------|---------|---------|
| `SIN_CODE_BIN_DIR` | Go binary install dir | `~/.local/bin` |
| `SIN_CODE_REPOS_DIR` | Parent dir of Go tool repos (unused by uninstall) | `~/dev` |
| `SIN_CODE_OPENCODE_CONFIG` | Path to opencode.json | `~/.config/opencode/opencode.json` |

---

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success (even if nothing was installed) |
| 1 | Refused without `--force` in non-interactive mode, or unrecoverable error |
| 2 | `--help` requested |

---

## Recovery / re-install

After running uninstall.sh, the system is in a clean state. To re-install:

```bash
bash install.sh        # full install (downloads, builds, registers MCP)
bash update.sh         # in-place refresh (pulls, pip --upgrade, go build)
```

The opencode.json backup (`$OPENCODE_CONFIG.bak-*-uninstall`) is preserved
in case of accidental removal of unrelated keys — restore with:

```bash
cp ~/.config/opencode/opencode.json.bak-20260604-123045-uninstall \
   ~/.config/opencode/opencode.json
```

---

## Known caveats

- The script does NOT remove the `sin-code-bundle` directory itself (the
  bundle repo). To remove the repo: `rm -rf ~/.dev/SIN-Code-Bundle`.
- `pip uninstall` of an editable install (`pip install -e`) leaves the
  `.egg-info` directory behind. To clean: `find . -name '*.egg-info' -type d
  -exec rm -rf {} +` inside the bundle repo, or just `rm -rf` the repo.
- If you installed via `uv tool install sin-code-bundle` (system-wide), the
  `sin` binary lives in `~/.cargo/bin` or `~/.local/bin/uv-tools/sin-code-bundle/bin`
  and must be removed manually. The default install path is editable, not
  `uv tool install`, so this caveat rarely applies.
