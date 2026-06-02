#!/usr/bin/env bash
# Purpose: Run all SIN-Code tool performance benchmarks.
# Docs: benchmark.sh.doc.md

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BENCHMARKS_DIR="${SCRIPT_DIR}/benchmarks"
RESULTS_DIR="${SCRIPT_DIR}/benchmark_results"

mkdir -p "${RESULTS_DIR}"

echo "========================================"
echo "SIN-Code Tool Performance Benchmarks"
echo "========================================"
echo ""

# Check prerequisites
for binary in discover execute map grasp scout harvest orchestrate; do
    if ! command -v "${binary}" &>/dev/null; then
        echo "WARNING: ${binary} not in PATH"
    fi
done

echo "Running benchmarks..."
echo ""

# 1. SCKG (Knowledge Graph)
echo "[1/5] SCKG Benchmarks..."
cd "${BENCHMARKS_DIR}"
python3 benchmark_sckg.py || true
if [ -f benchmark_sckg_results.json ]; then
    mv benchmark_sckg_results.json "${RESULTS_DIR}/"
fi
echo ""

# 2. Discover-Tool
echo "[2/5] Discover-Tool Benchmarks..."
cd "${BENCHMARKS_DIR}"
python3 benchmark_discover.py || true
if [ -f benchmark_discover_results.json ]; then
    mv benchmark_discover_results.json "${RESULTS_DIR}/"
fi
echo ""

# 3. Execute-Tool
echo "[3/5] Execute-Tool Benchmarks..."
cd "${BENCHMARKS_DIR}"
python3 benchmark_execute.py || true
if [ -f benchmark_execute_results.json ]; then
    mv benchmark_execute_results.json "${RESULTS_DIR}/"
fi
echo ""

# 4. SIN-Brain
echo "[4/5] SIN-Brain Benchmarks..."
cd "${BENCHMARKS_DIR}"
python3 benchmark_brain.py || true
if [ -f benchmark_brain_results.json ]; then
    mv benchmark_brain_results.json "${RESULTS_DIR}/"
fi
echo ""

# 5. Bundle/MCP
echo "[5/5] Bundle/MCP Benchmarks..."
cd "${BENCHMARKS_DIR}"
python3 benchmark_bundle.py || true
if [ -f benchmark_bundle_results.json ]; then
    mv benchmark_bundle_results.json "${RESULTS_DIR}/"
fi
echo ""

# Aggregate results
echo "========================================"
echo "Aggregate Results"
echo "========================================"
echo ""

# Generate markdown report
REPORT="${RESULTS_DIR}/benchmark_report.md"
{
    echo "# SIN-Code Tool Performance Benchmark Report"
    echo ""
    echo "Generated: $(date -Iseconds)"
    echo ""
    echo "| Tool | Benchmark | Result | Target | Status |"
    echo "|------|-----------|--------|--------|--------|"
    
    for f in "${RESULTS_DIR}"/*.json; do
        if [ -f "$f" ]; then
            python3 -c "
import json, sys
data = json.load(open('$f'))
for r in data:
    print(f\"| {r['tool']:<10} | {r['benchmark']:<30} | {r['result']:<10} | {r['target']:<10} | {r['status']:<6} |\")
"
        fi
    done
    
    echo ""
    echo "## Summary"
    echo ""
    
    # Count passes/fails
    python3 -c "
import json, glob, sys
results = []
for f in glob.glob('${RESULTS_DIR}/*.json'):
    results.extend(json.load(open(f)))

total = len(results)
passes = sum(1 for r in results if r['status'] == 'PASS')
fails = sum(1 for r in results if r['status'] == 'FAIL')
errors = sum(1 for r in results if r['status'] == 'ERROR')

print(f'- **Total benchmarks**: {total}')
print(f'- **PASS**: {passes}')
print(f'- **FAIL**: {fails}')
print(f'- **ERROR**: {errors}')
print('')

if fails > 0:
    print('### Performance Issues Found')
    print('')
    for r in results:
        if r['status'] == 'FAIL':
            print(f\"- **{r['tool']} — {r['benchmark']}**: {r['result']} (target: {r['target']})\")
    print('')
"
    
    echo ""
    echo "## Raw Data"
    echo ""
    echo "Individual JSON files are in \`${RESULTS_DIR}\`."
} > "${REPORT}"

echo "Report saved to: ${REPORT}"
echo ""

# Print summary table
cat "${REPORT}"

# Check for failures and create issues
FAIL_COUNT=$(python3 -c "
import json, glob
count = 0
for f in glob.glob('${RESULTS_DIR}/*.json'):
    for r in json.load(open(f)):
        if r['status'] == 'FAIL':
            count += 1
print(count)
")

if [ "$FAIL_COUNT" -gt 0 ]; then
    echo ""
    echo "⚠️  $FAIL_COUNT benchmark(s) failed target."
    echo "    See ${REPORT} for details."
    echo ""
    echo "Creating performance issues..."
    
    # Create ISSUES.md entries
    ISSUES_FILE="${SCRIPT_DIR}/ISSUES.md"
    {
        echo ""
        echo "## Performance Issues — $(date +%Y-%m-%d)"
        echo ""
        python3 -c "
import json, glob
results = []
for f in glob.glob('${RESULTS_DIR}/*.json'):
    results.extend(json.load(open(f)))

for r in results:
    if r['status'] == 'FAIL':
        print(f\"### {r['tool']} — {r['benchmark']}\")
        print(f\"- **Result**: {r['result']}\")
        print(f\"- **Target**: {r['target']}\")
        print(f\"- **Gap**: {r['raw_seconds'] - float(r['target'].replace('s','')):.2f}s over target\")
        print(f\"- **Recommendation**: Optimize critical path or adjust target.\")
        print('')
"
    } >> "${ISSUES_FILE}"
    
    echo "Appended to ${ISSUES_FILE}"
    exit 1
else
    echo ""
    echo "✅ All benchmarks passed target!"
    exit 0
fi
