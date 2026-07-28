#!/usr/bin/env bash
# Purpose: CEO Audit Axis 5 — Dependencies (5 gates)
set -euo pipefail

REPO="$1"
OUT_DIR="$2"
OUT="$OUT_DIR/deps.json"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB="$SCRIPT_DIR/../lib"

echo '{"axis":"deps","gates":[],"findings":[]}' > "$OUT"

# ── Gate 5.1: Known CVEs ───────────────────────────────────────────────
if [[ -f "$REPO/go.mod" ]] && command -v govulncheck >/dev/null 2>&1; then
  GOVULN=$(cd "$REPO" && govulncheck ./... 2>&1 || true)
  CVES=$(echo "$GOVULN" | grep -cE "GO-[0-9]{4}-[0-9]+" 2>/dev/null | tr -d "\n" || echo "0")
  [[ "$CVES" -gt 0 ]] && python3 "$LIB/add_finding.py" "$OUT" "5.1" "CRITICAL" "DEP-CVE" \
    "Known CVEs in Go dependencies" "$CVES vulnerabilities" \
    "Run 'go get -u=patch' to update; review each finding"
elif [[ -f "$REPO/requirements.txt" || -f "$REPO/pyproject.toml" ]] && command -v pip-audit >/dev/null 2>&1; then
  PIPAUDIT=$(cd "$REPO" && pip-audit -f json 2>/dev/null || echo "[]")
  CVES=$(echo "$PIPAUDIT" | jq 'length' 2>/dev/null | tr -d "\n" || echo "0")
  [[ "$CVES" -gt 0 ]] && python3 "$LIB/add_finding.py" "$OUT" "5.1" "CRITICAL" "DEP-CVE" \
    "Known CVEs in Python dependencies" "$CVES vulnerabilities" \
    "Update vulnerable packages; check pip-audit output for specific fixes"
elif [[ -f "$REPO/package.json" ]] && command -v npm >/dev/null 2>&1; then
  NPM=$(cd "$REPO" && npm audit --json 2>/dev/null | jq '.metadata.vulnerabilities // {}' 2>/dev/null || echo "{}")
  TOTAL_CVE=$(echo "$NPM" | jq '[.[] | select(. > 0)] | add // 0' 2>/dev/null | tr -d "\n" || echo "0")
  [[ "$TOTAL_CVE" -gt 0 ]] && python3 "$LIB/add_finding.py" "$OUT" "5.1" "CRITICAL" "DEP-CVE" \
    "Known CVEs in npm dependencies" "$TOTAL_CVE vulnerabilities" \
    "Run 'npm audit fix' to update; review breaking changes"
else
  python3 "$LIB/add_finding.py" "$OUT" "5.1" "INFO" "DEP-CVE" \
    "CVE scan skipped" "No manifest or scanner installed" \
    "Install govulncheck (Go), pip-audit (Python), or use npm audit (Node)"
fi

# ── Gate 5.2: Outdated major versions ──────────────────────────────────
if [[ -f "$REPO/requirements.txt" ]] && command -v pip >/dev/null 2>&1; then
  OUTDATED=$(cd "$REPO" && pip list --outdated --format=json 2>/dev/null | jq 'length' 2>/dev/null | tr -d "\n" || echo "0")
  [[ "$OUTDATED" -gt 5 ]] && python3 "$LIB/add_finding.py" "$OUT" "5.2" "LOW" "DEP-OUTDATED" \
    "Outdated packages" "$OUTDATED packages behind" \
    "Update incrementally; test after each major version bump"
elif [[ -f "$REPO/go.mod" ]]; then
  GOOUTDATED=$(cd "$REPO" && go list -m -u all 2>/dev/null | grep -c "\[" || tr -d "\n" || echo "0")
  [[ "$GOOUTDATED" -gt 5 ]] && python3 "$LIB/add_finding.py" "$OUT" "5.2" "LOW" "DEP-OUTDATED" \
    "Outdated Go modules" "$GOOUTDATED modules behind" \
    "Update incrementally with 'go get -u'"
fi

