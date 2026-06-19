// SPDX-License-Identifier: MIT
// Purpose: governance capture — secret/policy/approval detection in
// observations (issue #354). Detects API keys, passwords, tokens,
// policy violations, and approval requests in tool-call content.
// Critical findings (secrets) must be redacted before memory writes.
package memory

import (
	"regexp"
	"strings"
)

// GovernanceFindingType classifies the kind of governance issue found.
type GovernanceFindingType string

const (
	GovSecret   GovernanceFindingType = "secret"
	GovPolicy   GovernanceFindingType = "policy"
	GovApproval GovernanceFindingType = "approval"
)

// GovernanceSeverity rates the urgency of a finding.
type GovernanceSeverity string

const (
	SeverityCritical GovernanceSeverity = "critical"
	SeverityHigh     GovernanceSeverity = "high"
	SeverityMedium   GovernanceSeverity = "medium"
	SeverityLow      GovernanceSeverity = "low"
)

// GovernanceFinding represents a single governance-relevant detection.
type GovernanceFinding struct {
	Type           GovernanceFindingType `json:"type"`
	Match          string                `json:"match"`
	Severity       GovernanceSeverity    `json:"severity"`
	Recommendation string                `json:"recommendation"`
}

// secretPatterns maps compiled regexes to a human-readable label and
// severity. Order matters — more specific patterns first.
var secretPatterns []secretPattern

type secretPattern struct {
	re             *regexp.Regexp
	label          string
	severity       GovernanceSeverity
	recommendation string
}

