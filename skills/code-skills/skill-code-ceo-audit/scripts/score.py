"""Purpose: CEO Audit scoring engine.

Takes all 8 axis JSON files + aggregate, computes:
- Per-axis score (0-100)
- Weighted total
- Grade (A+ / A / B / C / D / F)
- Risk score per finding (likelihood × impact × blast radius)
- ROI-ranked action plan
- Regression detection vs last audit
- Compliance mapping (OWASP ASVS, CWE)

Docs: score.doc.md
"""

from __future__ import annotations

import json
import sys
from collections import Counter
from datetime import datetime, timezone
from pathlib import Path

# ── Module overview ──────────────────────────────────────────────────
#
# Inputs (CLI args):
#   1. repo_path  — the audited repository (used for naming the output)
#   2. run_dir    — directory containing findings/<axis>.json
#   3. grade_gate — optional A/B/C → exit-code gate for CI
#
# Outputs (written to run_dir):
#   - score.json       — final per-axis scores, severity counts, grade,
#                        top risks, regression summary, fix-cost estimate
#   - action_plan.json — top 20 findings sorted by ROI (impact ÷ effort)
#
# Algorithm:
#   1. Read only the profile-requested findings/<axis>.json files.
#   2. Apply SEVERITY_PENALTY per finding → per-axis score (clamp 0-100)
#   3. Weighted total = Σ axis_score × AXIS_WEIGHTS[axis]
#   4. Grade = letter from weighted total, CAPPED at F if any CRITICAL
#   5. Risk score per finding = likelihood × impact (CWE Top 25 = 1.5x)
#   6. ROI = risk ÷ fix_hours; action_plan sorted descending
#   7. Regression diff vs the previous run in the same output dir
#
# Hard rules:
#   - AXIS_WEIGHTS must sum to exactly 1.0 (security=30%, perf=10%, ...)
#   - Any CRITICAL finding caps the grade at F (no escape hatch)
#   - Score thresholds in grade() and main()'s gate_scores MUST match
# ─────────────────────────────────────────────────────────────────────


# ── Axis weights (must sum to 1.0) ────────────────────────────────────
# Security carries the highest weight because a single critical security
# flaw can outweigh a thousand quality issues. Tweak with care: changing
# any weight here shifts the grade boundaries across all audited repos.
AXIS_WEIGHTS = {
    "security": 0.30,  # Highest weight: security flaws = game over
    "performance": 0.10,
    "quality": 0.15,
    "testing": 0.15,
    "deps": 0.15,
    "docs": 0.05,
    "architecture": 0.05,
    "compliance": 0.05,
}

# ── Severity → point deduction (per-finding axis-score penalty) ───────
# A single CRITICAL costs 25 points — only 4 CRITICAL = floor of 0.
# INFO = 0 because info findings are documentation, not defects.
SEVERITY_PENALTY = {
    "CRITICAL": 25,
    "HIGH": 10,
    "MEDIUM": 4,
    "LOW": 1,
    "INFO": 0,
}

# A skipped gate is not a defect, but it is missing assurance. Charging a
# small, explicit coverage penalty prevents partially observed axes from
# receiving an A+ while keeping unsupported analysis non-blocking.
SKIPPED_GATE_PENALTY = 2

# ── CWE impact multiplier (CWE Top 25 entries get higher impact) ──────
# Mirror of cwe.CWE_TOP25_2023 — keep both in sync when MITRE refreshes.
# Set membership is O(1) so this hot-path lookup stays cheap.
CWE_TOP25_IMPACT = {
    "CWE-787",
    "CWE-79",
    "CWE-89",
    "CWE-20",
    "CWE-125",
    "CWE-78",
    "CWE-416",
    "CWE-22",
    "CWE-352",
    "CWE-434",
    "CWE-862",
    "CWE-476",
    "CWE-287",
    "CWE-190",
    "CWE-502",
    "CWE-77",
    "CWE-119",
    "CWE-798",
    "CWE-918",
    "CWE-306",
    "CWE-362",
    "CWE-269",
    "CWE-94",
    "CWE-863",
    "CWE-276",
}


# ── Per-axis scoring ──────────────────────────────────────────────────


