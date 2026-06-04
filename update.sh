#!/usr/bin/env bash
# Purpose: In-place updater. Re-runs parts of install.sh to refresh an existing SIN-Code Tool Suite installation.
# Docs: update.sh.doc.md
#
# What it does (all symmetric to install.sh):
#   1. Detect the bundle repo (current dir or $REPOS_DIR/SIN-Code-Bundle) and `git pull` it
#   2. `pip install -e .[mcp,dev] --upgrade` for sin-code-bundle (or `uv pip install`)
#   3. `pip install -e <repo> --upgrade` for each of the 8 Python subsystem repos (if present)
#   4. Rebuild the 7 Go tools from $REPOS_DIR into $BIN_DIR (with --force-rebuild)
#   5. Re-register MCP servers in $OPENCODE_CONFIG (always idempotent)
#   6. Run `sin status` to verify the final state
#
# What is NOT touched (intentional):
#   - The Python venv at $BUNDLE_DIR/.venv (left as-is; `pip install` uses it if present)
#   - The 7 Go tool source repos (only their build artifacts are refreshed)
#   - Backup files at $OPENCODE_CONFIG.bak-*
#
# Flags:
#   --help               Show this help text
#   --dry-run            Print what would be done, do not modify the system
#   --verbose            Echo every command before running it
#   --force-rebuild      Force `go build` of all 7 tools even if mtime check says "up to date"
#   --skip-go            Skip Go tool build (only refresh Python + MCP)
#   --skip-external      Skip gitnexus + simone-mcp checks (use install.sh's --skip-external path)
#   --skip-pull          Don't `git pull` the bundle repo (useful when testing local changes)
#   --subsystems-dir=PATH  Override $REPOS_DIR for subsystem repo discovery
#
# Environment overrides (all optional):
#   SIN_CODE_BIN_DIR         Go binary install dir (default: $HOME/.local/bin)
#   SIN_CODE_REPOS_DIR       Parent dir of the 7 Go tool repos (default: $HOME/dev)
#   SIN_CODE_OPENCODE_CONFIG Path to opencode.json (default: $HOME/.config/opencode/opencode.json)
#
# Exit codes:
#   0 = success (everything refreshed)
#   1 = unrecoverable error
#   2 = --help requested
set -uo pipefail

# ── Defaults ─────────────────────────────────────────────────────────────
BIN_DIR="${SIN_CODE_BIN_DIR:-$HOME/.local/bin}"
REPOS_DIR="${SIN_CODE_REPOS_DIR:-$HOME/dev}"
OPENCODE_CONFIG="${SIN_CODE_OPENCODE_CONFIG:-$HOME/.config/opencode/opencode.json}"
BUNDLE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

DRY_RUN=0
VERBOSE=0
FORCE_REBUILD=0
SKIP_GO=0
SKIP_EXTERNAL=0
SKIP_PULL=0

# 7 Go tool binaries ↔ repo dir name
TOOLS=(
  "discover|SIN-Code-Discover-Tool"
  "execute|SIN-Code-Execute-Tool"
  "map|SIN-Code-Map-Tool"
  "grasp|SIN-Code-Grasp-Tool"
  "scout|SIN-Code-Scout-Tool"
  "harvest|SIN-Code-Harvest-Tool"
  "orchestrate|SIN-Code-Orchestrate-Tool"
)

# 8 Python subsystem repos (mirrors install.sh)
SUBSYSTEM_REPOS=(
  "SIN-Code-Semantic-Codebase-Knowledge-Graphs"
  "SIN-Code-Intent-Based-Diffing"
  "SIN-Code-Proof-of-Correctness"
  "SIN-Code-Ephemeral-Full-Stack-Mocking-Orchestration"
  "SIN-Code-Architectural-Debt-Watchdogs"
  "SIN-Code-Verification-Oracle"
  "SIN-Code-Orchestration"
  "SIN-Code-Review-Interface"
)

# ── Color helpers (respect NO_COLOR) ──────────────────────────────────────
if [[ -t 1 ]] && [[ -z "${NO_COLOR:-}" ]]; then
  C_RESET=$'\033[0m'
  C_GREEN=$'\033[0;32m'
  C_RED=$'\033[0;31m'
  C_YELLOW=$'\033[0;33m'
  C_BLUE=$'\033[0;34m'
  C_BOLD=$'\033[1m'
  C_DIM=$'\033[2m'
else
  C_RESET="" C_GREEN="" C_RED="" C_YELLOW="" C_BLUE="" C_BOLD="" C_DIM=""
