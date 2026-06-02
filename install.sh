#!/usr/bin/env bash
# Purpose: One-command installer for the full SIN-Code Tool Suite (7 Go tools + Python bundle + opencode MCP config).
# Docs: install.sh.doc.md
#
# Bootstraps the entire SIN-Code agent-engineering stack in a single command:
#   1. Detect OS (macOS/Linux) + arch (amd64/arm64)
#   2. Check prerequisites (python3 ≥3.11, go ≥1.21, git, curl)
#   3. pip install -e . the SIN-Code-Bundle (uses `uv` if available, else pip)
#   4. Build & install all 7 Go tools into ~/.local/bin (resume-aware)
#   5. Smoke-test each binary in --mcp mode (JSON-RPC initialize)
#   6. Idempotently register all 7 tools in ~/.config/opencode/opencode.json (mcp block)
#   7. Run `sin status` and emit a final summary
#
# Flags:
#   --help      Show this help text
#   --dry-run   Print what would be done, do not modify the system
#   --verbose   Echo every command before running it
#   --force     Rebuild Go tools even if binary is already up to date
#   --skip-go   Skip Go tool build (only install Python bundle + register MCP)
#   --bundle-only  Alias for --skip-go
#
# Environment overrides (all optional):
#   SIN_CODE_BIN_DIR     Install dir for Go binaries (default: $HOME/.local/bin)
#   SIN_CODE_REPOS_DIR   Parent dir of the 7 Go tool repos (default: $HOME/dev)
#   SIN_CODE_OPENCODE_CONFIG  Path to opencode.json (default: $HOME/.config/opencode/opencode.json)
#
# Exit codes:
#   0 = success (also if all components already installed and healthy)
#   1 = prereq missing or unrecoverable error
#   2 = --help requested
set -euo pipefail

# ── Defaults ─────────────────────────────────────────────────────────────
BIN_DIR="${SIN_CODE_BIN_DIR:-$HOME/.local/bin}"
REPOS_DIR="${SIN_CODE_REPOS_DIR:-$HOME/dev}"
OPENCODE_CONFIG="${SIN_CODE_OPENCODE_CONFIG:-$HOME/.config/opencode/opencode.json}"
BUNDLE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

DRY_RUN=0
VERBOSE=0
FORCE=0
SKIP_GO=0

# Tool registry: binary name ↔ module path ↔ repo dir name
# Order is deliberate: discover first (most-used), orchestrate last.
TOOLS=(
  "discover|SIN-Code-Discover-Tool"
  "execute|SIN-Code-Execute-Tool"
  "map|SIN-Code-Map-Tool"
  "grasp|SIN-Code-Grasp-Tool"
  "scout|SIN-Code-Scout-Tool"
  "harvest|SIN-Code-Harvest-Tool"
  "orchestrate|SIN-Code-Orchestrate-Tool"
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
step()    { printf "%s[%s/%s]%s %s\n" "$C_DIM" "$1" "$TOTAL_STEPS" "$C_RESET" "$2"; }
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
SIN-Code Tool Suite — One-Command Installer

Usage: install.sh [OPTIONS]

Options:
  --help           Show this help and exit
  --dry-run        Print what would be done, do not modify the system
  --verbose        Echo every command before running it
  --force          Rebuild Go tools even if binary is already up to date
  --skip-go        Skip Go tool build (only install Python bundle + register MCP)
  --bundle-only    Alias for --skip-go

Environment overrides:
  SIN_CODE_BIN_DIR         Install dir for Go binaries (default: ~/.local/bin)
  SIN_CODE_REPOS_DIR       Parent dir of the 7 Go tool repos (default: ~/dev)
  SIN_CODE_OPENCODE_CONFIG Path to opencode.json (default: ~/.config/opencode/opencode.json)

What gets installed:
  • Python bundle: `sin` CLI (editable pip install into current interpreter or .venv)
  • 7 Go binaries: discover, execute, map, grasp, scout, harvest, orchestrate
  • opencode.json mcp registrations for all 7 tools
  • PATH hint if ~/.local/bin is not on PATH

Idempotency:
  • Re-runs are safe: existing binaries are kept if newer than their source
  • opencode.json is only patched for missing sin-* entries
  • Use --force to force a rebuild from source
EOF
}

