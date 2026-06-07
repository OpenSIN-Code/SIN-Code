# SPDX-License-Identifier: MIT
"""
Purpose: Test the Auditor (degraded mode + JSON parsing).
Docs: test_auditor.doc.md
"""

import json

from sin_code_bundle.tools.mcp_server_builder.auditor import Auditor, AuditReport


class TestAuditorDegraded:
    def test_sin_missing_returns_degraded(self, tmp_path, monkeypatch):
        """When `sin` is missing and install is disabled, return degraded report."""
        monkeypatch.setattr("shutil.which", lambda cmd: None)
        monkeypatch.setattr("subprocess.run", lambda *a, **kw: None)  # noqa
        auditor = Auditor(install_if_missing=False)
        report = auditor.audit(tmp_path)
        assert not report.ok
        assert any(f.get("code") == "AUDIT_UNAVAILABLE" for f in report.findings)


class TestAuditorParse:
    def test_parse_json_block(self):
        auditor = Auditor()
        raw = json.dumps(
            {
                "grade": "A",
                "score": 0.95,
                "gates_passed": 45,
                "gates_total": 47,
                "findings": [{"code": "X", "message": "y"}],
            }
        )
        report = auditor._parse(True, "demo", "prefix " + raw + " suffix")
        assert report.grade == "A"
        assert report.score == 0.95
        assert report.gates_passed == 45
        assert report.gates_total == 47
        assert len(report.findings) == 1

    def test_parse_invalid_json_falls_back(self):
        auditor = Auditor()
        report = auditor._parse(False, "demo", "no json here")
        assert report.grade == "?"
        assert report.gates_total == 47

    def test_parse_success_no_json(self):
        auditor = Auditor(grade="B")
        report = auditor._parse(True, "demo", "all good")
        assert report.ok
        assert report.grade == "B"


class TestAuditReportToDict:
    def test_shape(self):
        r = AuditReport(
            ok=True,
            project="x",
            grade="A",
            score=0.9,
            gates_passed=45,
            gates_total=47,
            findings=[],
        )
        d = r.to_dict()
        assert d["ok"] is True
        assert d["grade"] == "A"
        assert d["gates_total"] == 47
        assert d["findings_count"] == 0