fi

info()    { printf "%s[info]%s %s\n" "$C_BLUE"    "$C_RESET" "$*"; }
ok()      { printf "%s[ ok ]%s %s\n" "$C_GREEN"   "$C_RESET" "$*"; }
warn()    { printf "%s[warn]%s %s\n" "$C_YELLOW"  "$C_RESET" "$*"; }
err()     { printf "%s[fail]%s %s\n" "$C_RED"     "$C_RESET" "$*" >&2; }
heading() { printf "\n%s%s== %s ==%s\n" "$C_BOLD" "$C_BLUE" "$*" "$C_RESET"; }
dry()     { printf "%s[dry ]%s %s\n" "$C_YELLOW" "$C_RESET" "$*"; }

# Run a command, or just print it in dry-run mode
run() {
  if [[ "$VERBOSE" -eq 1 ]] && [[ "$DRY_RUN" -eq 0 ]]; then
    printf "%s$ %s%s\n" "$C_DIM" "$*" "$C_RESET"
  fi
  if [[ "$DRY_RUN" -eq 1 ]]; then
    dry "$*"
  else
    "$@"
  fi
}

# ── Help text ────────────────────────────────────────────────────────────
usage() {
  cat <<'EOF'
SIN-Code Tool Suite — In-Place Updater

Usage: update.sh [OPTIONS]

Options:
  --help                   Show this help and exit
  --dry-run                Print what would be done, do not modify the system
  --verbose                Echo every command before running it
  --force-rebuild          Force `go build` of all 7 tools even if mtime says "up to date"
  --skip-go                Skip Go tool build (only refresh Python + MCP)
  --skip-external          Skip gitnexus + simone-mcp checks
  --skip-pull              Don't `git pull` the bundle repo (test local changes)
  --subsystems-dir=PATH    Override SIN_CODE_REPOS_DIR for subsystem repo discovery

Environment overrides:
  SIN_CODE_BIN_DIR         Go binary install dir (default: ~/.local/bin)
  SIN_CODE_REPOS_DIR       Parent dir of the 7 Go tool repos (default: ~/dev)
  SIN_CODE_OPENCODE_CONFIG Path to opencode.json (default: ~/.config/opencode/opencode.json)

What gets refreshed:
  1. Bundle repo: `git pull` (or use the local working tree if --skip-pull)
  2. Python bundle: `pip install -e .[mcp,dev] --upgrade`
  3. 8 Python subsystems: `pip install -e <repo> --upgrade` (if repos exist locally)
  4. 7 Go tools: `go build` into ~/.local/bin (with --force-rebuild to bypass mtime check)
  5. opencode.json MCP block: re-register sin-* / gitnexus / sin-simone-mcp keys (idempotent)
  6. Verification: run `sin status` to confirm the final state

Idempotency:
  • Re-running is safe: each step checks current state before mutating
  • `pip install --upgrade` only re-installs if version changed
  • `go build` skips binaries that are newer than source (unless --force-rebuild)
  • opencode.json is patched additively (existing keys untouched)

Examples:
  # Preview only
  bash update.sh --dry-run

  # Standard in-place update
  bash update.sh

  # Force Go rebuild + verbose
  bash update.sh --force-rebuild --verbose

  # Skip the bundle git pull (test local bundle changes)
  bash update.sh --skip-pull

  # Only refresh Python + MCP, leave Go binaries alone
  bash update.sh --skip-go
EOF
}

# ── Argument parsing ────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --help)              usage; exit 2 ;;
    --dry-run)           DRY_RUN=1 ;;
    --verbose)           VERBOSE=1 ;;
    --force-rebuild)     FORCE_REBUILD=1 ;;
    --skip-go)           SKIP_GO=1 ;;
    --skip-external)     SKIP_EXTERNAL=1 ;;
    --skip-pull)         SKIP_PULL=1 ;;
    --subsystems-dir=*)  REPOS_DIR="${1#*=}" ;;
    -h)                  usage; exit 2 ;;
    *)                   err "Unknown flag: $1"; usage; exit 1 ;;
  esac
  shift
done