# ── Argument parsing ────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --help)     usage; exit 2 ;;
    --dry-run)  DRY_RUN=1 ;;
    --verbose)  VERBOSE=1 ;;
    --force)    FORCE=1 ;;
    --skip-go|--bundle-only) SKIP_GO=1 ;;
    -h)         usage; exit 2 ;;
    *)          err "Unknown flag: $1"; usage; exit 1 ;;
  esac
  shift
done

TOTAL_STEPS=7

# ── Step 1: OS / arch detection ────────────────────────────────────────
detect_platform() {
  local os arch
  case "$(uname -s 2>/dev/null || echo unknown)" in
    Darwin)  os="darwin" ;;
    Linux)   os="linux" ;;
    *)       err "Unsupported OS: $(uname -s). Supported: macOS, Linux"; exit 1 ;;
  esac

  case "$(uname -m 2>/dev/null || echo unknown)" in
    arm64|aarch64) arch="arm64" ;;
    x86_64|amd64)  arch="amd64" ;;
    *)             err "Unsupported arch: $(uname -m). Supported: arm64, amd64"; exit 1 ;;
  esac

  echo "${os}/${arch}"
}

# ── Step 2: prereq checks ─────────────────────────────────────────────
version_ge() {
  # Return 0 if $1 >= $2 (semver-ish: 1.23.4 vs 1.21)
  local cur want
  cur=$(printf '%s' "$1" | awk -F. '{ printf("%d.%d.%d\n", $1+0, $2+0, $3+0) }')
  want=$(printf '%s' "$2" | awk -F. '{ printf("%d.%d.%d\n", $1+0, $2+0, $3+0) }')
  # Use sort -V (works on macOS with coreutils, or native on Linux)
  local highest
  highest=$(printf '%s\n%s\n' "$cur" "$want" | sort -V | tail -n1)
  [[ "$highest" == "$cur" ]]
}

check_prereqs() {
  local ok=1
  for bin in python3 go git curl; do
    if command -v "$bin" >/dev/null 2>&1; then
      ok "$bin found: $(command -v "$bin")"
    else
      err "Missing prerequisite: $bin"
      ok=0
    fi
  done

  if [[ "$ok" -eq 0 ]]; then
    err "Install the missing tools above and re-run."
    exit 1
  fi

  local py ver
  py=$(command -v python3)
  ver=$("$py" -c 'import sys; print("%d.%d.%d" % (sys.version_info[:3]))' 2>/dev/null || echo "0.0.0")
  if version_ge "$ver" "3.11"; then
    ok "python3 $ver (>= 3.11)"
  else
    err "python3 $ver is too old. Need >= 3.11"
    exit 1
  fi

  local gover
  gover=$(go version | awk '{print $3}' | sed 's/^go//')
  if version_ge "$gover" "1.21"; then
    ok "go $gover (>= 1.21)"
  else
    err "go $gover is too old. Need >= 1.21"
    exit 1
  fi
}

# ── Step 3: pip install the bundle ────────────────────────────────────
install_bundle() {
  heading "Step 3/7: Install Python bundle (sin-code-bundle)"
  step 3 "Installing sin-code-bundle in editable mode"

  if [[ ! -f "$BUNDLE_DIR/pyproject.toml" ]]; then
    err "pyproject.toml not found at $BUNDLE_DIR"
    err "Re-run from the SIN-Code-Bundle repo root."
    exit 1
  fi

  # Prefer `uv` (fast, deterministic via uv.lock) if available.
  if command -v uv >/dev/null 2>&1; then
    ok "uv found: $(command -v uv) — using uv pip install"
    # Use a project-local venv if .venv exists, else install into current env.
    if [[ -d "$BUNDLE_DIR/.venv" ]]; then
      run uv pip install --python "$BUNDLE_DIR/.venv/bin/python" -e "$BUNDLE_DIR[mcp,dev]"
    else
      run uv pip install --system -e "$BUNDLE_DIR[mcp,dev]"
    fi
  else
    warn "uv not found, falling back to pip3"
    if [[ -d "$BUNDLE_DIR/.venv" ]]; then
      run "$BUNDLE_DIR/.venv/bin/pip" install -e "$BUNDLE_DIR[mcp,dev]"
    else
      run python3 -m pip install -e "$BUNDLE_DIR[mcp,dev]"
    fi
  fi
}

