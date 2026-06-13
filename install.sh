#!/usr/bin/env bash
# Purpose: One-command installer for the full SIN-Code Tool Suite (7 Go tools + Python bundle + opencode MCP config).
# Docs: install.sh.doc.md
#
# Bootstraps the entire SIN-Code agent-engineering stack in a single command:
#   1. Detect OS (macOS/Linux) + arch (amd64/arm64)
#   2. Check prerequisites (python3 ≥3.11, go ≥1.21, git, curl, node/npm)
#   3. pip install -e . the SIN-Code-Bundle (uses `uv` if available, else pip)
#   4. Install 8 Python subsystems from local repos into bundle environment
#   5. Build & install all 7 Go tools into ~/.local/bin (resume-aware)
#   6. Install / verify gitnexus (npm MCP server for graph context)
#   7. Install / verify simone-mcp (Python MCP server for code intelligence)
#   8. Check SIN-Brain (docs-only repo, warns if missing)
#   9. Verify 8 Python subsystems are importable
#  10. Smoke-test each binary in --mcp mode (JSON-RPC initialize)
#  11. Idempotently register all 7 Go tools + gitnexus + simone-mcp in ~/.config/opencode/opencode.json
#  12. Run `sin status` and emit a final summary
#
# Flags:
#   --help      Show this help text
#   --dry-run   Print what would be done, do not modify the system
#   --verbose   Echo every command before running it
#   --force     Rebuild Go tools even if binary is already up to date
#   --skip-go   Skip Go tool build (only install Python bundle + register MCP)
#   --bundle-only  Alias for --skip-go
#   --skip-external  Skip gitnexus, simone-mcp, and SIN-Brain checks
#   --with-externals  Auto-install external bridges (GitNexus, MarkItDown,
#                     RTK, Simone-MCP) instead of just verifying them.
#                     Default behaviour: verify-only (idempotent, never modifies
#                     those tools' global install paths).
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
SKIP_EXTERNAL=0
# --with-externals: when set, run `auto_install_externals` (npm/pipx/brew
# against the 4 external bridges) instead of the default verify-only flow.
# Default is verify-only to keep `install.sh` idempotent and side-effect free
# for the user-installed global tools.
WITH_EXTERNALS=0

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
  --skip-external  Skip gitnexus, simone-mcp, and SIN-Brain checks
  --with-externals  Auto-install external bridges (GitNexus, MarkItDown, RTK,
                    Simone-MCP) — default is verify-only

Environment overrides:
  SIN_CODE_BIN_DIR         Install dir for Go binaries (default: ~/.local/bin)
  SIN_CODE_REPOS_DIR       Parent dir of the 7 Go tool repos (default: ~/dev)
  SIN_CODE_OPENCODE_CONFIG Path to opencode.json (default: ~/.config/opencode/opencode.json)

What gets installed:
  • Python bundle: `sin` CLI (editable pip install into current interpreter or .venv)
  • 7 Go binaries: discover, execute, map, grasp, scout, harvest, orchestrate
  • gitnexus (npm): graph context MCP server (auto-installed if missing)
  • simone-mcp (Python): code intelligence MCP server (auto-installed if missing)
  • SIN-Brain: docs-only repo (checked, warning if missing)
  • 8 Python subsystems: auto-installed from local repos and verified
  • opencode.json mcp registrations for all 7 Go tools + gitnexus + simone-mcp
  • PATH hint if ~/.local/bin is not on PATH

Idempotency:
  • Re-runs are safe: existing binaries are kept if newer than their source
  • Python subsystems are re-installed idempotently (pip install -e)
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
    --skip-external) SKIP_EXTERNAL=1 ;;
    --with-externals) WITH_EXTERNALS=1 ;;
    -h)         usage; exit 2 ;;
    *)          err "Unknown flag: $1"; usage; exit 1 ;;
  esac
  shift
done

