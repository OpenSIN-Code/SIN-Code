"""SARIF and effort-estimation regression tests."""

from __future__ import annotations

import importlib.util
import sys
from pathlib import Path

SKILL_ROOT = Path(__file__).resolve().parents[1]


def load(name: str, path: Path):
    spec = importlib.util.spec_from_file_location(name, path)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


def test_sarif_contains_primary_and_related_source_locations(tmp_path: Path) -> None:
    report = load("ceo_report_locations", SKILL_ROOT / "scripts" / "report.py")
    sarif = report.make_sarif(
        {
            "axes": {
                "quality": {
                    "findings": [
                        {
                            "gate": "3.1",
                            "severity": "MEDIUM",
                            "cwe": "QUALITY-COMPLEXITY",
                            "title": "Complex function",
                            "description": "two sites",
                            "fix": "split it",
                            "occurrence_count": 2,
                            "locations": ["src/a.py:12", "src/b.py:34"],
                        }
                    ]
                }
            }
        },
        tmp_path,
    )
    result = sarif["runs"][0]["results"][0]
    primary = result["locations"][0]["physicalLocation"]
    assert primary["artifactLocation"]["uri"] == "src/a.py"
    assert primary["region"]["startLine"] == 12
    assert result["relatedLocations"][0]["physicalLocation"]["region"]["startLine"] == 34
    assert result["properties"]["occurrence-count"] == 2


def test_occurrence_count_scales_but_caps_effort() -> None:
    score = load("ceo_score_occurrences", SKILL_ROOT / "scripts" / "score.py")
    one = score.estimate_fix_hours(
        {"severity": "MEDIUM", "fix": "Split function", "occurrence_count": 1}
    )
    many = score.estimate_fix_hours(
        {"severity": "MEDIUM", "fix": "Split function", "occurrence_count": 200}
    )
    assert one == 2.0
    assert many == 12.0
