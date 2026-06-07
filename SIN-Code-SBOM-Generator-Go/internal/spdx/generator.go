// SPDX-License-Identifier: MIT
package spdx

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code-SBOM-Generator-Go/pkg/models"
)

const (
	spdxVersion   = "SPDX-2.3"
	spdxDataLicense = "CC0-1.0"
)

// SPDXDocument represents an SPDX 2.3 JSON document.
type SPDXDocument struct {
	SPDXVersion            string                   `json:"spdxVersion"`
	DataLicense            string                   `json:"dataLicense"`
	SPDXID                 string                   `json:"SPDXID"`
	Name                   string                   `json:"name"`
	DocumentNamespace      string                   `json:"documentNamespace"`
	CreationInfo           SPDXCreationInfo         `json:"creationInfo"`
	Packages               []SPDXPackage            `json:"packages"`
	Files                  []interface{}            `json:"files"`
	Relationships          []SPDXRelationship         `json:"relationships"`
	HasExtractedLicensingInfos []SPDXExtractedLicense `json:"hasExtractedLicensingInfos,omitempty"`
}

type SPDXCreationInfo struct {
	Created  string   `json:"created"`
	Creators []string `json:"creators"`
}

type SPDXPackage struct {
	SPDXID             string            `json:"SPDXID"`
	Name               string            `json:"name"`
	VersionInfo        string            `json:"versionInfo"`
	DownloadLocation   string            `json:"downloadLocation"`
	FilesAnalyzed      bool              `json:"filesAnalyzed"`
	LicenseConcluded   string            `json:"licenseConcluded"`
	LicenseDeclared    string            `json:"licenseDeclared"`
	CopyrightText      string            `json:"copyrightText"`
	Supplier           string            `json:"supplier"`
	Originator         string            `json:"originator"`
	Homepage           string            `json:"homepage"`
	SourceInfo         string            `json:"sourceInfo"`
	ExternalRefs       []SPDXExternalRef   `json:"externalRefs,omitempty"`
	Checksums          []SPDXChecksum      `json:"checksums,omitempty"`
}

type SPDXExternalRef struct {
	ReferenceCategory string `json:"referenceCategory"`
	ReferenceType     string `json:"referenceType"`
	ReferenceLocator  string `json:"referenceLocator"`
}

type SPDXChecksum struct {
	Algorithm     string `json:"algorithm"`
	ChecksumValue string `json:"checksumValue"`
}

type SPDXRelationship struct {
	SPDXElementID       string `json:"spdxElementId"`
	RelatedSPDXElement  string `json:"relatedSpdxElement"`
	RelationshipType    string `json:"relationshipType"`
}

type SPDXExtractedLicense struct {
	LicenseID     string `json:"licenseId"`
	ExtractedText string `json:"extractedText"`
	Name          string `json:"name"`
}

// Generate creates an SPDX 2.3 JSON document from an SBOM model.
func Generate(sbom models.SBOM) map[string]interface{} {
	doc := map[string]interface{}{
		"spdxVersion":       spdxVersion,
		"dataLicense":       spdxDataLicense,
		"SPDXID":            "SPDXRef-DOCUMENT",
		"name":              sbom.Metadata.DocumentName,
		"documentNamespace": sbom.Metadata.DocumentNamespace,
		"creationInfo": map[string]interface{}{
			"created":  sbom.Metadata.Timestamp,
			"creators": append([]string{fmt.Sprintf("Tool: %s-%s", sbom.Metadata.ToolName, sbom.Metadata.ToolVersion)}, formatCreators(sbom.Metadata.Authors)...),
		},
		"packages":      []interface{}{},
		"files":         []interface{}{},
		"relationships": []interface{}{},
	}

	// Add document-to-DESCRIBES relationship
	if len(sbom.Packages) > 0 {
		rels := doc["relationships"].([]interface{})
		rels = append(rels, map[string]interface{}{
			"spdxElementId":       "SPDXRef-DOCUMENT",
			"relatedSpdxElement":  "SPDXRef-Package-0",
			"relationshipType":    "DESCRIBES",
		})
		doc["relationships"] = rels
	}

	uniqueLicenses := make(map[string]bool)

	for idx, pkg := range sbom.Packages {
		spdxPkg := packageToSPDX(pkg, idx)
		packages := doc["packages"].([]interface{})
		packages = append(packages, spdxPkg)
		doc["packages"] = packages

		// Dependencies
		for _, depName := range pkg.Dependencies {
			depID := findPackageID(sbom.Packages, depName)
			if depID == "" {
				depID = fmt.Sprintf("SPDXRef-Package-%s", depName)
			}
			rels := doc["relationships"].([]interface{})
			rels = append(rels, map[string]interface{}{
				"spdxElementId":       spdxPkg["SPDXID"],
				"relatedSpdxElement":  depID,
				"relationshipType":    "DEPENDS_ON",
			})
			doc["relationships"] = rels
		}

		if pkg.LicenseConcluded != "" {
			uniqueLicenses[pkg.LicenseConcluded] = true
		}
	}

	// Add unique licenses
	if len(uniqueLicenses) > 0 {
		licenses := []interface{}{}
		i := 0
		for lic := range uniqueLicenses {
			licenses = append(licenses, map[string]interface{}{
				"licenseId":     fmt.Sprintf("LicenseRef-%d", i),
				"extractedText": lic,
				"name":          lic,
			})
			i++
		}
		doc["hasExtractedLicensingInfos"] = licenses
	}

	return doc
}