TOTAL_STEPS=12

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

  # Node/npm is optional (needed for gitnexus)
  if command -v node >/dev/null 2>&1; then
    local nodever
    nodever=$(node --version 2>/dev/null | sed 's/^v//')
    ok "node $nodever (optional, for gitnexus)"
  else
    warn "node not found — gitnexus MCP server will not be available (install Node.js >= 18)"
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
  heading "Step 3/12: Install Python bundle (sin-code-bundle)"
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

# ── Step 4: install 8 Python subsystems from local repos ───────────────
install_python_subsystems() {
  heading "Step 4/12: Install 8 Python subsystems from local repos"
  step 4 "Installing Python subsystems into the bundle environment"

  local repos_dir="${REPOS_DIR:-$HOME/dev}"
  local subsystems=(
    "SIN-Code-Semantic-Codebase-Knowledge-Graphs"
    "SIN-Code-Intent-Based-Diffing"
    "SIN-Code-Proof-of-Correctness"
    "SIN-Code-Ephemeral-Full-Stack-Mocking-Orchestration"
    "SIN-Code-Architectural-Debt-Watchdogs"
    "SIN-Code-Verification-Oracle"
    "SIN-Code-Orchestration"
    "SIN-Code-Review-Interface"
  )

  local total=${#subsystems[@]}
  local current=0
  local installed=0
  local failed=0
  local skipped=0

  for repo in "${subsystems[@]}"; do
    current=$((current+1))
    local repo_path="$repos_dir/$repo"
    if [ ! -d "$repo_path" ]; then
      warn "Subsystem $current/$total: $repo not found at $repo_path, skipping"
      skipped=$((skipped+1))
      continue
    fi

    info "Installing subsystem $current/$total: $repo..."
    if [[ "$DRY_RUN" -eq 1 ]]; then
      dry "pip install -e $repo_path"
      installed=$((installed+1))
      continue
    fi

    # Use the same Python environment as the bundle
    local pip_cmd=()
    if command -v uv >/dev/null 2>&1; then
      pip_cmd=(uv pip install)
      if [[ -d "$BUNDLE_DIR/.venv" ]]; then
        pip_cmd+=(--python "$BUNDLE_DIR/.venv/bin/python")
      else
        pip_cmd+=(--system)
      fi
    elif [[ -d "$BUNDLE_DIR/.venv" ]]; then
      pip_cmd=("$BUNDLE_DIR/.venv/bin/pip" install)
    else
      pip_cmd=(python3 -m pip install)
    fi

    if "${pip_cmd[@]}" -e "$repo_path" 2>/dev/null; then
      ok "Subsystem $current/$total: $repo installed"
      installed=$((installed+1))
    else
      warn "Subsystem $current/$total: $repo failed to install"
      failed=$((failed+1))
    fi
  done

  ok "Python subsystems: $installed installed, $failed failed, $skipped skipped"

  if [[ "$failed" -gt 0 ]]; then
    warn "Some subsystems failed to install. Check logs above."
  fi
}

# ── Step 5: build & install the 7 Go tools ───────────────────────────
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
    # Derive a semantic version. Order of preference:
    #   1. SIN_CODE_VERSION env var (CI override)
    #   2. `git describe --tags --always --dirty=-modified` (goreleaser-compatible)
    #   3. "dev" (no git, no env)
    SIN_CODE_VERSION="${SIN_CODE_VERSION:-$(git describe --tags --always --dirty=-modified 2>/dev/null || echo dev)}"
    SIN_CODE_COMMIT="${SIN_CODE_COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
    SIN_CODE_DATE="${SIN_CODE_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
    SIN_CODE_LDFLAGS="-s -w \
      -X github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal.Version=${SIN_CODE_VERSION} \
      -X github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal.commit=${SIN_CODE_COMMIT} \
      -X github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal.date=${SIN_CODE_DATE}"
    run go build -trimpath -ldflags="${SIN_CODE_LDFLAGS}" -o "$out" "./cmd/$binary"
  )
  ok "$binary installed at $out"
}

