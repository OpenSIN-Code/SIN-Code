#!/usr/bin/env bash
# WS5: iterate every SIN-Code repo present next to the Bundle, run its test
# suite, and aggregate pass/fail results. Exits non-zero if any repo fails.
set -uo pipefail

BUNDLE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKSPACE="$(cd "${BUNDLE_DIR}/.." && pwd)"

REPOS=(
  "SIN-Code-Semantic-Codebase-Knowledge-Graphs"
  "SIN-Code-Intent-Based-Diffing"
  "SIN-Code-Proof-of-Correctness"
  "SIN-Code-Ephemeral-Full-Stack-Mocking-Orchestration"
  "SIN-Code-Architectural-Debt-Watchdogs"
  "SIN-Code-Verification-Oracle"
  "SIN-Code-Orchestration"
  "SIN-Code-Review-Interface"
  "SIN-Code-Bundle"
)

declare -a RESULTS
overall=0

for repo in "${REPOS[@]}"; do
  path="${WORKSPACE}/${repo}"
  [[ "${repo}" == "SIN-Code-Bundle" ]] && path="${BUNDLE_DIR}"
  if [[ ! -d "${path}" ]]; then
    RESULTS+=("SKIP  ${repo} (not present)")
    continue
  fi
  echo "== ${repo} =="
  if (cd "${path}" && pytest -q); then
    RESULTS+=("PASS  ${repo}")
  else
    RESULTS+=("FAIL  ${repo}")
    overall=1
  fi
done

echo
echo "== aggregate results =="
printf '%s\n' "${RESULTS[@]}"
exit "${overall}"
