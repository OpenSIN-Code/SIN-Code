#!/usr/bin/env bash
# Purpose: Measure CEO Audit performance per axis + per phase
# Docs: SKILL.md
#
# Runs the audit against a target repo and reports per-axis timing
# in human-readable form. Saves a JSON baseline to
# /tmp/ceo-audit-baseline.json so future runs can detect regressions
# via --compare.
#
# Usage:
#   bash scripts/benchmark.sh [REPO_PATH] [--save] [--compare] [--profile=PROFILE]
#
# Flags:
#   REPO_PATH           repo to audit (default: current dir)
#   --save              write the timings to /tmp/ceo-audit-baseline.json
#   --compare           diff against the saved baseline (requires --save previously)
#   --profile=PROFILE   FULL|SECURITY|RELEASE|QUICK (default: FULL)
#   --baseline=PATH     use a custom baseline path (default: /tmp/ceo-audit-baseline.json)
#   --rounds=N          run N times and report median (default: 1)
#
# Output (stdout):
#   per-axis seconds + percentage of total
#   total wall time
#   (with --compare) Δ vs baseline per axis (slower/faster/new)
#
# Exit codes:
#   0   benchmark ran (regardless of grade)
#   1   repo not found or missing dependency
#   2   --compare requested but no baseline exists
set -euo pipefail
# Force C locale so awk prints decimal points, not commas.
# (The user might have LC_ALL=de_DE.UTF-8 which turns "0.218" into "0,218".)
export LC_ALL=C
export LANG=C

# ── Defaults ───────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_PATH="."
PROFILE="FULL"
SAVE=0
COMPARE=0
ROUNDS=1
BASELINE="${BASELINE:-/tmp/ceo-audit-baseline.json}"

# ── Color helpers ──────────────────────────────────────────────────────
if [[ -t 1 ]] && [[ -z "${NO_COLOR:-}" ]]; then
  C_RESET=$'\033[0m'; C_RED=$'\033[0;31m'; C_GREEN=$'\033[0;32m'
  C_YELLOW=$'\033[1;33m'; C_BLUE=$'\033[0;34m'; C_BOLD=$'\033[1m'
else
  C_RESET=""; C_RED=""; C_GREEN=""; C_YELLOW=""; C_BLUE=""; C_BOLD=""
fi

ok()    { printf "%s[OK]%s    %s\n" "$C_GREEN"  "$C_RESET" "$*"; }
info()  { printf "%s[INFO]%s  %s\n" "$C_BLUE"   "$C_RESET" "$*"; }
warn()  { printf "%s[WARN]%s  %s\n" "$C_YELLOW" "$C_RESET" "$*"; }
err()   { printf "%s[FAIL]%s  %s\n" "$C_RED"    "$C_RESET" "$*" >&2; }
heading(){ printf "\n%s%s== %s ==%s\n" "$C_BOLD" "$C_BLUE" "$*" "$C_RESET"; }

# ── Parse args ─────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --save)             SAVE=1; shift ;;
    --compare)          COMPARE=1; shift ;;
    --profile=*)        PROFILE="${1#*=}"; shift ;;
    --baseline=*)       BASELINE="${1#*=}"; shift ;;
    --rounds=*)         ROUNDS="${1#*=}"; shift ;;
    -h|--help)
      sed -n '2,30p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    -*) err "Unknown option: $1"; exit 1 ;;
    *)  REPO_PATH="$1"; shift ;;
  esac
done

case "$PROFILE" in
  FULL|SECURITY|RELEASE|QUICK) ;;
  *) err "Invalid profile: $PROFILE"; exit 1 ;;
esac

# Resolve repo
if [[ ! -d "$REPO_PATH" ]]; then
  err "Repo not found: $REPO_PATH"
  exit 1
fi
REPO_PATH="$(cd "$REPO_PATH" && pwd)"
REPO_NAME="$(basename "$REPO_PATH")"

# Determine axes for this profile
case "$PROFILE" in
  FULL)    AXES=(security performance quality testing deps docs architecture compliance) ;;
  SECURITY) AXES=(security) ;;
  RELEASE) AXES=(security testing deps) ;;
  QUICK)   AXES=(security quality) ;;
esac

# ── Pre-flight ─────────────────────────────────────────────────────────
heading "CEO Audit Benchmark"
info "Repo:    $REPO_PATH"
info "Profile: $PROFILE"
info "Rounds:  $ROUNDS"
info "Axes:    ${AXES[*]}"
[[ $SAVE -eq 1 ]]    && info "Save:    $BASELINE"
[[ $COMPARE -eq 1 ]] && info "Compare: $BASELINE"

if [[ $COMPARE -eq 1 ]] && [[ ! -f "$BASELINE" ]]; then
  err "Baseline not found: $BASELINE"
  err "  Run with --save first to create one"
  exit 2
fi

# ── Run N rounds, collect per-axis seconds ─────────────────────────────
# Use python to keep the associative-array semantics. zsh (opencode's
# default shell) does not support `declare -A` the same way bash does,
# and we want this script to run identically under bash 5+ and zsh 5+.
TIMES_FILE="$(mktemp -t ceo-bench-times.XXXXXX)"
python3 - "$TIMES_FILE" "${AXES[@]}" <<'PY'
import json, sys
path, *axes = sys.argv[1:]
with open(path, "w") as f:
    json.dump({a: 0.0 for a in axes}, f)
PY

