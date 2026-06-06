"""Tests for SPDX generator.

Docs: test_spdx.doc.md
"""

import json
import pytest

from sbom_generator.spdx_generator import generate_spdx, spdx_to_json
from sbom_generator.models import SBOM, SBOMPackage, SBOMMetadata


class TestSPDXGenerator:

    def test_generate_spdx_basic(self):
        metadata = SBOMMetadata(document_name="test-sbom")
        packages = [
            SBOMPackage(name="lodash", version="4.17.21", license_concluded="MIT"),
        ]
        sbom = SBOM(metadata=metadata, packages=packages)
        doc = generate_spdx(sbom)
        assert doc["spdxVersion"] == "SPDX-2.3"
        assert doc["dataLicense"] == "CC0-1.0"
        assert doc["SPDXID"] == "SPDXRef-DOCUMENT"
        assert doc["name"] == "test-sbom"
        assert "documentNamespace" in doc
        assert len(doc["packages"]) == 1
        assert doc["packages"][0]["name"] == "lodash"
        assert doc["packages"][0]["versionInfo"] == "4.17.21"
        assert doc["packages"][0]["licenseConcluded"] == "MIT"

    def test_generate_spdx_with_purl(self):
        metadata = SBOMMetadata(document_name="test-sbom")
        packages = [
            SBOMPackage(name="requests", version="2.31.0", license_concluded="Apache-2.0", purl="pkg:pypi/requests@2.31.0"),
        ]
        sbom = SBOM(metadata=metadata, packages=packages)
        doc = generate_spdx(sbom)
        pkg = doc["packages"][0]
        assert "externalRefs" in pkg
        assert pkg["externalRefs"][0]["referenceType"] == "purl"
        assert pkg["externalRefs"][0]["referenceLocator"] == "pkg:pypi/requests@2.31.0"

    def test_generate_spdx_with_cpe(self):
        metadata = SBOMMetadata(document_name="test-sbom")
        packages = [
            SBOMPackage(name="openssl", version="3.1.2", license_concluded="Apache-2.0", cpe="cpe:2.3:a:openssl:openssl:3.1.2:*:*:*:*:*:*:*"),
        ]
        sbom = SBOM(metadata=metadata, packages=packages)
        doc = generate_spdx(sbom)
        pkg = doc["packages"][0]
        assert "externalRefs" in pkg
        assert any(ref["referenceType"] == "cpe23Type" for ref in pkg["externalRefs"])

    def test_generate_spdx_with_checksums(self):
        metadata = SBOMMetadata(document_name="test-sbom")
        packages = [
            SBOMPackage(name="test", version="1.0.0", checksums={"sha256": "abc123"}),
        ]
        sbom = SBOM(metadata=metadata, packages=packages)
        doc = generate_spdx(sbom)
        pkg = doc["packages"][0]
        assert "checksums" in pkg
        assert pkg["checksums"][0]["algorithm"] == "SHA256"
        assert pkg["checksums"][0]["checksumValue"] == "abc123"

    def test_generate_spdx_with_dependencies(self):
        metadata = SBOMMetadata(document_name="test-sbom")
        packages = [
            SBOMPackage(name="parent", version="1.0.0", dependencies=["child"]),
            SBOMPackage(name="child", version="2.0.0"),
        ]
        sbom = SBOM(metadata=metadata, packages=packages)
        doc = generate_spdx(sbom)
        relationships = [r for r in doc["relationships"] if r["relationshipType"] == "DEPENDS_ON"]
        assert len(relationships) == 1
        assert relationships[0]["spdxElementId"] == "SPDXRef-Package-0"
        assert relationships[0]["relatedSpdxElement"] == "SPDXRef-Package-1"

    def test_generate_spdx_extracted_licenses(self):
        metadata = SBOMMetadata(document_name="test-sbom")
        packages = [
            SBOMPackage(name="a", version="1.0.0", license_concluded="MIT"),
            SBOMPackage(name="b", version="2.0.0", license_concluded="Apache-2.0"),
        ]
        sbom = SBOM(metadata=metadata, packages=packages)
        doc = generate_spdx(sbom)
        assert "hasExtractedLicensingInfos" in doc
        assert len(doc["hasExtractedLicensingInfos"]) == 2

    def test_spdx_to_json(self):
        metadata = SBOMMetadata(document_name="test-sbom")
        sbom = SBOM(metadata=metadata, packages=[])
        json_str = spdx_to_json(sbom)
        data = json.loads(json_str)
        assert data["spdxVersion"] == "SPDX-2.3"