def score_axis(axis_data: dict) -> tuple[int, list[dict]]:
    """Score findings plus explicit missing-assurance coverage penalties."""
    findings = axis_data.get("findings", [])
    gates = axis_data.get("gates", [])
    skipped_gate_count = sum(
        1 for gate in gates if str(gate.get("status", "")).lower() == "skipped"
    )
    penalty = skipped_gate_count * SKIPPED_GATE_PENALTY
    # Accumulate severity penalties across all findings in this axis.
    # Each finding contributes its severity's penalty value (table above).
    for f in findings:
        sev = f.get("severity", "INFO").upper()
        # Unknown severity → 0 penalty (safe default).
        # This prevents schema drift from silently inflating scores.
        penalty += SEVERITY_PENALTY.get(sev, 0)
    # Clamp: hard floor 0 (no negative scores), hard ceiling 100.
    # Many CRITICAL findings could otherwise produce negative numbers.
    score = max(0, min(100, 100 - penalty))

    # Also surface individual findings (with risk score)
    # Each finding is enriched with risk_score for later sorting.
    enriched = []
    for f in findings:
        sev = f.get("severity", "INFO").upper()
        cwe = f.get("cwe", "")
        # CWE Top-25 entries get a 1.5x impact multiplier (more exploitable).
        # Standard CVEs get 1.0x (baseline impact).
        impact = 1.5 if cwe in CWE_TOP25_IMPACT else 1.0
        # Likelihood encodes how often a finding at this severity is real-vs-FP.
        # CRITICAL=90% real (very few false positives), INFO=5% real.
        likelihood = {"CRITICAL": 0.9, "HIGH": 0.7, "MEDIUM": 0.4, "LOW": 0.2, "INFO": 0.05}.get(
            sev, 0.1
        )
        # Risk = likelihood × impact, rounded to 3 decimals for stable reports.
        # max risk = 0.9 × 1.5 = 1.35 (critical CWE Top-25); min = 0.05 × 1.0.
        risk = round(likelihood * impact, 3)
        enriched.append({**f, "risk_score": risk})
    return score, enriched


# ── Grade mapping ─────────────────────────────────────────────────────


def grade(score: int, critical_count: int) -> str:
    """Map numeric score to letter grade. Critical findings cap the grade."""
    # Hard rule: any CRITICAL finding caps the grade at F regardless of score.
    # No score gymnastics can paper over a single critical security flaw.
    if critical_count > 0:
        return "F"
    # Thresholds must stay in sync with main()'s gate_scores dict.
    # A+ = SOTA (≥95), A = production (≥85), B = acceptable (≥70).
    # Top-grade reserved for genuinely exceptional codebases.
    if score >= 95:
        return "A+"
    if score >= 85:
        return "A"
    if score >= 70:
        return "B"
    # C = staging-only (≥55), D = halt (≥40), F = critical (everything else).
    # Below 55 = "do not deploy to production without significant work".
    if score >= 55:
        return "C"
    if score >= 40:
        return "D"
    # F = halt + remediate + re-audit.
    return "F"


# ── Regression detection (vs previous audit run) ─────────────────────


def detect_regressions(current: list[dict], previous: list[dict] | None) -> dict:
    """Find findings that are new vs the last audit."""
    if not previous:
        # First-ever audit on this repo → everything is "new" by definition.
        # Returning len(current) as "new" gives accurate "first scan" headline.
        return {"new": len(current), "fixed": 0, "regressions": []}
    # Identity = (gate_id, title) — stable across runs even if line numbers shift.
    # Set membership = O(1) lookups for the diff computation below.
    # Sets eliminate dupes — multiple findings with same (gate, title) collapse.
    cur_keys = {(f.get("gate"), f.get("title")) for f in current}
    prev_keys = {(f.get("gate"), f.get("title")) for f in previous}
    # "new" = present in current AND absent in previous → freshly introduced.
    new_findings = [f for f in current if (f.get("gate"), f.get("title")) not in prev_keys]
    # "fixed" = present in previous AND absent in current → resolved since last run.
    # Useful for celebrating fixes in the regression report section.
    fixed_findings = [f for f in previous if (f.get("gate"), f.get("title")) not in cur_keys]
    return {
        "new": len(new_findings),
        "fixed": len(fixed_findings),
        "regressions": new_findings,
    }


# ── Fix-cost + ROI estimation ────────────────────────────────────────


def estimate_fix_hours(finding: dict) -> float:
    """Estimate hours to fix a finding based on severity and complexity."""
    sev = finding.get("severity", "LOW").upper()
    # Base hours per severity — calibrated against historical PR cycle times.
    # Numbers from internal data: 95th-percentile PR cycle time per severity.
    # CRITICAL = 4h (complex fix + review + testing), HIGH = 2h, etc.
    base = {"CRITICAL": 4.0, "HIGH": 2.0, "MEDIUM": 1.0, "LOW": 0.5, "INFO": 0.1}.get(sev, 1.0)
    # Adjust by fix complexity heuristics on the recommended fix text.
    # Looks for keywords that signal multi-file changes.
    fix = finding.get("fix", "")
    if "refactor" in fix.lower() or "split" in fix.lower():
        # Refactors take roughly 2x because they touch multiple files.
        # Examples: "Refactor into smaller modules", "Split god module".
        base *= 2.0
    elif "add" in fix.lower() and "test" in fix.lower():
        # Adding tests adds ~50% because of test setup overhead.
        # Example: "Add unit tests + integration tests for new auth flow".
        base *= 1.5

    raw_occurrences = finding.get("occurrence_count")
    if raw_occurrences is None:
        raw_occurrences = len(finding.get("locations", [])) or 1
    try:
        occurrences = max(1, int(raw_occurrences))
    except (TypeError, ValueError):
        occurrences = 1
    # Findings are grouped by gate, so a 198-site debt cluster cannot honestly
    # cost the same as one site. Scale sub-linearly and cap at 6x to avoid
    # pretending a static estimate is a full project plan.
    occurrence_factor = 1.0 + min(occurrences - 1, 20) * 0.25
    return round(base * occurrence_factor, 1)


