#!/usr/bin/env python3
"""SIN-Brain Performance Benchmarks.

Recall speed for 1000/10000 memories
Remember speed (insert performance)
Consolidation speed
SQLite query performance
"""

import gc
import json
import os
import tempfile
import time
from pathlib import Path

BRAIN_VENV = "/Users/jeremy/dev/SIN-Brain/.venv/bin/python3"
BRAIN_ROOT = "/Users/jeremy/dev/SIN-Brain"
RESULTS = []


def run_benchmark(name: str, target: float, func, *args, **kwargs):
    """Run a benchmark and record result."""
    gc.collect()
    start = time.perf_counter()
    try:
        result = func(*args, **kwargs)
        duration = time.perf_counter() - start
        status = "PASS" if duration <= target else "FAIL"
        RESULTS.append(
            {
                "tool": "SIN-Brain",
                "benchmark": name,
                "result": f"{duration:.3f}s",
                "target": f"{target:.3f}s",
                "status": status,
                "raw_seconds": duration,
            }
        )
        return result
    except Exception as e:
        RESULTS.append(
            {
                "tool": "SIN-Brain",
                "benchmark": name,
                "result": f"ERROR: {e}",
                "target": f"{target:.3f}s",
                "status": "ERROR",
                "raw_seconds": -1,
            }
        )
        return None


def run_brain_script(script: str, timeout: int = 60) -> str:
    """Run a Python script in the Brain venv."""
    full_script = f"""
import sys
sys.path.insert(0, "{BRAIN_ROOT}/src")
{script}
"""
    result = subprocess.run(
        [BRAIN_VENV, "-c", full_script], capture_output=True, text=True, timeout=timeout
    )
    if result.returncode != 0:
        raise RuntimeError(f"Brain script failed: {result.stderr[:500]}")
    return result.stdout.strip()


def benchmark_recall(num_memories: int, db_path: str) -> float:
    """Benchmark recall with N memories."""
    script = f"""
from sin_brain import BrainCortex
import time

cortex = BrainCortex("{db_path}")
start = time.perf_counter()
results = cortex.recall("module helper", limit=10)
duration = time.perf_counter() - start
print(f"RECALL|{{duration:.4f}}|{{len(results)}}")
"""
    output = run_brain_script(script, timeout=60)
    parts = output.split("|")
    return float(parts[1])


def benchmark_remember(num_memories: int, db_path: str) -> float:
    """Benchmark remember (insert) N memories."""
    script = f"""
from sin_brain import BrainCortex
import time

cortex = BrainCortex("{db_path}")
start = time.perf_counter()
for i in range({num_memories}):
    cortex.remember(
        f"Memory number {{i}} about helper functions and modules",
        kind="fact",
        tier="episodic",
        confidence=0.9,
    )
duration = time.perf_counter() - start
print(f"REMEMBER|{{duration:.4f}}|{num_memories}")
"""
    output = run_brain_script(script, timeout=120)
    parts = output.split("|")
    return float(parts[1])


def benchmark_consolidation(num_memories: int, db_path: str) -> float:
    """Benchmark consolidation."""
    script = f"""
from sin_brain import BrainCortex
import time

cortex = BrainCortex("{db_path}")
start = time.perf_counter()
count = cortex.consolidate(age_seconds=0)  # consolidate all
duration = time.perf_counter() - start
print(f"CONSOLIDATE|{{duration:.4f}}|{{count}}")
"""
    output = run_brain_script(script, timeout=60)
    parts = output.split("|")
    return float(parts[1])


def benchmark_sqlite_raw(num_memories: int, db_path: str) -> float:
    """Benchmark raw SQLite query performance."""
    script = f"""
import sqlite3
import time

conn = sqlite3.connect("{db_path}")
start = time.perf_counter()
rows = conn.execute(
    "SELECT id, content FROM memories WHERE content LIKE ? LIMIT ?",
    ("%helper%", 10)
).fetchall()
duration = time.perf_counter() - start
print(f"SQLITE|{{duration:.4f}}|{{len(rows)}}")
"""
    output = run_brain_script(script, timeout=60)
    parts = output.split("|")
    return float(parts[1])


def main():
    print("=" * 60)
    print("SIN-Brain Benchmarks")
    print("=" * 60)

    with tempfile.TemporaryDirectory() as tmpdir:
        for num_memories in [1000, 10000]:
            db_path = os.path.join(tmpdir, f"brain_{num_memories}.db")

            # Populate first
            print(f"\n  Populating {num_memories} memories...")
            pop_time = run_benchmark(
                f"remember_{num_memories}",
                {1000: 5.0, 10000: 30.0}.get(num_memories, 60.0),
                benchmark_remember,
                num_memories,
                db_path,
            )
            if pop_time:
                print(f"    Remember {num_memories}: {pop_time:.3f}s")

            # Recall
            recall_time = run_benchmark(
                f"recall_{num_memories}",
                {1000: 0.5, 10000: 1.0}.get(num_memories, 1.0),
                benchmark_recall,
                num_memories,
                db_path,
            )
            if recall_time:
                print(f"    Recall {num_memories}: {recall_time:.3f}s")

            # Raw SQLite
            sqlite_time = run_benchmark(
                f"sqlite_query_{num_memories}",
                {1000: 0.2, 10000: 0.5}.get(num_memories, 1.0),
                benchmark_sqlite_raw,
                num_memories,
                db_path,
            )
            if sqlite_time:
                print(f"    SQLite query {num_memories}: {sqlite_time:.3f}s")

            # Consolidation
            cons_time = run_benchmark(
                f"consolidation_{num_memories}",
                {1000: 1.0, 10000: 5.0}.get(num_memories, 10.0),
                benchmark_consolidation,
                num_memories,
                db_path,
            )
            if cons_time:
                print(f"    Consolidation {num_memories}: {cons_time:.3f}s")

    # Save results
    out_path = Path("benchmark_brain_results.json")
    out_path.write_text(json.dumps(RESULTS, indent=2), encoding="utf-8")
    print(f"\nResults saved to {out_path}")

    print("\n" + "=" * 60)
    print("| Benchmark | Result | Target | Status |")
    print("=" * 60)
    for r in RESULTS:
        print(
            f"| {r['benchmark']:<30} | {r['result']:<10} | {r['target']:<10} | {r['status']:<6} |"
        )
    print("=" * 60)

    return RESULTS


if __name__ == "__main__":
    import subprocess

    main()