func init() {
	secretPatterns = []secretPattern{
		{regexp.MustCompile(`(?i)\bAKIA[0-9A-Z]{16}\b`), "AWS Access Key ID", SeverityCritical, "Rotate AWS key immediately and revoke from IAM"},
		{regexp.MustCompile(`(?i)\b(sk-)[a-zA-Z0-9]{20,}\b`), "OpenAI API Key", SeverityCritical, "Revoke key at platform.openai.com and rotate"},
		{regexp.MustCompile(`(?i)\b(ghp_)[a-zA-Z0-9]{36,}\b`), "GitHub Personal Access Token", SeverityCritical, "Revoke token at github.com/settings/tokens"},
		{regexp.MustCompile(`(?i)\b(gho_)[a-zA-Z0-9]{36,}\b`), "GitHub OAuth Token", SeverityCritical, "Revoke OAuth token at github.com/settings/applications"},
		{regexp.MustCompile(`(?i)\b(xox[bpsa]-)[a-zA-Z0-9-]{10,}\b`), "Slack Token", SeverityCritical, "Revoke token at api.slack.com/apps"},
		{regexp.MustCompile(`(?i)\b(sk-ant-)[a-zA-Z0-9_-]{20,}\b`), "Anthropic API Key", SeverityCritical, "Revoke key at console.anthropic.com"},
		{regexp.MustCompile(`(?i)\b(vck_)[a-zA-Z0-9]{20,}\b`), "Vercel AI Gateway Key", SeverityCritical, "Rotate key in Vercel dashboard"},
		{regexp.MustCompile(`(?i)-----BEGIN (RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----`), "Private Key Block", SeverityCritical, "Remove from output and rotate key pair"},
		{regexp.MustCompile(`(?i)\b(eyJ)[a-zA-Z0-9_-]*\.[a-zA-Z0-9_-]*\.[a-zA-Z0-9_-]*\b`), "JWT Token", SeverityHigh, "Verify token is not leaked; rotate signing key if exposed"},
		{regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[=:]\s*["']?[^\s"']{4,}`), "Password Assignment", SeverityHigh, "Remove hardcoded password; use env var or secret manager"},
		{regexp.MustCompile(`(?i)\b(api[_-]?key)\s*[=:]\s*["']?[a-zA-Z0-9]{20,}`), "API Key Assignment", SeverityHigh, "Move API key to environment variable or Infisical"},
		{regexp.MustCompile(`(?i)\b(token)\s*[=:]\s*["']?[a-zA-Z0-9]{20,}`), "Token Assignment", SeverityMedium, "Store token in secret manager, not in source"},
	}
}

var policyPatterns = []struct {
	re             *regexp.Regexp
	label          string
	recommendation string
}{
	{regexp.MustCompile(`(?i)TODO:\s*ask user`), "TODO: ask user", "Resolve pending user question before proceeding"},
	{regexp.MustCompile(`(?i)requires?\s+approval`), "Requires approval", "Obtain explicit approval before executing"},
	{regexp.MustCompile(`(?i)needs?\s+permission`), "Needs permission", "Request permission from user or admin"},
	{regexp.MustCompile(`(?i)requires?\s+authorization`), "Requires authorization", "Get authorization before continuing"},
	{regexp.MustCompile(`(?i)pending\s+review`), "Pending review", "Complete code review before merging"},
	{regexp.MustCompile(`(?i)do\s+not\s+(commit|push|merge)`), "Do not commit/push/merge", "Respect the constraint — do not commit, push, or merge"},
}

var approvalPatterns = []struct {
	re             *regexp.Regexp
	label          string
	recommendation string
}{
	{regexp.MustCompile(`(?i)approved\s+by\s+\w+`), "Approval granted", "Record approval in audit log"},
	{regexp.MustCompile(`(?i)denied\s+by\s+\w+`), "Approval denied", "Log denial and stop action"},
	{regexp.MustCompile(`(?i)pending\s+approval`), "Pending approval", "Await approval before proceeding"},
	{regexp.MustCompile(`(?i)rejected\s+by\s+\w+`), "Approval rejected", "Log rejection and halt"},
	{regexp.MustCompile(`(?i)waiting\s+for\s+approval`), "Waiting for approval", "Block until approval is received"},
}

// GovernanceCapture detects governance-relevant content in observations.
type GovernanceCapture struct{}

// NewGovernanceCapture creates a new governance scanner.
func NewGovernanceCapture() *GovernanceCapture {
	return &GovernanceCapture{}
}

// Scan examines content and returns all governance findings.
func (g *GovernanceCapture) Scan(content string) []GovernanceFinding {
	if content == "" {
		return nil
	}
	var findings []GovernanceFinding

	for _, sp := range secretPatterns {
		matches := sp.re.FindAllString(content, -1)
		for _, m := range matches {
			findings = append(findings, GovernanceFinding{
				Type:           GovSecret,
				Match:          m,
				Severity:       sp.severity,
				Recommendation: sp.recommendation,
			})
		}
	}

	for _, pp := range policyPatterns {
		if pp.re.MatchString(content) {
			findings = append(findings, GovernanceFinding{
				Type:           GovPolicy,
				Match:          pp.label,
				Severity:       SeverityMedium,
				Recommendation: pp.recommendation,
			})
		}
	}

	for _, ap := range approvalPatterns {
		if ap.re.MatchString(content) {
			findings = append(findings, GovernanceFinding{
				Type:           GovApproval,
				Match:          ap.label,
				Severity:       SeverityLow,
				Recommendation: ap.recommendation,
			})
		}
	}

	return findings
}

// IsCritical returns true if the finding is a secret or has
// critical/high severity.
func (g *GovernanceCapture) IsCritical(finding GovernanceFinding) bool {
	if finding.Type == GovSecret {
		return true
	}
	return finding.Severity == SeverityCritical || finding.Severity == SeverityHigh
}

// Redact replaces secret matches in content with a redaction marker.
// Non-secret findings are left in place.
func (g *GovernanceCapture) Redact(content string) string {
	for _, sp := range secretPatterns {
		content = sp.re.ReplaceAllString(content, "[REDACTED:"+sp.label+"]")
	}
	return content
}

// HasSecret is a convenience method that returns true if the content
// contains any secret findings.
func (g *GovernanceCapture) HasSecret(content string) bool {
	for _, sp := range secretPatterns {
		if sp.re.MatchString(content) {
			return true
		}
	}
	return false
}

// Summary returns a one-line summary of findings by type.
func (g *GovernanceCapture) Summary(findings []GovernanceFinding) string {
	if len(findings) == 0 {
		return "no governance findings"
	}
	counts := map[GovernanceFindingType]int{}
	critical := 0
	for _, f := range findings {
		counts[f.Type]++
		if g.IsCritical(f) {
			critical++
		}
	}
	var parts []string
	for _, t := range []GovernanceFindingType{GovSecret, GovPolicy, GovApproval} {
		if counts[t] > 0 {
			parts = append(parts, string(t)+":"+itoa(counts[t]))
		}
	}
	out := strings.Join(parts, " ")
	if critical > 0 {
		out += " (" + itoa(critical) + " critical)"
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
