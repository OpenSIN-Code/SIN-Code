#!/usr/bin/env bash
# Purpose: Symmetric counterpart to install.sh. Removes what install.sh installed.
# Docs: uninstall.sh.doc.md
#
# What gets removed (all symmetric to install.sh):
#   1. The 7 Go tool binaries from $BIN_DIR (default: ~/.local/bin)
#   2. The Python bundle: `sin-code-bundle` (pip uninstall -y)
#   3. The 8 Python subsystem packages: sin-code-sckg, sin-code-ibd, sin-code-poc,
#      sin-code-efsm, sin-code-adw, sin-code-oracle, sin-code-orchestration,
#      sin-code-review-interface
#   4. sin-brain (memory cortex, the 9th optional package)
#   5. The MCP server registrations in $OPENCODE_CONFIG (mcp.sin-* keys, plus
#      mcp.gitnexus and mcp.sin-simone-mcp if present)
#
# What is NOT removed (intentional):
#   - The 7 Go tool source repos under $REPOS_DIR (clone with `git clone` to recover)
#   - The Python venv at $BUNDLE_DIR/.venv (if it existed pre-install)
#   - The opencode.json file itself (only the sin-* keys are stripped, unless --keep-config is OFF)
#   - Any pre-existing config or other binaries
#   - Backups at $OPENCODE_CONFIG.bak-* (kept for forensic recovery)
#
# Flags:
#   --help           Show this help text
#   --dry-run        Print what would be removed, do not modify the system
#   --verbose        Echo every command before running it
#   --force          Skip the interactive "are you sure?" prompt
#   --keep-config    Do NOT strip sin-* MCP entries from opencode.json
#   --keep-bundle    Do NOT uninstall the sin-code-bundle Python package
#   --keep-go        Do NOT remove the 7 Go tool binaries
#   --keep-subsystems  Do NOT uninstall the 8 Python subsystems
#   --yes            Alias for --force
#
# Environment overrides (all optional):
#   SIN_CODE_BIN_DIR         Go binary install dir (default: $HOME/.local/bin)
#   SIN_CODE_REPOS_DIR       Parent dir of Go tool repos (default: $HOME/dev)
#   SIN_CODE_OPENCODE_CONFIG Path to opencode.json (default: $HOME/.config/opencode/opencode.json)
#
# Exit codes:
#   0 = success (even if nothing was installed, idempotent)
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
FORCE=0
KEEP_CONFIG=0
KEEP_BUNDLE=0
KEEP_GO=0
KEEP_SUBSYSTEMS=0

# 7 Go tool binaries (mirrors install.sh::TOOLS, just the names)
GO_BINARIES=(
  discover
  execute
  map
  grasp
  scout
  harvest
  orchestrate
)

# 8 Python subsystem packages (pip distribution names; mirror install.sh subsystems list)
PYTHON_SUBSYSTEMS=(
  sin-code-sckg
  sin-code-ibd
  sin-code-poc
  sin-code-efsm
  sin-code-adw
  sin-code-oracle
  sin-code-orchestration
  sin-code-review-interface
)

# Extra Python package from the [all] / [memory] extras
PYTHON_EXTRA=(
  sin-brain
)

# MCP keys registered by install.sh::patch_opencode_config
MCP_KEYS_TO_STRIP=(
  sin-discover
  sin-execute
  sin-map
  sin-grasp
  sin-scout
  sin-harvest
  sin-orchestrate
  gitnexus
  sin-simone-mcp
  sin-code-bundle
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
SIN-Code Tool Suite — Uninstaller

Usage: uninstall.sh [OPTIONS]

Options:
  --help              Show this help and exit
  --dry-run           Print what would be removed, do not modify the system
  --verbose           Echo every command before running it
  --force / --yes     Skip the interactive "are you sure?" prompt
  --keep-config       Do NOT strip sin-* MCP entries from opencode.json
  --keep-bundle       Do NOT uninstall the sin-code-bundle Python package
  --keep-go           Do NOT remove the 7 Go tool binaries
  --keep-subsystems   Do NOT uninstall the 8 Python subsystem packages

Environment overrides:
  SIN_CODE_BIN_DIR         Go binary install dir (default: ~/.local/bin)
  SIN_CODE_REPOS_DIR       Parent dir of Go tool repos (default: ~/dev)
  SIN_CODE_OPENCODE_CONFIG Path to opencode.json (default: ~/.config/opencode/opencode.json)

What gets removed:
  • 7 Go binaries: discover, execute, map, grasp, scout, harvest, orchestrate
  • Python bundle: sin-code-bundle
  • 8 Python subsystems (sckg, ibd, poc, efsm, adw, oracle, orchestration, review-interface)
  • sin-brain (memory cortex, optional)
  • MCP entries under the mcp block in opencode.json (sin-discover, sin-execute, ..., gitnexus, sin-simone-mcp, sin-code-bundle)

What is NOT removed:
  • The 7 Go tool source repos under $REPOS_DIR (they were never installed, only used as build sources)
  • The bundle Python venv at $BUNDLE_DIR/.venv
  • The opencode.json file itself — only the sin-* keys are stripped
  • Backups at $OPENCODE_CONFIG.bak-*

Idempotency:
  • Re-runs are safe: missing items are silently skipped (no error)
  • Use --dry-run to preview exactly what would be removed

Examples:
  # Preview only (no changes)
  bash uninstall.sh --dry-run

  # Full uninstall with confirmation prompt
  bash uninstall.sh

  # Skip the prompt (CI use)
  bash uninstall.sh --force

  # Remove Go tools + subsystems but KEEP the Python bundle (e.g. for debugging)
  bash uninstall.sh --force --keep-bundle

  # Remove everything except the opencode.json MCP config
  bash uninstall.sh --force --keep-config
EOF
}

