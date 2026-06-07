# SPDX-License-Identifier: MIT
"""Tests for the SBOM generator.

Docs: test_generator.doc.md
"""

import json
import tempfile
from pathlib import Path

import pytest

from sbom_generator.generator import SBOMGenerator
from sbom_generator.models import SBOM, SBOMPackage, SBOMMetadata


class TestSBOMGenerator:

    def test_init(self):
        gen = SBOMGenerator()
        assert gen.tool_name == "SIN-Code-SBOM-Generator"
        assert gen.tool_version == "1.0.0"

    def test_generate_from_sca_results_basic(self):
        gen = SBOMGenerator()
        sca_data = {
            "packages": [
                {"name": "lodash", "version": "4.17.21", "license": "MIT", "type": "library"},
                {"name": "express", "version": "4.18.2", "license": "MIT", "type": "library"},
            ],
            "files_scanned": ["package.json"],
        }
        sbom = gen.generate_from_sca_results(sca_data, document_name="test-sbom")
        assert sbom.metadata.document_name == "test-sbom"
        assert sbom.total_packages == 2
        assert sbom.source_type == "npm"
        assert len(sbom.packages) == 2

    def test_generate_from_sca_results_with_vulns(self):
        gen = SBOMGenerator()
        sca_data = {
            "packages": [
                {"name": "lodash", "version": "4.17.21", "license": "MIT", "type": "library"},
            ],
            "vulnerabilities": [
                {"package": "lodash", "severity": "high", "cve": "CVE-2021-23337"},
                {"package": "lodash", "severity": "critical", "cve": "CVE-2021-23337"},
            ],
        }
        sbom = gen.generate_from_sca_results(sca_data)
        assert sbom.total_packages == 1
        pkg = sbom.packages[0]
        assert pkg.has_vulnerabilities is True
        assert pkg.vulnerability_count == 2
        assert pkg.high_vulns == 1
        assert pkg.critical_vulns == 1

    def test_generate_from_raw_dependencies(self):
        gen = SBOMGenerator()
        deps = [
            {"name": "requests", "version": "2.31.0", "license": "Apache-2.0", "purl": "pkg:pypi/requests@2.31.0"},
            {"name": "urllib3", "version": "2.0.7", "license": "MIT"},
        ]
        sbom = gen.generate_from_raw_dependencies(deps, document_name="python-deps")
        assert sbom.total_packages == 2
        assert sbom.packages[0].purl == "pkg:pypi/requests@2.31.0"
        assert "Apache-2.0" in sbom.unique_licenses

    def test_export_spdx(self):
        gen = SBOMGenerator()
        sbom = self._create_test_sbom()
        json_str = gen.export_spdx(sbom)
        data = json.loads(json_str)
        assert data["spdxVersion"] == "SPDX-2.3"
        assert data["SPDXID"] == "SPDXRef-DOCUMENT"
        assert data["name"] == "test-sbom"
        assert len(data["packages"]) == 2
        assert data["packages"][0]["name"] == "lodash"
        assert data["packages"][0]["licenseConcluded"] == "MIT"

    def test_export_spdx_to_file(self):
        gen = SBOMGenerator()
        sbom = self._create_test_sbom()
        with tempfile.TemporaryDirectory() as tmpdir:
            path = Path(tmpdir) / "test.spdx.json"
            gen.export_spdx(sbom, str(path))
            assert path.exists()
            data = json.loads(path.read_text())
            assert data["spdxVersion"] == "SPDX-2.3"

    def test_export_cyclonedx(self):
        gen = SBOMGenerator()
        sbom = self._create_test_sbom()
        json_str = gen.export_cyclonedx(sbom)
        data = json.loads(json_str)
        assert data["bomFormat"] == "CycloneDX"
        assert data["specVersion"] == "1.5"
        assert len(data["components"]) == 2
        assert data["components"][0]["name"] == "lodash"
        assert data["components"][0]["type"] == "library"

    def test_export_cyclonedx_to_file(self):
        gen = SBOMGenerator()
        sbom = self._create_test_sbom()
        with tempfile.TemporaryDirectory() as tmpdir:
            path = Path(tmpdir) / "test.cyclonedx.json"
            gen.export_cyclonedx(sbom, str(path))
            assert path.exists()
            data = json.loads(path.read_text())
            assert data["bomFormat"] == "CycloneDX"

    def test_export_summary(self):
        gen = SBOMGenerator()
        sbom = self._create_test_sbom()
        summary = gen.export_summary(sbom)
        assert "SBOM Summary: test-sbom" in summary
        assert "lodash" in summary
        assert "express" in summary
        assert "MIT" in summary

    def test_detect_source_type(self):
        gen = SBOMGenerator()
        assert gen._detect_source_type(["package.json"]) == "npm"
        assert gen._detect_source_type(["requirements.txt"]) == "pypi"
        assert gen._detect_source_type(["go.mod"]) == "go"
        assert gen._detect_source_type([]) == ""

    def test_parse_package(self):
        gen = SBOMGenerator()
        raw = {
            "name": "test-pkg",
            "version": "1.0.0",
            "license": "MIT",
            "type": "library",
            "purl": "pkg:npm/test-pkg@1.0.0",
            "dependencies": ["dep1", "dep2"],
        }
        pkg = gen._parse_package(raw)
        assert pkg is not None
        assert pkg.name == "test-pkg"
        assert pkg.version == "1.0.0"
        assert pkg.license_concluded == "MIT"
        assert pkg.purl == "pkg:npm/test-pkg@1.0.0"
        assert pkg.dependencies == ["dep1", "dep2"]

    def test_parse_package_none(self):
        gen = SBOMGenerator()
        assert gen._parse_package(None) is None
        assert gen._parse_package({}) is None  # Empty dict returns None

    def test_annotate_vulnerabilities(self):
        gen = SBOMGenerator()
        packages = [SBOMPackage(name="lodash", version="1.0.0")]
        vulns = [
            {"package": "lodash", "severity": "critical"},
            {"package": "lodash", "severity": "high"},
            {"package": "lodash", "severity": "medium"},
        ]
        gen._annotate_vulnerabilities(packages, vulns)
        assert packages[0].critical_vulns == 1
        assert packages[0].high_vulns == 1
        assert packages[0].medium_vulns == 1

    def _create_test_sbom(self) -> SBOM:
        metadata = SBOMMetadata(document_name="test-sbom")
        packages = [
            SBOMPackage(name="lodash", version="4.17.21", license_concluded="MIT", type="library"),
            SBOMPackage(name="express", version="4.18.2", license_concluded="MIT", type="library"),
        ]
        return SBOM(metadata=metadata, packages=packages, total_packages=2, total_dependencies=0, unique_licenses=["MIT"])
