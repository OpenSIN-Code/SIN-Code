#!/usr/bin/env bash
# Purpose: CEO Audit Axis 4 — Testing (5 gates)
set -euo pipefail

REPO="$1"
OUT_DIR="$2"
OUT="$OUT_DIR/testing.json"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB="$SCRIPT_DIR/../lib"

echo '{"axis":"testing","gates":[],"findings":[]}' > "$OUT"

# ── Gate 4.1: Test coverage ────────────────────────────────────────────
if [[ -f "$REPO/pyproject.toml" || -f "$REPO/setup.py" || -f "$REPO/requirements.txt" ]]; then
  (cd "$REPO" && python3 -m pytest tests/ --cov=. --cov-report=json -q >/dev/null 2>&1) || true
  if [[ -f "$REPO/coverage.json" ]]; then
    PCT=$(jq '.totals.percent_covered_display // .totals.percent_covered' "$REPO/coverage.json" 2>/dev/null | tr -d "\n" || echo "0")
    if (( $(echo "$PCT < 70" | bc -l 2>/dev/null || echo 0) )); then
      python3 "$LIB/add_finding.py" "$OUT" "4.1" "MEDIUM" "TEST-COVERAGE" \
        "Low test coverage" "${PCT}% (< 70%)" \
        "Add unit tests for uncovered branches; focus on critical paths first"
    fi
  fi
elif [[ -f "$REPO/go.mod" ]]; then
  # Compute average coverage across all packages (skip the first 0.0% from packages with no tests).
  GO_COV=$(cd "$REPO" && go test ./... -cover 2>/dev/null | grep -oE '[0-9]+\.[0-9]+%' | sed 's/%//' | awk '{s+=$1; n++} END {if(n>0) printf "%.1f", s/n; else print "0"}')
  PCT="$GO_COV"
  if (( $(echo "$PCT < 70" | bc -l 2>/dev/null || echo 0) )); then
    python3 "$LIB/add_finding.py" "$OUT" "4.1" "MEDIUM" "TEST-COVERAGE" \
      "Low test coverage (Go)" "${PCT}%" \
      "Add tests, focus on handlers and business logic"
  fi
fi

# ── Gate 4.2: Flaky tests (time.sleep) ─────────────────────────────────
SLEEP_TESTS=$(scout --mcp 2>/dev/null <<EOF | jq -r '.result.content[0].text // ""' 2>/dev/null
{"jsonrpc":"2.0","method":"tools/call","id":42,"params":{"name":"scout","arguments":{"path":"$REPO","query":"time\\.sleep\\s*\\(\\s*[0-9]","search_type":"regex","max_results":50,"include_context":true}}}
EOF
)
SLEEP_COUNT=$(echo "$SLEEP_TESTS" | grep -c "Match" 2>/dev/null | tr -d "\n" || echo "0")
[[ "$SLEEP_COUNT" -gt 0 ]] && python3 "$LIB/add_finding.py" "$OUT" "4.2" "MEDIUM" "TEST-FLAKY" \
  "Flaky test patterns" "$SLEEP_COUNT uses of time.sleep in tests" \
  "Replace with polling, condition variables, or proper async waits"

# ── Gate 4.3: Test files > production files (over-tested) ──────────────
PROD_FILES=$(find "$REPO" -name "*.py" -not -path "*/test*" -not -path "*/.venv/*" -not -path "*/node_modules/*" 2>/dev/null | wc -l | tr -d ' ')
TEST_FILES=$(find "$REPO" -name "test_*.py" -o -name "*_test.py" 2>/dev/null | wc -l | tr -d ' ')
if [[ "$PROD_FILES" -gt 0 && "$TEST_FILES" -gt $((PROD_FILES * 2)) ]]; then
  python3 "$LIB/add_finding.py" "$OUT" "4.3" "LOW" "TEST-RATIO" \
    "Over-tested (test:prod ratio)" "${TEST_FILES}:${PROD_FILES}" \
    "Consider consolidating tests; ensure each test provides unique value"