# ── Argument parsing ────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --help)              usage; exit 2 ;;
    --dry-run)           DRY_RUN=1 ;;
    --verbose)           VERBOSE=1 ;;
    --force|--yes)       FORCE=1 ;;
    --keep-config)       KEEP_CONFIG=1 ;;
    --keep-bundle)       KEEP_BUNDLE=1 ;;
    --keep-go)           KEEP_GO=1 ;;
    --keep-subsystems)   KEEP_SUBSYSTEMS=1 ;;
    -h)                  usage; exit 2 ;;
    *)                   err "Unknown flag: $1"; usage; exit 1 ;;
  esac
  shift
done

# ── Preflight: detect Python interpreter (same logic as install.sh) ─────
# Globals set by _detect_pip:
#   PIP_TOOL:  "uv" | "pip" | "python3-pip"
#   PIP_VENV:  path to .venv if applicable, else ""
#   PIP_PREFIX: shell-quoted argument prefix (e.g. '--python /path' or '--system' or '')
_detect_pip() {
  PIP_VENV=""
  PIP_PREFIX=""
  if [[ -d "$BUNDLE_DIR/.venv" ]] && [[ -x "$BUNDLE_DIR/.venv/bin/pip" ]]; then
    PIP_TOOL="pip"
    PIP_VENV="$BUNDLE_DIR/.venv/bin/pip"
    return 0
  fi
  if command -v uv >/dev/null 2>&1; then
    PIP_TOOL="uv"
    if [[ -d "$BUNDLE_DIR/.venv" ]]; then
      PIP_VENV="$BUNDLE_DIR/.venv/bin/python"
      PIP_PREFIX="--python $BUNDLE_DIR/.venv/bin/python"
    else
      PIP_PREFIX="--system"
    fi
    return 0
  fi
  if command -v pip3 >/dev/null 2>&1; then
    PIP_TOOL="pip"
    PIP_VENV="pip3"
    return 0
  fi
  if command -v pip >/dev/null 2>&1; then
    PIP_TOOL="pip"
    PIP_VENV="pip"
    return 0
  fi
  if command -v python3 >/dev/null 2>&1; then
    PIP_TOOL="python3-pip"
    PIP_VENV="python3 -m pip"
    return 0
  fi
  err "No pip / uv / python3 found — cannot uninstall Python packages"
  return 1
}

# Run a pip uninstall command.
pip_uninstall() {
  local pkg="$1"
  _detect_pip || return 0
  case "$PIP_TOOL" in
    uv)
      if [[ "$DRY_RUN" -eq 1 ]]; then
        dry "uv pip uninstall -y $PIP_PREFIX $pkg"
        return 0
      fi
      # shellcheck disable=SC2086
      uv pip uninstall -y $PIP_PREFIX "$pkg" 2>/dev/null || true
      ;;
    pip)
      if [[ "$DRY_RUN" -eq 1 ]]; then
        dry "$PIP_VENV uninstall -y $pkg"
        return 0
      fi
      $PIP_VENV uninstall -y "$pkg" 2>/dev/null || true
      ;;
    python3-pip)
      if [[ "$DRY_RUN" -eq 1 ]]; then
        dry "$PIP_VENV uninstall -y $pkg"
        return 0
      fi
      $PIP_VENV uninstall -y "$pkg" 2>/dev/null || true
      ;;
  esac
}

