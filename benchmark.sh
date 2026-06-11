#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

echo "=== SIN-Code Benchmark Suite ==="
echo "Running benchmarks with -benchmem (memory allocation profiling)"
echo ""

BENCH_FILE="${BENCH_FILE:-benchmark.out}"
MEM_FILE="${MEM_FILE:-benchmark.mem}"
CPU_FILE="${CPU_FILE:-benchmark.cpu}"

# Run all benchmarks with memory profiling
go test ./cmd/sin-code/internal/ \
  -bench='Benchmark' \
  -benchmem \
  -count=5 \
  -timeout=300s \
  2>&1 | tee "$BENCH_FILE"

# Extract key metrics for comparison
echo ""
echo "=== Key Metrics (median of 5 runs) ==="

grep -E '^Benchmark' "$BENCH_FILE" | while read -r line; do
  name=$(echo "$line" | awk '{print $1}')
  ns=$(echo "$line" | awk '{print $2}')
  allocs=$(echo "$line" | awk '{print $4}')
  bytes=$(echo "$line" | awk '{print $6}')
  printf "%-45s %12s/op  %10s allocs  %10s bytes\n" "$name" "$ns" "$allocs" "$bytes"
done

echo ""
echo "=== Indexed vs Full-Scan Comparison ==="

echo "Full benchmarks saved to $BENCH_FILE"

# Optional: run pprof CPU profile
if [ "${PPROF:-}" = "1" ]; then
  echo ""
  echo "=== Running CPU Profile (5s) ==="
  go test ./cmd/sin-code/internal/ \
    -bench='BenchmarkScout_Indexed_1000files' \
    -cpuprofile="$CPU_FILE" \
    -benchtime=5s
  echo "CPU profile saved to $CPU_FILE"
  echo "View with: go tool pprof -http=:8080 $CPU_FILE"
fi

echo ""
echo "=== Done ==="
