// SPDX-License-Identifier: MIT
package models

// SASTFinding represents a single security finding
type SASTFinding struct {
	RuleID       string `json:"rule_id" yaml:"rule_id"`
	RuleName     string `json:"rule_name" yaml:"rule_name"`
	Severity     string `json:"severity" yaml:"severity"`
	CWE          string `json:"cwe" yaml:"cwe"`
	OWASP        string `json:"owasp" yaml:"owasp"`
	Language     string `json:"language" yaml:"language"`
	File         string `json:"file" yaml:"file"`
	Line         int    `json:"line" yaml:"line"`
	Column       int    `json:"column" yaml:"column"`
	Match        string `json:"match" yaml:"match"`
	Context      string `json:"context" yaml:"context"`
	Remediation  string `json:"remediation" yaml:"remediation"`
	Confidence   string `json:"confidence" yaml:"confidence"`
	Description  string `json:"description" yaml:"description"`
}

// SASTResult represents the complete scan result
type SASTResult struct {
	Path                string        `json:"path" yaml:"path"`
	Status              string        `json:"status" yaml:"status"`
	Findings            []SASTFinding `json:"findings" yaml:"findings"`
	Summary             SASTSummary   `json:"summary" yaml:"summary"`
	ScanDurationSeconds float64       `json:"scan_duration_seconds" yaml:"scan_duration_seconds"`
	Timestamp           string        `json:"timestamp" yaml:"timestamp"`
}

// SASTSummary represents the scan summary
type SASTSummary struct {
	Critical        int            `json:"critical" yaml:"critical"`
	High            int            `json:"high" yaml:"high"`
	Medium          int            `json:"medium" yaml:"medium"`
	Low             int            `json:"low" yaml:"low"`
	FilesScanned    int            `json:"files_scanned" yaml:"files_scanned"`
	LinesScanned    int            `json:"lines_scanned" yaml:"lines_scanned"`
	RulesTriggered  int            `json:"rules_triggered" yaml:"rules_triggered"`
	ByLanguage      map[string]int `json:"by_language" yaml:"by_language"`
	ByOWASP         map[string]int `json:"by_owasp" yaml:"by_owasp"`
}

// ScanOptions represents scan configuration
type ScanOptions struct {
	Path       string
	Languages  []string
	Severity   string
	Rules      []string
	Exclude    []string
	Output     string
	Verbose    bool
}

// Rule represents a SAST detection rule
type Rule struct {
	ID          string   `json:"id" yaml:"id"`
	Name        string   `json:"name" yaml:"name"`
	Description string   `json:"description" yaml:"description"`
	Severity    string   `json:"severity" yaml:"severity"`
	CWE         string   `json:"cwe" yaml:"cwe"`
	OWASP       string   `json:"owasp" yaml:"owasp"`
	Languages   []string `json:"languages" yaml:"languages"`
	Patterns    []string `json:"patterns" yaml:"patterns"`
	Remediation string   `json:"remediation" yaml:"remediation"`
	Confidence  string   `json:"confidence" yaml:"confidence"`
	Category    string   `json:"category" yaml:"category"`
}
