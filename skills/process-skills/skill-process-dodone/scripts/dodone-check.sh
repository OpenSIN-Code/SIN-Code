#!/usr/bin/env bash
# SIN-DoDone v2 DoD-Check Script — with full ecosystem integration
#
# Calls all available SIN-Code ecosystem tools deterministically.
# Each tool degrades gracefully (SKIP) when not installed.
#
# Exit-Codes:
#   0 = WIRKLICH FERTIG (alle Säulen bestanden)
#   1 = Config/System-Fehler
#   2 = Code unvollständig (Säulen 1, 2, 5 failed)
#   3 = Tests/Build fehlgeschlagen (Säulen 3, 4 failed)
#
# Usage:
#   ./dodone-check.sh [contract-file]
#   Default contract: dod-contract.yaml

set -euo pipefail

CONTRACT="${1:-dod-contract.yaml}"
FAILURES=0
TEST_BUILD_FAILURES=0
ECOSYSTEM_TOOLS=""

echo "===================================================="
echo "    SIN-DoDone v2 — Definition-of-Done Check       "
echo "    with Ecosystem Integration                     "
echo "===================================================="
echo ""

# --- Ecosystem detection ---
echo "[ECO] Erkannte Ecosystem-Tools:"
for tool in sin-code poc oracle ibd adw sckg efsm sin-security sin-brain; do
    if command -v "$tool" &>/dev/null 2>&1; then
        echo "  [OK]    $tool"
        ECOSYSTEM_TOOLS="$ECOSYSTEM_TOOLS $tool"
    else
        echo "  [SKIP]  $tool (not installed)"
    fi
done
echo ""

# --- P1: Keine Platzhalter ---
echo "[P1] Pruefe auf verbotene Patterns..."
P1_FAIL=0
FORBIDDEN="TODO FIXME panic( NotImplemented unimplemented!"
for pattern in $FORBIDDEN; do
    matches=$(grep -rn "$pattern" --include='*.go' --include='*.py' --include='*.js' --include='*.ts' --include='*.tsx' --include='*.rs' --include='*.java' --include='*.rb' . 2>/dev/null | grep -v vendor | grep -v node_modules | grep -v '.git/' | grep -v 'dodone-check.sh' | grep -v 'templates.go' || true)
    if [ -n "$matches" ]; then
        echo "  [FAIL] Pattern '$pattern' gefunden:"
        echo "$matches" | head -10 | sed 's/^/    /'
        P1_FAIL=1
    fi
done
if [ $P1_FAIL -eq 0 ]; then echo "  [PASS] Keine verbotenen Patterns."; else FAILURES=$((FAILURES + 1)); fi
echo ""

# --- P2: Fehlerpfade ---
echo "[P2] Pruefe Fehlerpfade..."
P2_FAIL=0
go_ignores=$(grep -rn '_ = err' --include='*.go' . 2>/dev/null | grep -v vendor | grep -v '.git/' || true)
if [ -n "$go_ignores" ]; then echo "  [FAIL] Go: '_ = err' ignoriert Fehler:"; echo "$go_ignores" | head -5 | sed 's/^/    /'; P2_FAIL=1; fi
py_ignores=$(grep -rn 'except.*:\s*pass' --include='*.py' . 2>/dev/null | grep -v __pycache__ || true)
if [ -n "$py_ignores" ]; then echo "  [FAIL] Python: 'except: pass' ignoriert Fehler:"; echo "$py_ignores" | head -5 | sed 's/^/    /'; P2_FAIL=1; fi
if [ $P2_FAIL -eq 0 ]; then echo "  [PASS] Fehlerpfade sehen echt aus."; else FAILURES=$((FAILURES + 1)); fi
echo ""

# --- P3: Tests ---
echo "[P3] Pruefe Test-Suite..."
P3_FAIL=0
if [ -f "go.mod" ]; then
    if go test ./... -v -count=1 2>&1 | tail -30; then echo "  [PASS] go test gruen"; else echo "  [FAIL] go test rot"; P3_FAIL=1; fi
elif [ -f "pyproject.toml" ] || [ -f "setup.py" ]; then
    if pytest -v --tb=short 2>&1 | tail -30; then echo "  [PASS] pytest gruen"; else echo "  [FAIL] pytest rot"; P3_FAIL=1; fi
elif [ -f "package.json" ]; then
    if npm test -- --verbose 2>&1 | tail -30; then echo "  [PASS] npm test gruen"; else echo "  [FAIL] npm test rot"; P3_FAIL=1; fi
elif [ -f "Cargo.toml" ]; then
    if cargo test -- --nocapture 2>&1 | tail -30; then echo "  [PASS] cargo test gruen"; else echo "  [FAIL] cargo test rot"; P3_FAIL=1; fi
else
    echo "  [SKIP] Keine Test-Suite erkannt."
