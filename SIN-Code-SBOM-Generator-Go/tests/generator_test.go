package generator_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code-SBOM-Generator-Go/internal/cyclonedx"
	"github.com/OpenSIN-Code/SIN-Code-SBOM-Generator-Go/internal/generator"
	"github.com/OpenSIN-Code/SIN-Code-SBOM-Generator-Go/internal/spdx"
	"github.com/OpenSIN-Code/SIN-Code-SBOM-Generator-Go/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── SPDX Tests ──────────────────────────────────────

func TestSPDXGenerateBasic(t *testing.T) {
	metadata := models.DefaultMetadata()
	metadata.DocumentName = "test-sbom"
	packages := []models.SBOMPackage{
		{Name: "lodash", Version: "4.17.21", LicenseConcluded: "MIT", Type: "library"},
	}
	sbom := models.SBOM{Metadata: metadata, Packages: packages}

	doc := spdx.Generate(sbom)
	assert.Equal(t, "SPDX-2.3", doc["spdxVersion"])
	assert.Equal(t, "CC0-1.0", doc["dataLicense"])
	assert.Equal(t, "SPDXRef-DOCUMENT", doc["SPDXID"])
	assert.Equal(t, "test-sbom", doc["name"])
	assert.Contains(t, doc, "documentNamespace")

	packagesList := doc["packages"].([]interface{})
	assert.Len(t, packagesList, 1)
	pkg := packagesList[0].(map[string]interface{})
	assert.Equal(t, "lodash", pkg["name"])
	assert.Equal(t, "4.17.21", pkg["versionInfo"])
	assert.Equal(t, "MIT", pkg["licenseConcluded"])
}

func TestSPDXGenerateWithPURL(t *testing.T) {
	metadata := models.DefaultMetadata()
	metadata.DocumentName = "test-sbom"
	packages := []models.SBOMPackage{
		{Name: "requests", Version: "2.31.0", LicenseConcluded: "Apache-2.0", PURL: "pkg:pypi/requests@2.31.0", Type: "library"},
	}
	sbom := models.SBOM{Metadata: metadata, Packages: packages}

	doc := spdx.Generate(sbom)
	pkg := doc["packages"].([]interface{})[0].(map[string]interface{})
	assert.Contains(t, pkg, "externalRefs")
	refs := pkg["externalRefs"].([]interface{})
	assert.Len(t, refs, 1)
	ref := refs[0].(map[string]interface{})
	assert.Equal(t, "purl", ref["referenceType"])
	assert.Equal(t, "pkg:pypi/requests@2.31.0", ref["referenceLocator"])
}

func TestSPDXGenerateWithCPE(t *testing.T) {
	metadata := models.DefaultMetadata()
	metadata.DocumentName = "test-sbom"
	packages := []models.SBOMPackage{
		{Name: "openssl", Version: "3.1.2", LicenseConcluded: "Apache-2.0", CPE: "cpe:2.3:a:openssl:openssl:3.1.2:*:*:*:*:*:*:*", Type: "library"},
	}
	sbom := models.SBOM{Metadata: metadata, Packages: packages}

	doc := spdx.Generate(sbom)
	pkg := doc["packages"].([]interface{})[0].(map[string]interface{})
	assert.Contains(t, pkg, "externalRefs")
	refs := pkg["externalRefs"].([]interface{})
	assert.True(t, hasRefType(refs, "cpe23Type"))
}

func TestSPDXGenerateWithChecksums(t *testing.T) {
	metadata := models.DefaultMetadata()
	metadata.DocumentName = "test-sbom"
	packages := []models.SBOMPackage{
		{Name: "test", Version: "1.0.0", Checksums: map[string]string{"sha256": "abc123"}, Type: "library"},
	}
	sbom := models.SBOM{Metadata: metadata, Packages: packages}

	doc := spdx.Generate(sbom)
	pkg := doc["packages"].([]interface{})[0].(map[string]interface{})
	assert.Contains(t, pkg, "checksums")
	checksums := pkg["checksums"].([]interface{})
	assert.Len(t, checksums, 1)
	cs := checksums[0].(map[string]interface{})
	assert.Equal(t, "SHA256", cs["algorithm"])
	assert.Equal(t, "abc123", cs["checksumValue"])
}

