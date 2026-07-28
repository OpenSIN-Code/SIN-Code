#!/usr/bin/env bash
# Purpose: CEO Audit Axis 7 — Architecture (4 gates)
set -euo pipefail

REPO="$1"
OUT_DIR="$2"
OUT="$OUT_DIR/architecture.json"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB="$SCRIPT_DIR/../lib"

echo '{"axis":"architecture","gates":[],"findings":[]}' > "$OUT"

# ── Gate 7.1: Circular dependencies ────────────────────────────────────
if command -v sckg >/dev/null 2>&1; then
  CYCLES=$(cd "$REPO" && sckg query --query "MATCH (a:File)-[:IMPORTS]->(b:File)-[:IMPORTS]->(a:File) RETURN a.path LIMIT 20" 2>/dev/null | grep -c "path" || tr -d "\n" || echo "0")
  if [[ "$CYCLES" -gt 0 ]]; then
    python3 "$LIB/add_finding.py" "$OUT" "7.1" "HIGH" "ARCH-CYCLE" \
      "Circular dependencies" "$CYCLES cycles detected" \
      "Break cycles: extract shared types to a new module, or invert the dependency"
  fi
else
  python3 "$LIB/add_finding.py" "$OUT" "7.1" "INFO" "ARCH-CYCLE" \
    "Architecture analysis limited" "SCKG not installed — install for full graph analysis" \
    "Install SIN-Code-SCKG to detect cycles: pip install sin-code-sckg"
fi

# ── Gate 7.2: God modules (imports > 30) ───────────────────────────────
GOD_MODULES=$(scout --mcp 2>/dev/null <<EOF | jq -r '.result.content[0].text // ""' 2>/dev/null
{"jsonrpc":"2.0","method":"tools/call","id":72,"params":{"name":"scout","arguments":{"path":"$REPO","query":"^(from\\s+\\w+\\s+import|import\\s+[\\\"\\']\\w+)","search_type":"regex","max_results":1000,"include_context":false}}}
EOF
)
# This is per-repo; need per-file aggregation
HIGH_IMPORT_FILES=$(echo "$GOD_MODULES" | awk -F: '{print $1}' | sort | uniq -c | sort -rn | head -5 | awk '$1 > 30' | wc -l | tr -d ' ')
if [[ "$HIGH_IMPORT_FILES" -gt 0 ]]; then
  python3 "$LIB/add_finding.py" "$OUT" "7.2" "MEDIUM" "ARCH-GODMODULE" \
    "God modules (imports > 30)" "$HIGH_IMPORT_FILES files" \
    "Split into smaller modules with clear responsibilities"
fi

# ── Gate 7.3: Orphan code (no caller, no test) ──────────────────────────
if command -v sckg >/dev/null 2>&1; then
  ORPHAN=$(cd "$REPO" && sckg query --query "MATCH (f:Function) WHERE NOT (()-[:CALLS]->(f)) AND NOT (f:Function)-[:STEP_IN_PROCESS]->(:Process) RETURN f.name LIMIT 20" 2>/dev/null | grep -c "name" || tr -d "\n" || echo "0")
  if [[ "$ORPHAN" -gt 10 ]]; then
    python3 "$LIB/add_finding.py" "$OUT" "7.3" "MEDIUM" "ARCH-ORPHAN" \
      "Orphan code (unreferenced functions)" "$ORPHAN functions" \
      "Remove or document: dead code is technical debt"
  fi
else
  python3 "$LIB/add_finding.py" "$OUT" "7.3" "INFO" "ARCH-ORPHAN" \
    "Orphan analysis skipped" "SCKG not installed" \
    "Install SIN-Code-SCKG for call graph analysis"
fi

# ── Gate 7.4: Hot paths not in tests ───────────────────────────────────
# If sckg can map test files, find functions that should be tested
if command -v sckg >/dev/null 2>&1; then
  # Functions that are in the call graph but not in any test file
  NOTESTED=$(cd "$REPO" && sckg query --query "MATCH (f:Function) WHERE f.complexity > 10 AND NOT EXISTS { MATCH (:File)-[:CONTAINS]->(f) WHERE (:File)-[:IS_TEST]->() } RETURN f.name LIMIT 20" 2>/dev/null | grep -c "name" || tr -d "\n" || echo "0")
  if [[ "$NOTESTED" -gt 0 ]]; then
    python3 "$LIB/add_finding.py" "$OUT" "7.4" "MEDIUM" "ARCH-UNTESTED" \
      "Complex functions without tests" "$NOTESTED functions" \
      "Add tests for these hot paths; prioritize by complexity and call frequency"
  fi
else
  # Heuristic: complex functions (many branches) in non-test files
  COMPLEX_NO_TEST=$(scout --mcp 2>/dev/null <<EOF | jq -r '.result.content[0].text // ""' 2>/dev/null
{"jsonrpc":"2.0","method":"tools/call","id":74,"params":{"name":"scout","arguments":{"path":"$REPO","query":"^(if|elif|else if|switch|case)\\s+[\\(\\w]","search_type":"regex","max_results":500,"include_context":false}}}
EOF
)
  COMPLEX_COUNT=$(echo "$COMPLEX_NO_TEST" | grep -c "Match" 2>/dev/null | tr -d "\n" || echo "0")
  python3 "$LIB/add_finding.py" "$OUT" "7.4" "INFO" "ARCH-UNTESTED" \
    "Architecture analysis limited" "SCKG not installed — $COMPLEX_COUNT branch points detected" \
    "Install SIN-Code-SCKG for accurate hot path detection"
fi

GATE_COUNT=$(jq '.gates | length' "$OUT")
FINDING_COUNT=$(jq '.findings | length' "$OUT")
echo "  [architecture] $GATE_COUNT gates, $FINDING_COUNT findings" >&2
