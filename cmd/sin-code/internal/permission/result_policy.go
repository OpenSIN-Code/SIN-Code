// SPDX-License-Identifier: MIT
// Purpose: Reactive permission policy from tool results (issue #374).
package permission

import (
	"regexp"
	"strings"
	"sync"
	"time"
)

type PolicyAction int

const (
	ActionNoOp PolicyAction = iota
	ActionWarn
	ActionEscalate
)

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

type ResultPolicy struct {
	secretRe      *regexp.Regexp
	destructiveRe *regexp.Regexp
	networkRe     *regexp.Regexp
	once          sync.Once
}

func NewResultPolicy() *ResultPolicy {
	return &ResultPolicy{}
}

func (rp *ResultPolicy) compile() {
	rp.once.Do(func() {
		rp.secretRe = regexp.MustCompile(`(?i)(AKIA[0-9A-Z]{16}|` +
			`eyJ[A-Za-z0-9_-]*\.eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]*|` +
			`ghp_[A-Za-z0-9]{36}|gho_[A-Za-z0-9]{36}|glpat-[A-Za-z0-9\-]{20,}|` +
			`\b(?:api[_-]?key|apikey|secret|token|password)\s*[:=]\s*['"]?[A-Za-z0-9_\-+/]{16,}['"]?` +
			`)`)
		rp.destructiveRe = regexp.MustCompile(`(?i)\b(deleted|removed|destroyed|dropped|purged|truncated|wiped)\b`)
		rp.networkRe = regexp.MustCompile(`(?i)\b(egress|outbound|external\s+(?:ip|host|domain|address))\b`)
	})
}

func (rp *ResultPolicy) ScanResult(toolName, result string) (PolicyAction, string) {
	rp.compile()
	t := strings.ToLower(toolName)
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

type SampleDetection struct {
	Tool   string
	Result string
}

func SampleDetections() []SampleDetection {
	return []SampleDetection{
		{Tool: "sin_bash", Result: "AKIAIOSFODNN7EXAMPLE"},
		{Tool: "sin_bash", Result: "deleted 42 files"},
	}
}

type ResultPolicyAdjustment struct {
	Trigger   string `json:"trigger"`
	Severity  string `json:"severity"`
	Message   string `json:"message"`
	Action    string `json:"action"`
	ToolName  string `json:"tool_name"`
	Timestamp string `json:"timestamp"`
}

type ResultPolicyEntry struct {
	ToolName      string                   `json:"tool_name"`
	ResultSnippet string                   `json:"result_snippet"`
	Adjustments   []ResultPolicyAdjustment `json:"adjustments"`
	Timestamp     string                   `json:"timestamp"`
}

type ResultScanner struct {
	secretPatterns      []*regexp.Regexp
	destructivePatterns []*regexp.Regexp
	egressPatterns      []*regexp.Regexp
}

func NewResultScanner() *ResultScanner {
	return &ResultScanner{
		secretPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)(AKIA[0-9A-Z]{16})`),
			regexp.MustCompile(`(?i)(eyJ[a-zA-Z0-9_-]+\.eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+)`),
			regexp.MustCompile(`(?i)(ghp_[a-zA-Z0-9]{36})`),
			regexp.MustCompile(`(?i)(gho_[a-zA-Z0-9]{36})`),
			regexp.MustCompile(`(?i)(sk-[a-zA-Z0-9]{32,})`),
			regexp.MustCompile(`(?i)(xox[bpras]-[a-zA-Z0-9-]{24,})`),
			regexp.MustCompile(`(?i)(-----BEGIN (RSA |EC )?PRIVATE KEY-----)`),
			regexp.MustCompile(`(?i)(password|passwd|pwd|secret)\s*[:=]\s*['"][^'"]{8,}['"]`),
		},
		destructivePatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)\b(deleted|removed|unlinked)\s+\d+\s+(file|files|directory|directories)`),
			regexp.MustCompile(`(?i)\b(rm\s+-rf|rmdir|rm\s+-r)\b`),
			regexp.MustCompile(`(?i)\b(drop\s+table|truncate\s+table|drop\s+database)\b`),
			regexp.MustCompile(`(?i)\b(git\s+push\s+--force|git\s+reset\s+--hard)\b`),
		},
		egressPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)\b(curl\s+-[^\s]*\s+https?://)`),
			regexp.MustCompile(`(?i)\bwget\s+https?://`),
			regexp.MustCompile(`(?i)\bfetch\s+https?://`),
			regexp.MustCompile(`(?i)\b(nc|netcat)\s+-[^\s]*\s+\d+`),
		},
	}
}

