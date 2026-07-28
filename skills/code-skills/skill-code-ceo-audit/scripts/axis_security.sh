#!/usr/bin/env bash
# Purpose: CEO Audit Axis 1 — production-focused security scan (12 gates)
set -euo pipefail
REPO="$1"
OUT_DIR="$2"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT="$OUT_DIR/security.json"
python3 "$SCRIPT_DIR/security_scan.py" "$REPO" "$OUT"
GATE_COUNT=$(jq '.gates | length' "$OUT")
FINDING_COUNT=$(jq '.findings | length' "$OUT")
echo "  [security] $GATE_COUNT gates, $FINDING_COUNT findings" >&2
