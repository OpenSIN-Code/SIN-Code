package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/OpenSIN-Code/SIN-Code-SBOM-Generator-Go/internal/cyclonedx"
	"github.com/OpenSIN-Code/SIN-Code-SBOM-Generator-Go/internal/spdx"
	"github.com/OpenSIN-Code/SIN-Code-SBOM-Generator-Go/pkg/models"
)

// SBOMGenerator generates SBOMs from security scan results.
type SBOMGenerator struct {
	ToolName    string
	ToolVersion string
}

// New creates a new SBOM generator.
func New(toolName, toolVersion string) *SBOMGenerator {
	if toolName == "" {
		toolName = "SIN-Code-SBOM-Generator"
	}
	if toolVersion == "" {
		toolVersion = "1.0.0"
	}
	return &SBOMGenerator{
		ToolName:    toolName,
		ToolVersion: toolVersion,
	}
}

// GenerateFromSCAResults generates SBOM from SCA scan results.
func (g *SBOMGenerator) GenerateFromSCAResults(scaResults map[string]interface{}, documentName string) *models.SBOM {
	if documentName == "" {
		documentName = "sbom"
	}

	metadata := models.SBOMMetadata{
		ToolName:          g.ToolName,
		ToolVersion:       g.ToolVersion,
		Authors:           []string{"OpenSIN-Code"},
		Timestamp:         time.Now().UTC().Format(time.RFC3339) + "Z",
		DocumentName:      documentName,
		DocumentNamespace: fmt.Sprintf("https://opensin-code.org/sbom/%s", uuid.New().String()),
	}

	var packages []models.SBOMPackage

	// Extract packages from SCA results
	rawPackages := extractPackages(scaResults)
	for _, rawPkg := range rawPackages {
		if pkg := parsePackage(rawPkg); pkg != nil {
			packages = append(packages, *pkg)
		}
	}

	// Annotate vulnerabilities
	vulns := extractVulnerabilities(scaResults)
	annotateVulnerabilities(packages, vulns)

	// Detect source type
	filesScanned := extractFilesScanned(scaResults)
	sourceType := detectSourceType(filesScanned)

	// Collect unique licenses
	licenses := collectUniqueLicenses(packages)

	totalDeps := 0
	for _, p := range packages {
		totalDeps += len(p.Dependencies)
	}

	return &models.SBOM{
		Metadata:          metadata,
		Packages:          packages,
		TotalPackages:     len(packages),
		TotalDependencies: totalDeps,
		UniqueLicenses:    licenses,
		SourceType:        sourceType,
		SourceFiles:       filesScanned,
	}
}

// GenerateFromRawDependencies generates SBOM from a raw list of dependencies.
func (g *SBOMGenerator) GenerateFromRawDependencies(deps []map[string]interface{}, documentName string) *models.SBOM {
	if documentName == "" {
		documentName = "sbom"
	}

	metadata := models.SBOMMetadata{
		ToolName:          g.ToolName,
		ToolVersion:       g.ToolVersion,
		Authors:           []string{"OpenSIN-Code"},
		Timestamp:         time.Now().UTC().Format(time.RFC3339) + "Z",
		DocumentName:      documentName,
		DocumentNamespace: fmt.Sprintf("https://opensin-code.org/sbom/%s", uuid.New().String()),
	}

	var packages []models.SBOMPackage
	for _, dep := range deps {
		if pkg := parsePackage(dep); pkg != nil {
			packages = append(packages, *pkg)
		}
	}

	licenses := collectUniqueLicenses(packages)

	totalDeps := 0
	for _, p := range packages {
		totalDeps += len(p.Dependencies)
	}

	return &models.SBOM{
		Metadata:          metadata,
		Packages:          packages,
		TotalPackages:     len(packages),
		TotalDependencies: totalDeps,
		UniqueLicenses:    licenses,
	}
}

// ExportSPDX exports SBOM as SPDX JSON.
func (g *SBOMGenerator) ExportSPDX(sbom *models.SBOM, outputPath string) string {
	jsonStr := spdx.ToJSON(*sbom)
	if outputPath != "" {
		os.WriteFile(outputPath, []byte(jsonStr), 0644)
	}
	return jsonStr
}

