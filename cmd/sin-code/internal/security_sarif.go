// SPDX-License-Identifier: MIT
// Purpose: SARIF 2.1.0 converter for SIN-Code security findings.
// Produces valid SARIF 2.1.0 JSON from the unified SecurityFinding model.
// Docs: security.doc.md
package internal

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// SecurityFinding is a tool-agnostic representation of a single security issue.
// It can be constructed from the vendored SAST scanner, the vendored secrets
// scanner, or the coarse ToolResult entries produced by the main security scan.
type SecurityFinding struct {
	RuleID      string         `json:"rule_id"`
	RuleName    string         `json:"rule_name,omitempty"`
	Severity    string         `json:"severity"`
	Confidence  string         `json:"confidence,omitempty"`
	Kind        string         `json:"kind,omitempty"`
	Scanner     string         `json:"scanner,omitempty"`
	Tool        string         `json:"tool,omitempty"`
	File        string         `json:"file,omitempty"`
	Line        int            `json:"line,omitempty"`
	Column      int            `json:"column,omitempty"`
	Match       string         `json:"match,omitempty"`
	Context     string         `json:"context,omitempty"`
	Remediation string         `json:"remediation,omitempty"`
	CWE         string         `json:"cwe,omitempty"`
	OWASP       string         `json:"owasp,omitempty"`
	Description string         `json:"description,omitempty"`
	SecretType  string         `json:"secret_type,omitempty"`
	Package     string         `json:"package,omitempty"`
	Version     string         `json:"version,omitempty"`
	Title       string         `json:"title,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool        sarifTool         `json:"tool"`
	Results     []sarifResult     `json:"results"`
	Invocations []sarifInvocation `json:"invocations,omitempty"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version,omitempty"`
	InformationURI string      `json:"informationUri,omitempty"`
	Rules          []sarifRule `json:"rules,omitempty"`
}

type sarifRule struct {
	ID               string         `json:"id"`
	Name             string         `json:"name,omitempty"`
	ShortDescription *sarifMessage  `json:"shortDescription,omitempty"`
	FullDescription  *sarifMessage  `json:"fullDescription,omitempty"`
	HelpURI          string         `json:"helpUri,omitempty"`
	Properties       map[string]any `json:"properties,omitempty"`
}

type sarifResult struct {
	RuleID     string          `json:"ruleId"`
	RuleIndex  int             `json:"ruleIndex,omitempty"`
	Level      string          `json:"level"`
	Message    sarifMessage    `json:"message"`
	Locations  []sarifLocation `json:"locations,omitempty"`
	Properties map[string]any  `json:"properties,omitempty"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           *sarifRegion          `json:"region,omitempty"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine   int                   `json:"startLine,omitempty"`
	StartColumn int                   `json:"startColumn,omitempty"`
	Snippet     *sarifArtifactContent `json:"snippet,omitempty"`
}

type sarifArtifactContent struct {
	Text string `json:"text"`
}

type sarifInvocation struct {
	ExecutionSuccessful bool   `json:"executionSuccessful"`
	CommandLine         string `json:"commandLine,omitempty"`
}

func toSarif(findings []SecurityFinding) ([]byte, error) {
	if findings == nil {
		findings = []SecurityFinding{}
	}
	ordered := append([]SecurityFinding(nil), findings...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].RuleID != ordered[j].RuleID {
			return ordered[i].RuleID < ordered[j].RuleID
		}
		if ordered[i].File != ordered[j].File {
			return ordered[i].File < ordered[j].File
		}
		if ordered[i].Line != ordered[j].Line {
			return ordered[i].Line < ordered[j].Line
		}
		return ordered[i].MessageText() < ordered[j].MessageText()
	})

	rules := buildSarifRules(ordered)
	ruleIndex := make(map[string]int, len(rules))
	for i, r := range rules {
		ruleIndex[r.ID] = i
	}

	results := make([]sarifResult, 0, len(ordered))
	for _, f := range ordered {
		idx := -1
		if f.RuleID != "" {
			idx = ruleIndex[f.RuleID]
		}
		res := f.toSarifResult(idx)
		results = append(results, res)
	}

	log := sarifLog{
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Version: "2.1.0",
		Runs: []sarifRun{
			{
				Tool: sarifTool{
					Driver: sarifDriver{
						Name:           "sin-code security",
						InformationURI: "https://docs.opensin.ai/sin-code/security",
						Rules:          rules,
					},
				},
				Results:     results,
				Invocations: []sarifInvocation{{ExecutionSuccessful: true}},
			},
		},
	}

	return json.MarshalIndent(log, "", "  ")
}

