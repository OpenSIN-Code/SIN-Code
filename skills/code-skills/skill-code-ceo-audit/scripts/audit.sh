#!/usr/bin/env bash
# Purpose: CEO Audit — main entry point
# Docs: SKILL.md
#
# Runs the 8-axis, 47-gate audit. Designed to be run from the repo root
# or with --repo=<path>. Uses orchestrate to fan-out the 8 axes in parallel.
#
# Usage:
#   ceo-audit [options] [path]
#
# Options:
#   --profile=FULL|SECURITY|RELEASE|QUICK   default: FULL
#   --grade=A|B|C                           CI mode: exit 0 only on grade >= X
#   --output=DIR                            default: ~/ceo-audits/
#   --no-color                              disable colors
#   --json                                  also write JSON sidecar
#   --help                                  show help
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export PATH="$SCRIPT_DIR/compat-bin:$PATH"

# ── Defaults ───────────────────────────────────────────────────────────
PROFILE="FULL"
GRADE_GATE=""
OUTPUT_DIR="${CEO_AUDIT_OUTPUT:-$HOME/ceo-audits}"
REPO_PATH="."
WRITE_JSON=0

# ── Color helpers ──────────────────────────────────────────────────────
if [[ -t 1 ]] && [[ -z "${NO_COLOR:-}" ]]; then
  C_RESET=$'\033[0m'; C_RED=$'\033[0;31m'; C_GREEN=$'\033[0;32m'
  C_YELLOW=$'\033[1;33m'; C_BLUE=$'\033[0;34m'; C_BOLD=$'\033[1m'
else
  C_RESET=""; C_RED=""; C_GREEN=""; C_YELLOW=""; C_BLUE=""; C_BOLD=""
fi

info()    { printf "%s[INFO]%s  %s\n" "$C_BLUE"   "$C_RESET" "$*"; }
ok()      { printf "%s[OK]%s    %s\n" "$C_GREEN"  "$C_RESET" "$*"; }
warn()    { printf "%s[WARN]%s  %s\n" "$C_YELLOW" "$C_RESET" "$*"; }
err()     { printf "%s[FAIL]%s  %s\n" "$C_RED"    "$C_RESET" "$*" >&2; }
heading() { printf "\n%s%s== %s ==%s\n" "$C_BOLD" "$C_BLUE" "$*" "$C_RESET"; }

# ── Help ───────────────────────────────────────────────────────────────
usage() {
  cat <<'EOF'
CEO Audit — SOTA Repository Review (47 gates, 8 axes)

USAGE
  ceo-audit [OPTIONS] [REPO_PATH]

OPTIONS
  --profile=PROFILE       FULL (default) | SECURITY | RELEASE | QUICK
                          SECURITY: only axis 1 (12 gates, ~1 min)
                          RELEASE: security + tests + deps (skip perf/docs, ~2 min)
                          QUICK:   security + quality (14 gates, ~30 sec)
  --grade=X               CI mode: exit 0 only if grade >= X (A, B, or C)
  --output=DIR            Output directory (default: ~/ceo-audits/)
  --no-color              Disable ANSI colors
  --json                  Also write JSON sidecar
  --help                  Show this help

EXAMPLES
  ceo-audit                                 # audit current dir
  ceo-audit /path/to/repo                   # audit specific path
  ceo-audit --profile=SECURITY              # security-only, faster
  ceo-audit --grade=B                       # CI: fail if worse than B
  ceo-audit --output=/tmp/audit             # custom output

GRADING
  A+ (95-100)  SOTA-ready
  A  (85-94)   Production-ready
  B  (70-84)   Acceptable, monitor
  C  (55-69)   Needs work
  D  (40-54)   Significant risk
  F  (0-39)    Halt

EXIT CODES
  0  grade >= A (or --grade gate passed)
  1  grade B-C (acceptable with plan)
  2  grade D
  3  grade F or any CRITICAL finding
  4  audit failed (missing tool, unreadable repo)

OUTPUT
  <output>/<repo>-ceo-audit-<timestamp>/
    ├─ report.md          (board-ready Markdown)
    ├─ report.sarif       (GitHub Code Scanning)
    ├─ report.json        (programmatic)
    ├─ findings/          (raw per-axis output)
    └─ score.json         (numeric score breakdown)
EOF
}