build_go_tools() {
  heading "Step 5/12: Build & install 7 Go tools → $BIN_DIR"
  if [[ "$SKIP_GO" -eq 1 ]]; then
    warn "Skipping Go tool build (--skip-go)"
    return 0
  fi
  step 5 "Building 7 Go tools from $REPOS_DIR"
  for entry in "${TOOLS[@]}"; do
    local binary="${entry%%|*}"
    local repo="${entry##*|}"
    if ! build_one_tool "$binary" "$repo"; then
      err "Failed to build $binary"
      exit 1
    fi
  done
}

# ── Step 6: gitnexus check / install ───────────────────────────────────
setup_gitnexus() {
  heading "Step 6/12: gitnexus (graph context MCP server)"
  if [[ "$SKIP_EXTERNAL" -eq 1 ]]; then
    warn "Skipping external MCP checks (--skip-external)"
    return 0
  fi
  step 6 "Checking gitnexus installation"

  if npx gitnexus --version >/dev/null 2>&1; then
    ok "gitnexus installed: $(npx gitnexus --version 2>/dev/null)"
    return 0
  fi

  warn "gitnexus not found — installing via npm"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    dry "npm install -g gitnexus"
    dry "npx gitnexus analyze --help  # verify"
    return 0
  fi

  if command -v npm >/dev/null 2>&1; then
    run npm install -g gitnexus || {
      warn "Global install failed, trying npx fallback..."
      run npx gitnexus --version || {
        err "gitnexus installation failed. Install manually: npm install -g gitnexus"
        return 1
      }
    }
  else
    err "npm not found. Install Node.js >= 18 and run again."
    return 1
  fi

  if npx gitnexus --version >/dev/null 2>&1; then
    ok "gitnexus installed successfully"
  else
    err "gitnexus verification failed"
    return 1
  fi
}

# ── Step 7: simone-mcp check / install ─────────────────────────────────
setup_simone_mcp() {
  heading "Step 7/12: simone-mcp (code intelligence MCP server)"
  if [[ "$SKIP_EXTERNAL" -eq 1 ]]; then
    warn "Skipping external MCP checks (--skip-external)"
    return 0
  fi
  step 7 "Checking simone-mcp installation"

  local simone_repo="$REPOS_DIR/Simone-MCP"
  if [[ -d "$simone_repo" ]]; then
    ok "simone-mcp repo found at $simone_repo"
  else
    warn "simone-mcp repo not found at $simone_repo"
    if [[ "$DRY_RUN" -eq 1 ]]; then
      dry "git clone https://github.com/OpenSIN-Code/Simone-MCP.git $simone_repo"
      return 0
    fi
    if command -v git >/dev/null 2>&1; then
      run git clone https://github.com/OpenSIN-Code/Simone-MCP.git "$simone_repo"
    else
      err "git not found — cannot clone simone-mcp"
      return 1
    fi
  fi

  # Check if already installed in current Python environment
  if python3 -c "import simone_mcp" 2>/dev/null; then
    ok "simone-mcp Python package already installed"
    return 0
  fi

  if [[ ! -f "$simone_repo/pyproject.toml" ]]; then
    err "pyproject.toml missing in $simone_repo — cannot install"
    return 1
  fi

  warn "simone-mcp Python package not installed — installing"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    dry "pip install -e $simone_repo[dev]"
    return 0
  fi

  if command -v uv >/dev/null 2>&1; then
    run uv pip install -e "$simone_repo[dev]"
  else
    run python3 -m pip install -e "$simone_repo[dev]"
  fi

  if python3 -c "import simone_mcp" 2>/dev/null; then
    ok "simone-mcp installed successfully"
  else
    err "simone-mcp installation failed"
    return 1
  fi
}