// ExportCycloneDX exports SBOM as CycloneDX JSON.
func (g *SBOMGenerator) ExportCycloneDX(sbom *models.SBOM, outputPath string) string {
	jsonStr := cyclonedx.ToJSON(*sbom)
	if outputPath != "" {
		os.WriteFile(outputPath, []byte(jsonStr), 0644)
	}
	return jsonStr
}

// ExportSummary generates a human-readable summary of the SBOM.
func (g *SBOMGenerator) ExportSummary(sbom *models.SBOM) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# SBOM Summary: %s\n\n", sbom.Metadata.DocumentName))
	sb.WriteString(fmt.Sprintf("- **Tool**: %s v%s\n", sbom.Metadata.ToolName, sbom.Metadata.ToolVersion))
	sb.WriteString(fmt.Sprintf("- **Timestamp**: %s\n", sbom.Metadata.Timestamp))
	sb.WriteString(fmt.Sprintf("- **Total Packages**: %d\n", sbom.TotalPackages))
	sb.WriteString(fmt.Sprintf("- **Total Dependencies**: %d\n", sbom.TotalDependencies))
	sb.WriteString(fmt.Sprintf("- **Unique Licenses**: %d\n", len(sbom.UniqueLicenses)))
	sb.WriteString(fmt.Sprintf("- **Source Type**: %s\n\n", sbom.SourceType))

	sb.WriteString("## Packages\n\n")
	sb.WriteString("| Name | Version | Type | License | Vulnerabilities |\n")
	sb.WriteString("|------|---------|------|---------|------------------|\n")

	for _, pkg := range sbom.Packages {
		vulnStr := "0"
		if pkg.HasVulnerabilities {
			vulnStr = fmt.Sprintf("%d (C:%d H:%d M:%d)", pkg.VulnerabilityCount, pkg.CriticalVulns, pkg.HighVulns, pkg.MediumVulns)
		}
		license := pkg.LicenseConcluded
		if license == "" {
			license = "-"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n", pkg.Name, pkg.Version, pkg.Type, license, vulnStr))
	}

	if len(sbom.UniqueLicenses) > 0 {
		sb.WriteString("\n## Licenses\n\n")
		for _, lic := range sbom.UniqueLicenses {
			sb.WriteString(fmt.Sprintf("- %s\n", lic))
		}
	}

	return sb.String()
}

// extractPackages extracts packages from SCA results map.
func extractPackages(scaResults map[string]interface{}) []map[string]interface{} {
	if pkgs, ok := scaResults["packages"].([]interface{}); ok {
		var result []map[string]interface{}
		for _, p := range pkgs {
			if m, ok := p.(map[string]interface{}); ok {
				result = append(result, m)
			}
		}
		return result
	}
	if deps, ok := scaResults["dependencies"].([]interface{}); ok {
		var result []map[string]interface{}
		for _, d := range deps {
			if m, ok := d.(map[string]interface{}); ok {
				result = append(result, m)
			}
		}
		return result
	}
	return nil
}

// extractVulnerabilities extracts vulnerabilities from SCA results map.
func extractVulnerabilities(scaResults map[string]interface{}) []map[string]interface{} {
	if vulns, ok := scaResults["vulnerabilities"].([]interface{}); ok {
		var result []map[string]interface{}
		for _, v := range vulns {
			if m, ok := v.(map[string]interface{}); ok {
				result = append(result, m)
			}
		}
		return result
	}
	return nil
}

