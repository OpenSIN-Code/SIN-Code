#!/usr/bin/env python3
"""SCKG Performance Benchmarks.

Build time for 100/1000/10000 files
Query time for simple/complex queries
Memory usage during build
"""

import gc
import json
import subprocess
import tempfile
import time
from pathlib import Path

# Use SCKG venv
VENV_PYTHON = "/Users/jeremy/dev/SIN-Code-Semantic-Codebase-Knowledge-Graphs/.venv/bin/python3"
SCKG_ROOT = "/Users/jeremy/dev/SIN-Code-Semantic-Codebase-Knowledge-Graphs"

RESULTS = []


def generate_synthetic_repo(num_files: int, root: str) -> str:
    """Generate a synthetic Python repo with N files."""
    repo = Path(root) / f"synthetic_{num_files}"
    repo.mkdir(parents=True, exist_ok=True)

    for i in range(num_files):
        file_path = repo / f"module_{i}.py"
        lines = [
            f'"""Module {i} — auto-generated for benchmarking."""',
            "import os",
            "import sys",
            f"from module_{(i + 1) % num_files} import helper_{(i + 1) % num_files}",
            "",
            f"class ClassA_{i}:",
            '    """A sample class."""',
            f"    def method_{i}(self, x: int) -> int:",
            "        if x > 0:",
            "            for j in range(x):",
            "                if j % 2 == 0:",
            "                    continue",
            "        return x * 2",
            "",
            f"class ClassB_{i}(ClassA_{i}):",
            "    pass",
            "",
            f"def helper_{i}(a: int, b: int) -> int:",
            "    result = a + b",
            "    while result < 100:",
            "        result += 1",
            "    return result",
            "",
            f"def main_{i}():",
            f"    obj = ClassA_{i}()",
            f"    obj.method_{i}(10)",
            f"    helper_{i}(1, 2)",
            "",
            'if __name__ == "__main__":',
            f"    main_{i}()",
            "",
        ]
        file_path.write_text("\n".join(lines), encoding="utf-8")

    return str(repo)


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
                "tool": "SCKG",
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
                "tool": "SCKG",
                "benchmark": name,
                "result": f"ERROR: {e}",
                "target": f"{target:.3f}s",
                "status": "ERROR",
                "raw_seconds": -1,
            }
        )
        return None


def benchmark_build(files: int, repo_path: str) -> dict:
    """Benchmark graph build for N files."""
    script = f"""
import sys
sys.path.insert(0, "{SCKG_ROOT}/src")
from sin_code_sckg.graph import KnowledgeGraph
import time
kg = KnowledgeGraph(storage_path="/tmp/sckg_bench_{files}.graph")
start = time.perf_counter()
stats = kg.build_from_repo("{repo_path}")
duration = time.perf_counter() - start
print(f"BUILD|{{duration:.4f}}|{{stats['files']}}|{{stats['functions']}}|{{stats['classes']}}|{{stats['edges']}}")
"""
    result = subprocess.run(
        [VENV_PYTHON, "-c", script], capture_output=True, text=True, timeout=300
    )
    if result.returncode != 0:
        raise RuntimeError(f"Build failed: {result.stderr}")
    parts = result.stdout.strip().split("|")
    return {
        "duration": float(parts[1]),
        "files": int(parts[2]),
        "functions": int(parts[3]),
        "classes": int(parts[4]),
        "edges": int(parts[5]),
    }


def benchmark_query(files: int, query: str) -> float:
    """Benchmark query time."""
    script = f"""
import sys
sys.path.insert(0, "{SCKG_ROOT}/src")
from sin_code_sckg.graph import KnowledgeGraph
import time
kg = KnowledgeGraph(storage_path="/tmp/sckg_bench_{files}.graph")
# Ensure loaded
start = time.perf_counter()
results = kg.query("{query}")
duration = time.perf_counter() - start
print(f"QUERY|{{duration:.4f}}|{{len(results)}}")
"""
    result = subprocess.run([VENV_PYTHON, "-c", script], capture_output=True, text=True, timeout=60)
    if result.returncode != 0:
        raise RuntimeError(f"Query failed: {result.stderr}")
    parts = result.stdout.strip().split("|")
    return float(parts[1])


def benchmark_memory(files: int, repo_path: str) -> float:
    """Benchmark memory usage during build (RSS in MB)."""
    script = f"""
import sys
sys.path.insert(0, "{SCKG_ROOT}/src")
from sin_code_sckg.graph import KnowledgeGraph
import os
import resource
kg = KnowledgeGraph(storage_path="/tmp/sckg_bench_mem_{files}.graph")
kg.build_from_repo("{repo_path}")
# Peak RSS in KB -> MB
peak_kb = resource.getrusage(resource.RUSAGE_SELF).ru_maxrss
if sys.platform == "darwin":
    # macOS reports in bytes, not KB
    peak_kb = peak_kb // 1024
peak_mb = peak_kb / 1024.0
print(f"MEM|{{peak_mb:.2f}}")
"""
    result = subprocess.run(
        [VENV_PYTHON, "-c", script], capture_output=True, text=True, timeout=300
    )
    if result.returncode != 0:
        raise RuntimeError(f"Memory benchmark failed: {result.stderr}")
    parts = result.stdout.strip().split("|")
    return float(parts[1])


def main():
    print("=" * 60)
    print("SCKG (Knowledge Graph) Benchmarks")
    print("=" * 60)

    with tempfile.TemporaryDirectory() as tmpdir:
        # Build benchmarks
        for num_files in [100, 1000, 10000]:
            if num_files == 10000:
                # Skip 10000 if too slow — generate and test
                pass

            repo = generate_synthetic_repo(num_files, tmpdir)

            # Build time
            target = {100: 2.0, 1000: 15.0, 10000: 120.0}.get(num_files, 300.0)
            stats = run_benchmark(
                f"build_{num_files}_files", target, benchmark_build, num_files, repo
            )
            if stats:
                print(
                    f"  Build {num_files}: {stats['duration']:.3f}s "
                    f"({stats['files']} files, {stats['functions']} funcs, "
                    f"{stats['classes']} classes, {stats['edges']} edges)"
                )

            # Query benchmarks
            for query_name, query, q_target in [
                ("simple_query", "helper", {100: 0.05, 1000: 0.2, 10000: 1.0}.get(num_files, 1.0)),
                ("complex_query", "ClassA", {100: 0.05, 1000: 0.2, 10000: 1.0}.get(num_files, 1.0)),
            ]:
                q_time = run_benchmark(
                    f"{query_name}_{num_files}_files", q_target, benchmark_query, num_files, query
                )
                if q_time:
                    print(f"  Query '{query}' on {num_files}: {q_time:.4f}s")

            # Memory benchmark
            mem_target = {100: 50.0, 1000: 150.0, 10000: 500.0}.get(num_files, 1000.0)
            mem_mb = run_benchmark(
                f"memory_{num_files}_files", mem_target, benchmark_memory, num_files, repo
            )
            if mem_mb:
                print(f"  Memory {num_files}: {mem_mb:.2f} MB")

    # Save results
    out_path = Path("benchmark_sckg_results.json")
    out_path.write_text(json.dumps(RESULTS, indent=2), encoding="utf-8")
    print(f"\nResults saved to {out_path}")

    # Print summary table
    print("\n" + "=" * 60)
    print("| Benchmark | Result | Target | Status |")
    print("=" * 60)
    for r in RESULTS:
        print(
            f"| {r['benchmark']:<25} | {r['result']:<10} | {r['target']:<10} | {r['status']:<6} |"
        )
    print("=" * 60)

    return RESULTS


if __name__ == "__main__":
    main()
