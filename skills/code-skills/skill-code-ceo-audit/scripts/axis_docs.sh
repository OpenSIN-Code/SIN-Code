#!/usr/bin/env bash
# Purpose: CEO Audit Axis 6 — Documentation (4 gates)
set -euo pipefail

REPO="$1"
OUT_DIR="$2"
OUT="$OUT_DIR/docs.json"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB="$SCRIPT_DIR/../lib"

echo '{"axis":"docs","gates":[],"findings":[]}' > "$OUT"

# ── Gate 6.1: README missing or < 50 lines ─────────────────────────────
README=""
for f in README.md README.rst README.txt; do
  [[ -f "$REPO/$f" ]] && README="$REPO/$f" && break
done
if [[ -z "$README" ]]; then
  python3 "$LIB/add_finding.py" "$OUT" "6.1" "HIGH" "DOC-README" \
    "Missing README" "No README file found" \
    "Add a README.md with: project description, install, usage, contributing"
else
  LINES=$(wc -l < "$README" | tr -d ' ')
  if [[ "$LINES" -lt 50 ]]; then
    python3 "$LIB/add_finding.py" "$OUT" "6.1" "MEDIUM" "DOC-README" \
      "Sparse README" "Only $LINES lines" \
      "Expand README: add overview, install, usage examples, API, contributing, license"
  fi
fi

# ── Gate 6.2: CHANGELOG not updated in last release ────────────────────
CHANGELOG=""
for f in CHANGELOG.md CHANGELOG.rst CHANGES.md; do
  [[ -f "$REPO/$f" ]] && CHANGELOG="$REPO/$f" && break
done
if [[ -n "$CHANGELOG" ]]; then
  # Check if CHANGELOG was modified in the last release commit
  LATEST_TAG=$(cd "$REPO" && git describe --tags --abbrev=0 2>/dev/null || echo "")
  if [[ -n "$LATEST_TAG" ]]; then
    CHANGELOG_AT_TAG=$(cd "$REPO" && git show "$LATEST_TAG:$CHANGELOG" 2>/dev/null | wc -l | tr -d ' ')
    CHANGELOG_NOW=$(wc -l < "$CHANGELOG" | tr -d ' ')
    if [[ "$CHANGELOG_NOW" -le "$CHANGELOG_AT_TAG" ]]; then
      python3 "$LIB/add_finding.py" "$OUT" "6.2" "MEDIUM" "DOC-CHANGELOG" \
        "CHANGELOG not updated since $LATEST_TAG" \
        "Add a new entry describing the latest release changes"
    fi
  fi
else
  python3 "$LIB/add_finding.py" "$OUT" "6.2" "LOW" "DOC-CHANGELOG" \
    "No CHANGELOG" "Consider adding one for users to track changes"
fi

# ── Gate 6.3: .doc.md missing for public modules ───────────────────────
# Use sin-codocs check
if command -v sin >/dev/null 2>&1 || python3 -c "import sin_code_bundle" 2>/dev/null; then
  CODOCS=$(cd "$REPO" && python3 -m sin_code_bundle codocs check 2>&1 | head -50 || echo "")
  MISSING_DOCS=$(echo "$CODOCS" | grep -c "MISSING\|missing\|broken" 2>/dev/null | tr -d "\n" || echo "0")
  if [[ "$MISSING_DOCS" -gt 0 ]]; then
    python3 "$LIB/add_finding.py" "$OUT" "6.3" "MEDIUM" "DOC-CODOCS" \
      "Missing .doc.md companions" "$MISSING_DOCS modules" \
      "Run 'sin codocs list' for full report; add .doc.md files for public modules"
  fi
else
  python3 "$LIB/add_finding.py" "$OUT" "6.3" "INFO" "DOC-CODOCS" \
    "CoDocs check skipped" "sin_code_bundle not installed" \
    "Install SIN-Code: pip install sin-code"
fi

# ── Gate 6.4: Inline comments explaining "why" ─────────────────────────
# Heuristic: check ratio of inline comments to code in non-test files
COMMENTED=$(scout --mcp 2>/dev/null <<EOF | jq -r '.result.content[0].text // ""' 2>/dev/null
{"jsonrpc":"2.0","method":"tools/call","id":64,"params":{"name":"scout","arguments":{"path":"$REPO","query":"(#[^!].{20,}|//[^/!].{20,})","search_type":"regex","max_results":200,"include_context":false}}}
EOF
)
COMMENT_COUNT=$(echo "$COMMENTED" | grep -c "Match" 2>/dev/null | tr -d "\n" || echo "0")
CODE_FILES=$(find "$REPO" -type f \( -name "*.py" -o -name "*.go" -o -name "*.ts" -o -name "*.js" -o -name "*.rs" \) 2>/dev/null | awk '!/(test|spec|vendor|node_modules|\.venv)/ {count++} END {print count+0}')
if [[ "$CODE_FILES" -gt 0 ]]; then
  RATIO=$((COMMENT_COUNT / CODE_FILES))
  if [[ "$RATIO" -lt 3 && "$CODE_FILES" -gt 10 ]]; then
    python3 "$LIB/add_finding.py" "$OUT" "6.4" "LOW" "DOC-COMMENTS" \
      "Sparse inline comments" "Ratio: $RATIO comments per code file" \
      "Add comments explaining WHY (not WHAT); use sin-codocs for the overview"
  fi
fi

GATE_COUNT=$(jq '.gates | length' "$OUT")
FINDING_COUNT=$(jq '.findings | length' "$OUT")
echo "  [docs] $GATE_COUNT gates, $FINDING_COUNT findings" >&2
