#!/usr/bin/env bash
# Purpose: One-time PyPI Trusted Publisher setup for OpenSIN-Code/SIN-Code-Bundle.
# Docs: setup_pypi_publisher.doc.md
#
# Authenticates with PyPI via username + password, then POSTs a "pending
# publisher" registration. PyPI emails the maintainer a magic link to confirm.
# Once confirmed, every `v*` tag push triggers an automatic, tokenless publish
# via release.yml (id-token: write + environment: pypi).
#
# Ref: https://docs.pypi.org/trusted-publishers/adding-a-publisher/
#
# Usage:
#   bash tools/setup_pypi_publisher.sh
#   bash tools/setup_pypi_publisher.sh OpenSIN-Code SIN-Code-Bundle release.yml pypi
#
# After confirmation:
#   git tag v0.6.6 && git push origin v0.6.6
#   → release.yml builds sdist+wheel, attaches to GitHub Release, publishes to PyPI.

set -euo pipefail

OWNER="${1:-OpenSIN-Code}"
REPO="${2:-SIN-Code-Bundle}"
WORKFLOW="${3:-release.yml}"
ENVIRONMENT="${4:-pypi}"

# PyPI normalises project names (PEP 503): "SIN-Code-Bundle" → "sin-code-bundle".
# The Trusted Publisher entry must use the normalised name, not the display name.
PROJECT_NAME="$(printf '%s' "$REPO" | tr '[:upper:]' '[:lower:]' | tr '_' '-' | tr '/' '-' | tr -s '-')"
PYPI_NAME="$PROJECT_NAME"  # PEP 503 normalisation is just lower + replace _ with -

PYPI_API="https://pypi.org/_/v1/publisher"
PYPI_FALLBACK_URL="https://pypi.org/manage/account/publishing/"

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

info() { printf "%s[info]%s %s\n" "$C_BLUE"   "$C_RESET" "$*"; }
ok()   { printf "%s[ ok ]%s %s\n" "$C_GREEN"  "$C_RESET" "$*"; }
warn() { printf "%s[warn]%s %s\n" "$C_YELLOW" "$C_RESET" "$*"; }
err()  { printf "%s[fail]%s %s\n" "$C_RED"    "$C_RESET" "$*" >&2; }
heading() { printf "\n%s%s== %s ==%s\n" "$C_BOLD" "$C_BLUE" "$*" "$C_RESET"; }

# ── Preflight ────────────────────────────────────────────────────────────
if ! command -v curl >/dev/null 2>&1; then
  err "curl is required but not installed."
  exit 1
fi

if ! command -v python3 >/dev/null 2>&1; then
  err "python3 is required to parse PyPI's JSON response."
  exit 1
fi

heading "PyPI Trusted Publisher setup"
printf "  Owner:         %s\n" "$OWNER"
printf "  Repo:          %s\n" "$REPO"
printf "  Project name:  %s (PEP 503 normalised)\n" "$PYPI_NAME"
printf "  Workflow file: %s\n" "$WORKFLOW"
printf "  Environment:   %s\n" "$ENVIRONMENT"
echo

# ── Step 1: PyPI credentials ─────────────────────────────────────────────
info "Step 1/3: PyPI credentials"
info "Tip: PyPI now supports 2FA — if you have a TOTP device, append the 6-digit code to your password (e.g. 'hunter2 123456')."

read -r -p "  PyPI username (email): " PYPI_USER
if [[ -z "$PYPI_USER" ]]; then
  err "Username cannot be empty."
  exit 1
fi

# `read -s` keeps the password off the terminal scrollback.
read -r -s -p "  PyPI password (+ optional TOTP suffix): " PYPI_PASS
echo
if [[ -z "$PYPI_PASS" ]]; then
  err "Password cannot be empty."
  exit 1
fi

# ── Step 2: POST the pending publisher registration ─────────────────────
heading "Step 2/3: Register pending Trusted Publisher"

# PyPI's `_/v1/publisher` endpoint accepts a JSON body. Use --user for basic
# auth and --data-binary to keep the JSON intact (no URL encoding).
# Ref: https://docs.pypi.org/trusted-publishers/adding-a-publisher/#api
PAYLOAD=$(printf '{
  "name": "%s",
  "owner": "%s",
  "repository": "%s",
  "workflow_filename": "%s",
  "environment": "%s"
}' "$PYPI_NAME" "$OWNER" "$REPO" "$WORKFLOW" "$ENVIRONMENT")

info "POST $PYPI_API"
info "  name:                $PYPI_NAME"
info "  owner:               $OWNER"
info "  repository:          $REPO"
info "  workflow_filename:   $WORKFLOW"
info "  environment:         $ENVIRONMENT"
echo