# ── Parse args ─────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --profile=*) PROFILE="${1#*=}"; shift ;;
    --grade=*)   GRADE_GATE="${1#*=}"; shift ;;
    --output=*)  OUTPUT_DIR="${1#*=}"; shift ;;
    --no-color)  NO_COLOR=1; C_RESET=""; C_RED=""; C_GREEN=""; C_YELLOW=""; C_BLUE=""; C_BOLD=""; shift ;;
    --json)      WRITE_JSON=1; shift ;;
    --help|-h)   usage; exit 0 ;;
    -*)          err "Unknown option: $1"; usage; exit 4 ;;
    *)           REPO_PATH="$1"; shift ;;
  esac
done

# Validate
case "$PROFILE" in
  FULL|SECURITY|RELEASE|QUICK) ;;
  *) err "Invalid profile: $PROFILE (must be FULL|SECURITY|RELEASE|QUICK)"; exit 4 ;;
esac

case "$PROFILE" in
  FULL)     REQUESTED_AXES=(security performance quality testing deps docs architecture compliance) ;;
  SECURITY) REQUESTED_AXES=(security) ;;
  RELEASE)  REQUESTED_AXES=(security testing deps) ;;
  QUICK)    REQUESTED_AXES=(security quality) ;;
esac

# Resolve repo path
REPO_PATH="$(cd "$REPO_PATH" 2>/dev/null && pwd || echo "$REPO_PATH")"
if [[ ! -d "$REPO_PATH" ]]; then
  err "Repo not found: $REPO_PATH"
  exit 4
fi

REPO_NAME="$(basename "$REPO_PATH")"
RUN_ID="$(date +%Y%m%d-%H%M%S)-$(echo "$REPO_NAME" | tr -cd 'a-zA-Z0-9' | head -c 8)"
RUN_DIR="$OUTPUT_DIR/${REPO_NAME}-ceo-audit-${RUN_ID}"

# ── Banner ─────────────────────────────────────────────────────────────
heading "CEO AUDIT — $REPO_NAME"
info "Profile:    $PROFILE"
info "Path:       $REPO_PATH"
info "Output:     $RUN_DIR"
info "Run ID:     $RUN_ID"
[[ -n "$GRADE_GATE" ]] && info "Grade gate: $GRADE_GATE or better"
[[ "$WRITE_JSON" -eq 1 ]] && info "JSON:       enabled"

# ── Sanity checks ──────────────────────────────────────────────────────
heading "Phase 0: Pre-flight checks"
MISSING=0
MISSING_TOOLS=()

# Smart tool detection: checks PATH + venv (.venv/bin/) + Python imports
# because tools installed via `pip install -e .` only show in venv.
#
# Order: Python module check FIRST for tools whose names collide with
# npm packages (e.g., `oracle`). This avoids false positives where
# `command -v oracle` returns a pnpm-installed Node tool.
_tool_available() {
  local tool="$1"
  # Detect venv Python (if present) so Python module checks use it
  local py="python3"
  for venv in .venv venv env .env; do
    if [[ -x "$REPO_PATH/$venv/bin/python3" ]]; then
      py="$REPO_PATH/$venv/bin/python3"
      break
    fi
  done

  # 1. Python module check FIRST (avoids npm-binary false positives)
  case "$tool" in
    bandit)    $py -c "import bandit" 2>/dev/null && echo "python:bandit" && return 0 ;;
    ruff)      $py -c "import ruff" 2>/dev/null && echo "python:ruff" && return 0 ;;
    mypy)      $py -c "import mypy" 2>/dev/null && echo "python:mypy" && return 0 ;;
    sckg)       $py -c "import sin_code_sckg" 2>/dev/null && echo "python:sin_code_sckg" && return 0 ;;
    adw)       $py -c "import sin_code_adw" 2>/dev/null && echo "python:sin_code_adw" && return 0 ;;
    ibd)       $py -c "import sin_code_ibd" 2>/dev/null && echo "python:sin_code_ibd" && return 0 ;;
    poc)       $py -c "import sin_code_poc" 2>/dev/null && echo "python:sin_code_poc" && return 0 ;;
    efsm)      $py -c "import sin_code_efsm" 2>/dev/null && echo "python:sin_code_efsm" && return 0 ;;
    oracle)    $py -c "import sin_code_oracle" 2>/dev/null && echo "python:sin_code_oracle" && return 0 ;;
    orchestration) $py -c "import sin_code_orchestration" 2>/dev/null && echo "python:sin_code_orchestration" && return 0 ;;
    review)    $py -c "import sin_code_review_interface" 2>/dev/null && echo "python:sin_code_review_interface" && return 0 ;;
    brain)     $py -c "import sin_brain" 2>/dev/null && echo "python:sin_brain" && return 0 ;;
    simone)    $py -c "import simone_mcp" 2>/dev/null && echo "python:simone_mcp" && return 0 ;;
    honcho)    $py -c "import honcho" 2>/dev/null && echo "python:honcho" && return 0 ;;
  esac

  # 2. Standard PATH
  if command -v "$tool" >/dev/null 2>&1; then
    command -v "$tool"
    return 0
  fi
  # 3. Common venv locations (relative to cwd)
  for venv in .venv venv env .env; do
    if [[ -x "$REPO_PATH/$venv/bin/$tool" ]]; then
      echo "$REPO_PATH/$venv/bin/$tool"
      return 0
    fi
  done
  return 1
}

