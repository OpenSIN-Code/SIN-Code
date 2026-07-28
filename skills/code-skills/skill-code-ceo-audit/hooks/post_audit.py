#!/usr/bin/env python3
"""Purpose: Post-audit hook — opens the report, suggests follow-ups, and
records this audit in the SIN-Brain memory.

Invoked automatically by audit.sh after the report is written.

Docs: post_audit.doc.md
"""

from __future__ import annotations

import json
import os
import platform
import subprocess
import sys
from pathlib import Path

# ── Module overview ──────────────────────────────────────────────────
#
# Order of operations (called by main):
#   1. show_summary       — prints a one-line "Grade Score crit High Total"
#   2. suggest_follow_ups — actionable list based on grade
#   3. record_in_sin_brain — soft-record into SIN-Brain (skipped on ImportError)
#   4. open_report         — launches OS default viewer on report.md
#
# All four functions are best-effort. We never raise after the audit
# has already produced score.json — the caller (audit.sh) considers
# the pipeline successful at that point.
#
# Platform support: macOS, Linux, Windows. open_report uses the
# correct launcher for each (open / xdg-open / startfile).
# ─────────────────────────────────────────────────────────────────────


# ── Report viewer (platform-aware) ────────────────────────────────────


def open_report(run_dir: Path) -> None:
    """Open the Markdown report in the default browser/viewer."""
    md = run_dir / "report.md"
    if not md.exists():
        # Silent no-op when the report is missing — caller already warned.
        return
    # Platform.system() returns "Darwin"/"Linux"/"Windows" — the de-facto names.
    system = platform.system()
    try:
        # Platform-specific viewer launch — best-effort, never fatal.
        if system == "Darwin":
            # macOS: `open` launches the default Markdown viewer (often a browser).
            # check=False so we don't raise if the viewer returns non-zero.
            subprocess.run(["open", str(md)], check=False)
        elif system == "Linux":
            # Linux: xdg-open is the desktop-environment-agnostic launcher.
            # Works under GNOME, KDE, XFCE, etc.
            subprocess.run(["xdg-open", str(md)], check=False)
        elif system == "Windows":
            # Windows: startfile uses the file-association registry.
            # type: ignore because os.startfile is Windows-only in mypy stubs.
            os.startfile(str(md))  # type: ignore
    except Exception as e:
        # Never fail the audit because the viewer crashed — just log + show path.
        # User can still cat the file manually using the printed path.
        print(f"Could not open report: {e}", file=sys.stderr)
        print(f"Report path: {md}", file=sys.stderr)


# ── Console summary ───────────────────────────────────────────────────


def show_summary(score_path: Path) -> None:
    """Print a one-line summary to console."""
    if not score_path.exists():
        # No score = the audit never produced a result — silent return.
        return
    data = json.loads(score_path.read_text())
    # Defensive .get() — missing keys default to safe placeholder values.
    grade = data.get("grade", "?")
    score = data.get("score", 0)
    critical = data.get("critical", 0)
    high = data.get("high", 0)
    total = data.get("total_findings", 0)
    # Compact format: `  A  92.5/100  crit=0  high=2  total=15`
    # Format specifiers: {grade:3s} = left-pad grade to 3 chars (handles "A+").
    print(f"\n  {grade:3s}  {score:5.1f}/100  crit={critical}  high={high}  total={total}\n")


# ── Optional: SIN-Brain memory observation ───────────────────────────