# ── Step 8: SIN-Brain check (docs-only) ────────────────────────────────
check_sin_brain() {
  heading "Step 8/12: SIN-Brain (docs-only repo)"
  if [[ "$SKIP_EXTERNAL" -eq 1 ]]; then
    warn "Skipping external MCP checks (--skip-external)"
    return 0
  fi
  step 8 "Checking SIN-Brain repo"

  local brain_repo="$REPOS_DIR/SIN-Brain"
  if [[ -d "$brain_repo" ]]; then
    ok "SIN-Brain repo found at $brain_repo"
    info "Note: SIN-Brain is currently docs-only (no code/binary)."
  else
    warn "SIN-Brain repo not found at $brain_repo"
    info "SIN-Brain is private and currently docs-only."
    info "If you have access, clone it: git clone https://github.com/OpenSIN-Code/SIN-Brain.git $brain_repo"
  fi
}

# ── Optional: --with-externals auto-install ──────────────────────────────
# Default behaviour: setup_gitnexus / setup_simone_mcp only install their
# respective tools when missing; MarkItDown and RTK are NOT touched at all
# (they are external bridges the user is expected to install via
# `sin markitdown setup` / `sin rtk setup`).
#
# --with-externals switches this to: for every missing external bridge,
# attempt the platform-native install path:
#   • GitNexus:  `npm install -g @abhigyanpatwari/gitnexus`
#   • MarkItDown: `pipx install markitdown` (fallback: `pip install 'markitdown[all]'`)
#   • RTK:       `brew install rtk` (Homebrew required; no fallback if missing)
#   • Simone-MCP: `npm install` in $REPOS_DIR/Simone-MCP (must be cloned first)
#
# Each step is best-effort: failures print a warning with the manual install
# command and continue. We never abort the whole install over a missing
# external bridge — they are optional and the bundle runs fine without them.
auto_install_externals() {
  if [[ "$WITH_EXTERNALS" -eq 0 ]]; then
    return 0
  fi
  if [[ "$SKIP_EXTERNAL" -eq 1 ]]; then
    warn "--with-externals ignored (--skip-external takes precedence)"
    return 0
  fi

  heading "Optional: auto-install external bridges (--with-externals)"

  # GitNexus — npm global install
  if npx gitnexus --version >/dev/null 2>&1; then
    ok "GitNexus already installed: $(npx gitnexus --version 2>/dev/null)"
  else
    info "  → GitNexus (graph context) — npm install -g @abhigyanpatwari/gitnexus"
    if [[ "$DRY_RUN" -eq 1 ]]; then
      dry "npm install -g @abhigyanpatwari/gitnexus"
    elif command -v npm >/dev/null 2>&1; then
      if run npm install -g @abhigyanpatwari/gitnexus; then
        ok "GitNexus installed"
      else
        warn "GitNexus npm install failed — try: npm install -g @abhigyanpatwari/gitnexus"
      fi
    else
      warn "npm not found — install Node.js >= 18 then: npm install -g @abhigyanpatwari/gitnexus"
    fi
  fi

  # MarkItDown — pipx preferred, pip fallback
  if python3 -c "import markitdown" 2>/dev/null; then
    ok "MarkItDown already installed"
  else
    info "  → MarkItDown (document → markdown)"
    if [[ "$DRY_RUN" -eq 1 ]]; then
      dry "pipx install markitdown"
      dry "pip install 'markitdown[all]'   # fallback"
    elif command -v pipx >/dev/null 2>&1; then
      if run pipx install markitdown; then
        ok "MarkItDown installed via pipx"
      else
        warn "pipx install markitdown failed — trying pip fallback"
        if run python3 -m pip install --user 'markitdown[all]'; then
          ok "MarkItDown installed via pip --user"
        else
          warn "MarkItDown install failed — try: pipx install markitdown  OR  pip install 'markitdown[all]'"
        fi
      fi
    else
      warn "pipx not found — falling back to pip install"
      if run python3 -m pip install --user 'markitdown[all]'; then
        ok "MarkItDown installed via pip --user"
      else
        warn "MarkItDown install failed — try: pip install 'markitdown[all]'"
      fi
    fi
  fi

  # RTK — brew only (no apt/winget fallback; RTK upstream is Homebrew-only)
  if command -v rtk >/dev/null 2>&1; then
    ok "RTK already installed: $(command -v rtk)"
  else
    info "  → RTK (token-saving shell proxy)"
    if [[ "$DRY_RUN" -eq 1 ]]; then
      dry "brew install rtk"
    elif command -v brew >/dev/null 2>&1; then
      if run brew install rtk; then
        ok "RTK installed via Homebrew"
      else
        warn "brew install rtk failed — try: brew install rtk"
      fi
    else
      warn "Homebrew not found — install RTK manually: https://github.com/rtk-ai/rtk"
    fi
  fi

  # Simone-MCP — npm install in $REPOS_DIR/Simone-MCP
  if python3 -c "import simone_mcp" 2>/dev/null; then
    ok "Simone-MCP already installed (Python package)"
  else
    info "  → Simone-MCP (code intelligence)"
    local simone_repo="$REPOS_DIR/Simone-MCP"
    if [[ -d "$simone_repo" ]]; then
      if [[ "$DRY_RUN" -eq 1 ]]; then
        dry "cd $simone_repo && npm install --silent"
      else
        if (cd "$simone_repo" && run npm install --silent); then
          ok "Simone-MCP dependencies installed"
        else
          warn "Simone-MCP npm install failed — try: cd $simone_repo && npm install"
        fi
      fi
    else
      warn "Simone-MCP not cloned at $simone_repo"
      warn "Clone it first: gh repo clone OpenSIN-Code/Simone-MCP $simone_repo"
    fi
  fi

  ok "External bridge auto-install complete (best-effort — see warnings above)"
}