func buildSarifRules(findings []SecurityFinding) []sarifRule {
	seen := make(map[string]sarifRule)
	ids := make([]string, 0)
	for _, f := range findings {
		if f.RuleID == "" {
			continue
		}
		if _, ok := seen[f.RuleID]; !ok {
			seen[f.RuleID] = f.toSarifRule()
			ids = append(ids, f.RuleID)
		}
	}
	sort.Strings(ids)
	rules := make([]sarifRule, 0, len(ids))
	for _, id := range ids {
		rules = append(rules, seen[id])
	}
	return rules
}

func (f SecurityFinding) toSarifRule() sarifRule {
	rule := sarifRule{
		ID:   f.RuleID,
		Name: f.RuleName,
	}
	if rule.Name == "" {
		rule.Name = f.Title
	}
	if rule.Name == "" {
		rule.Name = f.RuleID
	}
	if f.Description != "" {
		rule.ShortDescription = &sarifMessage{Text: f.Description}
		rule.FullDescription = &sarifMessage{Text: f.Description}
	} else if f.Context != "" {
		rule.ShortDescription = &sarifMessage{Text: truncateString(f.Context, 120)}
		rule.FullDescription = &sarifMessage{Text: f.Context}
	}
	props := make(map[string]any)
	if f.Severity != "" {
		props["severity"] = f.Severity
	}
	if f.Confidence != "" {
		props["confidence"] = f.Confidence
	}
	if f.CWE != "" {
		props["cwe"] = f.CWE
	}
	if f.OWASP != "" {
		props["owasp"] = f.OWASP
	}
	if f.Kind != "" {
		props["kind"] = f.Kind
	} else if f.Scanner != "" {
		props["kind"] = f.Scanner
	}
	if f.Tool != "" {
		props["tool"] = f.Tool
	} else if f.Scanner != "" {
		props["tool"] = f.Scanner
	}
	if f.SecretType != "" {
		props["secretType"] = f.SecretType
	}
	if f.Package != "" {
		props["package"] = f.Package
	}
	if f.Version != "" {
		props["version"] = f.Version
	}
	if f.Remediation != "" {
		props["remediation"] = f.Remediation
	}
	if len(props) > 0 {
		rule.Properties = props
	}
	return rule
}

func (f SecurityFinding) toSarifResult(ruleIndex int) sarifResult {
	res := sarifResult{
		RuleID:     f.RuleID,
		Level:      severityToSarifLevel(f.Severity),
		Message:    sarifMessage{Text: f.MessageText()},
		Properties: map[string]any{"kind": f.Kind},
	}
	if ruleIndex >= 0 {
		res.RuleIndex = ruleIndex
	}
	if res.Properties["kind"] == "" && f.Scanner != "" {
		res.Properties["kind"] = f.Scanner
	}
	if f.File != "" {
		loc := sarifLocation{
			PhysicalLocation: sarifPhysicalLocation{
				ArtifactLocation: sarifArtifactLocation{URI: f.File},
			},
		}
		if f.Line > 0 || f.Column > 0 {
			region := &sarifRegion{}
			if f.Line > 0 {
				region.StartLine = f.Line
			}
			if f.Column > 0 {
				region.StartColumn = f.Column
			}
			if f.Match != "" {
				region.Snippet = &sarifArtifactContent{Text: f.Match}
			}
			loc.PhysicalLocation.Region = region
		}
		res.Locations = []sarifLocation{loc}
	}
	if f.CWE != "" {
		res.Properties["cwe"] = f.CWE
	}
	if f.OWASP != "" {
		res.Properties["owasp"] = f.OWASP
	}
	if f.Confidence != "" {
		res.Properties["confidence"] = f.Confidence
	}
	if f.Remediation != "" {
		res.Properties["remediation"] = f.Remediation
	}
	if f.SecretType != "" {
		res.Properties["secretType"] = f.SecretType
	}
	if f.Package != "" {
		res.Properties["package"] = f.Package
	}
	if f.Version != "" {
		res.Properties["version"] = f.Version
	}
	tool := f.Tool
	if tool == "" {
		tool = f.Scanner
	}
	if tool != "" {
		res.Properties["tool"] = tool
	}
	for k, v := range f.Metadata {
		if _, ok := res.Properties[k]; !ok {
			res.Properties[k] = v
		}
	}
	return res
}