// ToJSON generates a formatted SPDX JSON string.
func ToJSON(sbom models.SBOM) string {
	doc := Generate(sbom)
	data, _ := json.MarshalIndent(doc, "", "  ")
	return string(data)
}

func packageToSPDX(pkg models.SBOMPackage, idx int) map[string]interface{} {
	spdxID := fmt.Sprintf("SPDXRef-Package-%d", idx)
	spdxPkg := map[string]interface{}{
		"SPDXID":           spdxID,
		"name":             pkg.Name,
		"versionInfo":      pkg.Version,
		"downloadLocation": orDefault(pkg.DownloadURL, "NOASSERTION"),
		"filesAnalyzed":    false,
		"licenseConcluded": orDefault(pkg.LicenseConcluded, "NOASSERTION"),
		"licenseDeclared":  orDefault(pkg.LicenseDeclared, "NOASSERTION"),
		"copyrightText":    orDefault(pkg.CopyrightText, "NOASSERTION"),
		"supplier":         orDefault(pkg.Supplier, "NOASSERTION"),
		"originator":       orDefault(pkg.Originator, "NOASSERTION"),
		"homepage":         orDefault(pkg.Homepage, "NOASSERTION"),
		"sourceInfo":       fmt.Sprintf("Identified as %s package by SIN-Code SBOM Generator", pkg.Type),
	}

	if pkg.PURL != "" {
		externalRefs := []interface{}{}
		externalRefs = append(externalRefs, map[string]interface{}{
			"referenceCategory": "PACKAGE-MANAGER",
			"referenceType":     "purl",
			"referenceLocator":  pkg.PURL,
		})
		if pkg.CPE != "" {
			externalRefs = append(externalRefs, map[string]interface{}{
				"referenceCategory": "SECURITY",
				"referenceType":     "cpe23Type",
				"referenceLocator":  pkg.CPE,
			})
		}
		spdxPkg["externalRefs"] = externalRefs
	} else if pkg.CPE != "" {
		spdxPkg["externalRefs"] = []interface{}{
			map[string]interface{}{
				"referenceCategory": "SECURITY",
				"referenceType":     "cpe23Type",
				"referenceLocator":  pkg.CPE,
			},
		}
	}

	if len(pkg.Checksums) > 0 {
		checksums := []interface{}{}
		for algo, val := range pkg.Checksums {
			checksums = append(checksums, map[string]interface{}{
				"algorithm":     strings.ToUpper(algo),
				"checksumValue": val,
			})
		}
		spdxPkg["checksums"] = checksums
	}

	return spdxPkg
}

func findPackageID(packages []models.SBOMPackage, name string) string {
	for idx, pkg := range packages {
		if pkg.Name == name {
			return fmt.Sprintf("SPDXRef-Package-%d", idx)
		}
	}
	return ""
}

func formatCreators(authors []string) []string {
	var result []string
	for _, a := range authors {
		result = append(result, fmt.Sprintf("Organization: %s", a))
	}
	return result
}

func orDefault(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}