# Capture body + status separately. --fail-with-body makes curl exit non-zero
# on 4xx/5xx, but we want to inspect the body in both success and failure.
HTTP_CODE=$(curl -sS -o /tmp/pypi_publisher_resp.json -w '%{http_code}' \
  -X POST "$PYPI_API" \
  -u "$PYPI_USER:$PYPI_PASS" \
  -H "Content-Type: application/json" \
  --data-binary "$PAYLOAD" \
  --max-time 30 \
  --connect-timeout 10 \
  2>/tmp/pypi_publisher_curl.err || echo "000")

RESP_BODY="$(cat /tmp/pypi_publisher_resp.json 2>/dev/null || echo "")"
CURL_ERR="$(cat /tmp/pypi_publisher_curl.err 2>/dev/null || true)"

# ── Step 3: interpret the response ───────────────────────────────────────
heading "Step 3/3: Result"

if [[ "$HTTP_CODE" == "000" ]]; then
  err "curl failed to reach PyPI."
  [[ -n "$CURL_ERR" ]] && err "  curl error: $CURL_ERR"
  err "Check your internet connection and DNS resolution for pypi.org."
  printf "\n  Manual fallback: %s\n" "$PYPI_FALLBACK_URL"
  exit 1
fi

case "$HTTP_CODE" in
  201|200)
    ok "PyPI accepted the pending publisher registration (HTTP $HTTP_CODE)."
    echo
    printf "  %sCHECK YOUR EMAIL%s for a message from PyPI titled\n" "$C_BOLD" "$C_RESET"
    printf "  'OpenSIN-Code/SIN-Code-Bundle: confirm pending publisher'.\n"
    printf "  Click the magic link in that email to complete the registration.\n"
    echo
    info "After confirmation, every 'git tag v*.*.* && git push origin v*.*.*' will auto-publish."
    rm -f /tmp/pypi_publisher_resp.json /tmp/pypi_publisher_curl.err
    exit 0
    ;;
  400|409|422)
    err "PyPI rejected the registration (HTTP $HTTP_CODE)."
    if [[ -n "$RESP_BODY" ]]; then
      # Pretty-print the JSON error if python3 is available, else dump raw.
      PRETTY=$(python3 -c "
import json, sys
try:
    print(json.dumps(json.loads('''$RESP_BODY'''), indent=2))
except Exception:
    print('(could not parse as JSON)')
" 2>/dev/null || echo "(could not parse as JSON)")
      err "Response body:"
      printf "%s\n" "$PRETTY" | sed 's/^/    /' >&2
    fi
    echo
    warn "Common causes:"
    warn "  • 400: Project name doesn't match an existing PyPI project."
    warn "         PyPI requires the project to already exist (uploaded once manually)."
    warn "         The first release must be a manual upload; subsequent ones use Trusted Publishing."
    warn "  • 409: A pending publisher for this repo + workflow already exists."
    warn "         Check $PYPI_FALLBACK_URL for an existing entry to confirm."
    warn "  • 422: Validation error (bad environment name, wrong owner/repo, etc.)"
    echo
    err "Manual fallback: register via the PyPI web UI:"
    printf "  1. Go to: %s\n" "$PYPI_FALLBACK_URL"
    printf "  2. Click 'Add a new pending publisher'\n"
    printf "  3. Project name:        %s\n" "$PYPI_NAME"
    printf "  4. Owner:               %s\n" "$OWNER"
    printf "  5. Repository name:     %s\n" "$REPO"
    printf "  6. Workflow filename:   %s\n" "$WORKFLOW"
    printf "  7. Environment name:    %s\n" "$ENVIRONMENT"
    rm -f /tmp/pypi_publisher_resp.json /tmp/pypi_publisher_curl.err
    exit 1
    ;;
  401|403)
    err "Authentication failed (HTTP $HTTP_CODE)."
    err "  • Wrong username or password."
    err "  • If you have 2FA, append the 6-digit TOTP to the password field."
    err "  • If you use a recovery code, paste it in the password field."
    echo
    err "Re-run with the correct credentials."
    rm -f /tmp/pypi_publisher_resp.json /tmp/pypi_publisher_curl.err
    exit 1
    ;;
  *)
    err "Unexpected HTTP $HTTP_CODE from PyPI."
    [[ -n "$RESP_BODY" ]] && printf "  body: %s\n" "$RESP_BODY" >&2
    printf "\n  Manual fallback: %s\n" "$PYPI_FALLBACK_URL"
    rm -f /tmp/pypi_publisher_resp.json /tmp/pypi_publisher_curl.err
    exit 1
    ;;
esac
