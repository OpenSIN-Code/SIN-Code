// SPDX-License-Identifier: MIT
// Purpose: reactive permission policy that scans tool *results* after
// execution and adjusts future posture when sensitive patterns are
// detected (issue #374). Patterns include secret/token leakage,
// destructive confirmations, and network egress markers.
package permission

import (
	"regexp"
	"strings"
	"sync"
)

// PolicyAction is the recommended reactive action after scanning a tool
// result. It is advisory: the caller (agent loop) decides how to surface
// the warning and whether to block subsequent work.
type PolicyAction int

const (
	// ActionNoOp means the result contained no reactive-policy triggers.
	ActionNoOp PolicyAction = iota
	// ActionWarn means the result contains a pattern that should be
	// surfaced to the model/operator but need not stop the run.
	ActionWarn
	// ActionEscalate means the result contains a high-sensitivity pattern
	// (e.g., credential leakage) that should be recorded prominently and
	// may prompt re-authorization.
	ActionEscalate
)

// String returns the canonical lowercase name of the action.
func (a PolicyAction) String() string {
	switch a {
	case ActionWarn:
		return "warn"
	case ActionEscalate:
		return "escalate"
	default:
		return "noop"
	}
}

// ResultPolicy scans tool outputs for reactive patterns. It is safe for
// concurrent use: regular expressions are compiled exactly once via
// sync.Once and then only read.
type ResultPolicy struct {
	secretRe      *regexp.Regexp
	destructiveRe *regexp.Regexp
	networkRe     *regexp.Regexp
	once          sync.Once
}

// NewResultPolicy creates a reactive result scanner. Regexes are compiled
// lazily on the first ScanResult call to keep startup cost minimal.
func NewResultPolicy() *ResultPolicy {
	return &ResultPolicy{}
}

func (rp *ResultPolicy) compile() {
	rp.once.Do(func() {
		// AWS Access Key ID (AKIA...), JWT-shaped blobs, and common
		// GitHub/GitLab token prefixes. These are heuristic: they catch
		// accidental paste in tool output, not every possible secret.
		rp.secretRe = regexp.MustCompile(`(?i)(AKIA[0-9A-Z]{16}|` +
			`eyJ[A-Za-z0-9_-]*\.eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]*|` +
			`ghp_[A-Za-z0-9]{36}|gho_[A-Za-z0-9]{36}|glpat-[A-Za-z0-9\-]{20,}|` +
			`\b(?:api[_-]?key|apikey|secret|token|password)\s*[:=]\s*['"]?[A-Za-z0-9_\-+/]{16,}['"]?` +
			`)`)
		// Destructive operation confirmations.
		rp.destructiveRe = regexp.MustCompile(`(?i)\b(deleted|removed|destroyed|dropped|purged|truncated|wiped)\b`)
		// Network egress markers (optional, conservative).
		rp.networkRe = regexp.MustCompile(`(?i)\b(egress|outbound|external\s+(?:ip|host|domain|address))\b`)
	})
}

// ScanResult inspects a tool result and returns a recommended action plus
// a short human-readable reason. The tool name is included because some
// tools (e.g., secret scanners) are expected to mention secrets; for most
// tools, mentioning a secret is a leakage signal.
func (rp *ResultPolicy) ScanResult(toolName, result string) (PolicyAction, string) {
	rp.compile()
	t := strings.ToLower(toolName)

	// Secret scanners are allowed to emit secret-like strings; do not
	// escalate them. Other tools mentioning these patterns are suspect.
	if !strings.Contains(t, "secret") && !strings.Contains(t, "scan") && rp.secretRe.MatchString(result) {
		return ActionEscalate, "possible secret/token leakage in tool output"
	}
	if rp.destructiveRe.MatchString(result) {
		return ActionWarn, "destructive operation confirmed in tool output"
	}
	if rp.networkRe.MatchString(result) {
		return ActionWarn, "network egress marker detected in tool output"
	}
	return ActionNoOp, ""
}

// SampleDetections returns a few deterministic (tool, result) pairs useful
// for demos and CLI smoke tests. None of the strings are real credentials.
func SampleDetections() []SampleDetection {
	return []SampleDetection{
		{
			Tool:   "sin_test",
			Result: "ok  1 test passed",
		},
		{
			Tool:   "sin_bash",
			Result: "removed 42 files and deleted directory /tmp/old",
		},
		{
			Tool:   "sin_http_get",
			Result: "fetched https://api.example.com/v1/data (egress via outbound proxy)",
		},
		{
			Tool:   "aws_cli",
			Result: "AKIAIOSFODNN7EXAMPLE",
		},
		{
			Tool:   "cat_env",
			Result: "api_key = '1234567890abcdef1234567890abcdef'",
		},
	}
}

// SampleDetection pairs a tool name with a result string for demo purposes.
type SampleDetection struct {
	Tool   string
	Result string
}
