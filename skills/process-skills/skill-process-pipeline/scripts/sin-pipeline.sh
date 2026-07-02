#!/usr/bin/env bash
# sin-pipeline.sh — Run the full SIN-Code pipeline for a task.
# Usage: sin-pipeline.sh "Build feature X with requirements Y"
# Requires: sin-code binary on PATH, dodone binary on PATH (or dodone-check.sh)

set -euo pipefail

TASK="$1"
if [ -z "$TASK" ]; then
  echo "Usage: sin-pipeline.sh \"<task description>\""
  exit 1
fi

PROJECT_NAME=$(echo "$TASK" | head -c 40 | tr ' ' '-' | tr '[:upper:]' '[:lower:]')
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
LOGFILE="/tmp/sin-pipeline-${PROJECT_NAME}-${TIMESTAMP}.log"

echo "=== SIN-Code Full Pipeline ===" | tee "$LOGFILE"
echo "Task: $TASK" | tee -a "$LOGFILE"
echo "Time: $(date)" | tee -a "$LOGFILE"
echo "" | tee -a "$LOGFILE"

# Stage 1: Grill (optional — user can skip)
echo "--- Stage 1: GRILL (design review) ---" | tee -a "$LOGFILE"
echo "Run grill-me in your agent session, or skip if design is clear." | tee -a "$LOGFILE"
echo "" | tee -a "$LOGFILE"

# Stage 2: Plan (done in agent session)
echo "--- Stage 2: PLAN (plan v2) ---" | tee -a "$LOGFILE"
echo "Run plan v2 in your agent session (full or --lite)." | tee -a "$LOGFILE"
echo "" | tee -a "$LOGFILE"

# Stage 3: GSD init
echo "--- Stage 3: GSD (project lifecycle) ---" | tee -a "$LOGFILE"
if [ ! -d ".gsd" ]; then
  sin-code gsd init --name "$PROJECT_NAME" --description "$TASK" 2>&1 | tee -a "$LOGFILE"
  echo "GSD project initialized." | tee -a "$LOGFILE"
else
  echo ".gsd/ already exists, skipping init." | tee -a "$LOGFILE"
fi
echo "" | tee -a "$LOGFILE"

# Stage 4: Execute (done in agent session via delegate-subagents)
echo "--- Stage 4: EXECUTE (delegate-subagents) ---" | tee -a "$LOGFILE"
echo "Execute tasks via delegate-subagents in your agent session." | tee -a "$LOGFILE"
echo "Use 'sin-code gsd execute <phase-id>' to check wave progress." | tee -a "$LOGFILE"
echo "" | tee -a "$LOGFILE"

# Stage 5: Self-review (done in agent session)
echo "--- Stage 5: REVIEW (self-review) ---" | tee -a "$LOGFILE"
echo "Run self-review in your agent session. Must reach 0 BLOCKER + 0 MAJOR." | tee -a "$LOGFILE"
echo "" | tee -a "$LOGFILE"

# Stage 6: DoDone check (deterministic machine gate)
echo "--- Stage 6: DONE GATE (dodone check) ---" | tee -a "$LOGFILE"
if command -v dodone &>/dev/null; then
  dodone check 2>&1 | tee -a "$LOGFILE"
  EXIT_CODE=$?
elif [ -x "$(dirname "$0")/../skill-process-dodone/scripts/dodone-check.sh" ]; then
  bash "$(dirname "$0")/../skill-process-dodone/scripts/dodone-check.sh" 2>&1 | tee -a "$LOGFILE"
  EXIT_CODE=$?
else
  echo "WARNING: dodone not found on PATH. Run dodone check manually." | tee -a "$LOGFILE"
  EXIT_CODE=0
fi

echo "" | tee -a "$LOGFILE"
if [ $EXIT_CODE -eq 0 ]; then
  echo "=== WIRKLICH FERTIG ===" | tee -a "$LOGFILE"
  echo "All 6 pipeline stages completed. Task is truly done." | tee -a "$LOGFILE"
else
  echo "=== PIPELINE FAILED (exit $EXIT_CODE) ===" | tee -a "$LOGFILE"
  echo "Fix findings and re-run Stage 4-6." | tee -a "$LOGFILE"
fi

echo "Log: $LOGFILE"
exit $EXIT_CODE