def record_in_sin_brain(run_dir: Path, score: dict) -> None:
    """If SIN-Brain is installed, record this audit as a memory observation."""
    try:
        # SIN-Brain is optional — gracefully skip when not installed.
        # The import is inside the try block to keep startup time low.
        import sin_brain  # type: ignore

        # Cortex is SIN-Brain's primary entry point for observations.
        cortex = sin_brain.Cortex()
        cortex.observe(
            # Category buckets observations for later querying.
            category="ceo-audit",
            # Headline used by SIN-Brain UI for quick grep.
            summary=f"{score.get('grade', '?')} ({score.get('score', 0)}/100) — "
            f"{score.get('critical', 0)} critical, {score.get('high', 0)} high",
            metadata={
                # Full audit details for forensic recall in later sessions.
                "repo": score.get("repo"),
                "grade": score.get("grade"),
                "score": score.get("score"),
                "axes": score.get("axes"),
                "severity_counts": score.get("severity_counts"),
                "run_dir": str(run_dir),
            },
            # Tags enable filtering by grade in SIN-Brain queries.
            # Lowercased so queries are case-insensitive.
            tags=["ceo-audit", "audit", score.get("grade", "unknown").lower()],
        )
        print("  Recorded in SIN-Brain.")
    except ImportError:
        pass  # SIN-Brain not installed, skip
    except Exception as e:
        # Catch-all: never fail the audit due to a memory write error.
        # SIN-Brain bugs should never block the user's audit pipeline.
        print(f"  SIN-Brain record failed: {e}", file=sys.stderr)


# ── Follow-up checklist (grade-conditional) ──────────────────────────


def suggest_follow_ups(score: dict, run_dir: Path) -> None:
    """Print a follow-up checklist based on the findings."""
    # Defensive .get() — all fields default to safe zero values.
    grade = score.get("grade", "?")
    critical = score.get("critical", 0)
    high = score.get("high", 0)
    print("\n  Follow-ups:")
    # Always surface CRITICAL/HIGH counts first — most urgent.
    # These show up regardless of grade because they're always actionable.
    if critical > 0:
        print(f"    - [ ] Address {critical} CRITICAL finding(s) immediately")
    if high > 0:
        print(f"    - [ ] Address {high} HIGH finding(s) before next release")
    # Grade-conditional next-step suggestions.
    # Each branch covers a contiguous grade band — no overlap, exhaustive.
    if grade in ("F", "D"):
        # Worst grades → schedule re-audit soon to confirm fix.
        # 1-2 weeks gives time to land fixes without losing momentum.
        print("    - [ ] Schedule a follow-up audit in 1-2 weeks")
    if grade in ("C", "B"):
        # Middle grades → re-audit after addressing this batch of findings.
        # No fixed cadence — driven by the team's fix velocity.
        print("    - [ ] Re-audit after addressing findings")
    if grade in ("A", "A+"):
        # Good grades → quarterly cadence to catch drift.
        # Sufficient to catch new CVEs in dependencies + tech-debt accumulation.
        print("    - [ ] Schedule quarterly re-audit")
    # Always show paths to the artifacts.
    # action_plan.json + report.md are the two primary review surfaces.
    print(f"    - [ ] Review the action plan: {run_dir}/action_plan.json")
    print(f"    - [ ] Read the full report: {run_dir}/report.md")
    print()


# ── CLI entry ─────────────────────────────────────────────────────────


def main():
    """CLI entry point — invoked by audit.sh after the report is written.

    Args (from sys.argv):
        run_dir: Path to the audit run directory containing score.json.

    Steps (in order):
        1. show_summary       — one-line grade/score to stdout
        2. suggest_follow_ups — actionable next steps based on grade
        3. record_in_sin_brain — best-effort memory observation (skipped
                                  silently if `sin_brain` package is absent)
        4. open_report         — open report.md in the default viewer
                                  (macOS / Linux / Windows)

    Exits:
        1 if run_dir is missing or contains no score.json (audit failed).
    """
    if len(sys.argv) < 2:
        print("Usage: post_audit.py <run_dir>", file=sys.stderr)
        sys.exit(1)
    run_dir = Path(sys.argv[1])
    score_path = run_dir / "score.json"
    if not score_path.exists():
        # No score.json means score.py failed — surface that clearly.
        print("No score.json found — audit failed.", file=sys.stderr)
        sys.exit(1)
    score = json.loads(score_path.read_text())
    # Order matters: summary → follow-ups → record → open report.
    show_summary(score_path)
    suggest_follow_ups(score, run_dir)
    record_in_sin_brain(run_dir, score)
    open_report(run_dir)


if __name__ == "__main__":
    main()
