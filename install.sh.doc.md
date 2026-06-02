# install.sh — One-Command Installer (CoDocs companion)

What this file does: Bootstraps the entire SIN-Code Tool Suite
(7 Go tools + Python bundle + opencode MCP config) in a single invocation.

Docs: install.sh (source of truth — this file is the "what and why")

---

## What it does (stage by stage)

1. **Platform detection** — validates macOS/Linux and amd64/arm64. Sets
   `PLATFORM="{os}/{arch}"` (e.g. `darwin/arm64`). Exits 1 if unsupported.

2. **Prereq checks** — verifies `python3 >= 3.11`, `go >= 1.21`, `git`, `curl`
   via `command -v`. Uses `sort -V` for semver-ish comparison. Exits 1 on
   any missing or too-old prerequisite.

3. **Python bundle install** — prefers `uv` (fast, lockfile-correct) if on PATH:
   `uv pip install --python "$BUNDLE_DIR/.venv/bin/python" -e "$BUNDLE_DIR[mcp,dev]"`
   if a project-local `.venv` exists, otherwise `uv pip install --system -e ...`.
   Falls back to `python3 -m pip install -e ...` (or `.venv/bin/pip`) if `uv` is absent.

4. **Go tool build & install** — compiles all 7 tools from sibling repos into
   `~/.local/bin` (default). Resume-aware: skips a tool if its binary mtime is
   newer than the newest `.go` source in `cmd/<binary>`. `--force` bypasses this.

5. **Smoke test** — for each binary, pipes a JSON-RPC `initialize` request
   `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
   into `<bin> --mcp`. Green if the response contains `"serverInfo"`.
   Exits 1 on first failure.

6. **opencode.json patch (idempotent)** — adds `mcp.sin-{tool}` entries under
   the `mcp` block (not the deprecated `mcpServers`). Uses python3 for safe
   JSON manipulation; backs up to `opencode.json.bak-<timestamp>` before mutating.

7. **sin status + PATH hint** — runs `sin status` if available, warns if
   `$BIN_DIR` is not on PATH.

---

## Environment overrides

| Variable | Purpose | Default |
|----------|---------|---------|
| `SIN_CODE_BIN_DIR` | Go binary install dir | `~/.local/bin` |
| `SIN_CODE_REPOS_DIR` | Parent dir of the 7 Go tool repos | `~/dev` |
| `SIN_CODE_OPENCODE_CONFIG` | Path to opencode.json | `~/.config/opencode/opencode.json` |

---

## Flags

| Flag | Behaviour |
|------|-----------|
| `--help` / `-h` | Show usage text, exit 2 |
| `--dry-run` | Print all actions, skip all mutations |
| `--verbose` | Echo every command via `set -x`-style logging |
| `--force` | Rebuild all Go tools even if up-to-date |
| `--skip-go` / `--bundle-only` | Skip Go build; only install Python bundle + register MCP |

---

## Idempotency guarantees

- **Go binaries**: skipped if `bin_mtime >= newest_src_mtime` (unless `--force`).
- **opencode.json**: only adds missing `sin-*` keys; existing entries untouched.
- **Python bundle**: `pip install -e` is naturally idempotent.
- Re-running `bash install.sh` is safe at any point.

---

## Tool registry

```
binary name         repo dir name                     installs as
discover            SIN-Code-Discover-Tool            discover
execute             SIN-Code-Execute-Tool             execute
map                 SIN-Code-Map-Tool                 map
grasp               SIN-Code-Grasp-Tool               grasp
scout               SIN-Code-Scout-Tool               scout
harvest             SIN-Code-Harvest-Tool             harvest
orchestrate         SIN-Code-Orchestrate-Tool         orchestrate
```

Each binary implements a `--mcp` flag that starts a JSON-RPC MCP server
on stdin/stdout. No special build tags required — vanilla `go build`.

---

## Relationship to the repo layout

```
~/dev/
  SIN-Code-Discover-Tool/       cmd/discover/
  SIN-Code-Execute-Tool/        cmd/execute/
  SIN-Code-Map-Tool/            cmd/map/
  SIN-Code-Grasp-Tool/          cmd/grasp/
  SIN-Code-Scout-Tool/          cmd/scout/
  SIN-Code-Harvest-Tool/        cmd/harvest/
  SIN-Code-Orchestrate-Tool/    cmd/orchestrate/
  SIN-Code-Bundle/              install.sh  ← this file
                               install.sh.doc.md  ← you are here
                               pyproject.toml
                               src/
```

---

## Failure modes

| Scenario | Behaviour |
|----------|-----------|
| Repo dir missing | `err "Repo not found: $REPOS_DIR/SIN-Code-X-Tool"` → exit 1 |
| `cmd/<binary>` missing in repo | `err "Expected cmd/<binary>/ in $repo"` → exit 1 |
| `go build` fails | bubbles up from `run` → exit 1 |
| Smoke test fails | `err "$binary --mcp failed: <response>"` → exit 1 |
| Prereq too old | `err "<tool> X.Y.Z is too old. Need >= N.M"` → exit 1 |
| opencode.json mutated | backup written as `opencode.json.bak-<YYYYmmdd-HHMMSS>` first |

---

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success (or everything already installed and healthy) |
| 1 | Prerequisite missing, build failure, smoke-test failure |
| 2 | `--help` requested |
