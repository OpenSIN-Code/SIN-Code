# SPDX-License-Identifier: MIT
"""Main SBOM generation orchestrator.

Aggregates data from SCA scans and generates SBOMs in SPDX and CycloneDX formats.

Docs: generator.doc.md
"""

import json
from typing import List, Dict, Any, Optional
from pathlib import Path
from .models import SBOM, SBOMPackage, SBOMMetadata, ScanResult
from .spdx_generator import spdx_to_json
from .cyclonedx_generator import cyclonedx_to_json


class SBOMGenerator:
    """Generates SBOMs from security scan results."""

    def __init__(self, tool_name: str = "SIN-Code-SBOM-Generator", tool_version: str = "1.0.0"):
        self.tool_name = tool_name
        self.tool_version = tool_version

    def generate_from_sca_results(self, sca_results: Dict[str, Any], document_name: str = "") -> SBOM:
        """Generate SBOM from SCA (Software Composition Analysis) scan results.

        Args:
            sca_results: Dict from SCA scan, typically containing 'packages' or 'dependencies'.
            document_name: Name for the SBOM document.

        Returns:
            SBOM model ready for export.
        """
        metadata = SBOMMetadata(
            tool_name=self.tool_name,
            tool_version=self.tool_version,
            document_name=document_name or "sbom",
        )

        packages: List[SBOMPackage] = []

        # Try to extract packages from SCA results
        raw_packages = sca_results.get("packages", sca_results.get("dependencies", []))
        for raw_pkg in raw_packages:
            pkg = self._parse_package(raw_pkg)
            if pkg:
                packages.append(pkg)

        # Try to map vulnerabilities to packages
        vulns = sca_results.get("vulnerabilities", [])
        self._annotate_vulnerabilities(packages, vulns)

        # Determine source type from files
        source_files = sca_results.get("files_scanned", [])
        source_type = self._detect_source_type(source_files)

        # Collect unique licenses
        licenses = list(set(filter(None, [p.license_concluded for p in packages])))

        return SBOM(
            metadata=metadata,
            packages=packages,
            total_packages=len(packages),
            total_dependencies=sum(len(p.dependencies) for p in packages),
            unique_licenses=licenses,
            source_type=source_type,
            source_files=source_files,
        )

    def generate_from_raw_dependencies(self, deps: List[Dict[str, Any]], document_name: str = "") -> SBOM:
        """Generate SBOM from a raw list of dependencies (e.g., from package.json, requirements.txt).

        Args:
            deps: List of dependency dicts with keys: name, version, license, purl, etc.
            document_name: Name for the SBOM document.

        Returns:
            SBOM model ready for export.
        """
        metadata = SBOMMetadata(
            tool_name=self.tool_name,
            tool_version=self.tool_version,
            document_name=document_name or "sbom",
        )

        packages = [self._parse_package(d) for d in deps if d]

        licenses = list(set(filter(None, [p.license_concluded for p in packages])))

        return SBOM(
            metadata=metadata,
            packages=packages,
            total_packages=len(packages),
            total_dependencies=sum(len(p.dependencies) for p in packages),
            unique_licenses=licenses,
        )

    def export_spdx(self, sbom: SBOM, output_path: Optional[str] = None) -> str:
        """Export SBOM as SPDX JSON.

        Args:
            sbom: SBOM model.
            output_path: Optional file path to write to.

        Returns:
            SPDX JSON string.
        """
        json_str = spdx_to_json(sbom)
        if output_path:
            Path(output_path).write_text(json_str, encoding="utf-8")
        return json_str

    def export_cyclonedx(self, sbom: SBOM, output_path: Optional[str] = None) -> str:
        """Export SBOM as CycloneDX JSON.

        Args:
            sbom: SBOM model.
            output_path: Optional file path to write to.

        Returns:
            CycloneDX JSON string.
        """
        json_str = cyclonedx_to_json(sbom)
        if output_path:
            Path(output_path).write_text(json_str, encoding="utf-8")
        return json_str

    def export_summary(self, sbom: SBOM) -> str:
        """Generate a human-readable summary of the SBOM.

        Returns:
            Markdown-formatted summary string.
        """
        lines = [
            f"# SBOM Summary: {sbom.metadata.document_name}",
            "",
            f"- **Tool**: {sbom.metadata.tool_name} v{sbom.metadata.tool_version}",
            f"- **Timestamp**: {sbom.metadata.timestamp}",
            f"- **Total Packages**: {sbom.total_packages}",
            f"- **Total Dependencies**: {sbom.total_dependencies}",
            f"- **Unique Licenses**: {len(sbom.unique_licenses)}",
            f"- **Source Type**: {sbom.source_type or 'unknown'}",
            "",
            "## Packages",
            "",
            "| Name | Version | Type | License | Vulnerabilities |",
            "|------|---------|------|---------|-----------------|",
        ]
        for pkg in sbom.packages:
            vuln_str = f"{pkg.vulnerability_count} (C:{pkg.critical_vulns} H:{pkg.high_vulns} M:{pkg.medium_vulns})" if pkg.has_vulnerabilities else "0"
            lines.append(f"| {pkg.name} | {pkg.version} | {pkg.type} | {pkg.license_concluded or '-'} | {vuln_str} |")

        if sbom.unique_licenses:
            lines += ["", "## Licenses", ""]
            for lic in sbom.unique_licenses:
                lines.append(f"- {lic}")

        return "\n".join(lines)

    # ── Internal helpers ────────────────────────────────

    def _parse_package(self, raw: Dict[str, Any]) -> Optional[SBOMPackage]:
        """Parse a raw dependency dict into an SBOMPackage."""
        if not raw or not isinstance(raw, dict):
            return None

        name = raw.get("name", raw.get("package", "unknown"))
        version = raw.get("version", raw.get("current_version", "unknown"))

        pkg = SBOMPackage(
            name=name,
            version=version,
            type=raw.get("type", "library"),
            purl=raw.get("purl"),
            license_concluded=raw.get("license", raw.get("spdx_license")),
            license_declared=raw.get("license", raw.get("spdx_license")),
            description=raw.get("description"),
            homepage=raw.get("homepage"),
            source_repo=raw.get("source_repo", raw.get("repository_url")),
            dependencies=raw.get("dependencies", []),
        )

        # Security annotations from raw data
        if "vulnerability_count" in raw:
            pkg.has_vulnerabilities = True
            pkg.vulnerability_count = int(raw.get("vulnerability_count", 0))
            pkg.critical_vulns = int(raw.get("critical", 0))
            pkg.high_vulns = int(raw.get("high", 0))
            pkg.medium_vulns = int(raw.get("medium", 0))
            pkg.low_vulns = int(raw.get("low", 0))

        return pkg

    def _annotate_vulnerabilities(self, packages: List[SBOMPackage], vulns: List[Dict[str, Any]]) -> None:
        """Map vulnerability list to packages by name."""
        pkg_map = {p.name: p for p in packages}
        for vuln in vulns:
            pkg_name = vuln.get("package", vuln.get("pkg_name", ""))
            if pkg_name in pkg_map:
                pkg = pkg_map[pkg_name]
                pkg.has_vulnerabilities = True
                pkg.vulnerability_count += 1
                severity = (vuln.get("severity") or "").lower()
                if severity == "critical":
                    pkg.critical_vulns += 1
                elif severity == "high":
                    pkg.high_vulns += 1
                elif severity == "medium":
                    pkg.medium_vulns += 1
                elif severity == "low":
                    pkg.low_vulns += 1

    def _detect_source_type(self, files: List[str]) -> str:
        """Detect package manager type from scanned file names."""
        type_map = {
            "package.json": "npm",
            "package-lock.json": "npm",
            "yarn.lock": "yarn",
            "requirements.txt": "pypi",
            "poetry.lock": "poetry",
            "Pipfile.lock": "pipenv",
            "go.mod": "go",
            "go.sum": "go",
            "pom.xml": "maven",
            "build.gradle": "gradle",
            "Cargo.toml": "cargo",
            "Cargo.lock": "cargo",
            "Gemfile": "rubygems",
            "Gemfile.lock": "rubygems",
            "composer.json": "composer",
            "composer.lock": "composer",
        }
        for filename in files:
            base = filename.split("/")[-1]
            if base in type_map:
                return type_map[base]
        return ""