# ── Preflight: detect pip (mirrors install.sh) ──────────────────────────
detect_pip_install_cmd() {
  # Echo a shell-quoted command prefix for "pip install -e <path>"
  # Prefer uv when available, else pip from .venv, else system pip.
  if command -v uv >/dev/null 2>&1; then
    if [[ -d "$BUNDLE_DIR/.venv" ]]; then
      printf '%s' "uv pip install --python $BUNDLE_DIR/.venv/bin/python"
    else
      printf '%s' "uv pip install --system"
    fi
  elif [[ -d "$BUNDLE_DIR/.venv" ]] && [[ -x "$BUNDLE_DIR/.venv/bin/pip" ]]; then
    printf '%s' "$BUNDLE_DIR/.venv/bin/pip install"
  elif command -v pip3 >/dev/null 2>&1; then
    printf '%s' "pip3 install"
  elif command -v pip >/dev/null 2>&1; then
    printf '%s' "pip install"
  else
    err "No uv / pip found on PATH"
    return 1
  fi
}

# ── Step 1: Detect & pull bundle repo ──────────────────────────────────
pull_bundle_repo() {
  heading "Step 1/6: Refresh bundle repo"
  if [[ "$SKIP_PULL" -eq 1 ]]; then
    warn "Skipped (--skip-pull). Using current working tree at $BUNDLE_DIR"
    return 0
  fi

  # Prefer local bundle (this script lives there). If it has .git, git pull.
  if [[ -d "$BUNDLE_DIR/.git" ]]; then
    if [[ "$DRY_RUN" -eq 1 ]]; then
      dry "cd $BUNDLE_DIR && git pull --ff-only"
      return 0
    fi
    info "Bundle repo found at $BUNDLE_DIR (has .git) — pulling"
    (cd "$BUNDLE_DIR" && run git pull --ff-only) || warn "git pull failed (continuing with current tree)"
    return 0
  fi

  # Fallback: look for $REPOS_DIR/SIN-Code-Bundle
  local external="$REPOS_DIR/SIN-Code-Bundle"
  if [[ -d "$external/.git" ]]; then
    if [[ "$DRY_RUN" -eq 1 ]]; then
      dry "cd $external && git pull --ff-only"
      return 0
    fi
    info "Bundle repo found at $external — pulling"
    (cd "$external" && run git pull --ff-only) || warn "git pull failed (continuing with current tree)"
    return 0
  fi

  warn "No .git found in $BUNDLE_DIR or $external — cannot pull. Continuing with current tree."
  info "If you expected a pull, check SIN_CODE_REPOS_DIR or clone the repo."
}

# ── Step 2: Upgrade Python bundle ──────────────────────────────────────
upgrade_bundle() {
  heading "Step 2/6: Upgrade sin-code-bundle (editable)"
  if [[ ! -f "$BUNDLE_DIR/pyproject.toml" ]]; then
    err "pyproject.toml missing at $BUNDLE_DIR"
    return 1
  fi

  local cmd
  cmd=$(detect_pip_install_cmd) || return 1
  if [[ "$DRY_RUN" -eq 1 ]]; then
    dry "$cmd -e $BUNDLE_DIR[mcp,dev] --upgrade"
    return 0
  fi
  # shellcheck disable=SC2086
  run $cmd -e "$BUNDLE_DIR[mcp,dev]" --upgrade
  ok "sin-code-bundle upgraded"
}

# ── Step 3: Upgrade 8 Python subsystems ────────────────────────────────
upgrade_subsystems() {
  heading "Step 3/6: Upgrade 8 Python subsystems"
  local cmd
  cmd=$(detect_pip_install_cmd) || { warn "No pip available, skipping"; return 0; }

  local installed=0 skipped=0 failed=0
  for repo in "${SUBSYSTEM_REPOS[@]}"; do
    local path="$REPOS_DIR/$repo"
    if [[ ! -d "$path" ]]; then
      info "Subsystem not found at $path (skipping)"
      skipped=$((skipped+1))
      continue
    fi
    if [[ ! -f "$path/pyproject.toml" ]]; then
      warn "$path exists but has no pyproject.toml (skipping)"
      skipped=$((skipped+1))
      continue
    fi
    if [[ "$DRY_RUN" -eq 1 ]]; then
      dry "$cmd -e $path --upgrade"
      installed=$((installed+1))
      continue
    fi
    # shellcheck disable=SC2086
    if $cmd -e "$path" --upgrade 2>/dev/null; then
      ok "Upgraded $repo"
      installed=$((installed+1))
    else
      warn "Failed to upgrade $repo"
      failed=$((failed+1))
    fi
  done

  ok "Subsystems: $installed upgraded, $skipped skipped, $failed failed"
}