// extractFilesScanned extracts scanned file names from SCA results.
func extractFilesScanned(scaResults map[string]interface{}) []string {
	if files, ok := scaResults["files_scanned"].([]interface{}); ok {
		var result []string
		for _, f := range files {
			if s, ok := f.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// parsePackage parses a raw dependency map into an SBOMPackage.
func parsePackage(raw map[string]interface{}) *models.SBOMPackage {
	if raw == nil || len(raw) == 0 {
		return nil
	}

	name := getString(raw, "name")
	if name == "" {
		name = getString(raw, "package")
	}
	if name == "" {
		name = "unknown"
	}

	version := getString(raw, "version")
	if version == "" {
		version = getString(raw, "current_version")
	}
	if version == "" {
		version = "unknown"
	}

	pkg := &models.SBOMPackage{
		Name:             name,
		Version:          version,
		Type:             getString(raw, "type"),
		PURL:             getString(raw, "purl"),
		LicenseConcluded: getString(raw, "license"),
		LicenseDeclared:  getString(raw, "license"),
		Description:      getString(raw, "description"),
		Homepage:         getString(raw, "homepage"),
		SourceRepo:       getString(raw, "source_repo"),
	}

	if pkg.Type == "" {
		pkg.Type = "library"
	}

	// Dependencies
	if deps, ok := raw["dependencies"].([]interface{}); ok {
		for _, d := range deps {
			if s, ok := d.(string); ok {
				pkg.Dependencies = append(pkg.Dependencies, s)
			}
		}
	}

	// Security annotations
	if _, ok := raw["vulnerability_count"]; ok {
		pkg.HasVulnerabilities = true
		pkg.VulnerabilityCount = getInt(raw, "vulnerability_count")
		pkg.CriticalVulns = getInt(raw, "critical")
		pkg.HighVulns = getInt(raw, "high")
		pkg.MediumVulns = getInt(raw, "medium")
		pkg.LowVulns = getInt(raw, "low")
	}

	return pkg
}

// annotateVulnerabilities maps vulnerability list to packages by name.
func annotateVulnerabilities(packages []models.SBOMPackage, vulns []map[string]interface{}) {
	pkgMap := make(map[string]*models.SBOMPackage)
	for i := range packages {
		pkgMap[packages[i].Name] = &packages[i]
	}

	for _, vuln := range vulns {
		pkgName := getString(vuln, "package")
		if pkgName == "" {
			pkgName = getString(vuln, "pkg_name")
		}
		if pkg, ok := pkgMap[pkgName]; ok {
			pkg.HasVulnerabilities = true
			pkg.VulnerabilityCount++
			severity := strings.ToLower(getString(vuln, "severity"))
			switch severity {
			case "critical":
				pkg.CriticalVulns++
			case "high":
				pkg.HighVulns++
			case "medium":
				pkg.MediumVulns++
			case "low":
				pkg.LowVulns++
			}
		}
	}
}

// detectSourceType detects package manager type from scanned file names.
func detectSourceType(files []string) string {
	typeMap := map[string]string{
		"package.json":        "npm",
		"package-lock.json":   "npm",
		"yarn.lock":           "yarn",
		"requirements.txt":    "pypi",
		"poetry.lock":         "poetry",
		"Pipfile.lock":        "pipenv",
		"go.mod":              "go",
		"go.sum":              "go",
		"pom.xml":             "maven",
		"build.gradle":        "gradle",
		"Cargo.toml":          "cargo",
		"Cargo.lock":          "cargo",
		"Gemfile":             "rubygems",
		"Gemfile.lock":        "rubygems",
		"composer.json":       "composer",
		"composer.lock":       "composer",
	}

	for _, filename := range files {
		base := filepath.Base(filename)
		if t, ok := typeMap[base]; ok {
			return t
		}
	}
	return ""
}

// collectUniqueLicenses collects unique licenses from packages.
func collectUniqueLicenses(packages []models.SBOMPackage) []string {
	seen := make(map[string]bool)
	var result []string
	for _, p := range packages {
		if p.LicenseConcluded != "" && !seen[p.LicenseConcluded] {
			seen[p.LicenseConcluded] = true
			result = append(result, p.LicenseConcluded)
		}
	}
	return result
}

// getString extracts a string value from a map.
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// getInt extracts an int value from a map.
func getInt(m map[string]interface{}, key string) int {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case float64:
			return int(val)
		case string:
			var i int
			fmt.Sscanf(val, "%d", &i)
			return i
		}
	}
	return 0
}
