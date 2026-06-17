// SPDX-License-Identifier: MIT
// Purpose: YOLO risk classifier (issue #272). When --yolo is active the
// permission engine consults this classifier instead of blanket-approving
// every Ask-level tool. Tools are classified into Low/Medium/High/Critical
// and only operations at or below the configured threshold are auto-approved.
package permission

import (
	"strings"
	"sync"
)

type RiskLevel int

const (
	RiskLow RiskLevel = iota
	RiskMedium
	RiskHigh
	RiskCritical
)

func (r RiskLevel) String() string {
	switch r {
	case RiskLow:
		return "low"
	case RiskMedium:
		return "medium"
	case RiskHigh:
		return "high"
	case RiskCritical:
		return "critical"
	default:
		return "unknown"
	}
}

func ParseRiskLevel(s string) (RiskLevel, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "low":
		return RiskLow, nil
	case "medium", "":
		return RiskMedium, nil
	case "high":
		return RiskHigh, nil
	case "critical":
		return RiskCritical, nil
	default:
		return RiskMedium, fmtErrorf("permission: unknown risk level %q", s)
	}
}

var (
	lowRiskTools = map[string]bool{
		"read": true, "ls": true, "glob": true, "grep": true,
		"cat": true, "head": true, "tail": true, "find": true,
		"scout": true, "discover": true, "map": true, "grasp": true,
		"harvest": true, "sckg": true, "oracle": true, "efm": true,
		"poc": true, "adw": true, "ibd": true,
		"sin_read": true, "sin_scout": true, "sin_discover": true,
		"sin_map": true, "sin_grasp": true, "sin_harvest": true,
		"sin_memory_search": true, "sin_memory_list": true,
		"sin_memory_stats": true, "sin_memory_prime": true,
		"git_status": true, "git_log": true, "git_diff": true,
		"git_show": true, "git_branch": true,
	}

	mediumRiskTools = map[string]bool{
		"edit": true, "write": true, "sin_edit": true, "sin_write": true,
		"sin_test": true, "test": true, "multi_edit": true,
		"sin_quality_gate": true, "sin_mutation": true,
		"sin_fuzz": true, "sin_property": true,
	}

	highRiskTools = map[string]bool{
		"execute": true, "bash": true, "sin_bash": true, "shell": true,
		"sin_git_commit": true, "git_commit": true,
		"sin_test_generate": true, "test_generate": true,
		"install": true, "sin_install": true,
	}

	criticalRiskTools = map[string]bool{
		"sin_browser_navigate": true, "browser_navigate": true,
		"rm": true, "delete": true, "remove": true,
		"sin_git_push": true, "git_push": true,
		"sin_delete": true, "sin_rm": true,
	}
)

var criticalArgPatterns = []string{
	"rm -rf", "rm -r ", "rm -f ", "rmdir",
	"sudo ", "git push --force", "git push -f",
	"git reset --hard", "git reset --soft",
	"curl", "| sh", "| bash", "|sh", "|bash",
	"dd if=", "mkfs", "chmod 777",
	"drop table", "drop database", "truncate",
	":(){:|:&};:",
}

type RiskClassifier struct {
	mu        sync.Mutex
	threshold RiskLevel
}

func NewRiskClassifier() *RiskClassifier {
	return &RiskClassifier{threshold: RiskMedium}
}

func (c *RiskClassifier) SetThreshold(level RiskLevel) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.threshold = level
}

func (c *RiskClassifier) Threshold() RiskLevel {
	if c == nil {
		return RiskMedium
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.threshold
}

func (c *RiskClassifier) Classify(toolName string, args map[string]any) RiskLevel {
	if c == nil {
		return RiskMedium
	}
	if args != nil {
		if level := classifyArgs(args); level == RiskCritical {
			return RiskCritical
		}
	}
	return classifyTool(toolName)
}

func classifyTool(toolName string) RiskLevel {
	t := strings.ToLower(strings.TrimSpace(toolName))
	if t == "" {
		return RiskMedium
	}
	if criticalRiskTools[t] {
		return RiskCritical
	}
	for prefix := range criticalRiskTools {
		if strings.HasPrefix(t, prefix) {
			return RiskCritical
		}
	}
	if strings.Contains(t, "force") || strings.Contains(t, "reset") {
		return RiskCritical
	}
	if strings.Contains(t, "delete") || strings.Contains(t, "remove") {
		return RiskCritical
	}
	if highRiskTools[t] {
		return RiskHigh
	}
	for prefix := range highRiskTools {
		if strings.HasPrefix(t, prefix) {
			return RiskHigh
		}
	}
	if mediumRiskTools[t] {
		return RiskMedium
	}
	for prefix := range mediumRiskTools {
		if strings.HasPrefix(t, prefix) {
			return RiskMedium
		}
	}
	if lowRiskTools[t] {
		return RiskLow
	}
	for prefix := range lowRiskTools {
		if strings.HasPrefix(t, prefix) {
			return RiskLow
		}
	}
	return RiskMedium
}

func classifyArgs(args map[string]any) RiskLevel {
	for _, key := range []string{"command", "cmd", "script", "args", "query", "input"} {
		if v, ok := args[key]; ok {
			if s := anyToString(v); s != "" {
				lower := strings.ToLower(s)
				for _, pat := range criticalArgPatterns {
					if strings.Contains(lower, pat) {
						return RiskCritical
					}
				}
				if strings.Contains(lower, "force") || strings.Contains(lower, "reset") {
					return RiskCritical
				}
			}
		}
	}
	return RiskLow
}

func anyToString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case []any:
		var parts []string
		for _, item := range val {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " ")
	case []string:
		return strings.Join(val, " ")
	default:
		return ""
	}
}
