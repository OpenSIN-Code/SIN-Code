#!/usr/bin/env bash
# Purpose: CEO Audit Axis 8 — Compliance (4 gates)
set -euo pipefail

REPO="$1"
OUT_DIR="$2"
OUT="$OUT_DIR/compliance.json"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB="$SCRIPT_DIR/../lib"

echo '{"axis":"compliance","gates":[],"findings":[]}' > "$OUT"

# ── Gate 8.1: License headers in source files ──────────────────────────
# Sample 20 Python/Go source files and check for license header
MISSING_HEADER=0
CHECKED=0
while IFS= read -r f; do
  CHECKED=$((CHECKED + 1))
  HEADER=$(head -10 "$f" 2>/dev/null)
  if ! echo "$HEADER" | grep -qiE "(copyright|license|spdx-license|apache|mit|gpl|all rights reserved)"; then
    MISSING_HEADER=$((MISSING_HEADER + 1))
  fi
  [[ "$CHECKED" -ge 20 ]] && break
done < <(find "$REPO" -type f \( -name "*.py" -o -name "*.go" -o -name "*.ts" -o -name "*.js" \) \
          -not -path "*/node_modules/*" -not -path "*/.venv/*" -not -path "*/vendor/*" \
          -not -path "*/test*" -not -path "*/.git/*" 2>/dev/null)

if [[ "$CHECKED" -gt 0 ]]; then
  PCT=$((MISSING_HEADER * 100 / CHECKED))
  if [[ "$PCT" -gt 50 ]]; then
    python3 "$LIB/add_finding.py" "$OUT" "8.1" "MEDIUM" "COMPL-HEADER" \
      "Missing license headers" "$MISSING_HEADER/$CHECKED files sampled (${PCT}%)" \
      "Add SPDX-License-Identifier to all source files"
  fi
fi

# ── Gate 8.2: SECURITY.md present ─────────────────────────────────────
if [[ ! -f "$REPO/SECURITY.md" ]]; then
  python3 "$LIB/add_finding.py" "$OUT" "8.2" "MEDIUM" "COMPL-SECURITY" \
    "Missing SECURITY.md" "No security policy file" \
    "Add SECURITY.md with: how to report vulnerabilities, supported versions, response time"
fi

# ── Gate 8.3: SBOM (Software Bill of Materials) ───────────────────────
HAS_SBOM=0
for f in "bom.json" "bom.xml" "sbom.json" "sbom.xml" "sbom.spdx" "sbom.cdx"; do
  [[ -f "$REPO/$f" ]] && HAS_SBOM=1 && break
done
# Also check for cdxgen output
[[ -d "$REPO/.cdxgen" ]] && HAS_SBOM=1

if [[ "$HAS_SBOM" -eq 0 ]]; then
  python3 "$LIB/add_finding.py" "$OUT" "8.3" "LOW" "COMPL-SBOM" \
    "No SBOM" "No CycloneDX/SPDX file" \
    "Generate with: cdxgen -o bom.json (compliance with NTIA minimum elements)"
fi

# ── Gate 8.4: PII in logs (emails, IPs, names) ─────────────────────────
# Scan for patterns that suggest PII in print/log statements
PII=$(scout --mcp 2>/dev/null <<EOF | jq -r '.result.content[0].text // ""' 2>/dev/null
{"jsonrpc":"2.0","method":"tools/call","id":84,"params":{"name":"scout","arguments":{"path":"$REPO","query":"(print|log|logger)\\s*\\([^)]*(\".*\\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Z|a-z]{2,}\".*|.*\\b[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}\\b.*)","search_type":"regex","max_results":50,"include_context":true}}}
EOF
)
PII_COUNT=$(echo "$PII" | grep -c "Match" 2>/dev/null | tr -d "\n" || echo "0")
if [[ "$PII_COUNT" -gt 0 ]]; then
  python3 "$LIB/add_finding.py" "$OUT" "8.4" "HIGH" "COMPL-PII" \
    "Potential PII in logs" "$PII_COUNT log statements contain emails or IPs" \
    "Redact PII before logging: hash emails, mask IP octets, use structured logging"
fi

GATE_COUNT=$(jq '.gates | length' "$OUT")
FINDING_COUNT=$(jq '.findings | length' "$OUT")
echo "  [compliance] $GATE_COUNT gates, $FINDING_COUNT findings" >&2
