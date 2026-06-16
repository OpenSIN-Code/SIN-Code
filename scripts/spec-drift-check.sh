#!/usr/bin/env bash
# spec-drift-check.sh — pre-commit hook for the Spec-Layer (issue #157).
# Docs: docs/SPEC-LAYER.md §"Drift detection"
#
# Behavior:
#   1. Run `sin spec check --all` on every *.spec.md tracked by git.
#   2. If any must-priority criterion fails, exit 1 and block the
#      commit. The operator can override with `git commit --no-verify`
#      (M3-mandated override path).
#
# Install (one-time, per-developer):
#   ln -s ../../scripts/spec-drift-check.sh .git/hooks/pre-commit
#
# Or via dev_install.sh which already installs the standard hook set.

set -euo pipefail

# Always run from the repo root so `git ls-files` and `sin spec check`
# see the right tree.
cd "$(git rev-parse --show-toplevel)"

# Policy: SIN_SPEC_DRIFT env var overrides the default (error).
#   off   — never block (developers opt-in)
#   warn  — print warnings, never block
#   error — block on must-failures (CI gate mode; default)
: "${SIN_SPEC_DRIFT:=error}"
export SIN_SPEC_DRIFT

# Locate the sin-code binary. Prefer the user's PATH; fall back to the
# locally-built binary at ./sin-code (dev workflow).
SIN_BIN="${SIN_BIN:-sin-code}"
if ! command -v "$SIN_BIN" >/dev/null 2>&1; then
	if [ -x "./sin-code" ]; then
		SIN_BIN="./sin-code"
	else
		echo "spec-drift-check: $SIN_BIN not on PATH and ./sin-code not found" >&2
		echo "  build with: go build -o sin-code ./cmd/sin-code/" >&2
		exit 0 # soft-skip: don't block commits when the binary is missing
	fi
fi

# Run the spec check. --all uses `git ls-files *.spec.md` so this is
# fast (one git invocation, then N subshells per spec).
if ! "$SIN_BIN" spec check --all; then
	echo "" >&2
	echo "spec-drift-check: at least one must-priority criterion failed." >&2
	echo "  Fix the spec or the code, then commit again." >&2
	echo "  Override with: git commit --no-verify" >&2
	exit 1
fi
