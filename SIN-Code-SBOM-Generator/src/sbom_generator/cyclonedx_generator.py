"""CycloneDX 1.5 JSON SBOM generator.

Docs: cyclonedx_generator.doc.md
"""

import json
import uuid
import hashlib
from datetime import datetime, timezone
from typing import Dict, List, Any, Optional
from .models import SBOM, SBOMPackage


CYCLONEDX_SPEC_VERSION = "1.5"
CYCLONEDX_SCHEMA = "http://cyclonedx.org/schema/bom-1.5.schema.json"


def generate_cyclonedx(sbom: SBOM) -> Dict[str, Any]:
    """Generate a CycloneDX 1.5 JSON document from an SBOM model.

    Returns a dict that can be serialized to JSON.
    """
    bom = {
        "bomFormat": "CycloneDX",
        "specVersion": CYCLONEDX_SPEC_VERSION,
        "serialNumber": f"urn:uuid:{uuid_from_namespace(sbom.metadata.document_namespace)}",
        "version": 1,
        "metadata": {
            "timestamp": sbom.metadata.timestamp,
            "tools": [{
                "vendor": "OpenSIN-Code",
                "name": sbom.metadata.tool_name,
                "version": sbom.metadata.tool_version,
            }],
            "authors": [{"name": a} for a in sbom.metadata.authors],
        },
        "components": [],
        "dependencies": [],
    }

    for pkg in sbom.packages:
        component = _package_to_cyclonedx(pkg)
        bom["components"].append(component)

    # Dependencies graph
    for pkg in sbom.packages:
        dep_entry = {
            "ref": pkg.purl or pkg.name,
            "dependsOn": [dep_name for dep_name in pkg.dependencies],
        }
        if dep_entry["dependsOn"]:
            bom["dependencies"].append(dep_entry)

    return bom


def _package_to_cyclonedx(pkg: SBOMPackage) -> Dict[str, Any]:
    """Convert a single SBOMPackage to CycloneDX component dict."""
    component: Dict[str, Any] = {
        "type": _map_type_to_cyclonedx(pkg.type),
        "name": pkg.name,
        "version": pkg.version,
        "bom-ref": pkg.purl or pkg.name,
    }

    if pkg.purl:
        component["purl"] = pkg.purl

    if pkg.cpe:
        component["cpe"] = pkg.cpe

    if pkg.supplier:
        component["supplier"] = {"name": pkg.supplier}

    if pkg.description:
        component["description"] = pkg.description

    if pkg.homepage:
        component["externalReferences"] = [{
            "type": "website",
            "url": pkg.homepage,
        }]

    if pkg.checksums:
        component["hashes"] = [
            {"alg": _map_hash_algo(algo), "content": val}
            for algo, val in pkg.checksums.items()
        ]

    # License info
    if pkg.license_concluded or pkg.license_declared:
        component["licenses"] = []
        if pkg.license_concluded:
            component["licenses"].append({
                "license": {"id": pkg.license_concluded} if _is_spdx_license(pkg.license_concluded) else {"name": pkg.license_concluded}
            })
        if pkg.license_declared and pkg.license_declared != pkg.license_concluded:
            component["licenses"].append({
                "license": {"id": pkg.license_declared} if _is_spdx_license(pkg.license_declared) else {"name": pkg.license_declared}
            })

    # Vulnerability info (if present)
    if pkg.has_vulnerabilities:
        if "properties" not in component:
            component["properties"] = []
        component["properties"].append({
            "name": "sin:security:vulnerability_count",
            "value": str(pkg.vulnerability_count),
        })
        if pkg.critical_vulns > 0:
            component["properties"].append({
                "name": "sin:security:critical_vulns",
                "value": str(pkg.critical_vulns),
            })
        if pkg.high_vulns > 0:
            component["properties"].append({
                "name": "sin:security:high_vulns",
                "value": str(pkg.high_vulns),
            })

    return component


def _map_type_to_cyclonedx(pkg_type: str) -> str:
    """Map internal package type to CycloneDX component type."""
    mapping = {
        "library": "library",
        "application": "application",
        "framework": "framework",
        "container": "container",
        "operating-system": "operating-system",
        "device": "device",
        "file": "file",
        "firmware": "firmware",
    }
    return mapping.get(pkg_type, "library")


def _map_hash_algo(algo: str) -> str:
    """Map hash algorithm names to CycloneDX hash algorithm identifiers."""
    mapping = {
        "md5": "MD5",
        "sha1": "SHA-1",
        "sha256": "SHA-256",
        "sha384": "SHA-384",
        "sha512": "SHA-512",
        "sha3-256": "SHA3-256",
        "sha3-384": "SHA3-384",
        "sha3-512": "SHA3-512",
    }
    return mapping.get(algo.lower(), algo.upper())


def _is_spdx_license(license_id: str) -> bool:
    """Check if a license identifier is a known SPDX license ID."""
    # Simplified check: contains common SPDX licenses or no spaces
    known_spdx = {
        "MIT", "Apache-2.0", "BSD-2-Clause", "BSD-3-Clause", "GPL-2.0-only",
        "GPL-2.0-or-later", "GPL-3.0-only", "GPL-3.0-or-later", "LGPL-2.1-only",
        "LGPL-2.1-or-later", "LGPL-3.0-only", "LGPL-3.0-or-later", "MPL-2.0",
        "ISC", "Unlicense", "CC0-1.0", "EPL-2.0", "EPL-1.0", "BSL-1.0",
    }
    return license_id in known_spdx


def uuid_from_namespace(namespace: str) -> str:
    """Generate a deterministic UUID from a namespace string."""
    import hashlib
    return str(uuid.UUID(hashlib.md5(namespace.encode()).hexdigest()[:32]))


def cyclonedx_to_json(sbom: SBOM) -> str:
    """Generate formatted CycloneDX JSON string."""
    doc = generate_cyclonedx(sbom)
    return json.dumps(doc, indent=2, ensure_ascii=False)
