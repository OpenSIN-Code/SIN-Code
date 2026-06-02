#!/usr/bin/env python3
"""Discover-Tool Performance Benchmarks.

Discovery time for large repos
max_results performance (early stop)
Relevance scoring speed
Extension filtering overhead
"""

import gc
import json
import os
import subprocess
import sys
import tempfile
import time
from pathlib import Path

DISCOVER_BIN = "/Users/jeremy/.local/bin/discover"
RESULTS = []


def run_benchmark(name: str, target: float, func, *args, **kwargs):
    """Run a benchmark and record result."""
    gc.collect()
    start = time.perf_counter()
    try:
        result = func(*args, **kwargs)
        duration = time.perf_counter() - start
        status = "PASS" if duration <= target else "FAIL"
        RESULTS.append({
            "tool": "Discover",
            "benchmark": name,
            "result": f"{duration:.3f}s",
            "target": f"{target:.3f}s",
            "status": status,
            "raw_seconds": duration,
        })
        return result
    except Exception as e:
        RESULTS.append({
            "tool": "Discover",
            "benchmark": name,
            "result": f"ERROR: {e}",
            "target": f"{target:.3f}s",
            "status": "ERROR",
            "raw_seconds": -1,
        })
        return None


def run_discover(path: str, pattern: str = "**/*.py", **kwargs) -> float:
    """Run discover binary and return duration."""
    args = [
        DISCOVER_BIN,
        "-path", path,
        "-pattern", pattern,
        "-format", "json",
    ]
    for k, v in kwargs.items():
        args.extend([f"-{k}", str(v)])
    
    start = time.perf_counter()
    result = subprocess.run(
        args, capture_output=True, text=True, timeout=120
    )
    duration = time.perf_counter() - start
    if result.returncode != 0:
        raise RuntimeError(f"discover failed: {result.stderr[:200]}")
    return duration


def generate_repo(num_files: int, root: str, ext: str = ".py") -> str:
    """Generate a synthetic repo with N files."""
    repo = Path(root) / f"repo_{num_files}"
    repo.mkdir(parents=True, exist_ok=True)
    
    for i in range(num_files):
        file_path = repo / f"module_{i}{ext}"
        content = f"""// Module {i}
function helper_{i}(a, b) {{
    return a + b;
}}
"""
        if ext == ".py":
            content = f"""# Module {i}
def helper_{i}(a, b):
    return a + b
"""
        elif ext == ".go":
            content = f"""package module{i}
func Helper{i}(a, b int) int {{
    return a + b
}}
"""
        file_path.write_text(content, encoding="utf-8")
    
    return str(repo)


def main():
    print("=" * 60)
    print("Discover-Tool Benchmarks")
    print("=" * 60)
    
    # Verify binary exists
    if not Path(DISCOVER_BIN).exists():
        print(f"ERROR: discover binary not found at {DISCOVER_BIN}")
        sys.exit(1)
    
    with tempfile.TemporaryDirectory() as tmpdir:
        # 1. Discovery time for increasing repo sizes
        for num_files in [100, 500, 1000]:
            repo = generate_repo(num_files, tmpdir, ".py")
            target = {100: 1.0, 500: 3.0, 1000: 5.0}.get(num_files, 10.0)
            dur = run_benchmark(
                f"discovery_{num_files}_py_files", target,
                run_discover, repo, "**/*.py"
            )
            if dur:
                print(f"  Discovery {num_files} .py files: {dur:.3f}s")
        
        # 2. max_results early stop
        repo = generate_repo(1000, tmpdir, ".py")
        for max_results in [10, 100, 1000]:
            target = {10: 0.5, 100: 1.0, 1000: 3.0}.get(max_results, 3.0)
            # Pass show_dependencies=false and show_related=false to
            # trigger the fast path (skip full project walk).
            dur = run_benchmark(
                f"max_results_early_stop_{max_results}", target,
                run_discover, repo, "**/*.py", max_results=max_results,
                show_dependencies="false", show_related="false"
            )
            if dur:
                print(f"  max_results={max_results}: {dur:.3f}s")
        
        # 3. Relevance scoring speed (run with show_dependencies=true)
        repo = generate_repo(500, tmpdir, ".py")
        dur = run_benchmark(
            "relevance_scoring_500", 5.0,
            run_discover, repo, "**/*.py", show_dependencies="true"
        )
        if dur:
            print(f"  Relevance scoring 500 files: {dur:.3f}s")
        
        # 4. Extension filtering overhead
        repo = generate_repo(500, tmpdir, ".py")
        for ext_filter in ["py", "go", "js"]:
            target = 2.0
            # Generate some files with the filter ext
            for i in range(50):
                (Path(repo) / f"extra_{i}.{ext_filter}").write_text("// extra", encoding="utf-8")
            
            dur = run_benchmark(
                f"extension_filter_{ext_filter}", target,
                run_discover, repo, f"**/*.{ext_filter}"
            )
            if dur:
                print(f"  Extension filter .{ext_filter}: {dur:.3f}s")
    
    # Save results
    out_path = Path("benchmark_discover_results.json")
    out_path.write_text(json.dumps(RESULTS, indent=2), encoding="utf-8")
    print(f"\nResults saved to {out_path}")
    
    print("\n" + "=" * 60)
    print("| Benchmark | Result | Target | Status |")
    print("=" * 60)
    for r in RESULTS:
        print(f"| {r['benchmark']:<30} | {r['result']:<10} | {r['target']:<10} | {r['status']:<6} |")
    print("=" * 60)
    
    return RESULTS


if __name__ == "__main__":
    main()