# Check if a package is currently installed (best-effort)
pip_show() {
  local pkg="$1"
  _detect_pip || return 1
  case "$PIP_TOOL" in
    uv)
      # shellcheck disable=SC2086
      uv pip show $PIP_PREFIX "$pkg" >/dev/null 2>&1
      ;;
    *)
      $PIP_VENV show "$pkg" >/dev/null 2>&1
      ;;
  esac
}

# ── Step 1: Confirmation prompt ─────────────────────────────────────────
confirm() {
  # Dry-run NEVER touches the system and NEVER prompts — just preview
  if [[ "$DRY_RUN" -eq 1 ]]; then
    info "DRY RUN — no system changes will be made; skipping confirmation prompt"
    return 0
  fi
  if [[ "$FORCE" -eq 1 ]]; then
    return 0
  fi
  if [[ ! -t 0 ]]; then
    # Non-interactive (e.g. CI) — refuse without --force
    err "Refusing to uninstall without --force (non-interactive shell)"
    exit 1
  fi
  printf "%sThis will remove:%s\n" "$C_BOLD" "$C_RESET"
  [[ "$KEEP_GO" -eq 0 ]]         && printf "  • 7 Go binaries from %s\n" "$BIN_DIR"
  [[ "$KEEP_BUNDLE" -eq 0 ]]     && printf "  • sin-code-bundle (Python package)\n"
  [[ "$KEEP_SUBSYSTEMS" -eq 0 ]] && printf "  • 8 Python subsystems + sin-brain\n"
  [[ "$KEEP_CONFIG" -eq 0 ]]     && printf "  • sin-* MCP entries from %s\n" "$OPENCODE_CONFIG"
  printf "\n"
  local reply
  read -r -p "Continue? [y/N] " reply
  case "$reply" in
    y|Y|yes|YES|Yes) return 0 ;;
    *) err "Aborted by user"; exit 1 ;;
  esac
}

# ── Step 2: Remove 7 Go tool binaries ───────────────────────────────────
remove_go_binaries() {
  heading "Step 1/4: Remove 7 Go tool binaries from $BIN_DIR"
  if [[ "$KEEP_GO" -eq 1 ]]; then
    warn "Skipped (--keep-go)"
    return 0
  fi

  local removed=0 missing=0
  for binary in "${GO_BINARIES[@]}"; do
    local path="$BIN_DIR/$binary"
    if [[ -e "$path" ]] || [[ -L "$path" ]]; then
      if [[ "$DRY_RUN" -eq 1 ]]; then
        dry "rm -f $path"
      else
        run rm -f "$path"
      fi
      ok "Removed $binary"
      removed=$((removed+1))
    else
      info "$binary not present (skipping)"
      missing=$((missing+1))
    fi
  done
  ok "Go binaries: $removed removed, $missing already absent"
}

# ── Step 3: Uninstall Python bundle ────────────────────────────────────
remove_python_bundle() {
  heading "Step 2/4: Uninstall sin-code-bundle"
  if [[ "$KEEP_BUNDLE" -eq 1 ]]; then
    warn "Skipped (--keep-bundle)"
    return 0
  fi

  if ! _detect_pip; then
    warn "No pip available, skipping"
    return 0
  fi

  if [[ "$DRY_RUN" -eq 1 ]]; then
    pip_uninstall sin-code-bundle
    ok "Would uninstall sin-code-bundle"
    return 0
  fi

  if pip_show sin-code-bundle; then
    pip_uninstall sin-code-bundle
    ok "sin-code-bundle uninstalled"
  else
    info "sin-code-bundle not installed (skipping)"
  fi
}

# ── Step 4: Uninstall 8 Python subsystems + sin-brain ──────────────────
remove_python_subsystems() {
  heading "Step 3/4: Uninstall 8 Python subsystems + sin-brain"
  if [[ "$KEEP_SUBSYSTEMS" -eq 1 ]]; then
    warn "Skipped (--keep-subsystems)"
    return 0
  fi

  if ! _detect_pip; then
    warn "No pip available, skipping"
    return 0
  fi

  local removed=0 missing=0
  for pkg in "${PYTHON_SUBSYSTEMS[@]}" "${PYTHON_EXTRA[@]}"; do
    if [[ "$DRY_RUN" -eq 1 ]]; then
      pip_uninstall "$pkg"
      removed=$((removed+1))
      continue
    fi

    if pip_show "$pkg"; then
      pip_uninstall "$pkg"
      ok "Uninstalled $pkg"
      removed=$((removed+1))
    else
      info "$pkg not installed (skipping)"
      missing=$((missing+1))
    fi
  done

  ok "Subsystems: $removed processed, $missing already absent"
}

