CEO Audit
=========

**SOTA repository audit. 47 quality gates across 8 axes. 3-5 minutes for a typical repo.**

What it does
------------

CEO Audit is a board-grade, evidence-based, multi-axis review that surfaces
everything a CTO would want to know before approving a deployment:

| Axis | Gates | What it catches |
|------|-------|-----------------|
| Security | 12 | OWASP Top 10, ASVS v5.0, CWE Top 25, secrets, injection, SSRF, ReDoS |
| Performance | 6 | O(n²) traps, memory leaks, unbounded caches, sync I/O |
| Code Quality | 7 | Cyclomatic complexity, dead code, naming, TODOs |
| Testing | 5 | Coverage, flaky tests, edge cases, isolation |
| Dependencies | 5 | CVEs, abandonment, unpinned versions, license risk |
| Documentation | 4 | README, CHANGELOG, CoDocs, inline comments |
| Architecture | 4 | Cycles, god modules, orphans, untested hot paths |
| Compliance | 4 | License headers, SECURITY.md, SBOM, PII in logs |

Quick start
-----------

```bash
# Full audit on current directory
/ceo-audit

# Audit a specific repo
/ceo-audit /path/to/repo

# Security-only mode (faster)
/ceo-audit --profile=SECURITY

# Pre-release mode
/ceo-audit --profile=RELEASE

# CI mode (exit code reflects grade)
/ceo-audit --grade=B
```

Output
------

```
~/ceo-audits/<repo>-ceo-audit-<timestamp>/
  ├─ report.md          Board-ready Markdown
  ├─ report.sarif       GitHub Code Scanning compatible
  ├─ report.html        PDF-exportable
  ├─ report.json        Programmatic
  ├─ score.json         Numeric score breakdown
  ├─ action_plan.json   ROI-ranked fixes
  └─ findings/          Raw per-axis output (security.json, etc.)
```

Grading
-------

| Grade | Score | Verdict |
|-------|-------|---------|
| A+ | 95-100 | SOTA-ready |
| A | 85-94 | Production-ready |
| B | 70-84 | Acceptable, monitor |
| C | 55-69 | Needs work |
| D | 40-54 | Significant risk |
| F | 0-39 | Halt |

**Default:** any CRITICAL finding = automatic F.

Files
-----

- `SKILL.md` — Main entry (frontmatter + workflow)
- `scripts/audit.sh` — Main script
- `scripts/axis_*.sh` — 8 axis scripts (security, performance, ...)
- `scripts/score.py` — Aggregation + scoring
- `scripts/report.py` — Report generator (MD + SARIF + JSON + HTML)
- `lib/` — Python helpers (OWASP ASVS, CWE Top 25, SIN-Tools wrapper)
- `templates/` — Report templates
- `hooks/post_audit.py` — Open report + record in SIN-Brain

See `SKILL.md` for the full methodology and gate list.
