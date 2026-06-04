#!/usr/bin/env bash
# WS5: one-command editable dev setup for the full SIN-Code stack.
#
# Clones (if missing) and `pip install -e` each sibling subsystem next to this
# repo, then installs the Bundle itself with dev extras. Run from anywhere.
#
#   ./scripts/dev_install.sh            # clone missing repos + editable install
#   SIN_NO_CLONE=1 ./scripts/dev_install.sh   # only install repos already present
set -euo pipefail

BUNDLE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKSPACE="$(cd "${BUNDLE_DIR}/.." && pwd)"
ORG="https://github.com/OpenSIN-Code"

# Sibling repos in install order (dependencies before the bundle).
REPOS=(
  "SIN-Code-Semantic-Codebase-Knowledge-Graphs"
  "SIN-Code-Intent-Based-Diffing"
  "SIN-Code-Proof-of-Correctness"
  "SIN-Code-Ephemeral-Full-Stack-Mocking-Orchestration"
  "SIN-Code-Architectural-Debt-Watchdogs"
  "SIN-Code-Verification-Oracle"
  "SIN-Code-Orchestration"
  "SIN-Code-Review-Interface"
)

echo "== SIN-Code dev install =="
echo "workspace: ${WORKSPACE}"

for repo in "${REPOS[@]}"; do
  path="${WORKSPACE}/${repo}"
  if [[ ! -d "${path}" ]]; then
    if [[ "${SIN_NO_CLONE:-0}" == "1" ]]; then
      echo "SKIP  ${repo} (not present; SIN_NO_CLONE=1)"
      continue
    fi
    echo "CLONE ${repo}"
    git clone --depth 1 "${ORG}/${repo}.git" "${path}"
  fi
  echo "INSTALL ${repo}"
  pip install -e "${path}"
done

echo "INSTALL SIN-Code-Bundle [dev]"
pip install -e "${BUNDLE_DIR}[dev]"

echo "== done. run 'sin status' to verify subsystems =="