# ── Gate 5.3: Unpinned versions ────────────────────────────────────────
UNPINNED=$(scout --mcp 2>/dev/null <<EOF | jq -r '.result.content[0].text // ""' 2>/dev/null
{"jsonrpc":"2.0","method":"tools/call","id":53,"params":{"name":"scout","arguments":{"path":"$REPO","query":"(>=|\\^|~)\\s*[0-9]+\\.[0-9]+","search_type":"regex","include_context":false,"max_results":50}}}
EOF
)
UNPINNED_COUNT=$(echo "$UNPINNED" | grep -c "Match" 2>/dev/null | tr -d "\n" || echo "0")
if [[ "$UNPINNED_COUNT" -gt 0 ]] && { [[ -f "$REPO/requirements.txt" ]] || [[ -f "$REPO/pyproject.toml" ]]; }; then
  python3 "$LIB/add_finding.py" "$OUT" "5.3" "MEDIUM" "DEP-UNPINNED" \
    "Unpinned versions" "$UNPINNED_COUNT uses of >=, ^, or ~" \
    "Pin to exact versions in production: foo==1.2.3 (Python) or foo v1.2.3 (Go)"
fi

# ── Gate 5.4: Abandoned packages (heuristic) ───────────────────────────
# Hard to detect automatically without API call. Use harvest for npm/pypi
# and check last release date
if command -v harvest >/dev/null 2>&1; then
  # Get list of top-level deps
  DEPS_LIST=""
  if [[ -f "$REPO/requirements.txt" ]]; then
    DEPS_LIST=$(grep -E "^[a-zA-Z]" "$REPO/requirements.txt" | cut -d= -f1 | cut -d'>' -f1 | cut -d'<' -f1 | head -10)
  elif [[ -f "$REPO/package.json" ]]; then
    DEPS_LIST=$(jq -r '.dependencies // {} | keys[]' "$REPO/package.json" 2>/dev/null | head -10)
  fi
  ABANDONED_COUNT=0
  for pkg in $DEPS_LIST; do
    if [[ -n "$pkg" ]]; then
      DATA=$(harvest --mcp 2>/dev/null <<EOF | jq -r '.result.content[0].text // ""' 2>/dev/null
{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"harvest","arguments":{"url":"https://pypi.org/pypi/$pkg/json"}}}
EOF
)
      # Check if release is older than 2 years
      if echo "$DATA" | grep -q "no longer available\|not found"; then
        ABANDONED_COUNT=$((ABANDONED_COUNT + 1))
      fi
    fi
  done
  [[ "$ABANDONED_COUNT" -gt 0 ]] && python3 "$LIB/add_finding.py" "$OUT" "5.4" "MEDIUM" "DEP-ABANDONED" \
    "Potentially abandoned packages" "$ABANDONED_COUNT packages" \
    "Find actively maintained alternatives; consider forking if critical"
fi

# ── Gate 5.5: License risk ─────────────────────────────────────────────
if [[ ! -f "$REPO/LICENSE" && ! -f "$REPO/LICENSE.md" && ! -f "$REPO/LICENSE.txt" ]]; then
  python3 "$LIB/add_finding.py" "$OUT" "5.5" "HIGH" "DEP-LICENSE" \
    "Missing LICENSE file" "No LICENSE found" \
    "Add a LICENSE file (MIT, Apache 2.0, etc.) to clarify usage rights"
fi

# Check for GPL/AGPL in deps (proprietary code risk)
if command -v pip-licenses >/dev/null 2>&1 && [[ -f "$REPO/requirements.txt" ]]; then
  GPL_DEPS=$(cd "$REPO" && pip-licenses --format=json 2>/dev/null | jq '[.[] | select(.License | test("GPL|AGPL"; "i"))] | length' 2>/dev/null | tr -d "\n" || echo "0")
  [[ "$GPL_DEPS" -gt 0 ]] && python3 "$LIB/add_finding.py" "$OUT" "5.5" "HIGH" "DEP-LICENSE" \
    "GPL/AGPL dependencies" "$GPL_DEPS packages with copyleft license" \
    "Review compatibility with your distribution model; consider permissively-licensed alternatives"
fi

GATE_COUNT=$(jq '.gates | length' "$OUT")
FINDING_COUNT=$(jq '.findings | length' "$OUT")
echo "  [deps] $GATE_COUNT gates, $FINDING_COUNT findings" >&2