func TestSPDXGenerateWithDependencies(t *testing.T) {
	metadata := models.DefaultMetadata()
	metadata.DocumentName = "test-sbom"
	packages := []models.SBOMPackage{
		{Name: "parent", Version: "1.0.0", Dependencies: []string{"child"}, Type: "library"},
		{Name: "child", Version: "2.0.0", Type: "library"},
	}
	sbom := models.SBOM{Metadata: metadata, Packages: packages}

	doc := spdx.Generate(sbom)
	relationships := doc["relationships"].([]interface{})
	depRels := filterRelationships(relationships, "DEPENDS_ON")
	assert.Len(t, depRels, 1)
	rel := depRels[0].(map[string]interface{})
	assert.Equal(t, "SPDXRef-Package-0", rel["spdxElementId"])
	assert.Equal(t, "SPDXRef-Package-1", rel["relatedSpdxElement"])
}

func TestSPDXGenerateExtractedLicenses(t *testing.T) {
	metadata := models.DefaultMetadata()
	metadata.DocumentName = "test-sbom"
	packages := []models.SBOMPackage{
		{Name: "a", Version: "1.0.0", LicenseConcluded: "MIT", Type: "library"},
		{Name: "b", Version: "2.0.0", LicenseConcluded: "Apache-2.0", Type: "library"},
	}
	sbom := models.SBOM{Metadata: metadata, Packages: packages}

	doc := spdx.Generate(sbom)
	assert.Contains(t, doc, "hasExtractedLicensingInfos")
	licenses := doc["hasExtractedLicensingInfos"].([]interface{})
	assert.Len(t, licenses, 2)
}

func TestSPDXToJSON(t *testing.T) {
	metadata := models.DefaultMetadata()
	metadata.DocumentName = "test-sbom"
	sbom := models.SBOM{Metadata: metadata, Packages: []models.SBOMPackage{}}

	jsonStr := spdx.ToJSON(sbom)
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &data))
	assert.Equal(t, "SPDX-2.3", data["spdxVersion"])
	assert.Equal(t, "SPDXRef-DOCUMENT", data["SPDXID"])
}

// ── CycloneDX Tests ──────────────────────────────────────

func TestCycloneDXGenerateBasic(t *testing.T) {
	metadata := models.DefaultMetadata()
	metadata.DocumentName = "test-sbom"
	packages := []models.SBOMPackage{
		{Name: "lodash", Version: "4.17.21", LicenseConcluded: "MIT", Type: "library"},
	}
	sbom := models.SBOM{Metadata: metadata, Packages: packages}

	doc := cyclonedx.Generate(sbom)
	assert.Equal(t, "CycloneDX", doc["bomFormat"])
	assert.Equal(t, "1.5", doc["specVersion"])
	assert.Equal(t, 1, doc["version"])
	assert.Contains(t, doc, "serialNumber")

	components := doc["components"].([]interface{})
	assert.Len(t, components, 1)
	comp := components[0].(map[string]interface{})
	assert.Equal(t, "library", comp["type"])
	assert.Equal(t, "lodash", comp["name"])
	assert.Equal(t, "4.17.21", comp["version"])
}

func TestCycloneDXGenerateWithPURL(t *testing.T) {
	metadata := models.DefaultMetadata()
	metadata.DocumentName = "test-sbom"
	packages := []models.SBOMPackage{
		{Name: "requests", Version: "2.31.0", PURL: "pkg:pypi/requests@2.31.0", Type: "library"},
	}
	sbom := models.SBOM{Metadata: metadata, Packages: packages}

	doc := cyclonedx.Generate(sbom)
	comp := doc["components"].([]interface{})[0].(map[string]interface{})
	assert.Equal(t, "pkg:pypi/requests@2.31.0", comp["purl"])
	assert.Equal(t, "pkg:pypi/requests@2.31.0", comp["bom-ref"])
}

func TestCycloneDXGenerateWithCPE(t *testing.T) {
	metadata := models.DefaultMetadata()
	metadata.DocumentName = "test-sbom"
	packages := []models.SBOMPackage{
		{Name: "openssl", Version: "3.1.2", CPE: "cpe:2.3:a:openssl:openssl:3.1.2:*:*:*:*:*:*:*", Type: "library"},
	}
	sbom := models.SBOM{Metadata: metadata, Packages: packages}

	doc := cyclonedx.Generate(sbom)
	comp := doc["components"].([]interface{})[0].(map[string]interface{})
	assert.Equal(t, "cpe:2.3:a:openssl:openssl:3.1.2:*:*:*:*:*:*:*", comp["cpe"])
}