func (f SecurityFinding) MessageText() string {
	if f.Description != "" {
		return f.Description
	}
	parts := []string{}
	name := f.RuleName
	if name == "" {
		name = f.Title
	}
	if name != "" {
		parts = append(parts, name)
	} else if f.RuleID != "" {
		parts = append(parts, f.RuleID)
	}
	if f.Context != "" {
		parts = append(parts, truncateString(f.Context, 200))
	} else if f.Match != "" {
		parts = append(parts, truncateString(f.Match, 200))
	}
	msg := strings.Join(parts, " — ")
	if msg == "" {
		msg = fmt.Sprintf("%s finding", f.Kind)
	}
	if f.Kind == "" && f.Scanner != "" {
		msg = fmt.Sprintf("%s finding", f.Scanner)
	}
	if f.File != "" {
		loc := f.File
		if f.Line > 0 {
			loc = fmt.Sprintf("%s:%d", loc, f.Line)
		}
		msg = fmt.Sprintf("%s at %s", msg, loc)
	}
	return msg
}

func severityToSarifLevel(sev string) string {
	switch strings.ToLower(sev) {
	case "critical", "high":
		return "error"
	case "medium", "warning":
		return "warning"
	case "low", "note", "info":
		return "note"
	default:
		return "warning"
	}
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func sastFindingToSecurity(f sastScanFinding) SecurityFinding {
	return SecurityFinding{
		RuleID:      f.RuleID,
		RuleName:    f.RuleName,
		Severity:    f.Severity,
		Confidence:  f.Confidence,
		Kind:        "sast",
		Tool:        "sin-sast",
		File:        f.File,
		Line:        f.Line,
		Column:      f.Column,
		Match:       f.Match,
		Context:     f.Context,
		Remediation: f.Remediation,
		CWE:         f.CWE,
		OWASP:       f.OWASP,
		Description: f.Description,
	}
}

func secretsFindingToSecurity(f secretsScanFinding) SecurityFinding {
	return SecurityFinding{
		RuleID:      f.RuleID,
		RuleName:    f.RuleName,
		Severity:    f.Severity,
		Confidence:  f.Confidence,
		Kind:        "secret",
		Tool:        "sin-secrets",
		File:        f.File,
		Line:        f.Line,
		Column:      f.Column,
		Match:       maskSecuritySecret(f.Match),
		Context:     f.Context,
		Remediation: f.Remediation,
		Description: f.RuleName,
		SecretType:  f.SecretType,
		Metadata: map[string]any{
			"entropy":    f.Entropy,
			"isVerified": f.IsVerified,
		},
	}
}

func toolResultToSecurityFinding(scanRoot string, t ToolResult, idx int) SecurityFinding {
	ruleID := fmt.Sprintf("SIN-CODE-%s", strings.ToUpper(strings.ReplaceAll(t.Name, " ", "-")))
	if ruleID == "SIN-CODE-" {
		ruleID = fmt.Sprintf("SIN-CODE-TOOL-%d", idx)
	}
	f := SecurityFinding{
		RuleID:      ruleID,
		RuleName:    t.Name,
		Severity:    "medium",
		Kind:        "generic",
		Tool:        t.Name,
		Context:     t.Output,
		Description: fmt.Sprintf("%s reported %d issue(s)", t.Name, t.Issues),
	}
	if t.Error != "" {
		f.Description = fmt.Sprintf("%s error: %s", t.Name, t.Error)
		f.Severity = "warning"
		f.Kind = "error"
	}
	if t.Status == "issues" && t.Issues > 0 {
		f.Severity = "high"
	}
	if scanRoot != "" {
		f.File = scanRoot
	}
	return f
}

func securityResultToFindings(r SecurityResult) []SecurityFinding {
	findings := []SecurityFinding{}
	for i, t := range r.Tools {
		switch t.Status {
		case "issues", "error":
			findings = append(findings, toolResultToSecurityFinding(r.Path, t, i))
		}
	}
	return findings
}

func writeSarif(cmd *cobra.Command, findings []SecurityFinding) error {
	out, err := toSarif(findings)
	if err != nil {
		return fmt.Errorf("convert to SARIF: %w", err)
	}
	w := cmd.OutOrStdout()
	if _, err := fmt.Fprintln(w, string(out)); err != nil {
		return fmt.Errorf("write SARIF: %w", err)
	}
	return nil
}

func normalizeFindingPaths(scanRoot string, findings []SecurityFinding) []SecurityFinding {
	if scanRoot == "" {
		return findings
	}
	absRoot, err := filepath.Abs(scanRoot)
	if err != nil {
		absRoot = scanRoot
	}
	out := make([]SecurityFinding, len(findings))
	for i, f := range findings {
		out[i] = f
		out[i].File = relativeURI(absRoot, f.File)
	}
	return out
}

func relativeURI(scanRoot, file string) string {
	if file == "" {
		return ""
	}
	absFile := file
	if !filepath.IsAbs(file) {
		absFile = filepath.Join(scanRoot, file)
	}
	rel, err := filepath.Rel(scanRoot, absFile)
	if err == nil && rel != "" && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(file)
}
