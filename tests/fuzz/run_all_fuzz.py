#!/usr/bin/env python3
"""Master MCP fuzz orchestrator — runs all 7 Go tools + SIN-Brain.

Generates a combined crash/hang report for all SIN-Code MCP servers.

Usage:
    python3 run_all_fuzz.py              # Run all
    python3 run_all_fuzz.py --quick      # Quick mode
    python3 run_all_fuzz.py --tool execute  # Single tool
"""

from __future__ import annotations

import json
import subprocess
import sys
import time
import os
from dataclasses import dataclass, field
from typing import Any

BASE_DIR = os.path.dirname(os.path.abspath(__file__))
FUZZ_SCRIPT = os.path.join(BASE_DIR, "fuzz_go_mcp.py")
SIN_BRAIN_SCRIPT = os.path.join(BASE_DIR, "fuzz_sin_brain.py")

# All 7 Go tools + location
TOOLS = [
    ("discover", "/Users/jeremy/.local/bin/discover"),
    ("execute", "/Users/jeremy/.local/bin/execute"),
    ("map", "/Users/jeremy/.local/bin/map"),
    ("grasp", "/Users/jeremy/.local/bin/grasp"),
    ("scout", "/Users/jeremy/.local/bin/scout"),
    ("harvest", "/Users/jeremy/.local/bin/harvest"),
    ("orchestrate", "/Users/jeremy/.local/bin/orchestrate"),
]


@dataclass
class ToolResult:
    name: str
    status: str = "UNKNOWN"  # OK, CRASH, HANG, SKIP, FAIL
    total_attacks: int = 0
    crashes: int = 0
    hangs: int = 0
    violations: int = 0
    duration_s: float = 0.0
    error: str = ""
    raw_stdout: str = ""
    raw_stderr: str = ""


def run_go_fuzz(label: str, binary: str, timeout: float = 3.0) -> ToolResult:
    """Run fuzz_go_mcp.py against one Go tool."""
    result = ToolResult(name=label)
    print(f"\n{'='*60}")
    print(f"  FUZZING: {label} ({binary})")
    print(f"{'='*60}")

    t0 = time.time()
    try:
        proc = subprocess.run(
            [sys.executable, FUZZ_SCRIPT, binary, "--label", label,
             "--timeout", str(timeout)],
            capture_output=True, text=True, timeout=600,  # 10 min max per tool
        )
        result.duration_s = time.time() - t0
        result.raw_stdout = proc.stdout
        result.raw_stderr = proc.stderr

        # Parse the output for statistics
        stdout = proc.stdout
        crash_count = stdout.count("💥")
        hang_count = stdout.count("⏳")
        viol_count = stdout.count("⚠️") - stdout.count("⚠️  Startup failed")  # exclude startup warning

        result.total_attacks = max(
            stdout.count("✅") + crash_count + hang_count
            + stdout.count("⚠️  "),
            1
        )
        result.crashes = crash_count
        result.hangs = hang_count
        result.violations = viol_count

        if proc.returncode == 0:
            result.status = "OK"
        elif proc.returncode == 1:
            result.status = "CRASH"
        elif proc.returncode == 2:
            result.status = "HANG"
        else:
            result.status = f"EXIT_{proc.returncode}"

        if proc.stderr:
            result.error = proc.stderr[:500]

    except subprocess.TimeoutExpired:
        result.status = "TIMEOUT"
        result.error = "Process timed out after 10 min"
        result.duration_s = 600
    except Exception as e:
        result.status = "FAIL"
        result.error = f"{type(e).__name__}: {e}"

    return result


def run_sin_brain_fuzz() -> ToolResult:
    """Run sin_brain fuzz test."""
    result = ToolResult(name="SIN-Brain")
    print(f"\n{'='*60}")
    print(f"  FUZZING: SIN-Brain (Python MCP)")
    print(f"{'='*60}")

    t0 = time.time()
    try:
        proc = subprocess.run(
            [sys.executable, SIN_BRAIN_SCRIPT],
            capture_output=True, text=True, timeout=120,
        )
        result.duration_s = time.time() - t0
        result.raw_stdout = proc.stdout
        result.raw_stderr = proc.stderr

        crash_count = proc.stdout.count("💥")
        result.total_attacks = max(
            proc.stdout.count("✅") + crash_count, 1
        )
        result.crashes = crash_count

        if proc.returncode == 0:
            result.status = "OK"
        elif proc.returncode == 1:
            result.status = "CRASH"
        else:
            result.status = f"EXIT_{proc.returncode}"

    except subprocess.TimeoutExpired:
        result.status = "TIMEOUT"
        result.error = "Process timed out after 2 min"
        result.duration_s = 120
    except Exception as e:
        result.status = "FAIL"
        result.error = f"{type(e).__name__}: {e}"

    return result


