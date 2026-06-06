"""SPDX 2.3 JSON SBOM generator.

Docs: spdx_generator.doc.md
"""

import json
import uuid
from datetime import datetime, timezone
from typing import Dict, List, Any, Optional
from .models import SBOM, SBOMPackage, SBOMMetadata


SPDX_VERSION = "SPDX-2.3"
SPDX_DATA_LICENSE = "CC0-1.0"


def generate_spdx(sbom: SBOM) -> Dict[str, Any]:
    """Generate an SPDX 2.3 JSON document from an SBOM model.

    Returns a dict that can be serialized to JSON.
    """
    doc = {
        "spdxVersion": SPDX_VERSION,
        "dataLicense": SPDX_DATA_LICENSE,
        "SPDXID": "SPDXRef-DOCUMENT",
        "name": sbom.metadata.document_name or "sin-sbom",
        "documentNamespace": sbom.metadata.document_namespace,
        "creationInfo": {
            "created": sbom.metadata.timestamp,
            "creators": [
                f"Tool: {sbom.metadata.tool_name}-{sbom.metadata.tool_version}",
            ] + [f"Organization: {a}" for a in sbom.metadata.authors],
        },
        "packages": [],
        "files": [],
        "relationships": [],
    }

    # Add document-to-DESCRIBES relationship
    if sbom.packages:
        doc["relationships"].append({
            "spdxElementId": "SPDXRef-DOCUMENT",
            "relatedSpdxElement": "SPDXRef-Package-0",
            "relationshipType": "DESCRIBES",
        })

    for idx, pkg in enumerate(sbom.packages):
        spdx_pkg = _package_to_spdx(pkg, idx)
        doc["packages"].append(spdx_pkg)

        # Dependencies
        for dep_name in pkg.dependencies:
            dep_id = _find_package_id(sbom.packages, dep_name) or f"SPDXRef-Package-{dep_name}"
            doc["relationships"].append({
                "spdxElementId": spdx_pkg["SPDXID"],
                "relatedSpdxElement": dep_id,
                "relationshipType": "DEPENDS_ON",
            })

    # Add unique licenses (optional, kept in extractedLicensingInfo)
    unique_licenses = list(set(filter(None, [p.license_concluded for p in sbom.packages])))
    if unique_licenses:
        doc["hasExtractedLicensingInfos"] = [
            {"licenseId": f"LicenseRef-{i}", "extractedText": lic, "name": lic}
            for i, lic in enumerate(unique_licenses)
        ]

    return doc


def _package_to_spdx(pkg: SBOMPackage, idx: int) -> Dict[str, Any]:
    """Convert a single SBOMPackage to SPDX package dict."""
    spdx_id = f"SPDXRef-Package-{idx}"
    spdx_pkg: Dict[str, Any] = {
        "SPDXID": spdx_id,
        "name": pkg.name,
        "versionInfo": pkg.version,
        "downloadLocation": pkg.download_url or "NOASSERTION",
        "filesAnalyzed": False,
        "licenseConcluded": pkg.license_concluded or "NOASSERTION",
        "licenseDeclared": pkg.license_declared or "NOASSERTION",
        "copyrightText": pkg.copyright_text or "NOASSERTION",
        "supplier": pkg.supplier or "NOASSERTION",
        "originator": pkg.originator or "NOASSERTION",
        "homepage": pkg.homepage or "NOASSERTION",
        "sourceInfo": f"Identified as {pkg.type} package by SIN-Code SBOM Generator",
    }

    if pkg.purl:
        spdx_pkg["externalRefs"] = [{
            "referenceCategory": "PACKAGE-MANAGER",
            "referenceType": "purl",
            "referenceLocator": pkg.purl,
        }]

    if pkg.cpe:
        if "externalRefs" not in spdx_pkg:
            spdx_pkg["externalRefs"] = []
        spdx_pkg["externalRefs"].append({
            "referenceCategory": "SECURITY",
            "referenceType": "cpe23Type",
            "referenceLocator": pkg.cpe,
        })

    if pkg.checksums:
        spdx_pkg["checksums"] = [
            {"algorithm": algo.upper(), "checksumValue": val}
            for algo, val in pkg.checksums.items()
        ]

    return spdx_pkg


def _find_package_id(packages: List[SBOMPackage], name: str) -> Optional[str]:
    """Find SPDXID for a package by name."""
    for idx, pkg in enumerate(packages):
        if pkg.name == name:
            return f"SPDXRef-Package-{idx}"
    return None


def spdx_to_json(sbom: SBOM) -> str:
    """Generate formatted SPDX JSON string."""
    doc = generate_spdx(sbom)
    return json.dumps(doc, indent=2, ensure_ascii=False)
