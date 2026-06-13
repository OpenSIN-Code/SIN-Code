// SPDX-License-Identifier: MIT
// Purpose: grype subprocess client that parses grype's JSON output.
// Docs: sca.doc.md
package sca

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// GrypeClient invokes the grype binary and parses its JSON report.
type GrypeClient struct {
	// Path is the grype binary name or absolute path. Defaults to "grype".
	Path string
	// CommandRunner builds exec.Cmd instances. Overridable for tests.
	CommandRunner func(ctx context.Context, name string, arg ...string) *exec.Cmd
}

// NewGrypeClient creates a GrypeClient using the grype binary on PATH.
func NewGrypeClient() *GrypeClient {
	return &GrypeClient{
		Path:          "grype",
		CommandRunner: exec.CommandContext,
	}
}

// Available reports whether grype can be located.
func (c *GrypeClient) Available() bool {
	name := c.Path
	if name == "" {
		name = "grype"
	}
	_, err := exec.LookPath(name)
	return err == nil
}

// ScanDirectory runs grype -o json on dir and returns parsed vulnerabilities.
func (c *GrypeClient) ScanDirectory(ctx context.Context, dir string) ([]Vulnerability, error) {
	name := c.Path
	if name == "" {
		name = "grype"
	}
	runner := c.CommandRunner
	if runner == nil {
		runner = exec.CommandContext
	}
	cmd := runner(ctx, name, dir, "-o", "json")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("grype failed: %w (output: %s)", err, truncate(out))
	}
	return parseGrypeReport(out)
}

// grypeReport is the top-level grype JSON document.
type grypeReport struct {
	Matches []grypeMatch `json:"matches"`
}

// grypeMatch is the subset of a grype match we need.
type grypeMatch struct {
	Vulnerability struct {
		ID          string `json:"id"`
		Severity    string `json:"severity"`
		Description string `json:"description"`
		Fix         struct {
			Versions []string `json:"versions"`
			State    string   `json:"state"`
		} `json:"fix"`
	} `json:"vulnerability"`
	Artifact struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Type    string `json:"type"`
	} `json:"artifact"`
}

// parseGrypeReport parses raw grype JSON bytes into our Vulnerability model.
func parseGrypeReport(data []byte) ([]Vulnerability, error) {
	var report grypeReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("decode grype json: %w", err)
	}
	vulns := make([]Vulnerability, 0, len(report.Matches))
	for _, m := range report.Matches {
		vulns = append(vulns, Vulnerability{
			ID:          m.Vulnerability.ID,
			Severity:    m.Vulnerability.Severity,
			Package:     m.Artifact.Name,
			Version:     m.Artifact.Version,
			FixedIn:     m.Vulnerability.Fix.Versions,
			Description: m.Vulnerability.Description,
		})
	}
	return vulns, nil
}

// truncate limits long grype output for error messages.
func truncate(b []byte) string {
	const max = 400
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "..."
}
