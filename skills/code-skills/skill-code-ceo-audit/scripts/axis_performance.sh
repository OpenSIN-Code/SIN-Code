#!/usr/bin/env bash
# Purpose: CEO Audit Axis 2 — Performance (6 gates)
# Docs: SKILL.md#axis-2-performance
set -euo pipefail

REPO="$1"
OUT_DIR="$2"
OUT="$OUT_DIR/performance.json"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB="$SCRIPT_DIR/../lib"

echo '{"axis":"performance","gates":[],"findings":[]}' > "$OUT"

# ── Gate 2.1: Nested loops with outer N ─────────────────────────────────
NESTED=$(scout --mcp 2>/dev/null <<EOF | jq -r '.result.content[0].text // ""' 2>/dev/null
{"jsonrpc":"2.0","method":"tools/call","id":21,"params":{"name":"scout","arguments":{"path":"$REPO","query":"for\\s+\\w+\\s+in.*:\\s*\\n\\s*for\\s+\\w+\\s+in","search_type":"regex","max_results":50,"include_context":true}}}
EOF
)
NESTED_COUNT=$(echo "$NESTED" | grep -c "Match" 2>/dev/null | tr -d "\n" || echo "0")
[[ "$NESTED_COUNT" -gt 0 ]] && python3 "$LIB/add_finding.py" "$OUT" "2.1" "MEDIUM" "PERF-N2" \
  "Nested loops detected" "$NESTED_COUNT sites — O(n²) or worse" \
  "Replace with hashmap lookup, itertools.product, or numpy"

# ── Gate 2.2: Large allocations ────────────────────────────────────────
LARGE_ALLOC=$(scout --mcp 2>/dev/null <<EOF | jq -r '.result.content[0].text // ""' 2>/dev/null
{"jsonrpc":"2.0","method":"tools/call","id":22,"params":{"name":"scout","arguments":{"path":"$REPO","query":"(\\[0\\]\\s*\\*\\s*[0-9]{6,}|new (byte|int|Array\\[\\])\\s*\\([^)]*[0-9]{6,})","search_type":"regex","max_results":30,"include_context":true}}}
EOF
)
LARGE_COUNT=$(echo "$LARGE_ALLOC" | grep -c "Match" 2>/dev/null | tr -d "\n" || echo "0")
[[ "$LARGE_COUNT" -gt 0 ]] && python3 "$LIB/add_finding.py" "$OUT" "2.2" "LOW" "PERF-MEM" \
  "Large allocations" "$LARGE_COUNT sites allocating >100K elements" \
  "Use generators, arrays, or chunked processing"

# ── Gate 2.3: Unbounded caches ─────────────────────────────────────────
# Look for maps/dicts/sets with no eviction
UNBOUNDED=$(scout --mcp 2>/dev/null <<EOF | jq -r '.result.content[0].text // ""' 2>/dev/null
{"jsonrpc":"2.0","method":"tools/call","id":23,"params":{"name":"scout","arguments":{"path":"$REPO","query":"(self\\.cache\\s*=\\s*\\{\\}|self\\._cache\\s*=\\s*\\{\\}|make\\.map|make\\.sync\\.Map)\\s*$","search_type":"regex","max_results":30,"include_context":true}}}
EOF
)
UNBOUNDED_COUNT=$(echo "$UNBOUNDED" | grep -c "Match" 2>/dev/null | tr -d "\n" || echo "0")
if [[ "$UNBOUNDED_COUNT" -gt 0 ]]; then
  # Check if lru_cache or TTL or MaxSize appears in same file
  python3 "$LIB/add_finding.py" "$OUT" "2.3" "HIGH" "PERF-MEMLEAK" \
    "Unbounded caches detected" "$UNBOUNDED_COUNT sites — potential memory leak" \
    "Add LRU eviction (functools.lru_cache, sync.Map with MaxSize, lru.NewCache)"
fi

# ── Gate 2.4: Regex compilation per call ───────────────────────────────
REGEX_PER_CALL=$(scout --mcp 2>/dev/null <<EOF | jq -r '.result.content[0].text // ""' 2>/dev/null
{"jsonrpc":"2.0","method":"tools/call","id":24,"params":{"name":"scout","arguments":{"path":"$REPO","query":"(re\\.(match|search|findall|compile)|regexp\\.MustCompile)\\s*\\(","search_type":"regex","max_results":50,"include_context":true}}}
EOF
)
REGEX_COUNT=$(echo "$REGEX_PER_CALL" | grep -c "Match" 2>/dev/null | tr -d "\n" || echo "0")
# This is informational only — many regex usages are fine
[[ "$REGEX_COUNT" -gt 5 ]] && python3 "$LIB/add_finding.py" "$OUT" "2.4" "LOW" "PERF-REGEX" \
  "Frequent regex compilation" "$REGEX_COUNT sites — review for compilation caching" \
  "Compile once at module level, or use re.compile pattern"

# ── Gate 2.5: Sync I/O in hot path ─────────────────────────────────────
SYNC_IO=$(scout --mcp 2>/dev/null <<EOF | jq -r '.result.content[0].text // ""' 2>/dev/null
{"jsonrpc":"2.0","method":"tools/call","id":25,"params":{"name":"scout","arguments":{"path":"$REPO","query":"(open\\([^)]*\\)\\.read\\(\\)|requests\\.(get|post)|time\\.sleep|urllib\\.request\\.urlopen)","search_type":"regex","max_results":50,"include_context":true}}}
EOF
)
SYNC_COUNT=$(echo "$SYNC_IO" | grep -c "Match" 2>/dev/null | tr -d "\n" || echo "0")
[[ "$SYNC_COUNT" -gt 0 ]] && python3 "$LIB/add_finding.py" "$OUT" "2.5" "LOW" "PERF-IO" \
  "Sync I/O calls" "$SYNC_COUNT sites — review for async opportunities" \
  "Use aiohttp, asyncio, or thread pools for I/O-bound work"

# ── Gate 2.6: Missing parallelization ──────────────────────────────────
# Detect map/iter ops on large collections without concurrent helpers
# (This is a heuristic — we look for for-loops over db/file/api collections)
MISSING_PARALLEL=$(scout --mcp 2>/dev/null <<EOF | jq -r '.result.content[0].text // ""' 2>/dev/null
{"jsonrpc":"2.0","method":"tools/call","id":26,"params":{"name":"scout","arguments":{"path":"$REPO","query":"for\\s+\\w+\\s+in\\s+(items|results|rows|files|urls|requests)[:.]","search_type":"regex","max_results":30,"include_context":true}}}
EOF
)
PARALLEL_COUNT=$(echo "$MISSING_PARALLEL" | grep -c "Match" 2>/dev/null | tr -d "\n" || echo "0")
[[ "$PARALLEL_COUNT" -gt 3 ]] && python3 "$LIB/add_finding.py" "$OUT" "2.6" "LOW" "PERF-PARALLEL" \
  "Sequential iteration over large collections" "$PARALLEL_COUNT sites — consider concurrent execution" \
  "Use asyncio.gather, concurrent.futures, or errgroup.GO for parallel execution"

GATE_COUNT=$(jq '.gates | length' "$OUT")
FINDING_COUNT=$(jq '.findings | length' "$OUT")
echo "  [performance] $GATE_COUNT gates, $FINDING_COUNT findings" >&2
