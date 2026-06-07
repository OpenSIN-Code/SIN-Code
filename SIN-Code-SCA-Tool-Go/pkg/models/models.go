// SPDX-License-Identifier: MIT
package models

// Vulnerability represents a single security vulnerability.
type Vulnerability struct {
	ID          string   `json:"id"`
	Package     string   `json:"package"`
	Version     string   `json:"version"`
	Severity    string   `json:"severity"`
	Summary     string   `json:"summary"`
	FixedIn     string   `json:"fixed_in,omitempty"`
	References  []string `json:"references"`
	Aliases     []string `json:"aliases"`
}

// Package represents a parsed dependency package.
type Package struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Ecosystem string `json:"ecosystem"`
}

// ScanResult represents the result of a project scan.
type ScanResult struct {
	ProjectPath     string                 `json:"project_path"`
	Ecosystem       string                 `json:"ecosystem"`
	Vulnerabilities []Vulnerability        `json:"vulnerabilities"`
	Summary         map[string]int         `json:"summary"`
	PackagesScanned int                    `json:"packages_scanned"`
}

// SummaryKeys returns the ordered severity keys.
func SummaryKeys() []string {
	return []string{"total", "critical", "high", "medium", "low", "unknown"}
}

// NewSummary creates a new empty summary.
func NewSummary() map[string]int {
	return map[string]int{
		"total":    0,
		"critical": 0,
		"high":     0,
		"medium":   0,
		"low":      0,
		"unknown":  0,
	}
}
