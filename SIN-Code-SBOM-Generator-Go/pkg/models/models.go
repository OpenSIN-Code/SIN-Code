package models

// SBOMPackage represents a package/component in an SBOM.
type SBOMPackage struct {
	Name                string            `json:"name"`
	Version             string            `json:"version"`
	Type                string            `json:"type"` // library, application, framework, container, etc.
	PURL                string            `json:"purl,omitempty"`   // Package URL
	CPE                 string            `json:"cpe,omitempty"`    // Common Platform Enumeration
	Supplier            string            `json:"supplier,omitempty"`
	Originator          string            `json:"originator,omitempty"`
	DownloadURL         string            `json:"download_url,omitempty"`
	LicenseConcluded    string            `json:"license_concluded,omitempty"`
	LicenseDeclared     string            `json:"license_declared,omitempty"`
	CopyrightText       string            `json:"copyright_text,omitempty"`
	Checksums           map[string]string `json:"checksums,omitempty"` // algo -> hash
	Description         string            `json:"description,omitempty"`
	Homepage            string            `json:"homepage,omitempty"`
	SourceRepo          string            `json:"source_repo,omitempty"`
	IsInternal          bool              `json:"is_internal"`

	// Security metadata
	HasVulnerabilities  bool              `json:"has_vulnerabilities"`
	VulnerabilityCount  int               `json:"vulnerability_count"`
	CriticalVulns       int               `json:"critical_vulns"`
	HighVulns           int               `json:"high_vulns"`
	MediumVulns         int               `json:"medium_vulns"`
	LowVulns            int               `json:"low_vulns"`

	// Dependency info
	Dependencies        []string          `json:"dependencies"`
}

// SBOMMetadata represents metadata for an SBOM document.
type SBOMMetadata struct {
	ToolName           string   `json:"tool_name"`
	ToolVersion        string   `json:"tool_version"`
	Authors            []string `json:"authors"`
	Timestamp          string   `json:"timestamp"`
	DocumentName       string   `json:"document_name"`
	DocumentNamespace  string   `json:"document_namespace"`
}

// SBOM represents a complete SBOM.
type SBOM struct {
	Metadata           SBOMMetadata   `json:"metadata"`
	Packages           []SBOMPackage  `json:"packages"`

	// Additional properties
	TotalPackages      int            `json:"total_packages"`
	TotalDependencies  int            `json:"total_dependencies"`
	UniqueLicenses     []string       `json:"unique_licenses"`
	FilesAnalyzed      []string       `json:"files_analyzed"`

	// Relationship to source
	SourceType         string         `json:"source_type"`   // npm, pypi, maven, go, etc.
	SourceFiles        []string       `json:"source_files"`  // e.g., package.json, requirements.txt
}

// DefaultMetadata creates default SBOM metadata.
func DefaultMetadata() SBOMMetadata {
	return SBOMMetadata{
		ToolName:          "SIN-Code-SBOM-Generator",
		ToolVersion:       "1.0.0",
		Authors:           []string{"OpenSIN-Code"},
		DocumentNamespace: "https://opensin-code.org/sbom/",
	}
}
