# benchmark_test.go, benchmark.sh, go-ci.yml

What: Go benchmark suite measuring performance of scout, grasp, map, sckg, index,
and parseOutline operations. Includes CI gate that fails if indexed search is not
at least 5x faster than full scan.

Who touches it: benchmark.sh (local runs), go-ci.yml (GitHub Actions), benchmark_test.go (Go test framework).

Key decisions:
- Synthetic project trees with `makeTree()` (N Go files, M lines each) for reproducible benchmarks.
- `benchmem` flag captures allocations/byte counts alongside latency.
- BenchmarkComparisonTable sub-benchmarks directly compare fullscan vs indexed in one run.
- CI gate parses `BenchmarkComparisonTable` output and computes integer speedup ratio.
- `benchmark.sh` supports `PPROF=1` for CPU profiling via pprof.

Benchmarks included:
- Scout: fullscan vs indexed (100/1000 files), symbol search (1000 files)
- Grasp: small file (50 lines) vs large (1000 lines)
- Map: 100/1000 files
- SCKG: build graph 100/1000 files
- Index: build, save/load, refresh 1000 files
- ParseOutline: Go/Python/JS 1000 lines

No CoDocs companion needed for benchmark_test.go (test file) or benchmark.sh (shell script).