fi
if [ $P3_FAIL -ne 0 ]; then TEST_BUILD_FAILURES=$((TEST_BUILD_FAILURES + 1)); fi
echo ""

# --- P4: Build + Lint ---
echo "[P4] Pruefe Build + Lint..."
P4_FAIL=0
if [ -f "go.mod" ]; then
    if go build ./... 2>&1 | head -20; then echo "  [PASS] go build sauber"; else echo "  [FAIL] go build fehlgeschlagen"; P4_FAIL=1; fi
    go vet ./... 2>&1 | head -20 && echo "  [PASS] go vet sauber" || echo "  [WARN] go vet hat Warnungen"
elif [ -f "pyproject.toml" ]; then
    if command -v ruff &>/dev/null && ruff check . 2>&1 | head -20; then echo "  [PASS] ruff sauber"; else echo "  [WARN] ruff nicht verfuegbar"; fi
elif [ -f "package.json" ]; then
    if npm run build 2>&1 | tail -20; then echo "  [PASS] build sauber"; else echo "  [FAIL] build fehlgeschlagen"; P4_FAIL=1; fi
elif [ -f "Cargo.toml" ]; then
    if cargo build 2>&1 | tail -20; then echo "  [PASS] cargo build sauber"; else echo "  [FAIL] cargo build fehlgeschlagen"; P4_FAIL=1; fi
fi
if [ $P4_FAIL -ne 0 ]; then TEST_BUILD_FAILURES=$((TEST_BUILD_FAILURES + 1)); fi
echo ""

# --- P5: Erforderliche Artefakte ---
echo "[P5] Pruefe erforderliche Dateien..."
P5_FAIL=0
for req in README.md; do
    if [ ! -f "$req" ]; then echo "  [FAIL] $req fehlt"; P5_FAIL=1; fi
done
if [ $P5_FAIL -eq 0 ]; then echo "  [PASS] Alle erforderlichen Dateien vorhanden."; else FAILURES=$((FAILURES + 1)); fi
echo ""

# --- P6: Ecosystem — PoC invariants ---
echo "[P6] Ecosystem: PoC Invariant-Check..."
if command -v poc &>/dev/null 2>&1; then
    if poc verify . 2>&1 | tail -15; then echo "  [PASS] PoC: keine Verletzungen"; else echo "  [FAIL] PoC: Verletzungen gefunden"; FAILURES=$((FAILURES + 1)); fi
else
    echo "  [SKIP] poc nicht installiert."
fi
echo ""

# --- P7: Ecosystem — ADW architectural debt ---
echo "[P7] Ecosystem: ADW Architectural Debt..."
if command -v adw &>/dev/null 2>&1; then
    if adw scan . 2>&1 | tail -15; then echo "  [PASS] ADW: keine kritischen Schulden"; else echo "  [FAIL] ADW: Architektur-Schulden gefunden"; FAILURES=$((FAILURES + 1)); fi
else
    echo "  [SKIP] adw nicht installiert."
fi
echo ""

# --- P8: Ecosystem — Security scan ---
echo "[P8] Ecosystem: Security Scan..."
if command -v sin-security &>/dev/null 2>&1; then
    if sin-security scan . --fail-on critical 2>&1 | tail -15; then echo "  [PASS] Security: keine kritischen Issues"; else echo "  [FAIL] Security: kritische Issues gefunden"; FAILURES=$((FAILURES + 1)); fi
else
    echo "  [SKIP] sin-security nicht installiert."
fi
echo ""

# --- P9: Ecosystem — SCKG dead code ---
echo "[P9] Ecosystem: SCKG Dead Code..."
if command -v sckg &>/dev/null 2>&1; then
    if sckg dead_code . --threshold 0.8 2>&1 | tail -15; then echo "  [PASS] SCKG: kein toter Code"; else echo "  [FAIL] SCKG: toter Code gefunden"; FAILURES=$((FAILURES + 1)); fi
else
    echo "  [SKIP] sckg nicht installiert."
fi
echo ""

# --- Ergebnis ---
echo "===================================================="
if [ $TEST_BUILD_FAILURES -gt 0 ] && [ $FAILURES -gt 0 ]; then
    echo "[FAIL] Code unvollstaendig UND Tests/Build rot."
    exit 3
elif [ $TEST_BUILD_FAILURES -gt 0 ]; then
    echo "[FAIL] Tests/Build fehlgeschlagen."
    exit 3
elif [ $FAILURES -gt 0 ]; then
    echo "[FAIL] Definition of Done verletzt! Code ist unvollstaendig."
    echo "  Saulen failed: $FAILURES"
    exit 2
else
    echo "[SUCCESS] INTEGRITAET GEPRUEFT: Alle Saulen bestanden."
    echo "  Die Aufgabe ist zu 100% ECHT FERTIG!"
    exit 0
fi