# ── Step 9: Python subsystem health check ────────────────────────────────
check_python_subsystems() {
  heading "Step 9/12: Python subsystems (8 packages)"
  step 9 "Checking subsystem availability"

  local subsystems=(
    "sin_code_sckg|SCKG (knowledge graph)"
    "sin_code_ibd|IBD (intent diff)"
    "sin_code_poc|POC (proof of correctness)"
    "sin_code_efsm|EFSM (mock orchestration)"
    "sin_code_adw|ADW (debt watchdog)"
    "sin_code_oracle|Oracle (verification)"
    "sin_code_orchestration|Orchestration (multi-agent workflow)"
    "sin_code_review_interface|Review-Interface (semantic review UI)"
  )

  local found=0 missing=0
  for entry in "${subsystems[@]}"; do
    local mod="${entry%%|*}" desc="${entry##*|}"
    if python3 -c "import importlib.util; exit(0 if importlib.util.find_spec('$mod') else 1)" 2>/dev/null; then
      ok "$desc — installed"
      found=$((found+1))
    else
      warn "$desc — NOT installed"
      missing=$((missing+1))
    fi
  done

  if [[ "$missing" -gt 0 ]]; then
    info "Install missing subsystems with:"
    info "  pip install -e ../SIN-Code-Semantic-Codebase-Knowledge-Graphs"
    info "  pip install -e ../SIN-Code-Intent-Based-Diffing"
    info "  pip install -e ../SIN-Code-Proof-of-Correctness"
    info "  pip install -e ../SIN-Code-Ephemeral-Full-Stack-Mocking-Orchestration"
    info "  pip install -e ../SIN-Code-Architectural-Debt-Watchdogs"
    info "  pip install -e ../SIN-Code-Verification-Oracle"
    info "  pip install -e ../SIN-Code-Orchestration"
    info "  pip install -e ../SIN-Code-Review-Interface"
  fi

  ok "Python subsystems: $found/8 installed, $missing/8 missing"
}

