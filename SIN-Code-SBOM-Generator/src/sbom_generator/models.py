"""Data models for SBOM generation.

Docs: models.doc.md
"""

from dataclasses import dataclass, field
from typing import Optional, List, Dict, Any
from datetime import datetime
import uuid


@dataclass
class SBOMPackage:
    """Represents a package/component in an SBOM."""
    name: str
    version: str
    type: str = "library"  # library, application, framework, container, etc.
    purl: Optional[str] = None  # Package URL
    cpe: Optional[str] = None  # Common Platform Enumeration
    supplier: Optional[str] = None
    originator: Optional[str] = None
    download_url: Optional[str] = None
    license_concluded: Optional[str] = None
    license_declared: Optional[str] = None
    copyright_text: Optional[str] = None
    checksums: Dict[str, str] = field(default_factory=dict)  # algo -> hash
    description: Optional[str] = None
    homepage: Optional[str] = None
    source_repo: Optional[str] = None
    is_internal: bool = False
    
    # Security metadata
    has_vulnerabilities: bool = False
    vulnerability_count: int = 0
    critical_vulns: int = 0
    high_vulns: int = 0
    medium_vulns: int = 0
    low_vulns: int = 0
    
    # Dependency info
    dependencies: List[str] = field(default_factory=list)  # list of package names


@dataclass
class SBOMMetadata:
    """Metadata for SBOM document."""
    tool_name: str = "SIN-Code-SBOM-Generator"
    tool_version: str = "1.0.0"
    authors: List[str] = field(default_factory=lambda: ["OpenSIN-Code"])
    timestamp: str = field(default_factory=lambda: datetime.utcnow().isoformat() + "Z")
    document_name: str = ""
    document_namespace: str = ""
    
    def __post_init__(self):
        if not self.document_namespace:
            self.document_namespace = f"https://opensin-code.org/sbom/{uuid.uuid4()}"


@dataclass
class SBOM:
    """Complete SBOM representation."""
    metadata: SBOMMetadata
    packages: List[SBOMPackage] = field(default_factory=list)
    
    # Additional properties
    total_packages: int = 0
    total_dependencies: int = 0
    unique_licenses: List[str] = field(default_factory=list)
    files_analyzed: List[str] = field(default_factory=list)
    
    # Relationship to source
    source_type: str = ""  # npm, pypi, maven, go, etc.
    source_files: List[str] = field(default_factory=list)  # e.g., package.json, requirements.txt


@dataclass
class ScanResult:
    """Input from security scanning tools."""
    tool_name: str
    packages: List[Dict[str, Any]] = field(default_factory=list)
    vulnerabilities: List[Dict[str, Any]] = field(default_factory=list)
    raw_output: Optional[str] = None