def generate_summary(results: list[ToolResult]):
    print("\n\n")
    print("╔" + "═" * 70 + "╗")
    print("║" + "  MCP FUZZ ATTACK — COMBINED REPORT".center(70) + "║")
    print("╚" + "═" * 70 + "╝")

    # Table header
    total_crashes = sum(r.crashes for r in results)
    total_hangs = sum(r.hangs for r in results)
    total_violations = sum(r.violations for r in results)
    total_attacks = sum(r.total_attacks for r in results)
    total_time = sum(r.duration_s for r in results)

    print(f"\n{'Tool':<18} {'Status':<12} {'Attacks':>8} {'Crashes':>8} {'Hangs':>8} {'Viols':>8} {'Time':>8}")
    print("-" * 72)

    for r in results:
        status_icon = {
            "OK": "✅",
            "CRASH": "💥",
            "HANG": "⏳",
            "FAIL": "❌",
            "TIMEOUT": "⏰",
            "SKIP": "⏭️",
        }.get(r.status, "❓")

        print(f"{r.name:<18} {status_icon + ' ' + r.status:<10} "
              f"{r.total_attacks:>8} {r.crashes:>8} {r.hangs:>8} {r.violations:>8} {r.duration_s:>7.1f}s")

    print("-" * 72)
    print(f"{'TOTAL':<18} {'':<12} {total_attacks:>8} {total_crashes:>8} {total_hangs:>8} {total_violations:>8} {total_time:>7.1f}s")

    # Severity summary
    print()
    if total_crashes == 0 and total_hangs == 0 and total_violations == 0:
        print("🛡️  ALL TOOLS PASSED — No crashes, hangs, or violations detected")
    else:
        print(f"💀 VULNERABILITY REPORT:")
        if total_crashes:
            print(f"   💥 {total_crashes} CRASHES across {sum(1 for r in results if r.crashes > 0)} tools")
        if total_hangs:
            print(f"   ⏳ {total_hangs} HANGS across {sum(1 for r in results if r.hangs > 0)} tools")
        if total_violations:
            print(f"   ⚠️  {total_violations} PROTOCOL VIOLATIONS across {sum(1 for r in results if r.violations > 0)} tools")

    # Affected tools detail
    affected = [r for r in results if r.crashes > 0 or r.hangs > 0]
    if affected:
        print()
        print("AFFECTED TOOLS:")
        for r in affected:
            issues = []
            if r.crashes:
                issues.append(f"{r.crashes} crashes")
            if r.hangs:
                issues.append(f"{r.hangs} hangs")
            if r.violations:
                issues.append(f"{r.violations} violations")
            print(f"  {r.name}: {', '.join(issues)}")
            if r.error:
                print(f"    Error: {r.error[:200]}")

    print()
    print("═" * 72 + "\n")


def main():
    import argparse
    parser = argparse.ArgumentParser(description="Run all SIN-Code MCP fuzz tests")
    parser.add_argument("--quick", action="store_true", help="Quick mode (shorter timeout)")
    parser.add_argument("--tool", help="Run only one specific Go tool")
    parser.add_argument("--sin-brain-only", action="store_true", help="Only run SIN-Brain")
    parser.add_argument("--go-only", action="store_true", help="Only run Go tools (skip SIN-Brain)")
    args = parser.parse_args()

    timeout = 2.0 if args.quick else 3.0
    results: list[ToolResult] = []

    # Run Go tools
    if not args.sin_brain_only:
        for label, binary in TOOLS:
            if args.tool and args.tool.lower() != label:
                print(f"  SKIP: {label} (not selected)")
                results.append(ToolResult(name=label, status="SKIP"))
                continue
            r = run_go_fuzz(label, binary, timeout=timeout)
            results.append(r)

    # Run SIN-Brain
    if not args.go_only:
        r = run_sin_brain_fuzz()
        results.append(r)

    generate_summary(results)

    # Exit code: non-zero if any crashes
    if any(r.crashes > 0 for r in results):
        sys.exit(1)
    sys.exit(0)


if __name__ == "__main__":
    main()
