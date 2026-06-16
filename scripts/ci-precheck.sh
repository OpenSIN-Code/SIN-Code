#!/usr/bin/env bash
# ci-precheck.sh — mirror the §13.2 pre-PR checklist as an executable.
# Docs: docs/CI-RUNBOOK.md (the operating manual behind this script)
#
# Usage:
#   ./scripts/ci-precheck.sh              # full pre-PR check
#   ./scripts/ci-precheck.sh --fast      # skip the slow ones (go test, ceo-audit)
#   ./scripts/ci-precheck.sh --pr 144    # also poll a specific PR's status
#
# Exit codes (compose with && in shell pipelines):
#   0  all steps green
#   1  a local step failed
#   2  --pr reported a red required CI check on the remote
#
# NEVER pass -race to go test here. Race coverage is a pre-commit-hook
# concern, not a CI concern — including -race here would multiply
# runtime ~10x without changing what CI catches.

set -euo pipefail

# --- argument parsing -------------------------------------------------------

FAST=0
PR_NUMBER=""
while [[ $# -gt 0 ]]; do
	case "$1" in
		--fast)
			FAST=1
			shift
			;;
		--pr)
			PR_NUMBER="${2:-}"
			if [[ -z "$PR_NUMBER" ]]; then
				echo "ERROR: --pr requires a PR number" >&2
				exit 1
			fi
			shift 2
			;;
		--pr=*)
			PR_NUMBER="${1#--pr=}"
			shift
			;;
		-h|--help)
			grep -E '^#( |$)' "$0" | sed 's/^# \?//'
			exit 0
			;;
		*)
			echo "ERROR: unknown flag: $1" >&2
			exit 1
			;;
	esac
done

# --- helpers ----------------------------------------------------------------

# run_step <name> <cmd...>
# Runs <cmd...>, prints a labeled timing line, exits non-zero on failure.
run_step() {
	local name="$1"; shift
	local start end dur
	start=$(date +%s)
	printf "  %-32s " "$name"
	if "$@"; then
		end=$(date +%s)
		dur=$(( end - start ))
		printf "ok (%ss)\n" "$dur"
	else
		local rc=$?
		end=$(date +%s)
		dur=$(( end - start ))
		printf "FAIL (%ss, exit=%d)\n" "$dur" "$rc" >&2
		exit 1
	fi
}

# run_step_optional <name> <cmd...>
# Like run_step, but a non-zero exit is reported and the script continues
# with the next step. Used for steps whose failure we want to *see* without
# blocking (e.g. gosec SARIF warnings, which are exit-1 by default).
run_step_optional() {
	local name="$1"; shift
	local start end dur
	start=$(date +%s)
	printf "  %-32s " "$name"
	if "$@"; then
		end=$(date +%s)
		dur=$(( end - start ))
		printf "ok (%ss)\n" "$dur"
	else
		local rc=$?
		end=$(date +%s)
		dur=$(( end - start ))
		printf "WARN (%ss, exit=%d) — continuing\n" "$dur" "$rc"
		return 0
	fi
}

# --- preflight --------------------------------------------------------------

# Always run from the repo root so `go build ./cmd/sin-code` resolves.
cd "$(git rev-parse --show-toplevel)"

echo "ci-precheck.sh: §13.2 mirror"
echo "  repo:    $(basename "$(pwd)")"
echo "  branch:  $(git rev-parse --abbrev-ref HEAD)"
echo "  HEAD:    $(git rev-parse --short HEAD)"
if [[ -n "$PR_NUMBER" ]]; then
	echo "  pr:      #$PR_NUMBER"
fi
echo

# --- fast vs full -----------------------------------------------------------

if [[ "$FAST" -eq 1 ]]; then
	echo "  mode:    --fast (skipping go test, ceo-audit)"
else
	echo "  mode:    full"
fi
echo

# --- §13.2 steps in order ---------------------------------------------------

# Format check first — fastest, catches the most common lint failure
# (see CI-RUNBOOK.md §3.2 for why this is the #1 reason CI fails on PRs).
# Scope: only `cmd/sin-code/` — the path the lint-and-security/golangci-lint
# workflow also gates on. The other top-level Go submodules (sin-iac, sin-sast,
# etc.) are independent binaries with their own gofmt conventions and are
# out of scope for a PR to sin-code proper.
# `gofmt -l` exits 0 even when it finds unformatted files — the *list*
# on stdout is the signal. We check both: any unformatted file = fail.
printf "  %-32s " "gofmt (cmd/sin-code/)"
UNFMT=$(gofmt -l cmd/sin-code/ 2>/dev/null || true)
if [[ -n "$UNFMT" ]]; then
	printf "FAIL (%d unformatted files)\n" "$(echo "$UNFMT" | wc -l | tr -d ' ')" >&2
	echo "$UNFMT" | head -20 >&2
	if [[ $(echo "$UNFMT" | wc -l) -gt 20 ]]; then
		echo "  ... and more (truncated; run 'gofmt -l cmd/sin-code/' to see all)" >&2
	fi
	exit 1
else
	printf "ok\n"
fi

# Build the main binary. Exact go-ci step: `go build ./cmd/sin-code`.
run_step "go build (cmd/sin-code)" go build ./cmd/sin-code

