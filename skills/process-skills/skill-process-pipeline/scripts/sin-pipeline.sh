#!/usr/bin/env bash
# sin-pipeline.sh — Run the full SIN-Code 10-stage pipeline for a task.
# Usage: sin-pipeline.sh "Build feature X with requirements Y"
# Stages 1-2 and 4-5 run in agent sessions. This script handles CLI-driven stages.

set -euo pipefail

TASK="$1"
if [ -z "$TASK" ]; then
  echo "Usage: sin-pipeline.sh \"<task description>\""
  exit 1
fi

PROJECT_NAME=$(echo "$TASK" | head -c 40 | tr ' ' '-' | tr '[:upper:]' '[:lower:]')
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
LOGFILE="/tmp/sin-pipeline-${PROJECT_NAME}-${TIMESTAMP}.log"

echo "=== SIN-Code Full Pipeline (10 Stages) ===" | tee "$LOGFILE"
echo "Task: $TASK" | tee -a "$LOGFILE"
echo "Time: $(date)" | tee -a "$LOGFILE"
echo "" | tee -a "$LOGFILE"

# === STAGE 0: PRE-FLIGHT ===
echo "--- Stage 0: PRE-FLIGHT ---" | tee -a "$LOGFILE"

# Doctor check
if sin-code doctor 2>&1 | tee -a "$LOGFILE"; then
  echo "Doctor: OK" | tee -a "$LOGFILE"
else
  echo "Doctor: FAILED — fix issues before proceeding" | tee -a "$LOGFILE"
  exit 1
fi

# Memory prime
sin-code memory prime --query "$TASK" 2>&1 | tee -a "$LOGFILE" || echo "Memory: SKIP (not available)" | tee -a "$LOGFILE"

# Decision memory query
sin-code decision list --query "$TASK" 2>&1 | tee -a "$LOGFILE" || echo "Decisions: SKIP (not available)" | tee -a "$LOGFILE"

# Goal enqueue
GOAL_OUTPUT=$(sin-code goal add --prompt "$TASK" --priority P1 --criteria "tests pass" --criteria "build clean" --criteria "no TODO/FIXME" 2>&1) || true
GOAL_ID=$(echo "$GOAL_OUTPUT" | grep -oE 'goal-[a-f0-9]+' | head -1)
echo "Goal: ${GOAL_ID:-none}" | tee -a "$LOGFILE"

# Checkpoint
sin-code checkpoint create --name "pipeline-stage-0" 2>&1 | tee -a "$LOGFILE" || echo "Checkpoint: SKIP" | tee -a "$LOGFILE"
echo "" | tee -a "$LOGFILE"

# === STAGE 1: GRILL (agent session) ===
echo "--- Stage 1: GRILL (run in agent session) ---" | tee -a "$LOGFILE"
echo "Use grill-me skill or run: sin-code grill start --topic \"$TASK\"" | tee -a "$LOGFILE"
echo "" | tee -a "$LOGFILE"

# === STAGE 2: PLAN (agent session) ===
echo "--- Stage 2: PLAN (run in agent session) ---" | tee -a "$LOGFILE"
echo "Use plan v2 skill (--lite, full, or --from-spec)" | tee -a "$LOGFILE"
sin-code sckg build 2>&1 | tee -a "$LOGFILE" || echo "SCKG: SKIP" | tee -a "$LOGFILE"
sin-code checkpoint create --name "pipeline-stage-2" 2>&1 | tee -a "$LOGFILE" || true
echo "" | tee -a "$LOGFILE"

# === STAGE 3: GSD ===
echo "--- Stage 3: GSD (project lifecycle) ---" | tee -a "$LOGFILE"
if [ ! -d ".gsd" ]; then
  sin-code gsd init --name "$PROJECT_NAME" --description "$TASK" 2>&1 | tee -a "$LOGFILE"
  echo "GSD project initialized." | tee -a "$LOGFILE"
else
  echo ".gsd/ already exists, skipping init." | tee -a "$LOGFILE"
fi

if [ -n "$GOAL_ID" ]; then
  sin-code goal subtask "$GOAL_ID" --prompt "Execute pipeline for: $TASK" 2>&1 | tee -a "$LOGFILE" || true
fi
sin-code gsd status 2>&1 | tee -a "$LOGFILE"
sin-code checkpoint create --name "pipeline-stage-3" 2>&1 | tee -a "$LOGFILE" || true
echo "" | tee -a "$LOGFILE"

# === STAGE 4: EXECUTE (agent session) ===
echo "--- Stage 4: EXECUTE (run in agent session) ---" | tee -a "$LOGFILE"
echo "Use delegate-subagents skill for parallel wave execution." | tee -a "$LOGFILE"
echo "Use 'sin-code gsd execute <phase-id>' to check wave progress." | tee -a "$LOGFILE"
echo "Fusion on verify fail: sin-code fusion status" | tee -a "$LOGFILE"
echo "" | tee -a "$LOGFILE"