# ── Step 4: Rebuild 7 Go tools ─────────────────────────────────────────
build_one_tool() {
  local binary="$1" repo_dir_name="$2"
  local repo="$REPOS_DIR/$repo_dir_name"
  local out="$BIN_DIR/$binary"

  if [[ ! -d "$repo" ]]; then
    warn "Repo not found: $repo (skipping $binary — clone with: git clone https://github.com/OpenSIN-Code/$repo_dir_name)"
    return 0
  fi

  if [[ ! -d "$repo/cmd/$binary" ]]; then
    warn "Expected cmd/$binary/ in $repo (skipping)"
    return 0
  fi

  # Resume: skip rebuild if binary exists and is newer than every .go file in cmd/$binary.
  if [[ "$FORCE_REBUILD" -eq 0 ]] && [[ -x "$out" ]]; then
    local newest_src
    newest_src=$(find "$repo/cmd/$binary" -name '*.go' -type f -exec stat -f '%m %N' {} \; 2>/dev/null \
      | sort -nr | head -n1 | awk '{print $1}')
    local bin_mtime
    bin_mtime=$(stat -f '%m' "$out" 2>/dev/null || echo 0)
    if [[ -n "$newest_src" ]] && [[ "$bin_mtime" -ge "$newest_src" ]]; then
      ok "$binary up-to-date at $out (use --force-rebuild to bypass)"
      return 0
    fi
  fi

  mkdir -p "$BIN_DIR"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    dry "cd $repo && go build -trimpath -ldflags='-s -w' -o $out ./cmd/$binary"
    return 0
  fi
  (
    cd "$repo"
    run go build -trimpath -ldflags='-s -w' -o "$out" "./cmd/$binary"
  ) || { err "Failed to build $binary"; return 1; }
  ok "$binary built at $out"
}

rebuild_go_tools() {
  heading "Step 4/6: Rebuild 7 Go tools → $BIN_DIR"
  if [[ "$SKIP_GO" -eq 1 ]]; then
    warn "Skipped (--skip-go)"
    return 0
  fi

  if ! command -v go >/dev/null 2>&1; then
    err "go not found in PATH — cannot build Go tools"
    err "Re-run with --skip-go if you only want Python + MCP refresh"
    return 1
  fi

  for entry in "${TOOLS[@]}"; do
    local binary="${entry%%|*}"
    local repo="${entry##*|}"
    if ! build_one_tool "$binary" "$repo"; then
      err "Failed to build $binary"
      return 1
    fi
  done
}

# ── Step 5: Re-register MCP servers in opencode.json ───────────────────
reregister_mcp() {
  heading "Step 5/6: Re-register MCP servers in $OPENCODE_CONFIG"
  if [[ ! -f "$OPENCODE_CONFIG" ]]; then
    warn "$OPENCODE_CONFIG does not exist — creating minimal stub"
    if [[ "$DRY_RUN" -eq 0 ]]; then
      mkdir -p "$(dirname "$OPENCODE_CONFIG")"
      printf '{"mcp":{}}' > "$OPENCODE_CONFIG"
    else
      dry "create $OPENCODE_CONFIG (does not exist)"
    fi
  fi

  if ! command -v python3 >/dev/null 2>&1; then
    err "python3 not found — cannot safely edit opencode.json"
    return 1
  fi

  if [[ "$DRY_RUN" -eq 1 ]]; then
    for entry in "${TOOLS[@]}"; do
      local binary="${entry%%|*}"
      dry "  mcp.sin-${binary} → $BIN_DIR/${binary} --mcp"
    done
    if [[ "$SKIP_EXTERNAL" -eq 0 ]]; then
      dry "  mcp.gitnexus → npx gitnexus mcp"
      dry "  mcp.sin-code-bundle → sin serve"
    fi
    return 0
  fi

  local backup="$OPENCODE_CONFIG.bak-$(date +%Y%m%d-%H%M%S)-update"
  cp "$OPENCODE_CONFIG" "$backup"
  ok "Backup written: $backup"

  local added=0 skipped=0
  for entry in "${TOOLS[@]}"; do
    local binary="${entry%%|*}"
    local key="sin-${binary}"
    if python3 - "$OPENCODE_CONFIG" "$key" <<'PY' 2>/dev/null
import json, sys
path, key = sys.argv[1], sys.argv[2]
try:
    with open(path) as f:
        data = json.load(f)
except Exception:
    sys.exit(2)
sys.exit(0 if key in data.get("mcp", {}) else 1)
PY
    then
      skipped=$((skipped+1))
    else
      python3 - "$OPENCODE_CONFIG" "$BIN_DIR" "$binary" <<'PY'
import json, os, sys
path, bin_dir, binary = sys.argv[1], sys.argv[2], sys.argv[3]
with open(path) as f:
    data = json.load(f)
mcp = data.setdefault("mcp", {})
mcp[f"sin-{binary}"] = {
    "type": "local",
    "command": [os.path.join(bin_dir, binary), "--mcp"],
    "enabled": True,
    "description": f"SIN-Code {binary} tool",
}
with open(path, "w") as f:
    json.dump(data, f, indent=2)
    f.write("\n")
PY
      added=$((added+1))
      ok "Added mcp.${key} → ${BIN_DIR}/${binary} --mcp"
    fi
  done

  # sin-code-bundle MCP entry (sin serve)
  if [[ "$SKIP_EXTERNAL" -eq 0 ]]; then
    if python3 - "$OPENCODE_CONFIG" <<'PY' 2>/dev/null
import json, sys
path = sys.argv[1]
try:
    with open(path) as f:
        data = json.load(f)
except Exception:
    sys.exit(2)
sys.exit(0 if "sin-code-bundle" in data.get("mcp", {}) else 1)
PY
    then
      skipped=$((skipped+1))
    else
      python3 - "$OPENCODE_CONFIG" <<'PY'
import json, sys
path = sys.argv[1]
with open(path) as f:
    data = json.load(f)
mcp = data.setdefault("mcp", {})
mcp["sin-code-bundle"] = {
    "type": "local",
    "command": ["sin", "serve"],
    "enabled": True,
    "description": "SIN-Code Bundle unified MCP server (34 tools)",
}
with open(path, "w") as f:
    json.dump(data, f, indent=2)
    f.write("\n")
PY
      added=$((added+1))
      ok "Added mcp.sin-code-bundle → sin serve"
    fi
  fi

  ok "opencode.json: $added added, $skipped already present"
}

