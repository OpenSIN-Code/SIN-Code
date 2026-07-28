"""Regression tests for the production-focused CEO security scanner."""

from __future__ import annotations

import importlib.util
import shutil
import sys
from pathlib import Path

import pytest

SCRIPT = Path(__file__).parents[1] / "scripts" / "security_scan.py"
SPEC = importlib.util.spec_from_file_location("ceo_security_scan", SCRIPT)
assert SPEC and SPEC.loader
security_scan = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = security_scan
SPEC.loader.exec_module(security_scan)


def finding_gates(result: dict) -> set[str]:
    return {finding["gate"] for finding in result["findings"]}


def test_excludes_tests_docs_and_rule_definitions(tmp_path: Path) -> None:
    (tmp_path / "tests").mkdir()
    (tmp_path / "tests" / "test_bad.py").write_text(
        "import subprocess\nsubprocess.run(cmd, shell=True)\n"
    )
    (tmp_path / "docs").mkdir()
    (tmp_path / "docs" / "example.md").write_text("password = 'realisticsecretvalue123'")
    rules = tmp_path / "SIN-Code-SAST-Tool" / "pkg" / "rules"
    rules.mkdir(parents=True)
    (rules / "rules.go").write_text('var patterns = []string{"pickle.loads("}')

    secret_rules = tmp_path / "SIN-Code-Secrets-Scanner" / "pkg" / "rules"
    secret_rules.mkdir(parents=True)
    key_marker = "PRIVATE " + "KEY"
    synthetic_rule_key = "\n".join(
        [
            f"-----BEGIN RSA {key_marker}-----",
            "VGhpcyBpcyBhIHN5bnRoZXRpYyBydWxlIGZpeHR1cmU=",
            f"-----END RSA {key_marker}-----",
        ]
    )
    (secret_rules / "rules.go").write_text(f"var patterns = []string{{`{synthetic_rule_key}`}}")

    result = security_scan.scan(tmp_path)
    assert result["findings"] == []


def test_detects_production_shell_execution(tmp_path: Path) -> None:
    (tmp_path / "app.py").write_text("import subprocess\nsubprocess.run(command, shell=True)\n")
    result = security_scan.scan(tmp_path)
    assert "1.3" in finding_gates(result)
    assert result["findings"][0]["locations"] == ["app.py:2"]


def test_allow_shell_marker_requires_nearby_reasoned_boundary(tmp_path: Path) -> None:
    (tmp_path / "watcher.go").write_text(
        "// ceo-audit: allow-shell — operator-authored command boundary\n"
        'func run(command string) { exec.Command("sh", "-c", command) }\n'
    )
    result = security_scan.scan(tmp_path)
    assert "1.3" not in finding_gates(result)


def test_secret_scanner_ignores_dynamic_query_concatenation(tmp_path: Path) -> None:
    (tmp_path / "provider.go").write_text(
        'u := base + "&api_key=" + url.QueryEscape(key) + "&num="\n'
    )
    result = security_scan.scan(tmp_path)
    assert "1.1" not in finding_gates(result)


def test_secret_scanner_detects_literal_value(tmp_path: Path) -> None:
    (tmp_path / "config.py").write_text('api_key = "abcdefghijklmnopqrstuvwxyz123456"\n')
    result = security_scan.scan(tmp_path)
    assert "1.1" in finding_gates(result)


def test_ssrf_detects_untrusted_dynamic_url(tmp_path: Path) -> None:
    (tmp_path / "client.py").write_text("import requests\nrequests.get(user_url)\n")
    result = security_scan.scan(tmp_path)
    assert "1.5" in finding_gates(result)


def test_ssrf_respects_nearby_egress_gate(tmp_path: Path) -> None:
    path = tmp_path / "browser.go"
    path.write_text(
        "func navigate(ctx context.Context, targetURL string) {\n"
        "    if err := egress.Check(ctx, targetURL, egress.Policy{}); err != nil { return }\n"
        "    chromedp.Navigate(targetURL)\n"
        "}\n"
    )
    result = security_scan.scan(tmp_path)
    assert "1.5" not in finding_gates(result)


def test_dataflow_only_gates_are_explicitly_skipped(tmp_path: Path) -> None:
    result = security_scan.scan(tmp_path)
    statuses = {gate["id"]: gate for gate in result["gates"]}
    for gate_id in ("1.4", "1.10", "1.11"):
        assert statuses[gate_id]["status"] == "skipped"
        assert statuses[gate_id]["reason"]


@pytest.mark.skipif(shutil.which("gitleaks") is None, reason="gitleaks not installed")
def test_gitleaks_backend_detects_private_key_in_production_source(tmp_path: Path) -> None:
    marker = "RSA " + "PRIVATE KEY"
    synthetic_key = "\n".join(
        [
            f"-----BEGIN {marker}-----",
            "VGhpcyBpcyBhIHN5bnRoZXRpYyB0ZXN0IGZpeHR1cmUsIG5vdCBhIHJlYWwga2V5Lg==",
            f"-----END {marker}-----",
        ]
    )
    (tmp_path / "secrets.py").write_text(f'blob = """{synthetic_key}"""\n')
    result = security_scan.scan(tmp_path)
    finding = next(item for item in result["findings"] if item["gate"] == "1.1")
    assert finding["locations"] == ["secrets.py:1"]