# ── Step 4: build & install the 7 Go tools ───────────────────────────
build_one_tool() {
  local binary="$1" repo_dir_name="$2"
  local repo="$REPOS_DIR/$repo_dir_name"
  local out="$BIN_DIR/$binary"

  if [[ ! -d "$repo" ]]; then
    err "Repo not found: $repo"
    err "Set SIN_CODE_REPOS_DIR or clone https://github.com/OpenSIN-Code/$repo_dir_name"
    return 1
  fi

  if [[ ! -d "$repo/cmd/$binary" ]]; then
    err "Expected cmd/$binary/ in $repo"
    return 1
  fi

  # Resume: skip rebuild if binary exists and is newer than every .go file in cmd/$binary.
  if [[ "$FORCE" -eq 0 ]] && [[ -x "$out" ]]; then
    local newest_src
    newest_src=$(find "$repo/cmd/$binary" -name '*.go' -type f -exec stat -f '%m %N' {} \; 2>/dev/null \
      | sort -nr | head -n1 | awk '{print $1}')
    local bin_mtime
    bin_mtime=$(stat -f '%m' "$out" 2>/dev/null || echo 0)
    if [[ -n "$newest_src" ]] && [[ "$bin_mtime" -ge "$newest_src" ]]; then
      ok "$binary up-to-date at $out (use --force to rebuild)"
      return 0
    fi
  fi

  mkdir -p "$BIN_DIR"
  (
    cd "$repo"
    run go build -trimpath -ldflags='-s -w' -o "$out" "./cmd/$binary"
  )
  ok "$binary installed at $out"
}

build_go_tools() {
  heading "Step 4/7: Build & install 7 Go tools → $BIN_DIR"
  if [[ "$SKIP_GO" -eq 1 ]]; then
    warn "Skipping Go tool build (--skip-go)"
    return 0
  fi
  step 4 "Building 7 Go tools from $REPOS_DIR"
  for entry in "${TOOLS[@]}"; do
    local binary="${entry%%|*}"
    local repo="${entry##*|}"
    if ! build_one_tool "$binary" "$repo"; then
      err "Failed to build $binary"
      exit 1
    fi
  done
}

# ── Step 5: smoke-test each binary in --mcp mode ──────────────────────
smoke_test() {
  heading "Step 5/7: Smoke-test binaries (JSON-RPC initialize)"
  step 5 "Piping initialize request to each --mcp server"
  local pass=0 fail=0
  local init_req='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}'
  for entry in "${TOOLS[@]}"; do
    local binary="${entry%%|*}"
    local bin="$BIN_DIR/$binary"
    if [[ ! -x "$bin" ]]; then
      err "$binary missing at $bin"
      fail=$((fail+1))
      continue
    fi
    if [[ "$DRY_RUN" -eq 1 ]]; then
      dry "echo '$init_req' | $bin --mcp"
      pass=$((pass+1))
      continue
    fi
    local resp
    resp=$(echo "$init_req" | "$bin" --mcp 2>/dev/null || true)
    if printf '%s' "$resp" | grep -q '"serverInfo"'; then
      ok "$binary --mcp OK"
      pass=$((pass+1))
    else
      err "$binary --mcp failed: $resp"
      fail=$((fail+1))
    fi
  done
  echo
  if [[ "$fail" -eq 0 ]]; then
    ok "All $pass binaries passed smoke test"
  else
    err "$fail of $((pass+fail)) smoke tests failed"
    exit 1
  fi
}

