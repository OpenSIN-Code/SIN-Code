// SPDX-License-Identifier: MIT
package models

// ContainerVulnerability represents a single container vulnerability.
type ContainerVulnerability struct {
	ID          string   `json:"id"`
	Package     string   `json:"package"`
	Version     string   `json:"version"`
	Severity    string   `json:"severity"`
	FixedIn     string   `json:"fixed_in,omitempty"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Layer       string   `json:"layer,omitempty"`
	VulnType    string   `json:"vuln_type"`
	References  []string `json:"references"`
	CVSSScore   float64  `json:"cvss_score,omitempty"`
	PrimaryURL  string   `json:"primary_url,omitempty"`
}

// ContainerMisconfiguration represents a Dockerfile/Container misconfiguration.
type ContainerMisconfiguration struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Message     string `json:"message"`
	Severity    string `json:"severity"`
	FilePath    string `json:"file_path"`
	LineNumber  int    `json:"line_number,omitempty"`
	Resolution  string `json:"resolution"`
	Category    string `json:"category"`
}

// TrivyScanReport represents a complete scan report from Trivy.
type TrivyScanReport struct {
	Image             string                       `json:"image"`
	BaseImage         string                       `json:"base_image,omitempty"`
	OSFamily          string                       `json:"os_family,omitempty"`
	OSVersion         string                       `json:"os_version,omitempty"`
	Vulnerabilities   []ContainerVulnerability     `json:"vulnerabilities"`
	Misconfigurations []ContainerMisconfiguration  `json:"misconfigurations"`
	Summary           map[string]int               `json:"summary"`
}

// DockerfileIssue represents an issue found in a Dockerfile.
type DockerfileIssue struct {
	RuleID     string `json:"rule_id"`
	Message    string `json:"message"`
	Severity   string `json:"severity"`
	Line       int    `json:"line"`
	Column     int    `json:"column,omitempty"`
	Suggestion string `json:"suggestion"`
}

// DockerImageInfo represents metadata of a Docker image.
type DockerImageInfo struct {
	ID            string            `json:"id"`
	Tags          []string          `json:"tags"`
	Size          int64             `json:"size"`
	Created       string            `json:"created"`
	Architecture  string            `json:"architecture"`
	OS            string            `json:"os"`
	Layers        int               `json:"layers"`
	BaseImage     string            `json:"base_image,omitempty"`
	Author        string            `json:"author,omitempty"`
	ExposedPorts  []string          `json:"exposed_ports"`
	Environment   map[string]string `json:"environment"`
	User          string            `json:"user"`
	WorkingDir    string            `json:"working_dir,omitempty"`
	Entrypoint    []string          `json:"entrypoint,omitempty"`
	Cmd           []string          `json:"cmd,omitempty"`
	RunsAsRoot    bool              `json:"runs_as_root"`
}

// ContainerScanResult represents the complete result of a container scan.
type ContainerScanResult struct {
	Image             string                      `json:"image"`
	ImageInfo         *DockerImageInfo            `json:"image_info,omitempty"`
	ScanReport        TrivyScanReport             `json:"scan_report"`
	DockerfileIssues  []DockerfileIssue           `json:"dockerfile_issues"`
	Status            string                      `json:"status"`
	Recommendations   []string                    `json:"recommendations"`
}

// NewTrivySummary creates a new empty Trivy summary.
func NewTrivySummary() map[string]int {
	return map[string]int{
		"total":             0,
		"critical":          0,
		"high":              0,
		"medium":            0,
		"low":               0,
		"unknown":           0,
		"misconfigurations": 0,
		"fixable":           0,
	}
}
