// SPDX-License-Identifier: MIT
package cyclonedx

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code-SBOM-Generator-Go/pkg/models"
)

const (
	cycloneDXSpecVersion = "1.5"
	cycloneDXSchema      = "http://cyclonedx.org/schema/bom-1.5.schema.json"
)

// Generate creates a CycloneDX 1.5 JSON document from an SBOM model.
func Generate(sbom models.SBOM) map[string]interface{} {
	serialNumber := generateSerialNumber(sbom.Metadata.DocumentNamespace)

	bom := map[string]interface{}{
		"bomFormat":    "CycloneDX",
		"specVersion":  cycloneDXSpecVersion,
		"serialNumber": serialNumber,
		"version":      1,
		"metadata": map[string]interface{}{
			"timestamp": sbom.Metadata.Timestamp,
			"tools": []interface{}{
				map[string]interface{}{
					"vendor":  "OpenSIN-Code",
					"name":    sbom.Metadata.ToolName,
					"version": sbom.Metadata.ToolVersion,
				},
			},
			"authors": func() []interface{} {
				var authors []interface{}
				for _, a := range sbom.Metadata.Authors {
					authors = append(authors, map[string]interface{}{"name": a})
				}
				return authors
			}(),
		},
		"components":   []interface{}{},
		"dependencies": []interface{}{},
	}

	for _, pkg := range sbom.Packages {
		component := packageToCycloneDX(pkg)
		components := bom["components"].([]interface{})
		components = append(components, component)
		bom["components"] = components
	}

	// Dependencies graph
	for _, pkg := range sbom.Packages {
		if len(pkg.Dependencies) > 0 {
			ref := pkg.PURL
			if ref == "" {
				ref = pkg.Name
			}
			depEntry := map[string]interface{}{
				"ref":       ref,
				"dependsOn": pkg.Dependencies,
			}
			deps := bom["dependencies"].([]interface{})
			deps = append(deps, depEntry)
			bom["dependencies"] = deps
		}
	}

	return bom
}

// ToJSON generates a formatted CycloneDX JSON string.
func ToJSON(sbom models.SBOM) string {
	bom := Generate(sbom)
	data, _ := json.MarshalIndent(bom, "", "  ")
	return string(data)
}

func packageToCycloneDX(pkg models.SBOMPackage) map[string]interface{} {
	component := map[string]interface{}{
		"type":    mapTypeToCycloneDX(pkg.Type),
		"name":    pkg.Name,
		"version": pkg.Version,
		"bom-ref": pkg.PURL,
	}
	if component["bom-ref"] == nil || component["bom-ref"].(string) == "" {
		component["bom-ref"] = pkg.Name
	}

	if pkg.PURL != "" {
		component["purl"] = pkg.PURL
	}
	if pkg.CPE != "" {
		component["cpe"] = pkg.CPE
	}
	if pkg.Supplier != "" {
		component["supplier"] = map[string]interface{}{"name": pkg.Supplier}
	}
	if pkg.Description != "" {
		component["description"] = pkg.Description
	}
	if pkg.Homepage != "" {
		component["externalReferences"] = []interface{}{
			map[string]interface{}{
				"type": "website",
				"url":  pkg.Homepage,
			},
		}
	}
	if len(pkg.Checksums) > 0 {
		var hashes []interface{}
		for algo, val := range pkg.Checksums {
			hashes = append(hashes, map[string]interface{}{
				"alg":     mapHashAlgo(algo),
				"content": val,
			})
		}
		component["hashes"] = hashes
	}

	// License info
	if pkg.LicenseConcluded != "" || pkg.LicenseDeclared != "" {
		licenses := []interface{}{}
		if pkg.LicenseConcluded != "" {
			licenses = append(licenses, map[string]interface{}{
				"license": func() map[string]interface{} {
					if isSPDXLicense(pkg.LicenseConcluded) {
						return map[string]interface{}{"id": pkg.LicenseConcluded}
					}
					return map[string]interface{}{"name": pkg.LicenseConcluded}
				}(),
			})
		}
		if pkg.LicenseDeclared != "" && pkg.LicenseDeclared != pkg.LicenseConcluded {
			licenses = append(licenses, map[string]interface{}{
				"license": func() map[string]interface{} {
					if isSPDXLicense(pkg.LicenseDeclared) {
						return map[string]interface{}{"id": pkg.LicenseDeclared}
					}
					return map[string]interface{}{"name": pkg.LicenseDeclared}
				}(),
			})
		}
		if len(licenses) > 0 {
			component["licenses"] = licenses
		}
	}

	// Vulnerability info
	if pkg.HasVulnerabilities {
		properties := []interface{}{}
		properties = append(properties, map[string]interface{}{
			"name":  "sin:security:vulnerability_count",
			"value": fmt.Sprintf("%d", pkg.VulnerabilityCount),
		})
		if pkg.CriticalVulns > 0 {
			properties = append(properties, map[string]interface{}{
				"name":  "sin:security:critical_vulns",
				"value": fmt.Sprintf("%d", pkg.CriticalVulns),
			})
		}
		if pkg.HighVulns > 0 {
			properties = append(properties, map[string]interface{}{
				"name":  "sin:security:high_vulns",
				"value": fmt.Sprintf("%d", pkg.HighVulns),
			})
		}
		component["properties"] = properties
	}

	return component
}

func mapTypeToCycloneDX(pkgType string) string {
	mapping := map[string]string{
		"library":          "library",
		"application":      "application",
		"framework":        "framework",
		"container":        "container",
		"operating-system": "operating-system",
		"device":           "device",
		"file":             "file",
		"firmware":         "firmware",
	}
	if t, ok := mapping[pkgType]; ok {
		return t
	}
	return "library"
}

func mapHashAlgo(algo string) string {
	mapping := map[string]string{
		"md5":      "MD5",
		"sha1":     "SHA-1",
		"sha256":   "SHA-256",
		"sha384":   "SHA-384",
		"sha512":   "SHA-512",
		"sha3-256": "SHA3-256",
		"sha3-384": "SHA3-384",
		"sha3-512": "SHA3-512",
	}
	if a, ok := mapping[strings.ToLower(algo)]; ok {
		return a
	}
	return strings.ToUpper(algo)
}

func isSPDXLicense(license string) bool {
	spdxLicenses := []string{
		"MIT", "Apache-2.0", "BSD-2-Clause", "BSD-3-Clause", "GPL-2.0", "GPL-3.0",
		"LGPL-2.1", "LGPL-3.0", "MPL-2.0", "ISC", "Unlicense", "CC0-1.0",
	}
	for _, l := range spdxLicenses {
		if strings.EqualFold(l, license) {
			return true
		}
	}
	return false
}

func generateSerialNumber(namespace string) string {
	h := sha256.New()
	h.Write([]byte(namespace + "-cyclonedx"))
	return "urn:uuid:" + hex.EncodeToString(h.Sum(nil))[:32]
}