for tool in discover map grasp scout execute harvest orchestrate; do
  if path=$(_tool_available "$tool"); then
    ok "  $tool: $path"
  else
    warn "  $tool: MISSING (run 'install.sh' in SIN-Code)"
    MISSING=$((MISSING+1))
    MISSING_TOOLS+=("$tool")
  fi
done

# Optional but recommended — also checks Python-via-virtualenv
for tool in bandit mypy ruff gosec govulncheck pip-audit; do
  if path=$(_tool_available "$tool"); then
    ok "  $tool: $path"
  else
    info "  $tool: not installed (some gates will skip)"
  fi
done

# SIN-Code Python subsystems (always informational — graceful-degradation design)
heading "Phase 0b: SIN-Code Python subsystems (optional)"
for tool in sckg adw ibd poc efsm oracle orchestration review brain simone honcho; do
  if path=$(_tool_available "$tool"); then
    ok "  $tool: $path"
  else
    info "  $tool: not installed (optional backend — gates degrade gracefully)"
  fi
done

# Only FULL profile requires all 7 tools; QUICK/RELEASE/SECURITY can run with
# just ruff+bandit (the Go tools add depth but aren't blocking).
if [[ $MISSING -gt 0 ]] && [[ "${PROFILE:-FULL}" == "FULL" ]]; then
  err "Missing $MISSING core SIN-Code tools. Cannot run full audit."
  exit 4
elif [[ $MISSING -gt 0 ]]; then
  warn "Profile $PROFILE: running without $MISSING SIN-Code tools (some gates will skip)"
fi

# ── Setup output ───────────────────────────────────────────────────────
mkdir -p "$RUN_DIR/findings"
info "Run directory: $RUN_DIR"

# ── Phase 1: Recon ─────────────────────────────────────────────────────
heading "Phase 1: Recon (discover + map + sckg + sin-brain)"
START=$(date +%s)

# Run recon in parallel
RECON_FAILED=()

# Discover files
discover --mcp <<EOF > "$RUN_DIR/findings/01-discover.json" 2>&1 &
{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"discover","arguments":{"path":"$REPO_PATH","pattern":"**/*","max_results":5000,"sort_by":"relevance"}}}
EOF
discover_pid=$!

# Architecture map
map --mcp <<EOF > "$RUN_DIR/findings/02-map.json" 2>&1 &
{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"map","arguments":{"path":"$REPO_PATH","action":"map"}}}
EOF
map_pid=$!

# Wait for recon
if ! wait "$discover_pid"; then
  warn "discover returned non-zero"
  RECON_FAILED+=("discover")
fi
if ! wait "$map_pid"; then
  warn "map returned non-zero"
  RECON_FAILED+=("map")
fi

RECON_END=$(date +%s)
ok "Recon completed in $((RECON_END - START))s"

# ── Phase 2: Parallel audit axes ───────────────────────────────────────
heading "Phase 2: 8-axis parallel audit"

# Use the profile contract resolved during argument validation.
AXES=("${REQUESTED_AXES[@]}")

info "Running axes: ${AXES[*]}"