# Vet. Exact go-ci step: `go vet ./cmd/sin-code/... ./cmd/sin-code/internal/...`.
run_step "go vet" go vet ./cmd/sin-code/... ./cmd/sin-code/internal/...

# Validate bundled skills. Exact go-ci step:
#   `pip install pyyaml && python3 scripts/validate_skill.py --all-bundled --strict`
# The --strict flag is what fails the CI job on warnings; without it,
# local validation is misleading.
run_step "validate_skill.py" \
	bash -c 'command -v python3 >/dev/null || { echo "python3 not on PATH; install with dev_install.sh" >&2; exit 1; }; pip install --quiet pyyaml >/dev/null 2>&1 || true; python3 scripts/validate_skill.py --all-bundled --strict'

# Tests. SKIPPED in --fast mode. Never -race (see file header).
if [[ "$FAST" -eq 0 ]]; then
	run_step "go test (no race)" \
		bash -c 'go test ./cmd/sin-code/ ./cmd/sin-code/internal/ -count=1 2>&1 | tail -50'
else
	printf "  %-32s skipped (--fast)\n" "go test (no race)"
fi

# golangci-lint. Matches the lint-and-security/golangci-lint workflow.
# Use --timeout=5m as the workflow does; in tight loops this can also be
# set to 1m but tests may flake.
if command -v golangci-lint >/dev/null 2>&1; then
	run_step "golangci-lint" golangci-lint run --timeout=5m ./...
else
	printf "  %-32s skipped (golangci-lint not installed; install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@v2.5.0)\n" "golangci-lint"
fi

# govulncheck. Matches the lint-and-security/govulncheck workflow.
if command -v govulncheck >/dev/null 2>&1; then
	run_step "govulncheck" govulncheck ./...
else
	printf "  %-32s skipped (govulncheck not installed; install: go install golang.org/x/vuln/cmd/govulncheck@latest)\n" "govulncheck"
fi

# gosec. SARIF output (matches the lint-and-security/gosec (SARIF upload)
# workflow). The plain `gosec` line in the PR view is the GitHub-Checks-API
# artifact that always shows "fail" — verify the SARIF output yourself
# by opening gosec.sarif in a SARIF viewer. We do not gate on the plain
# gosec exit code (see AGENTS.md §13.5).
if command -v gosec >/dev/null 2>&1; then
	run_step_optional "gosec (SARIF)" gosec -no-fail -fmt sarif -out /tmp/gosec.sarif ./...
else
	printf "  %-32s skipped (gosec not installed; install: go install github.com/securego/gosec/v2/cmd/gosec@latest)\n" "gosec (SARIF)"
fi

# ceo-audit. SKIPPED in --fast mode. n8n-delegated per mandate M1; this
# script only does the local dry-run variant if ceo-audit.sh is on PATH.
if [[ "$FAST" -eq 0 ]]; then
	if command -v ceo-audit.sh >/dev/null 2>&1; then
		run_step "ceo-audit (local dry-run)" ceo-audit.sh --profile QUICK --grade B
	else
		printf "  %-32s skipped (ceo-audit.sh not on PATH; CI runs this via n8n delegation, see AGENTS.md §M1)\n" "ceo-audit"
	fi
else
	printf "  %-32s skipped (--fast)\n" "ceo-audit"
fi

echo

# --- --pr: remote status poll ----------------------------------------------

if [[ -n "$PR_NUMBER" ]]; then
	echo "remote CI status for PR #$PR_NUMBER:"
	if ! command -v gh >/dev/null 2>&1; then
		echo "  ERROR: gh CLI not on PATH; install with: brew install gh" >&2
		exit 1
	fi
	if ! gh pr checks "$PR_NUMBER" 2>&1 | sed 's/^/  /'; then
		echo "  ERROR: gh pr checks failed; verify the PR number and that you are authenticated" >&2
		exit 1
	fi
	echo

	# Translate the remote state into an exit code. The required checks
	# are: lint-and-security/golangci-lint, lint-and-security/govulncheck,
	# go-ci/test, ceo-audit/ceo-audit. Anything else is informational.
	# The bare 'gosec' line is the known Checks-API artifact and is
	# excluded from the gate (see AGENTS.md §13.5).
	REQUIRED='golangci-lint|govulncheck|go test|CEO Audit'
	RED=$(gh pr checks "$PR_NUMBER" 2>/dev/null | awk -v req="$REQUIRED" '
		{
			# The first column is the check name; we treat any other
			# whitespace-separated token after that as a candidate state.
			for (i=1; i<=NF; i++) {
				if ($i == "fail") { state = "fail"; break }
			}
			name=$1
			if (name ~ req && state == "fail") { print name; state=""; found=1 }
		}
		END { if (!found) exit 0; else exit 2 }
	') || rc=$?
	if [[ $rc -eq 2 ]]; then
		echo "ERROR: a required CI check is failing on PR #$PR_NUMBER" >&2
		echo "  See the runbook docs/CI-RUNBOOK.md §4 for recovery steps" >&2
		exit 2
	fi
	echo "  required checks: all green"
fi

echo "ci-precheck: all green"