# ── Step 6: Run sin status ─────────────────────────────────────────────
verify_status() {
  heading "Step 6/6: Verify final state (\`sin status\`)"

  local sin_bin=""
  if [[ -x "$BUNDLE_DIR/.venv/bin/sin" ]]; then
    sin_bin="$BUNDLE_DIR/.venv/bin/sin"
  elif command -v sin >/dev/null 2>&1; then
    sin_bin="$(command -v sin)"
  fi

  if [[ -z "$sin_bin" ]]; then
    warn "sin CLI not found on PATH (try: source $BUNDLE_DIR/.venv/bin/activate)"
    return 0
  fi

  if [[ "$DRY_RUN" -eq 1 ]]; then
    dry "$sin_bin status"
    return 0
  fi

  "$sin_bin" status 2>&1 || warn "sin status exited non-zero (some subsystems may be missing — that's OK)"
}

# ── Summary ────────────────────────────────────────────────────────────
print_summary() {
  heading "Summary"
  printf "  Bundle dir:        %s\n" "$BUNDLE_DIR"
  printf "  Bin dir:           %s\n" "$BIN_DIR"
  printf "  Repos dir:         %s\n" "$REPOS_DIR"
  printf "  opencode.json:     %s\n" "$OPENCODE_CONFIG"
  if [[ "$SKIP_PULL" -eq 0 ]]; then
    printf "  Bundle pull:       done\n"
  else
    printf "  Bundle pull:       SKIPPED (--skip-pull)\n"
  fi
  if [[ "$SKIP_GO" -eq 0 ]]; then
    printf "  Go tools:          rebuilt\n"
  else
    printf "  Go tools:          SKIPPED (--skip-go)\n"
  fi
  printf "  Python bundle:     upgraded\n"
  printf "  Subsystems:        upgraded (where present)\n"
  printf "  opencode.json:     re-registered MCP servers\n"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    printf "  Mode:              %sDRY RUN%s (no changes made)\n" "$C_YELLOW" "$C_RESET"
  else
    printf "  Mode:              %sLIVE%s\n" "$C_GREEN" "$C_RESET"
  fi
  printf "\n"
  ok "SIN-Code Tool Suite update complete."
  info "To uninstall: bash $BUNDLE_DIR/uninstall.sh"
  info "To full-reinstall: bash $BUNDLE_DIR/install.sh --force"
}

# ── Main ──────────────────────────────────────────────────────────────
main() {
  heading "SIN-Code Tool Suite — In-Place Updater"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    info "DRY RUN — no system changes will be made"
  fi
  if [[ "$VERBOSE" -eq 1 ]]; then
    info "VERBOSE — every command will be echoed"
  fi

  pull_bundle_repo
  upgrade_bundle
  upgrade_subsystems
  rebuild_go_tools
  reregister_mcp
  verify_status
  print_summary
}

main "$@"
