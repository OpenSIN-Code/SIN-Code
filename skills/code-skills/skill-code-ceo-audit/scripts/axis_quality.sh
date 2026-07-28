#!/usr/bin/env bash
# Purpose: CEO Audit Axis 3 — evidence-focused code-quality scan (7 gates)
set -euo pipefail
REPO="$1"
OUT_DIR="$2"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT="$OUT_DIR/quality.json"
python3 "$SCRIPT_DIR/quality_scan.py" "$REPO" "$OUT"
GATE_COUNT=$(jq '.gates | length' "$OUT")
FINDING_COUNT=$(jq '.findings | length' "$OUT")
echo "  [quality] $GATE_COUNT gates, $FINDING_COUNT findings" >&2