func TestCycloneDXGenerateWithChecksums(t *testing.T) {
	metadata := models.DefaultMetadata()
	metadata.DocumentName = "test-sbom"
	packages := []models.SBOMPackage{
		{Name: "test", Version: "1.0.0", Checksums: map[string]string{"sha256": "abc123", "sha512": "def456"}, Type: "library"},
	}
	sbom := models.SBOM{Metadata: metadata, Packages: packages}

	doc := cyclonedx.Generate(sbom)
	comp := doc["components"].([]interface{})[0].(map[string]interface{})
	assert.Contains(t, comp, "hashes")
	hashes := comp["hashes"].([]interface{})
	assert.Len(t, hashes, 2)
}

func TestCycloneDXGenerateWithLicenses(t *testing.T) {
	metadata := models.DefaultMetadata()
	metadata.DocumentName = "test-sbom"
	packages := []models.SBOMPackage{
		{Name: "a", Version: "1.0.0", LicenseConcluded: "MIT", Type: "library"},
		{Name: "b", Version: "2.0.0", LicenseConcluded: "Apache-2.0", Type: "library"},
	}
	sbom := models.SBOM{Metadata: metadata, Packages: packages}

	doc := cyclonedx.Generate(sbom)
	components := doc["components"].([]interface{})
	for _, c := range components {
		comp := c.(map[string]interface{})
		assert.Contains(t, comp, "licenses")
	}
}

func TestCycloneDXGenerateWithVulns(t *testing.T) {
	metadata := models.DefaultMetadata()
	metadata.DocumentName = "test-sbom"
	packages := []models.SBOMPackage{
		{Name: "vuln-pkg", Version: "1.0.0", HasVulnerabilities: true, VulnerabilityCount: 3, CriticalVulns: 2, HighVulns: 1, Type: "library"},
	}
	sbom := models.SBOM{Metadata: metadata, Packages: packages}

	doc := cyclonedx.Generate(sbom)
	comp := doc["components"].([]interface{})[0].(map[string]interface{})
	assert.Contains(t, comp, "properties")
	props := comp["properties"].([]interface{})
	propNames := extractPropertyNames(props)
	assert.Contains(t, propNames, "sin:security:vulnerability_count")
	assert.Contains(t, propNames, "sin:security:critical_vulns")
	assert.Contains(t, propNames, "sin:security:high_vulns")
}

func TestCycloneDXGenerateWithDependencies(t *testing.T) {
	metadata := models.DefaultMetadata()
	metadata.DocumentName = "test-sbom"
	packages := []models.SBOMPackage{
		{Name: "parent", Version: "1.0.0", PURL: "pkg:npm/parent@1.0.0", Dependencies: []string{"child"}, Type: "library"},
	}
	sbom := models.SBOM{Metadata: metadata, Packages: packages}

	doc := cyclonedx.Generate(sbom)
	deps := doc["dependencies"].([]interface{})
	assert.Len(t, deps, 1)
	dep := deps[0].(map[string]interface{})
	assert.Equal(t, "pkg:npm/parent@1.0.0", dep["ref"])
	assert.Contains(t, dep["dependsOn"], "child")
}

func TestCycloneDXToJSON(t *testing.T) {
	metadata := models.DefaultMetadata()
	metadata.DocumentName = "test-sbom"
	sbom := models.SBOM{Metadata: metadata, Packages: []models.SBOMPackage{}}

	jsonStr := cyclonedx.ToJSON(sbom)
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &data))
	assert.Equal(t, "CycloneDX", data["bomFormat"])
	assert.Equal(t, "1.5", data["specVersion"])
}

// ── Generator Tests ──────────────────────────────────────

func TestGeneratorInit(t *testing.T) {
	gen := generator.New("", "")
	assert.Equal(t, "SIN-Code-SBOM-Generator", gen.ToolName)
	assert.Equal(t, "1.0.0", gen.ToolVersion)
}