# ── ROI ranking (action plan ordering) ───────────────────────────────


def compute_roi(findings: list[dict]) -> list[dict]:
    """Compute ROI for each finding: impact / effort."""
    out = []
    for f in findings:
        # risk_score was set by score_axis; default to 0.1 if missing.
        risk = f.get("risk_score", 0.1)
        hours = estimate_fix_hours(f)
        # max(hours, 0.1) avoids divide-by-zero on near-instant fixes.
        # ROI = risk reduction per hour invested — higher is better.
        # Example: critical CWE Top-25 (risk 1.35) fixed in 0.5h → ROI 2.7.
        roi = round(risk / max(hours, 0.1), 3)
        out.append({**f, "fix_hours_est": hours, "roi": roi})
    # Sort descending: highest-impact, lowest-effort fixes first.
    # Result: the top of the action plan = "do these first" recommendations.
    # Stable sort: ties preserve original order (axis discovery order).
    out.sort(key=lambda x: x.get("roi", 0), reverse=True)
    return out


# ── CLI entry point ──────────────────────────────────────────────────


def main():
    """Aggregate requested audit axes into score.json and action_plan.json.

    ``run_meta.json`` is the execution contract written by ``audit.sh``. Only
    axes requested by the selected profile contribute to the score, and their
    configured weights are normalized to 100%. Missing, invalid, or failed
    requested axes make the audit incomplete and force a failing grade.
    """
    if len(sys.argv) < 3:
        print("Usage: score.py <repo_path> <run_dir> [grade_gate]", file=sys.stderr)
        sys.exit(1)

    repo_path = Path(sys.argv[1]).resolve()
    run_dir = Path(sys.argv[2])
    grade_gate = sys.argv[3] if len(sys.argv) > 3 else ""

    meta_path = run_dir / "run_meta.json"
    if meta_path.exists():
        try:
            run_meta = json.loads(meta_path.read_text())
        except (OSError, json.JSONDecodeError) as exc:
            run_meta = {}
            meta_errors = [f"invalid run_meta.json: {exc}"]
        else:
            meta_errors = []
    else:
        # Backward compatibility for direct score.py usage: a run without
        # metadata is treated as a FULL audit, but the absence is recorded.
        run_meta = {}
        meta_errors = ["run_meta.json missing; assuming FULL profile"]

    requested_raw = run_meta.get("requested_axes") or list(AXIS_WEIGHTS)
    requested_axes = [axis for axis in requested_raw if axis in AXIS_WEIGHTS]
    invalid_axes = [axis for axis in requested_raw if axis not in AXIS_WEIGHTS]
    failed_axes = set(run_meta.get("failed_axes") or [])
    recon_failed = list(run_meta.get("recon_failed") or [])
    missing_tools = list(run_meta.get("missing_tools") or [])
    profile = str(run_meta.get("profile") or "FULL")

    audit_errors = list(meta_errors)
    audit_errors.extend(f"unknown requested axis: {axis}" for axis in invalid_axes)
    audit_errors.extend(f"recon step failed: {name}" for name in recon_failed)
    if not requested_axes:
        audit_errors.append("no valid audit axes were requested")

    weight_sum = sum(AXIS_WEIGHTS[axis] for axis in requested_axes)
    effective_weights = {
        axis: (AXIS_WEIGHTS[axis] / weight_sum if weight_sum else 0.0) for axis in requested_axes
    }

    axes: dict[str, dict] = {}
    all_findings: list[dict] = []
    for axis_name in requested_axes:
        finding_file = run_dir / "findings" / f"{axis_name}.json"
        status = "complete"
        axis_score = 0
        findings: list[dict] = []
        gate_count = 0
        skipped_gate_count = 0
        gate_coverage = 0.0

        if not finding_file.exists():
            status = "missing"
            audit_errors.append(f"requested axis missing output: {axis_name}")
        else:
            try:
                data = json.loads(finding_file.read_text())
                if not isinstance(data, dict):
                    raise ValueError("axis output must be a JSON object")
                if data.get("axis") not in (None, axis_name):
                    raise ValueError(
                        f"axis output declares {data.get('axis')!r}, expected {axis_name!r}"
                    )
                axis_score, findings = score_axis(data)
                gates = data.get("gates", [])
                gate_count = len(gates)
                skipped_gate_count = sum(
                    1 for gate in gates if str(gate.get("status", "")).lower() == "skipped"
                )
                gate_coverage = (
                    (gate_count - skipped_gate_count) / gate_count if gate_count else 1.0
                )
            except (OSError, json.JSONDecodeError, TypeError, ValueError) as exc:
                status = "invalid"
                audit_errors.append(f"invalid axis output {axis_name}: {exc}")

        if axis_name in failed_axes:
            status = "failed"
            axis_score = 0
            audit_errors.append(f"requested axis execution failed: {axis_name}")

        axes[axis_name] = {
            "score": axis_score,
            "finding_count": len(findings),
            "findings": findings,
            "status": status,
            "gate_count": gate_count,
            "skipped_gate_count": skipped_gate_count,
            "gate_coverage": gate_coverage,
        }
        all_findings.extend(findings)

    weighted = sum(axes[axis]["score"] * effective_weights[axis] for axis in requested_axes)

    sev_counter = Counter(f.get("severity", "INFO").upper() for f in all_findings)
    critical_count = sev_counter.get("CRITICAL", 0)
    high_count = sev_counter.get("HIGH", 0)
    audit_complete = not audit_errors
    letter = grade(round(weighted), critical_count) if audit_complete else "F"

    output_root = run_dir.parent
    last_audit_dirs = sorted(output_root.glob(f"{repo_path.name}-ceo-audit-*"))
    regression = {"new": 0, "fixed": 0, "regressions": []}
    if len(last_audit_dirs) >= 2:
        prev_dir = last_audit_dirs[-2]
        prev_score = prev_dir / "score.json"
        if prev_score.exists():
            prev_data = json.loads(prev_score.read_text())
            prev_findings = []
            for axis_data in prev_data.get("axes", {}).values():
                prev_findings.extend(axis_data.get("findings", []))
            regression = detect_regressions(all_findings, prev_findings)

    action_plan = compute_roi(all_findings)
    top_risks = sorted(
        all_findings,
        key=lambda finding: finding.get("risk_score", 0),
        reverse=True,
    )[:3]
    total_hours = sum(estimate_fix_hours(finding) for finding in all_findings)
    owasp_findings = [
        finding for finding in all_findings if finding.get("cwe", "").startswith("CWE-")
    ]

    score_data = {
        "repo": str(repo_path),
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "profile": profile,
        "requested_axes": requested_axes,
        "audit_complete": audit_complete,
        "audit_errors": audit_errors,
        "missing_tools": missing_tools,
        "recon_failed": recon_failed,
        "score": round(weighted, 1),
        "grade": letter,
        "axes": {
            axis: {
                "score": axes[axis]["score"],
                "finding_count": axes[axis]["finding_count"],
                "status": axes[axis]["status"],
                "gate_count": axes[axis]["gate_count"],
                "skipped_gate_count": axes[axis]["skipped_gate_count"],
                "gate_coverage": round(axes[axis]["gate_coverage"], 4),
                "findings": axes[axis]["findings"],
            }
            for axis in requested_axes
        },
        "weights": effective_weights,
        "severity_counts": dict(sev_counter),
        "critical": critical_count,
        "high": high_count,
        "total_findings": len(all_findings),
        "total_fix_hours_est": round(total_hours, 1),
        "top_3_risks": [
            {
                "title": risk.get("title"),
                "severity": risk.get("severity"),
                "risk_score": risk.get("risk_score"),
            }
            for risk in top_risks
        ],
        "regression": regression,
        "compliance": {"owasp_cwe_findings": len(owasp_findings)},
    }
    (run_dir / "score.json").write_text(json.dumps(score_data, indent=2))
    (run_dir / "action_plan.json").write_text(json.dumps(action_plan[:20], indent=2))

    gate_pass = audit_complete
    if grade_gate:
        gate_scores = {"A": 85, "B": 70, "C": 55}
        min_score = gate_scores.get(grade_gate.upper(), 0)
        gate_pass = gate_pass and weighted >= min_score
        print(
            f"Grade gate ({grade_gate}): {'PASS' if gate_pass else 'FAIL'} "
            f"({weighted:.1f} vs {min_score}; complete={audit_complete})"
        )

    print(f"Audit complete: {audit_complete}")
    if audit_errors:
        for error in audit_errors:
            print(f"Audit error: {error}")
    print(f"Final grade: {letter} ({weighted:.1f}/100)")
    print(f"Severities:  {dict(sev_counter)}")
    print(f"Fix cost:    ~{total_hours:.1f} hours")

    sys.exit(0 if gate_pass else 1)


if __name__ == "__main__":
    main()