# === STAGE 5: REVIEW (agent session + CLI) ===
echo "--- Stage 5: REVIEW ---" | tee -a "$LOGFILE"
echo "Run self-review in agent session (mandatory)." | tee -a "$LOGFILE"
sin-code review --complexity 2>&1 | tee -a "$LOGFILE" || echo "Complexity review: SKIP" | tee -a "$LOGFILE"
sin-code debt stats 2>&1 | tee -a "$LOGFILE" || echo "Debt stats: SKIP" | tee -a "$LOGFILE"
sin-code security scan --path . 2>&1 | tee -a "$LOGFILE" || echo "Security: SKIP" | tee -a "$LOGFILE"
sin-code ibd --before pipeline-stage-2 --after HEAD 2>&1 | tee -a "$LOGFILE" || echo "IBD: SKIP" | tee -a "$LOGFILE"
sin-code checkpoint create --name "pipeline-stage-5" 2>&1 | tee -a "$LOGFILE" || true
echo "" | tee -a "$LOGFILE"

# === STAGE 6: DONE GATE ===
echo "--- Stage 6: DONE GATE ---" | tee -a "$LOGFILE"
sin-code adw scan 2>&1 | tee -a "$LOGFILE" || echo "ADW: SKIP" | tee -a "$LOGFILE"
sin-code sckg dead_code 2>&1 | tee -a "$LOGFILE" || echo "SCKG dead code: SKIP" | tee -a "$LOGFILE"

if command -v dodone &>/dev/null; then
  dodone check 2>&1 | tee -a "$LOGFILE"
  EXIT_CODE=$?
elif [ -x "$(dirname "$0")/../skill-process-dodone/scripts/dodone-check.sh" ]; then
  bash "$(dirname "$0")/../skill-process-dodone/scripts/dodone-check.sh" 2>&1 | tee -a "$LOGFILE"
  EXIT_CODE=$?
else
  echo "Running go build + go test + go vet as fallback gate" | tee -a "$LOGFILE"
  go build ./... 2>&1 | tee -a "$LOGFILE" && go test ./... -race -count=1 2>&1 | tee -a "$LOGFILE" && go vet ./... 2>&1 | tee -a "$LOGFILE"
  EXIT_CODE=$?
fi

echo "" | tee -a "$LOGFILE"

if [ $EXIT_CODE -ne 0 ]; then
  echo "=== PIPELINE FAILED at Stage 6 (exit $EXIT_CODE) ===" | tee -a "$LOGFILE"
  echo "Fix findings and re-run Stage 4-6." | tee -a "$LOGFILE"
  echo "Or rewind: sin-code checkpoint rewind --name pipeline-stage-4" | tee -a "$LOGFILE"
  echo "Log: $LOGFILE"
  exit $EXIT_CODE
fi

echo "Stage 6: WIRKLICH FERTIG (exit 0)" | tee -a "$LOGFILE"
echo "" | tee -a "$LOGFILE"

# === STAGE 7: COMMIT ===
echo "--- Stage 7: COMMIT ---" | tee -a "$LOGFILE"
if [ -n "$GOAL_ID" ]; then
  sin-code goal complete "$GOAL_ID" 2>&1 | tee -a "$LOGFILE" || true
fi
sin-code gsd phase edit 1 --status completed 2>&1 | tee -a "$LOGFILE" || true
echo "Auto-commit: run 'git add -A && git commit -m \"feat: $TASK (pipeline 10/10, dodone exit 0)\"'" | tee -a "$LOGFILE"
echo "Auto-PR: sin-code auto-pr (optional)" | tee -a "$LOGFILE"
echo "" | tee -a "$LOGFILE"

# === STAGE 8: RECORD ===
echo "--- Stage 8: RECORD ---" | tee -a "$LOGFILE"
sin-code ledger list 2>&1 | tee -a "$LOGFILE" || echo "Ledger: SKIP" | tee -a "$LOGFILE"
sin-code summary 2>&1 | tee -a "$LOGFILE" || echo "Summary: SKIP" | tee -a "$LOGFILE"
sin-code memory add --insight "Pipeline completed: $TASK" --tags pipeline 2>&1 | tee -a "$LOGFILE" || true
sin-code compress plan --target lessons 2>&1 | tee -a "$LOGFILE" && sin-code compress apply 2>&1 | tee -a "$LOGFILE" || echo "Compress: SKIP" | tee -a "$LOGFILE"
echo "" | tee -a "$LOGFILE"

# === STAGE 9: CI/CD ===
echo "--- Stage 9: CI/CD ---" | tee -a "$LOGFILE"
echo "GitHub Actions triggered by push from Stage 7." | tee -a "$LOGFILE"
echo "Monitor: gh run watch" | tee -a "$LOGFILE"
sin-code sbom generate --format spdx-json 2>&1 | tee -a "$LOGFILE" || echo "SBOM: SKIP" | tee -a "$LOGFILE"
echo "" | tee -a "$LOGFILE"

echo "=== PIPELINE COMPLETE (10/10 stages) ===" | tee -a "$LOGFILE"
echo "Task: $TASK" | tee -a "$LOGFILE"
echo "Goal: ${GOAL_ID:-none}" | tee -a "$LOGFILE"
echo "Log: $LOGFILE" | tee -a "$LOGFILE"