func TestGenerateFromSCAResultsBasic(t *testing.T) {
	gen := generator.New("", "")
	scaData := map[string]interface{}{
		"packages": []interface{}{
			map[string]interface{}{"name": "lodash", "version": "4.17.21", "license": "MIT", "type": "library"},
			map[string]interface{}{"name": "express", "version": "4.18.2", "license": "MIT", "type": "library"},
		},
		"files_scanned": []interface{}{"package.json"},
	}

	sbom := gen.GenerateFromSCAResults(scaData, "test-sbom")
	assert.Equal(t, "test-sbom", sbom.Metadata.DocumentName)
	assert.Equal(t, 2, sbom.TotalPackages)
	assert.Equal(t, "npm", sbom.SourceType)
	assert.Len(t, sbom.Packages, 2)
}

func TestGenerateFromSCAResultsWithVulns(t *testing.T) {
	gen := generator.New("", "")
	scaData := map[string]interface{}{
		"packages": []interface{}{
			map[string]interface{}{"name": "lodash", "version": "4.17.21", "license": "MIT", "type": "library"},
		},
		"vulnerabilities": []interface{}{
			map[string]interface{}{"package": "lodash", "severity": "high", "cve": "CVE-2021-23337"},
			map[string]interface{}{"package": "lodash", "severity": "critical", "cve": "CVE-2021-23337"},
		},
	}

	sbom := gen.GenerateFromSCAResults(scaData, "")
	assert.Equal(t, 1, sbom.TotalPackages)
	pkg := sbom.Packages[0]
	assert.True(t, pkg.HasVulnerabilities)
	assert.Equal(t, 2, pkg.VulnerabilityCount)
	assert.Equal(t, 1, pkg.HighVulns)
	assert.Equal(t, 1, pkg.CriticalVulns)
}

func TestGenerateFromRawDependencies(t *testing.T) {
	gen := generator.New("", "")
	deps := []map[string]interface{}{
		{"name": "requests", "version": "2.31.0", "license": "Apache-2.0", "purl": "pkg:pypi/requests@2.31.0"},
		{"name": "urllib3", "version": "2.0.7", "license": "MIT"},
	}

	sbom := gen.GenerateFromRawDependencies(deps, "python-deps")
	assert.Equal(t, 2, sbom.TotalPackages)
	assert.Equal(t, "pkg:pypi/requests@2.31.0", sbom.Packages[0].PURL)
	assert.Contains(t, sbom.UniqueLicenses, "Apache-2.0")
}

func TestExportSPDX(t *testing.T) {
	gen := generator.New("", "")
	sbom := createTestSBOM()

	jsonStr := gen.ExportSPDX(sbom, "")
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &data))
	assert.Equal(t, "SPDX-2.3", data["spdxVersion"])
	assert.Equal(t, "SPDXRef-DOCUMENT", data["SPDXID"])
	assert.Equal(t, "test-sbom", data["name"])
	assert.Len(t, data["packages"], 2)
}

func TestExportSPDXToFile(t *testing.T) {
	gen := generator.New("", "")
	sbom := createTestSBOM()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.spdx.json")
	gen.ExportSPDX(sbom, path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.Equal(t, "SPDX-2.3", doc["spdxVersion"])
}

func TestExportCycloneDX(t *testing.T) {
	gen := generator.New("", "")
	sbom := createTestSBOM()

	jsonStr := gen.ExportCycloneDX(sbom, "")
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &data))
	assert.Equal(t, "CycloneDX", data["bomFormat"])
	assert.Equal(t, "1.5", data["specVersion"])
	assert.Len(t, data["components"], 2)
	assert.Equal(t, "lodash", data["components"].([]interface{})[0].(map[string]interface{})["name"])
	assert.Equal(t, "library", data["components"].([]interface{})[0].(map[string]interface{})["type"])
}

