package models

// SecretFinding represents a detected secret
type SecretFinding struct {
	RuleID       string `json:"rule_id" yaml:"rule_id"`
	RuleName     string `json:"rule_name" yaml:"rule_name"`
	Severity     string `json:"severity" yaml:"severity"`
	SecretType   string `json:"secret_type" yaml:"secret_type"`
	File         string `json:"file" yaml:"file"`
	Line         int    `json:"line" yaml:"line"`
	Column       int    `json:"column" yaml:"column"`
	Match        string `json:"match" yaml:"match"`
	Context      string `json:"context" yaml:"context"`
	Remediation  string `json:"remediation" yaml:"remediation"`
	Confidence   string `json:"confidence" yaml:"confidence"`
	Entropy      float64 `json:"entropy" yaml:"entropy"`
	IsVerified   bool   `json:"is_verified" yaml:"is_verified"`
}

// SecretsResult represents the complete scan result
type SecretsResult struct {
	Path                string          `json:"path" yaml:"path"`
	Status              string          `json:"status" yaml:"status"`
	Findings            []SecretFinding `json:"findings" yaml:"findings"`
	Summary             SecretsSummary  `json:"summary" yaml:"summary"`
	ScanDurationSeconds float64         `json:"scan_duration_seconds" yaml:"scan_duration_seconds"`
	Timestamp           string          `json:"timestamp" yaml:"timestamp"`
}

// SecretsSummary represents the scan summary
type SecretsSummary struct {
	Critical        int            `json:"critical" yaml:"critical"`
	High            int            `json:"high" yaml:"high"`
	Medium          int            `json:"medium" yaml:"medium"`
	Low             int            `json:"low" yaml:"low"`
	FilesScanned    int            `json:"files_scanned" yaml:"files_scanned"`
	SecretsFound    int            `json:"secrets_found" yaml:"secrets_found"`
	ByType          map[string]int `json:"by_type" yaml:"by_type"`
	ByFile          map[string]int `json:"by_file" yaml:"by_file"`
}

// ScanOptions represents scan configuration
type ScanOptions struct {
	Path           string
	SecretTypes    []string
	Severity       string
	Exclude        []string
	ScanGitHistory bool
	EntropyCheck   bool
	Output         string
	Verbose        bool
}

// DetectionRule represents a secret detection rule
type DetectionRule struct {
	ID          string   `json:"id" yaml:"id"`
	Name        string   `json:"name" yaml:"name"`
	SecretType  string   `json:"secret_type" yaml:"secret_type"`
	Severity    string   `json:"severity" yaml:"severity"`
	Patterns    []string `json:"patterns" yaml:"patterns"`
	Remediation string   `json:"remediation" yaml:"remediation"`
	Confidence  string   `json:"confidence" yaml:"confidence"`
	MinEntropy  float64  `json:"min_entropy" yaml:"min_entropy"`
}