add_time() {
  # add_time <axis> <delta>
  python3 - "$TIMES_FILE" "$1" "$2" <<'PY'
import json, sys
path, axis, delta = sys.argv[1], sys.argv[2], float(sys.argv[3])
with open(path) as f:
    d = json.load(f)
d[axis] = d.get(axis, 0.0) + delta
with open(path, "w") as f:
    json.dump(d, f)
PY
}

get_time() {
  python3 -c "import json,sys; print(json.load(open('$TIMES_FILE')).get('$1', 0.0))"
}

TOTAL_START=$(date +%s)
for round in $(seq 1 "$ROUNDS"); do
  if [[ "$ROUNDS" -gt 1 ]]; then
    info "Round $round/$ROUNDS"
  fi

  RUN_DIR="$(mktemp -d -t ceo-bench.XXXXXX)"
  mkdir -p "$RUN_DIR/findings"

  # We measure each axis script by running it directly (bypassing the
  # outer orchestration fan-out so we can isolate per-axis time). This
  # matches what the real audit does internally, just sequentially.
  for axis in "${AXES[@]}"; do
    t0=$(date +%s.%N)
    if bash "$SCRIPT_DIR/axis_${axis}.sh" "$REPO_PATH" "$RUN_DIR/findings" \
        > "$RUN_DIR/findings/axis-${axis}.log" 2>&1; then
      t1=$(date +%s.%N)
      elapsed=$(awk "BEGIN{printf \"%.3f\", $t1 - $t0}")
      add_time "$axis" "$elapsed"
    else
      warn "axis_$axis failed in round $round (see $RUN_DIR/findings/axis-${axis}.log)"
    fi
  done

  rm -rf "$RUN_DIR"
done
TOTAL_END=$(date +%s)
TOTAL_ELAPSED=$((TOTAL_END - TOTAL_START))

# ── Render results ─────────────────────────────────────────────────────
heading "Per-axis timing (seconds, sum of $ROUNDS round(s))"

TOTAL_AXIS=0
for axis in "${AXES[@]}"; do
  now=$(get_time "$axis")
  TOTAL_AXIS=$(awk "BEGIN{printf \"%.3f\", $TOTAL_AXIS + $now}")
done

printf "  %-15s %10s   %s\n" "axis" "seconds" "% of total"
printf "  %-15s %10s   %s\n" "---------------" "----------" "----------"
for axis in "${AXES[@]}"; do
  now=$(get_time "$axis")
  pct=$(awk "BEGIN{ if ($TOTAL_AXIS>0) printf \"%.1f\", $now*100/$TOTAL_AXIS; else print \"0.0\" }")
  printf "  %-15s %10ss   %s%%\n" "$axis" "$now" "$pct"
done
printf "  %-15s %10s\n" "---" "---"
printf "  %-15s %10ss\n" "TOTAL (axes)" "$TOTAL_AXIS"
printf "  %-15s %10ss   (wall, includes file I/O + Python startup)\n" "TOTAL (wall)" "$TOTAL_ELAPSED"

# ── Compare vs baseline ───────────────────────────────────────────────
if [[ $COMPARE -eq 1 ]]; then
  heading "Comparison vs baseline ($BASELINE)"
  printf "  %-15s %10s %10s %10s\n" "axis" "baseline" "now" "Δ"
  printf "  %-15s %10s %10s %10s\n" "---------------" "----------" "----------" "----------"
  for axis in "${AXES[@]}"; do
    prev=$(jq -r --arg k "$axis" '.[$k] // 0' "$BASELINE" 2>/dev/null || echo "0")
    now=$(get_time "$axis")
    delta=$(awk "BEGIN{printf \"%+.3f\", $now - $prev}")
    # Pad values to 8 chars so columns align (e.g. "0.123s" / "10.456s")
    if awk "BEGIN{exit !($now > $prev * 1.20)}"; then
      printf "  %-15s %8ss  %8ss  %s%sSLOWER%s\n" "$axis" "$prev" "$now" "$C_RED" "$delta" "$C_RESET"
    elif awk "BEGIN{exit !($now < $prev * 0.80)}"; then
      printf "  %-15s %8ss  %8ss  %s%sFASTER%s\n" "$axis" "$prev" "$now" "$C_GREEN" "$delta" "$C_RESET"
    else
      printf "  %-15s %8ss  %8ss  %s\n" "$axis" "$prev" "$now" "$delta"
    fi
  done
fi

# ── Save baseline ──────────────────────────────────────────────────────
if [[ $SAVE -eq 1 ]]; then
  OUT_JSON="$BASELINE"
  python3 - "$OUT_JSON" "$REPO_NAME" "$PROFILE" "$ROUNDS" "$TIMES_FILE" "${AXES[@]}" <<'PY'
import json, sys
from datetime import datetime, timezone
out, repo, profile, rounds, times_file, *axes = sys.argv[1:]
with open(times_file) as f:
    times = json.load(f)
payload = {
    "repo": repo,
    "profile": profile,
    "rounds": int(rounds),
    "timestamp": datetime.now(timezone.utc).isoformat(timespec="seconds"),
    "axes": {a: times.get(a, 0.0) for a in axes},
}
with open(out, "w") as f:
    json.dump(payload, f, indent=2)
PY
  ok "Baseline saved → $OUT_JSON"
fi

# Cleanup
rm -f "$TIMES_FILE"

heading "Benchmark complete"
echo ""