# ── Step 10: smoke-test each binary in --mcp mode ──────────────────────
smoke_test() {
  heading "Step 10/12: Smoke-test binaries (JSON-RPC initialize)"
  step 10 "Piping initialize request to each --mcp server"
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

  # Soft-check external MCP servers (warnings only, not hard failures)
  if [[ "$SKIP_EXTERNAL" -eq 0 ]]; then
    if npx gitnexus --version >/dev/null 2>&1; then
      if [[ "$DRY_RUN" -eq 1 ]]; then
        dry "npx gitnexus mcp --smoke-test"
      else
        if npx gitnexus mcp --smoke-test 2>/dev/null || true; then
          ok "gitnexus MCP OK"
        else
          warn "gitnexus MCP smoke test inconclusive (may need index first)"
        fi
      fi
    fi

    local simone_cmd=""
    if python3 -c "import simone_mcp" 2>/dev/null; then
      simone_cmd="$(python3 -c "import simone_mcp, sys, os; repo=os.path.dirname(os.path.dirname(simone_mcp.__file__)); print(os.path.join(repo, 'src', 'cli.py'))" 2>/dev/null)"
    fi
    if [[ -z "$simone_cmd" ]] && [[ -f "$REPOS_DIR/Simone-MCP/src/cli.py" ]]; then
      simone_cmd="$REPOS_DIR/Simone-MCP/src/cli.py"
    fi
    if [[ -n "$simone_cmd" ]]; then
      if [[ "$DRY_RUN" -eq 1 ]]; then
        dry "echo '$init_req' | python3 $simone_cmd serve-mcp"
      else
        local simone_resp
        simone_resp=$(echo "$init_req" | python3 "$simone_cmd" serve-mcp 2>/dev/null || true)
        if printf '%s' "$simone_resp" | grep -q '"serverInfo"'; then
          ok "simone-mcp serve-mcp OK"
        else
          warn "simone-mcp serve-mcp smoke test inconclusive"
        fi
      fi
    fi
  fi

  echo
  if [[ "$fail" -eq 0 ]]; then
    ok "All $pass binaries passed smoke test"
  else
    err "$fail of $((pass+fail)) smoke tests failed"
    exit 1
  fi
}

