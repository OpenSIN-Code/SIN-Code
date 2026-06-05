# benchmark.sh

## What
Runs the SIN-Code tool performance benchmarks and persists results to
`benchmark_results/`.

## Why
Provides a reproducible, scriptable way to measure latency / throughput of
each SIN-Code tool across the full suite (`discover`, `execute`, `map`,
`grasp`, `scout`, `harvest`, `orchestrate`, `bundle`, `brain`, `sckg`).

## Usage
```bash
./benchmark.sh                # run all benchmarks
./benchmark.sh discover       # run a single tool
```

## Output
- `benchmark_results/<tool>_<timestamp>.json` — machine-readable results
- `benchmark_results/<tool>_<timestamp>.md`   — human-readable summary

## Caveats
- Requires all SIN-Code CLIs on `$PATH`
- Cold-cache runs include tool import overhead; warm-cache runs are stable
