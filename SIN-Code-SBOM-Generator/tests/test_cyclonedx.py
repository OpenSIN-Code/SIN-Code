"""Tests for CycloneDX generator.

Docs: test_cyclonedx.doc.md
"""

import json
import pytest

from sbom_generator.cyclonedx_generator import generate_cyclonedx, cyclonedx_to_json
from sbom_generator.models import SBOM, SBOMPackage, SBOMMetadata


class TestCycloneDXGenerator:

    def test_generate_cyclonedx_basic(self):
        metadata = SBOMMetadata(document_name="test-sbom")
        packages = [
            SBOMPackage(name="lodash", version="4.17.21", license_concluded="MIT"),
        ]
        sbom = SBOM(metadata=metadata, packages=packages)
        doc = generate_cyclonedx(sbom)
        assert doc["bomFormat"] == "CycloneDX"
        assert doc["specVersion"] == "1.5"
        assert doc["version"] == 1
        assert "serialNumber" in doc
        assert len(doc["components"]) == 1
        assert doc["components"][0]["name"] == "lodash"
        assert doc["components"][0]["version"] == "4.17.21"
        assert doc["components"][0]["type"] == "library"

    def test_generate_cyclonedx_with_purl(self):
        metadata = SBOMMetadata(document_name="test-sbom")
        packages = [
            SBOMPackage(name="requests", version="2.31.0", purl="pkg:pypi/requests@2.31.0"),
        ]
        sbom = SBOM(metadata=metadata, packages=packages)
        doc = generate_cyclonedx(sbom)
        pkg = doc["components"][0]
        assert pkg["purl"] == "pkg:pypi/requests@2.31.0"
        assert pkg["bom-ref"] == "pkg:pypi/requests@2.31.0"

    def test_generate_cyclonedx_with_cpe(self):
        metadata = SBOMMetadata(document_name="test-sbom")
        packages = [
            SBOMPackage(name="openssl", version="3.1.2", cpe="cpe:2.3:a:openssl:openssl:3.1.2:*:*:*:*:*:*:*"),
        ]
        sbom = SBOM(metadata=metadata, packages=packages)
        doc = generate_cyclonedx(sbom)
        pkg = doc["components"][0]
        assert pkg["cpe"] == "cpe:2.3:a:openssl:openssl:3.1.2:*:*:*:*:*:*:*"

    def test_generate_cyclonedx_with_checksums(self):
        metadata = SBOMMetadata(document_name="test-sbom")
        packages = [
            SBOMPackage(name="test", version="1.0.0", checksums={"sha256": "abc123", "sha512": "def456"}),
        ]
        sbom = SBOM(metadata=metadata, packages=packages)
        doc = generate_cyclonedx(sbom)
        pkg = doc["components"][0]
        assert "hashes" in pkg
        assert len(pkg["hashes"]) == 2

    def test_generate_cyclonedx_with_licenses(self):
        metadata = SBOMMetadata(document_name="test-sbom")
        packages = [
            SBOMPackage(name="a", version="1.0.0", license_concluded="MIT"),
            SBOMPackage(name="b", version="2.0.0", license_concluded="Apache-2.0"),
        ]
        sbom = SBOM(metadata=metadata, packages=packages)
        doc = generate_cyclonedx(sbom)
        # Check that licenses are present in components
        for comp in doc["components"]:
            assert "licenses" in comp

    def test_generate_cyclonedx_with_vulns(self):
        metadata = SBOMMetadata(document_name="test-sbom")
        packages = [
            SBOMPackage(name="vuln-pkg", version="1.0.0", has_vulnerabilities=True, critical_vulns=2, high_vulns=1),
        ]
        sbom = SBOM(metadata=metadata, packages=packages)
        doc = generate_cyclonedx(sbom)
        pkg = doc["components"][0]
        assert "properties" in pkg
        prop_names = [p["name"] for p in pkg["properties"]]
        assert "sin:security:vulnerability_count" in prop_names
        assert "sin:security:critical_vulns" in prop_names
        assert "sin:security:high_vulns" in prop_names

    def test_generate_cyclonedx_with_dependencies(self):
        metadata = SBOMMetadata(document_name="test-sbom")
        packages = [
            SBOMPackage(name="parent", version="1.0.0", purl="pkg:npm/parent@1.0.0", dependencies=["child"]),
        ]
        sbom = SBOM(metadata=metadata, packages=packages)
        doc = generate_cyclonedx(sbom)
        assert len(doc["dependencies"]) == 1
        assert doc["dependencies"][0]["ref"] == "pkg:npm/parent@1.0.0"
        assert "child" in doc["dependencies"][0]["dependsOn"]

    def test_cyclonedx_to_json(self):
        metadata = SBOMMetadata(document_name="test-sbom")
        sbom = SBOM(metadata=metadata, packages=[])
        json_str = cyclonedx_to_json(sbom)
        data = json.loads(json_str)
        assert data["bomFormat"] == "CycloneDX"