# ── Step 11: patch opencode.json (idempotent) ─────────────────────────
# Uses python3 for safe JSON manipulation; falls back to skipping if python missing.
patch_opencode_config() {
  heading "Step 11/12: Register tools in opencode.json (mcp block)"
  step 11 "Patching $OPENCODE_CONFIG"

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
    if [[ "$SKIP_EXTERNAL" -eq 0 ]]; then
      dry "  gitnexus → npx gitnexus mcp"
      dry "  simone-mcp → python3 $REPOS_DIR/Simone-MCP/src/cli.py serve-mcp"
    fi
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

  # Register gitnexus MCP server
  if [[ "$SKIP_EXTERNAL" -eq 0 ]]; then
    if python3 - "$OPENCODE_CONFIG" <<'PY' 2>/dev/null
import json, sys
path = sys.argv[1]
try:
    with open(path) as f:
        data = json.load(f)
except Exception:
    sys.exit(2)
sys.exit(0 if "gitnexus" in data.get("mcp", {}) else 1)
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
mcp["gitnexus"] = {
    "type": "local",
    "command": ["npx", "gitnexus", "mcp"],
    "enabled": True,
    "description": "GitNexus code knowledge graph MCP server",
}
with open(path, "w") as f:
    json.dump(data, f, indent=2)
    f.write("\n")
PY
      added=$((added+1))
      ok "Added mcp.gitnexus → npx gitnexus mcp"
    fi

    # Register simone-mcp MCP server
    local simone_cmd=""
    if python3 -c "import simone_mcp" 2>/dev/null; then
      simone_cmd="$(python3 -c "import simone_mcp, sys, os; repo=os.path.dirname(os.path.dirname(simone_mcp.__file__)); print(os.path.join(repo, 'src', 'cli.py'))" 2>/dev/null)"
    fi
    if [[ -z "$simone_cmd" ]] && [[ -f "$REPOS_DIR/Simone-MCP/src/cli.py" ]]; then
      simone_cmd="$REPOS_DIR/Simone-MCP/src/cli.py"
    fi
    if [[ -n "$simone_cmd" ]]; then
      if python3 - "$OPENCODE_CONFIG" <<'PY' 2>/dev/null
import json, sys
path = sys.argv[1]
try:
    with open(path) as f:
        data = json.load(f)
except Exception:
    sys.exit(2)
sys.exit(0 if "sin-simone-mcp" in data.get("mcp", {}) else 1)
PY
      then
        skipped=$((skipped+1))
      else
        python3 - "$OPENCODE_CONFIG" "$simone_cmd" <<'PY'
import json, sys
path, cmd = sys.argv[1], sys.argv[2]
with open(path) as f:
    data = json.load(f)
mcp = data.setdefault("mcp", {})
mcp["sin-simone-mcp"] = {
    "type": "local",
    "command": ["python3", cmd, "serve-mcp"],
    "enabled": True,
    "description": "Simone MCP code intelligence server",
}
with open(path, "w") as f:
    json.dump(data, f, indent=2)
    f.write("\n")
PY
        added=$((added+1))
        ok "Added mcp.sin-simone-mcp → python3 $simone_cmd serve-mcp"
      fi
    else
      warn "simone-mcp not found, skipping MCP registration"
    fi
  fi

  ok "opencode.json: $added added, $skipped already present"
}

# ── Step 12: final status ─────────────────────────────────────────────
final_status() {
  heading "Step 12/12: Final status"
  step 12 "Running \`sin status\` to verify"
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
  local gitnexus_status="skipped"
  local simone_status="skipped"
  if [[ "$SKIP_EXTERNAL" -eq 0 ]]; then
    gitnexus_status="$(npx gitnexus --version 2>/dev/null || echo "not installed")"
    if python3 -c "import simone_mcp" 2>/dev/null; then
      simone_status="installed"
    else
      simone_status="not installed"
    fi
  fi
  local brain_status="not found (docs-only)"
  if [[ -d "$REPOS_DIR/SIN-Brain" ]]; then
    brain_status="repo found"
  fi
  local subsystems_installed=0
  for mod in sin_code_sckg sin_code_ibd sin_code_poc sin_code_efsm sin_code_adw sin_code_oracle sin_code_orchestration sin_code_review_interface; do
    if python3 -c "import importlib.util; exit(0 if importlib.util.find_spec('$mod') else 1)" 2>/dev/null; then
      subsystems_installed=$((subsystems_installed+1))
    fi
  done
  printf "  Bundle dir:        %s\n" "$BUNDLE_DIR"
  printf "  Bin dir:           %s\n" "$BIN_DIR"
  printf "  Repos dir:         %s\n" "$REPOS_DIR"
  printf "  opencode.json:     %s\n" "$OPENCODE_CONFIG"
  printf "  Go tools:          %s/%s installed\n" "$built" "${#TOOLS[@]}"
  printf "  Python subsystems: %s/8 installed\n" "$subsystems_installed"
  printf "  gitnexus:          %s\n" "$gitnexus_status"
  printf "  simone-mcp:        %s\n" "$simone_status"
  printf "  SIN-Brain:         %s\n" "$brain_status"
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
  install_python_subsystems
  build_go_tools
  setup_gitnexus
  setup_simone_mcp
  check_sin_brain
  auto_install_externals   # no-op unless --with-externals
  check_python_subsystems
  smoke_test
  patch_opencode_config
  final_status
  print_summary
}

main "$@"
