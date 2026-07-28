# Changelog — ceo-audit skill

All notable changes to the CEO Audit skill are documented here. The format follows
[Conventional Changelog](https://www.conventionalcommits.org/) and this project
adheres to [Semantic Versioning](https://semver.org/).

> **TL;DR**: breaking changes bump the minor version (skill is pre-1.0); additive
> changes bump the patch version; major new axes or report formats bump the minor.

## [Unreleased]

### Added
- `scripts/install-skill.sh` — one-command install with `--dry-run` + smoke test
- `scripts/benchmark.sh` — per-axis timing + baseline diff (`--compare`)
- `scripts/validate-install.sh` — green/red verification of all dependencies
- `lib/sin_tools.py` per-axis `check_*()` methods (Security, Performance, Quality, Testing, Deps, Docs, Architecture, Compliance) wrapping the SIN-Code CLI tools
- `tests/test_audit_end_to_end.py` — full end-to-end test of the audit flow
- `examples/` directory: `sample-report.md`, `sample-sarif.json`, `integration-ci.yml`, `examples/README.md`
- `scripts/audit.doc.md` — CoDocs companion for the main entry script
- Per-axis `.doc.md` companions for the `lib/` modules (already present, now formally tracked)

## [0.2.0] — 2026-06-03

### Added
- OAuth-based GitHub App integration (`lib/github_app.py`, `scripts/post_audit_pr.py`)
  - **No Private Key required** — uses Client ID + Client Secret only
  - Idempotent PR comments via `COMMENT_MARKER`
  - Webhook signature verification (HMAC-SHA256)
- `templates/ceo-audit.yml` — canonical GitHub Actions workflow
- `tests/test_github_app.py` — 18 unit tests (credential resolution, token priority, comment builder, webhook signature, error handling)
- `hooks/post_audit.py` + `hooks/post_audit.doc.md`

### Changed
- SKILL.md rewritten to board-grade format with 47-gate table

## [0.1.0] — 2026-05-08

### Added
- Initial release: 8 axes, 47 gates, multi-language support
- `scripts/audit.sh` — main entry point
- `scripts/axis_*.sh` (8 axes): security, performance, quality, testing, deps, docs, architecture, compliance
- `scripts/score.py` — risk-weighted grading
- `scripts/report.py` — Markdown + SARIF 2.1.0 + JSON + HTML reports
- `lib/owasp_asvs.py` — ASVS v5.0 chapter/requirement lookup
- `lib/cwe.py` — CWE Top 25 lookup
- `lib/sin_tools.py` — initial wrapper for discover / map / grasp / scout
- `lib/add_finding.py` — JSON-line appender for axis findings
- `templates/report.md`, `templates/sarif.json` — report skeleton + SARIF template

[Unreleased]: https://github.com/OpenSIN-Code/SIN-Code/compare/ceo-audit-v0.2.0...HEAD
[0.2.0]: https://github.com/OpenSIN-Code/SIN-Code/compare/ceo-audit-v0.1.0...ceo-audit-v0.2.0
[0.1.0]: https://github.com/OpenSIN-Code/SIN-Code/releases/tag/ceo-audit-v0.1.0