# ── Step 5: Strip MCP entries from opencode.json ───────────────────────
strip_mcp_entries() {
  heading "Step 4/4: Strip sin-* MCP entries from $OPENCODE_CONFIG"
  if [[ "$KEEP_CONFIG" -eq 1 ]]; then
    warn "Skipped (--keep-config)"
    return 0
  fi

  if [[ ! -f "$OPENCODE_CONFIG" ]]; then
    info "opencode.json not present (skipping)"
    return 0
  fi

  if ! command -v python3 >/dev/null 2>&1; then
    warn "python3 not found — cannot safely edit opencode.json"
    warn "Manually remove these keys under the 'mcp' object: ${MCP_KEYS_TO_STRIP[*]}"
    return 0
  fi

  if [[ "$DRY_RUN" -eq 1 ]]; then
    dry "Strip keys [${MCP_KEYS_TO_STRIP[*]}] from mcp block in $OPENCODE_CONFIG"
    return 0
  fi

  # Back up first (mirrors install.sh behaviour)
  local backup="$OPENCODE_CONFIG.bak-$(date +%Y%m%d-%H%M%S)-uninstall"
  if ! cp "$OPENCODE_CONFIG" "$backup" 2>/dev/null; then
    warn "Could not write backup $backup (continuing anyway)"
  else
    ok "Backup written: $backup"
  fi

  # Use python for safe JSON edit
  python3 - "$OPENCODE_CONFIG" "${MCP_KEYS_TO_STRIP[@]}" <<'PY'
import json, os, sys
from pathlib import Path

path = Path(sys.argv[1])
keys = sys.argv[2:]

try:
    with open(path) as f:
        data = json.load(f)
except Exception as e:
    print(f"  Could not parse {path}: {e}", file=sys.stderr)
    sys.exit(2)

mcp = data.get("mcp", {})
if not isinstance(mcp, dict):
    print(f"  mcp block is not a dict (type={type(mcp).__name__}); skipping", file=sys.stderr)
    sys.exit(0)

removed = 0
for key in keys:
    if key in mcp:
        del mcp[key]
        removed += 1

data["mcp"] = mcp
with open(path, "w") as f:
    json.dump(data, f, indent=2)
    f.write("\n")
print(f"  Stripped {removed} keys from mcp block")
PY
  ok "opencode.json MCP block patched"
}

# ── Step 6: Final summary ──────────────────────────────────────────────
print_summary() {
  heading "Summary"
  printf "  Bundle dir:        %s\n" "$BUNDLE_DIR"
  printf "  Bin dir:           %s\n" "$BIN_DIR"
  printf "  Repos dir:         %s\n" "$REPOS_DIR"
  printf "  opencode.json:     %s\n" "$OPENCODE_CONFIG"
  if [[ "$KEEP_GO" -eq 0 ]]; then
    printf "  Go binaries:       removed\n"
  else
    printf "  Go binaries:       %sKEPT%s\n" "$C_YELLOW" "$C_RESET"
  fi
  if [[ "$KEEP_BUNDLE" -eq 0 ]]; then
    printf "  sin-code-bundle:   uninstalled\n"
  else
    printf "  sin-code-bundle:   %sKEPT%s\n" "$C_YELLOW" "$C_RESET"
  fi
  if [[ "$KEEP_SUBSYSTEMS" -eq 0 ]]; then
    printf "  Subsystems+sin-brain: uninstalled\n"
  else
    printf "  Subsystems+sin-brain: %sKEPT%s\n" "$C_YELLOW" "$C_RESET"
  fi
  if [[ "$KEEP_CONFIG" -eq 0 ]]; then
    printf "  opencode.json mcp: stripped\n"
  else
    printf "  opencode.json mcp: %sKEPT%s\n" "$C_YELLOW" "$C_RESET"
  fi
  if [[ "$DRY_RUN" -eq 1 ]]; then
    printf "  Mode:              %sDRY RUN%s (no changes made)\n" "$C_YELLOW" "$C_RESET"
  else
    printf "  Mode:              %sLIVE%s\n" "$C_GREEN" "$C_RESET"
  fi
  printf "\n"
  ok "SIN-Code Tool Suite uninstall complete."
  info "To re-install: bash $BUNDLE_DIR/install.sh"
}

# ── Main ──────────────────────────────────────────────────────────────
main() {
  heading "SIN-Code Tool Suite — Uninstaller"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    info "DRY RUN — no system changes will be made"
  fi
  if [[ "$VERBOSE" -eq 1 ]]; then
    info "VERBOSE — every command will be echoed"
  fi

  confirm
  remove_go_binaries
  remove_python_bundle
  remove_python_subsystems
  strip_mcp_entries
  print_summary
}

main "$@"