fi

# ── Gate 4.4: No edge-case tests ───────────────────────────────────────
# Heuristic: look for tests that don't test None, 0, "", []
EDGE_TESTS=$(scout --mcp 2>/dev/null <<EOF | jq -r '.result.content[0].text // ""' 2>/dev/null
{"jsonrpc":"2.0","method":"tools/call","id":44,"params":{"name":"scout","arguments":{"path":"$REPO","query":"(test_.*empty|test_.*none|test_.*null|test_.*zero|test_.*boundary)","search_type":"regex","max_results":50,"include_context":false}}}
EOF
)
EDGE_COUNT=$(echo "$EDGE_TESTS" | grep -c "Match" 2>/dev/null | tr -d "\n" || echo "0")
TEST_FUNCS=$(scout --mcp 2>/dev/null <<EOF | jq -r '.result.content[0].text // ""' 2>/dev/null
{"jsonrpc":"2.0","method":"tools/call","id":44b,"params":{"name":"scout","arguments":{"path":"$REPO","query":"^def\\s+test_","search_type":"regex","max_results":500,"include_context":false}}}
EOF
)
TOTAL_TESTS=$(echo "$TEST_FUNCS" | grep -c "Match" 2>/dev/null | tr -d "\n" || echo "0")
if [[ "$TOTAL_TESTS" -gt 10 && "$EDGE_COUNT" -lt 3 ]]; then
  python3 "$LIB/add_finding.py" "$OUT" "4.4" "MEDIUM" "TEST-EDGE" \
    "Few edge-case tests" "Only $EDGE_COUNT edge-case tests of $TOTAL_TESTS total" \
    "Add tests for: None, 0, empty string, empty list, max int, unicode, very long input"
fi

# ── Gate 4.5: Tests share state (t.TempDir not used in Go, tmp_path in Python) ──
TEMP_DIR_USE=$(scout --mcp 2>/dev/null <<EOF | jq -r '.result.content[0].text // ""' 2>/dev/null
{"jsonrpc":"2.0","method":"tools/call","id":45,"params":{"name":"scout","arguments":{"path":"$REPO","query":"(tmp_path|t\\.TempDir|with tempfile\\.TemporaryDirectory|tempfile\\.mkdtemp|os\\.MkdirTemp)","search_type":"regex","max_results":50,"include_context":false}}}
EOF
)
TEMP_COUNT=$(echo "$TEMP_DIR_USE" | grep -c "Match" 2>/dev/null | tr -d "\n" || echo "0")
# If tests do file operations but no temp dir, flag it
FILE_OPS_TESTS=$(scout --mcp 2>/dev/null <<EOF | jq -r '.result.content[0].text // ""' 2>/dev/null
{"jsonrpc":"2.0","method":"tools/call","id":45b,"params":{"name":"scout","arguments":{"path":"$REPO","query":"open\\([^_]","search_type":"regex","max_results":50,"include_context":true}}}
EOF
)
FILE_OPS_COUNT=$(echo "$FILE_OPS_TESTS" | grep -c "Match" 2>/dev/null | tr -d "\n" || echo "0")
if [[ "$FILE_OPS_COUNT" -gt 3 && "$TEMP_COUNT" -eq 0 ]]; then
  python3 "$LIB/add_finding.py" "$OUT" "4.5" "MEDIUM" "TEST-ISOLATION" \
    "Test isolation risk" "Tests use file ops but no tmp_path/t.TempDir" \
    "Use pytest's tmp_path fixture or t.TempDir() to isolate test state"
fi

GATE_COUNT=$(jq '.gates | length' "$OUT")
FINDING_COUNT=$(jq '.findings | length' "$OUT")
echo "  [testing] $GATE_COUNT gates, $FINDING_COUNT findings" >&2
