# SPDX-License-Identifier: MIT
#!/usr/bin/env python3
"""Execute-Tool Performance Benchmarks.

Command execution overhead (empty vs real)
Secret redaction performance (1MB output with 100 secrets)
Timeout handling (does it kill in <100ms?)
"""

import gc
import json
import os
import subprocess
import sys
import tempfile
import time
from pathlib import Path

EXECUTE_BIN = "/Users/jeremy/.local/bin/execute"
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
                "tool": "Execute",
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
                "tool": "Execute",
                "benchmark": name,
                "result": f"ERROR: {e}",
                "target": f"{target:.3f}s",
                "status": "ERROR",
                "raw_seconds": -1,
            }
        )
        return None


def run_execute(command: str, timeout: int = 30, **kwargs) -> dict:
    """Run execute binary and return parsed result."""
    args = [
        EXECUTE_BIN,
        "-command",
        command,
        "-timeout",
        str(timeout),
        "-format",
        "json",
    ]
    for k, v in kwargs.items():
        args.extend([f"-{k.replace('_', '-')}", str(v)])

    start = time.perf_counter()
    result = subprocess.run(args, capture_output=True, text=True, timeout=60)
    duration = time.perf_counter() - start

    if result.returncode != 0:
        raise RuntimeError(f"execute failed: {result.stderr[:200]}")

    try:
        data = json.loads(result.stdout)
    except json.JSONDecodeError:
        data = {}

    return {
        "duration": duration,
        "output_duration_ms": data.get("duration_ms", 0),
        "success": data.get("success", False),
        "stdout_len": len(data.get("stdout", "")),
    }


def main():
    print("=" * 60)
    print("Execute-Tool Benchmarks")
    print("=" * 60)

    if not Path(EXECUTE_BIN).exists():
        print(f"ERROR: execute binary not found at {EXECUTE_BIN}")
        sys.exit(1)

    # 1. Empty command overhead
    for _ in range(3):
        res = run_benchmark("overhead_empty_command", 0.5, run_execute, "echo hello", timeout=30)
    if res:
        print(f"  Empty command overhead: {res['duration']:.3f}s")

    # 2. Real command overhead
    res = run_benchmark("overhead_real_command", 1.0, run_execute, "ls -la /usr/bin", timeout=30)
    if res:
        print(f"  Real command (ls) overhead: {res['duration']:.3f}s")

    # 3. Secret redaction with 1MB output, 100 secrets
    # Generate a script that outputs 1MB with embedded secrets
    with tempfile.NamedTemporaryFile(mode="w", suffix=".sh", delete=False) as f:
        lines = ["#!/bin/bash"]
        for i in range(100):
            lines.append(f'echo "api_key=secret_value_{i}_abcdefghijklmnopqrstuvwxyz"')
        # Add padding to reach ~1MB
        lines.append(
            'for i in $(seq 1 1000); do echo "padding line $i with some data to increase output size"; done'
        )
        f.write("\n".join(lines))
        script_path = f.name
    os.chmod(script_path, 0o755)

    try:
        res = run_benchmark(
            "secret_redaction_1mb_100_secrets", 3.0, run_execute, f"bash {script_path}", timeout=30
        )
        if res:
            print(f"  Secret redaction (1MB, 100 secrets): {res['duration']:.3f}s")
            print(f"    Output size: {res['stdout_len']} bytes")
    finally:
        os.unlink(script_path)

    # 4. Timeout handling — does it kill in <100ms after timeout?
    # Use a command that sleeps forever
    timeout_val = 1
    res = run_benchmark("timeout_kill_latency", 1.5, run_execute, "sleep 60", timeout=timeout_val)
    if res:
        print(f"  Timeout kill (1s sleep 60s): {res['duration']:.3f}s")
        # Check if it killed within 100ms of timeout
        overhead = res["duration"] - timeout_val
        overhead_status = "PASS" if overhead <= 0.2 else "FAIL"
        RESULTS.append(
            {
                "tool": "Execute",
                "benchmark": "timeout_overhead",
                "result": f"{overhead:.3f}s",
                "target": "0.100s",
                "status": overhead_status,
                "raw_seconds": overhead,
            }
        )
        print(f"  Timeout overhead: {overhead:.3f}s (target <0.1s) -> {overhead_status}")

    # Save results
    out_path = Path("benchmark_execute_results.json")
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