# ── Step 6: patch opencode.json (idempotent) ─────────────────────────
# Uses python3 for safe JSON manipulation; falls back to skipping if python missing.
patch_opencode_config() {
  heading "Step 6/7: Register tools in opencode.json (mcp block)"
  step 6 "Patching $OPENCODE_CONFIG"

  if [[ ! -f "$OPENCODE_CONFIG" ]]; then
    if [[ "$DRY_RUN" -eq 1 ]]; then
      dry "create $OPENCODE_CONFIG (does not exist)"
      for entry in "${TOOLS[@]}"; do
        local binary="${entry%%|*}"
        dry "  add mcp.sin-${binary} = command[${BIN_DIR}/${binary}, --mcp]"
      done
      return 0
    fi
    warn "$OPENCODE_CONFIG does not exist — creating minimal stub"
    mkdir -p "$(dirname "$OPENCODE_CONFIG")"
    printf '{"mcp":{}}' > "$OPENCODE_CONFIG"
  fi

  if [[ "$DRY_RUN" -eq 1 ]]; then
    dry "patch $OPENCODE_CONFIG — add missing sin-* entries under mcp"
    for entry in "${TOOLS[@]}"; do
      local binary="${entry%%|*}"
      dry "  sin-${binary} → $BIN_DIR/${binary} --mcp"
    done
    return 0
  fi

  # python3 is guaranteed by prereq check, so use it for safe JSON edits.
  local backup="$OPENCODE_CONFIG.bak-$(date +%Y%m%d-%H%M%S)"
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

  ok "opencode.json: $added added, $skipped already present"
}

# ── Step 7: final status ─────────────────────────────────────────────
final_status() {
  heading "Step 7/7: Final status"
  step 7 "Running \`sin status\` to verify"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    dry "sin status"
  else
    # Find `sin` — prefer the bundle venv, fall back to PATH
    local sin_bin=""
    if [[ -x "$BUNDLE_DIR/.venv/bin/sin" ]]; then
      sin_bin="$BUNDLE_DIR/.venv/bin/sin"
    elif command -v sin >/dev/null 2>&1; then
      sin_bin="$(command -v sin)"
    fi
    if [[ -n "$sin_bin" ]]; then
      "$sin_bin" status 2>&1 || warn "sin status exited non-zero (some subsystems may be missing — that's OK)"
    else
      warn "sin CLI not found on PATH (try: source $BUNDLE_DIR/.venv/bin/activate)"
    fi
  fi

  # PATH hint
  if [[ ":$PATH:" != *":$BIN_DIR:"* ]]; then
    warn "$BIN_DIR is not on your PATH"
    printf "       Add to your shell rc:\n       export PATH=\"%s:\$PATH\"\n" "$BIN_DIR"
  fi
}

# ── Summary ───────────────────────────────────────────────────────────
print_summary() {
  local built=0 skipped=0 smoke_pass=0
  for entry in "${TOOLS[@]}"; do
    local binary="${entry%%|*}"
    local bin="$BIN_DIR/$binary"
    if [[ -x "$bin" ]]; then
      built=$((built+1))
    fi
  done
  heading "Summary"
  printf "  Bundle dir:        %s\n" "$BUNDLE_DIR"
  printf "  Bin dir:           %s\n" "$BIN_DIR"
  printf "  Repos dir:         %s\n" "$REPOS_DIR"
  printf "  opencode.json:     %s\n" "$OPENCODE_CONFIG"
  printf "  Go tools:          %s/%s installed\n" "$built" "${#TOOLS[@]}"
  printf "  opencode.json:     patched (idempotent)\n"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    printf "  Mode:              %sDRY RUN%s (no changes made)\n" "$C_YELLOW" "$C_RESET"
  else
    printf "  Mode:              %sLIVE%s\n" "$C_GREEN" "$C_RESET"
  fi
  printf "\n"
  ok "SIN-Code Tool Suite install complete."
}

# ── Main ──────────────────────────────────────────────────────────────
main() {
  heading "SIN-Code Tool Suite — One-Command Installer"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    info "DRY RUN — no system changes will be made"
  fi
  if [[ "$VERBOSE" -eq 1 ]]; then
    info "VERBOSE — every command will be echoed"
  fi
  if [[ "$FORCE" -eq 1 ]]; then
    info "FORCE — Go tools will be rebuilt from source"
  fi

  step 1 "Detecting platform"
  PLATFORM=$(detect_platform)
  ok "Platform: $PLATFORM"

  step 2 "Checking prerequisites"
  check_prereqs

  install_bundle
  build_go_tools
  smoke_test
  patch_opencode_config
  final_status
  print_summary
}

main "$@"