func TestExportCycloneDXToFile(t *testing.T) {
	gen := generator.New("", "")
	sbom := createTestSBOM()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.cyclonedx.json")
	gen.ExportCycloneDX(sbom, path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.Equal(t, "CycloneDX", doc["bomFormat"])
}

func TestExportSummary(t *testing.T) {
	gen := generator.New("", "")
	sbom := createTestSBOM()

	summary := gen.ExportSummary(sbom)
	assert.Contains(t, summary, "SBOM Summary: test-sbom")
	assert.Contains(t, summary, "lodash")
	assert.Contains(t, summary, "express")
	assert.Contains(t, summary, "MIT")
}

func TestDetectSourceType(t *testing.T) {
	gen := generator.New("", "")
	scaData := map[string]interface{}{
		"files_scanned": []interface{}{"package.json"},
	}
	sbom := gen.GenerateFromSCAResults(scaData, "")
	assert.Equal(t, "npm", sbom.SourceType)

	scaData2 := map[string]interface{}{
		"files_scanned": []interface{}{"requirements.txt"},
	}
	sbom2 := gen.GenerateFromSCAResults(scaData2, "")
	assert.Equal(t, "pypi", sbom2.SourceType)

	scaData3 := map[string]interface{}{
		"files_scanned": []interface{}{"go.mod"},
	}
	sbom3 := gen.GenerateFromSCAResults(scaData3, "")
	assert.Equal(t, "go", sbom3.SourceType)

	scaData4 := map[string]interface{}{
		"files_scanned": []interface{}{},
	}
	sbom4 := gen.GenerateFromSCAResults(scaData4, "")
	assert.Equal(t, "", sbom4.SourceType)
}

func TestParsePackage(t *testing.T) {
	gen := generator.New("", "")
	scaData := map[string]interface{}{
		"packages": []interface{}{
			map[string]interface{}{
				"name":         "test-pkg",
				"version":      "1.0.0",
				"license":      "MIT",
				"type":         "library",
				"purl":         "pkg:npm/test-pkg@1.0.0",
				"dependencies": []interface{}{"dep1", "dep2"},
			},
		},
	}

	sbom := gen.GenerateFromSCAResults(scaData, "")
	require.Len(t, sbom.Packages, 1)
	pkg := sbom.Packages[0]
	assert.Equal(t, "test-pkg", pkg.Name)
	assert.Equal(t, "1.0.0", pkg.Version)
	assert.Equal(t, "MIT", pkg.LicenseConcluded)
	assert.Equal(t, "pkg:npm/test-pkg@1.0.0", pkg.PURL)
	assert.Equal(t, []string{"dep1", "dep2"}, pkg.Dependencies)
}

func TestParsePackageNone(t *testing.T) {
	gen := generator.New("", "")
	scaData := map[string]interface{}{
		"packages": []interface{}{
			map[string]interface{}{},
		},
	}

	sbom := gen.GenerateFromSCAResults(scaData, "")
	assert.Equal(t, 0, sbom.TotalPackages)
}

func TestAnnotateVulnerabilities(t *testing.T) {
	gen := generator.New("", "")
	scaData := map[string]interface{}{
		"packages": []interface{}{
			map[string]interface{}{"name": "lodash", "version": "1.0.0"},
		},
		"vulnerabilities": []interface{}{
			map[string]interface{}{"package": "lodash", "severity": "critical"},
			map[string]interface{}{"package": "lodash", "severity": "high"},
			map[string]interface{}{"package": "lodash", "severity": "medium"},
		},
	}

	sbom := gen.GenerateFromSCAResults(scaData, "")
	require.Len(t, sbom.Packages, 1)
	pkg := sbom.Packages[0]
	assert.Equal(t, 1, pkg.CriticalVulns)
	assert.Equal(t, 1, pkg.HighVulns)
	assert.Equal(t, 1, pkg.MediumVulns)
}

// Helper functions

func hasRefType(refs []interface{}, refType string) bool {
	for _, r := range refs {
		ref := r.(map[string]interface{})
		if ref["referenceType"] == refType {
			return true
		}
	}
	return false
}

func filterRelationships(rels []interface{}, relType string) []interface{} {
	var result []interface{}
	for _, r := range rels {
		rel := r.(map[string]interface{})
		if rel["relationshipType"] == relType {
			result = append(result, rel)
		}
	}
	return result
}

func extractPropertyNames(props []interface{}) []string {
	var names []string
	for _, p := range props {
		prop := p.(map[string]interface{})
		if name, ok := prop["name"].(string); ok {
			names = append(names, name)
		}
	}
	return names
}

func createTestSBOM() *models.SBOM {
	metadata := models.DefaultMetadata()
	metadata.DocumentName = "test-sbom"
	return &models.SBOM{
		Metadata:       metadata,
		Packages: []models.SBOMPackage{
			{Name: "lodash", Version: "4.17.21", LicenseConcluded: "MIT", Type: "library"},
			{Name: "express", Version: "4.18.2", LicenseConcluded: "MIT", Type: "library"},
		},
		TotalPackages:     2,
		TotalDependencies: 0,
		UniqueLicenses:    []string{"MIT"},
	}
}