# Fan out axes via orchestrate or direct parallel execution
START=$(date +%s)
PIDS=()
PID_AXES=()
FAILED_AXES=()
for axis in "${AXES[@]}"; do
  if [[ -x "$SCRIPT_DIR/axis_${axis}.sh" ]]; then
    bash "$SCRIPT_DIR/axis_${axis}.sh" "$REPO_PATH" "$RUN_DIR/findings" \
      > "$RUN_DIR/findings/axis-${axis}.log" 2>&1 &
    PIDS+=("$!")
    PID_AXES+=("$axis")
  else
    warn "Axis script not found: axis_${axis}.sh — marking failed"
    FAILED_AXES+=("$axis")
  fi
done

# Wait for all axes and preserve the axis identity of every failure.
for i in "${!PIDS[@]}"; do
  if ! wait "${PIDS[$i]}"; then
    FAILED_AXES+=("${PID_AXES[$i]}")
  fi
done
AUDIT_END=$(date +%s)
ok "Audit completed in $((AUDIT_END - START))s (${#FAILED_AXES[@]} axes failed)"

# Machine-readable completeness metadata consumed by score.py.
PROFILE_ENV="$PROFILE" \
REQUESTED_AXES_ENV="${REQUESTED_AXES[*]}" \
FAILED_AXES_ENV="${FAILED_AXES[*]}" \
MISSING_TOOLS_ENV="${MISSING_TOOLS[*]}" \
RECON_FAILED_ENV="${RECON_FAILED[*]}" \
python3 - "$RUN_DIR/run_meta.json" <<'PYMETA'
import json
import os
import sys
from pathlib import Path

def words(name: str) -> list[str]:
    return [item for item in os.environ.get(name, "").split() if item]

Path(sys.argv[1]).write_text(json.dumps({
    "profile": os.environ["PROFILE_ENV"],
    "requested_axes": words("REQUESTED_AXES_ENV"),
    "failed_axes": words("FAILED_AXES_ENV"),
    "missing_tools": words("MISSING_TOOLS_ENV"),
    "recon_failed": words("RECON_FAILED_ENV"),
}, indent=2))
PYMETA

# ── Phase 3+4: Score and aggregate ─────────────────────────────────────
heading "Phase 3-4: Score + aggregate"
set +e
python3 "$SCRIPT_DIR/score.py" "$REPO_PATH" "$RUN_DIR" "${GRADE_GATE:-}"
SCORE_EXIT=$?
set -e

# ── Phase 5: Report ────────────────────────────────────────────────────
heading "Phase 5: Report generation"
python3 "$SCRIPT_DIR/report.py" "$REPO_PATH" "$RUN_DIR" "$PROFILE" "$WRITE_JSON"

# ── Final summary ──────────────────────────────────────────────────────
heading "Audit complete"
GRADE=$(jq -r '.grade // "?"' "$RUN_DIR/score.json" 2>/dev/null || echo "?")
SCORE=$(jq -r '.score // 0' "$RUN_DIR/score.json" 2>/dev/null || echo "0")
CRITICAL=$(jq -r '.critical // 0' "$RUN_DIR/score.json" 2>/dev/null || echo "0")
HIGH=$(jq -r '.high // 0' "$RUN_DIR/score.json" 2>/dev/null || echo "0")

echo ""
printf "  %sGrade:%s     %s%s%s\n" "$C_BOLD" "$C_RESET" "$C_BOLD$C_BLUE" "$GRADE" "$C_RESET"
printf "  %sScore:%s     %s/100\n" "$C_BOLD" "$C_RESET" "$SCORE"
printf "  %sCritical:%s  %s\n" "$C_BOLD" "$C_RESET" "$CRITICAL"
printf "  %sHigh:%s      %s\n" "$C_BOLD" "$C_RESET" "$HIGH"
printf "  %sReport:%s    %s/report.md\n" "$C_BOLD" "$C_RESET" "$RUN_DIR"
echo ""

# Grade gate enforcement
if [[ -n "$GRADE_GATE" ]]; then
  case "$GRADE_GATE" in
    A) MIN_SCORE=85 ;;
    B) MIN_SCORE=70 ;;
    C) MIN_SCORE=55 ;;
    *) MIN_SCORE=0 ;;
  esac
  if (( $(echo "$SCORE < $MIN_SCORE" | bc -l 2>/dev/null || echo 0) )); then
    err "Grade gate FAILED: $SCORE < $MIN_SCORE (require $GRADE_GATE+)"
    exit 1
  fi
fi

# Critical-finding-based exit
if [[ "$CRITICAL" -gt 0 ]]; then
  err "CRITICAL findings present — halting"
  exit 3
fi

exit $SCORE_EXIT
