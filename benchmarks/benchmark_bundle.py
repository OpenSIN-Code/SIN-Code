# SPDX-License-Identifier: MIT
#!/usr/bin/env python3
"""Bundle/MCP Performance Benchmarks.

MCP server startup time
Tool dispatch latency
Concurrent request handling
"""

import gc
import json
import subprocess
import time
from pathlib import Path

BUNDLE_VENV = "/Users/jeremy/dev/SIN-Code-Bundle/.venv/bin/python3"
BUNDLE_ROOT = "/Users/jeremy/dev/SIN-Code-Bundle"
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
                "tool": "Bundle",
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
                "tool": "Bundle",
                "benchmark": name,
                "result": f"ERROR: {e}",
                "target": f"{target:.3f}s",
                "status": "ERROR",
                "raw_seconds": -1,
            }
        )
        return None


def run_bundle_script(script: str, timeout: int = 60) -> str:
    """Run a Python script in the Bundle venv."""
    full_script = f"""
import sys
sys.path.insert(0, "{BUNDLE_ROOT}/src")
{script}
"""
    result = subprocess.run(
        [BUNDLE_VENV, "-c", full_script], capture_output=True, text=True, timeout=timeout
    )
    if result.returncode != 0:
        raise RuntimeError(f"Bundle script failed: {result.stderr[:500]}")
    return result.stdout.strip()


def benchmark_startup() -> float:
    """Benchmark MCP server startup time by importing and creating FastMCP."""
    script = """
import time
start = time.perf_counter()
from mcp.server.fastmcp import FastMCP
mcp = FastMCP("benchmark")
duration = time.perf_counter() - start
print(f"STARTUP|{duration:.4f}")
"""
    output = run_bundle_script(script, timeout=30)
    parts = output.split("|")
    return float(parts[1])


def benchmark_tool_dispatch() -> float:
    """Benchmark tool dispatch latency."""
    script = """
import time
from mcp.server.fastmcp import FastMCP

mcp = FastMCP("benchmark")

@mcp.tool()
def test_tool(x: int) -> int:
    return x * 2

start = time.perf_counter()
# Simulate a tool call by directly invoking the registered function
result = test_tool(5)
duration = time.perf_counter() - start
print(f"DISPATCH|{duration:.4f}|{result}")
"""
    output = run_bundle_script(script, timeout=30)
    parts = output.split("|")
    return float(parts[1])


def benchmark_concurrent_requests(num_requests: int) -> float:
    """Benchmark concurrent request handling."""
    script = f"""
import time
import threading
from mcp.server.fastmcp import FastMCP

mcp = FastMCP("benchmark")

results = []

@mcp.tool()
def test_tool(x: int) -> int:
    return x * 2

start = time.perf_counter()

threads = []
for i in range({num_requests}):
    t = threading.Thread(target=lambda: results.append(test_tool(i)))
    threads.append(t)
    t.start()

for t in threads:
    t.join()

duration = time.perf_counter() - start
print(f"CONCURRENT|{{duration:.4f}}|{{len(results)}}")
"""
    output = run_bundle_script(script, timeout=60)
    parts = output.split("|")
    return float(parts[1])


def main():
    print("=" * 60)
    print("Bundle/MCP Benchmarks")
    print("=" * 60)

    # 1. Startup time
    startup_time = run_benchmark("mcp_startup", 5.0, benchmark_startup)
    if startup_time:
        print(f"  MCP startup: {startup_time:.3f}s")

    # 2. Tool dispatch
    dispatch_time = run_benchmark("tool_dispatch", 2.0, benchmark_tool_dispatch)
    if dispatch_time:
        print(f"  Tool dispatch: {dispatch_time:.4f}s")

    # 3. Concurrent requests
    for num_requests in [10, 50, 100]:
        target = {10: 2.0, 50: 3.0, 100: 5.0}.get(num_requests, 5.0)
        dur = run_benchmark(
            f"concurrent_{num_requests}_requests",
            target,
            benchmark_concurrent_requests,
            num_requests,
        )
        if dur:
            print(f"  Concurrent {num_requests} requests: {dur:.3f}s")

    # Save results
    out_path = Path("benchmark_bundle_results.json")
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
    main()