func (rs *ResultScanner) Scan(toolName, result string) []ResultPolicyAdjustment {
	if result == "" {
		return nil
	}
	var out []ResultPolicyAdjustment
	now := time.Now().UTC().Format(time.RFC3339)

	for _, re := range rs.secretPatterns {
		if m := re.FindString(result); m != "" {
			out = append(out, ResultPolicyAdjustment{
				Trigger:   "secret_leak",
				Severity:  "high",
				Message:   "Potential secret leaked in tool output (matched: " + maskSecret(m) + ")",
				Action:    "block_write",
				ToolName:  toolName,
				Timestamp: now,
			})
		}
	}

	for _, re := range rs.destructivePatterns {
		if m := re.FindString(result); m != "" {
			out = append(out, ResultPolicyAdjustment{
				Trigger:   "destructive_op",
				Severity:  "medium",
				Message:   "Destructive operation detected in tool output",
				Action:    "require_confirm_destructive",
				ToolName:  toolName,
				Timestamp: now,
			})
		}
	}

	for _, re := range rs.egressPatterns {
		if m := re.FindString(result); m != "" {
			out = append(out, ResultPolicyAdjustment{
				Trigger:   "network_egress",
				Severity:  "low",
				Message:   "Network egress detected in tool output",
				Action:    "log_only",
				ToolName:  toolName,
				Timestamp: now,
			})
		}
	}

	return out
}

func maskSecret(s string) string {
	if len(s) <= 8 {
		return strings.Repeat("*", len(s))
	}
	return s[:4] + strings.Repeat("*", len(s)-8) + s[len(s)-4:]
}

type ResultPolicyStore struct {
	mu                     sync.Mutex
	entries                []ResultPolicyEntry
	blockWriteUntil        string
	confirmNextDestructive bool
	maxEntries             int
}

func NewResultPolicyStore(maxEntries int) *ResultPolicyStore {
	if maxEntries <= 0 {
		maxEntries = 1000
	}
	return &ResultPolicyStore{maxEntries: maxEntries}
}

func (s *ResultPolicyStore) Record(entry ResultPolicyEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, adj := range entry.Adjustments {
		switch adj.Action {
		case "block_write":
			ts := adj.Timestamp
			if ts == "" {
				ts = time.Now().UTC().Format(time.RFC3339)
			}
			s.blockWriteUntil = ts
		case "require_confirm_destructive":
			s.confirmNextDestructive = true
		}
	}

	s.entries = append(s.entries, entry)
	if len(s.entries) > s.maxEntries {
		s.entries = s.entries[len(s.entries)-s.maxEntries:]
	}
}

func (s *ResultPolicyStore) IsWriteBlocked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blockWriteUntil != "" {
		s.blockWriteUntil = ""
		return true
	}
	return false
}

func (s *ResultPolicyStore) NeedsDestructiveConfirm() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.confirmNextDestructive {
		s.confirmNextDestructive = false
		return true
	}
	return false
}

func (s *ResultPolicyStore) Entries() []ResultPolicyEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ResultPolicyEntry, len(s.entries))
	copy(out, s.entries)
	return out
}

func (s *ResultPolicyStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = nil
	s.blockWriteUntil = ""
	s.confirmNextDestructive = false
}
