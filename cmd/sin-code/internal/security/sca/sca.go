// SPDX-License-Identifier: MIT
// Purpose: Go-native software composition analysis (SCA) for Go projects.
// Parses go.mod and drives grype JSON output to report vulnerable dependencies.
// Docs: sca.doc.md
package sca

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Package represents a parsed dependency.
type Package struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Ecosystem string `json:"ecosystem"`
}

// Vulnerability represents a single vulnerable dependency finding.
type Vulnerability struct {
	ID          string   `json:"id"`
	Severity    string   `json:"severity"`
	Package     string   `json:"package"`
	Version     string   `json:"version"`
	FixedIn     []string `json:"fixed_in,omitempty"`
	Description string   `json:"description,omitempty"`
}

// Result is the output of an SCA scan.
type Result struct {
	Path            string          `json:"path"`
	Ecosystem       string          `json:"ecosystem"`
	PackagesScanned int             `json:"packages_scanned"`
	Vulnerabilities []Vulnerability `json:"vulnerabilities"`
	Summary         map[string]int  `json:"summary"`
}

// Scanner orchestrates go.mod parsing and grype invocation.
type Scanner struct {
	grype *GrypeClient
}

// New creates a Scanner with default settings.
func New() *Scanner {
	return &Scanner{grype: NewGrypeClient()}
}

// NewWithGrype creates a Scanner with a custom grype client. Useful for tests.
func NewWithGrype(grype *GrypeClient) *Scanner {
	return &Scanner{grype: grype}
}

// IsGoProject reports whether path contains a go.mod file.
func IsGoProject(path string) bool {
	return fileExists(filepath.Join(path, "go.mod"))
}

// DetectGoProject is an alias for IsGoProject that matches the detector style
// used elsewhere in the security subcommand.
func DetectGoProject(path string) bool {
	return IsGoProject(path)
}

// Scan runs an SCA scan on a Go project at path.
// It first parses go.mod to list direct and indirect dependencies, then invokes
// grype to obtain vulnerability findings. If grype is not available, the result
// still contains the parsed dependency list with zero vulnerabilities.
func (s *Scanner) Scan(ctx context.Context, path string) (*Result, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	if !IsGoProject(path) {
		return nil, fmt.Errorf("no go.mod found in %s", path)
	}

	packages, err := ParseGoMod(path)
	if err != nil {
		return nil, fmt.Errorf("parse go.mod: %w", err)
	}

	var vulns []Vulnerability
	if s.grype != nil && s.grype.Available() {
		found, err := s.grype.ScanDirectory(ctx, path)
		if err == nil {
			vulns = found
		}
	}
	if vulns == nil {
		vulns = []Vulnerability{}
	}

	summary := summarize(vulns, len(packages))
	return &Result{
		Path:            path,
		Ecosystem:       "Go",
		PackagesScanned: len(packages),
		Vulnerabilities: vulns,
		Summary:         summary,
	}, nil
}

// ScanPackages returns the dependency list for a Go project without running grype.
func (s *Scanner) ScanPackages(ctx context.Context, path string) ([]Package, error) {
	_ = ctx
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	if !IsGoProject(path) {
		return nil, fmt.Errorf("no go.mod found in %s", path)
	}
	return ParseGoMod(path)
}

// summarize builds a severity summary.
func summarize(vulns []Vulnerability, packages int) map[string]int {
	out := map[string]int{
		"total":      len(vulns),
		"packages":   packages,
		"critical":   0,
		"high":       0,
		"medium":     0,
		"low":        0,
		"negligible": 0,
		"unknown":    0,
	}
	for _, v := range vulns {
		sev := normalizeSeverity(v.Severity)
		out[sev]++
	}
	return out
}

// normalizeSeverity maps grype severity strings to a canonical lower-case key.
func normalizeSeverity(s string) string {
	switch s := lower(s); s {
	case "critical":
		return "critical"
	case "high":
		return "high"
	case "medium":
		return "medium"
	case "low":
		return "low"
	case "negligible":
		return "negligible"
	default:
		return "unknown"
	}
}

// lower converts ASCII letters to lower case without importing strings.
func lower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

// fileExists reports whether p exists.
func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
