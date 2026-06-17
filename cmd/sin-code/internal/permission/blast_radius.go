// SPDX-License-Identifier: MIT
// Purpose: Blast-radius estimation for tool calls (issue #322). Estimates
// how far the effects of a tool call could propagate — from a single file
// (sin_edit) to the entire system (rm -rf). The radius feeds into the
// risk classifier so the permission engine can upgrade allow → ask when
// a call has a large blast radius.
//
// Thread-safe (mandate M7).
package permission

import (
	"strings"
	"sync"
)

// RadiusLevel classifies the blast radius of a tool call.
type RadiusLevel int

const (
	// RadiusNone means the call is read-only with no side effects.
	RadiusNone RadiusLevel = iota
	// RadiusFile means the call affects a single file.
	RadiusFile
	// RadiusPackage means the call affects a package/directory scope.
	RadiusPackage
	// RadiusModule means the call affects an entire module or repo.
	RadiusModule
	// RadiusSystem means the call affects the system or is irreversible.
	RadiusSystem
)

func (l RadiusLevel) String() string {
	switch l {
	case RadiusNone:
		return "none"
	case RadiusFile:
		return "file"
	case RadiusPackage:
		return "package"
	case RadiusModule:
		return "module"
	case RadiusSystem:
		return "system"
	default:
		return "unknown"
	}
}

// ParseRadiusLevel converts a string to a RadiusLevel. Returns RadiusNone
// and false for unknown strings.
func ParseRadiusLevel(s string) (RadiusLevel, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "none":
		return RadiusNone, true
	case "file":
		return RadiusFile, true
	case "package":
		return RadiusPackage, true
	case "module":
		return RadiusModule, true
	case "system":
		return RadiusSystem, true
	default:
		return RadiusNone, false
	}
}

// BlastRadius estimates the blast radius of tool calls based on the tool
// name and its arguments. It is stateless beyond an optional cache; the
// estimation rules are deterministic.
type BlastRadius struct {
	mu sync.Mutex
}

// NewBlastRadius creates a new BlastRadius estimator.
func NewBlastRadius() *BlastRadius {
	return &BlastRadius{}
}

// Estimate returns the blast radius for a tool call. The estimation uses
// both the tool name and the arguments (e.g. sin_bash with "go test" is
// RadiusPackage, but sin_bash with "rm -rf" is RadiusSystem).
func (b *BlastRadius) Estimate(toolName string, args map[string]any) RadiusLevel {
	if b == nil {
		return RadiusFile
	}
	t := strings.ToLower(strings.TrimSpace(toolName))
	if t == "" {
		return RadiusFile
	}

	switch t {
	case "sin_read", "sin_scout", "sin_discover", "sin_map", "sin_grasp",
		"sin_harvest", "sin_memory_search", "sin_memory_list",
		"read", "scout", "discover", "map", "grasp", "harvest",
		"glob", "grep", "ls", "cat", "head", "tail", "find":
		return RadiusNone

	case "sin_edit", "sin_write", "edit", "write", "multi_edit":
		return RadiusFile

	case "sin_git_commit", "git_commit":
		return RadiusModule

	case "sin_git_push", "git_push":
		return RadiusSystem

	case "sin_bash", "bash", "execute", "shell":
		return estimateBashRadius(args)

	case "sin_git_reset", "git_reset":
		return RadiusSystem

	case "sin_delete", "sin_rm", "rm", "delete", "remove":
		return RadiusSystem
	}

	// Unknown tools default to file-level radius (conservative).
	return RadiusFile
}

// estimateBashRadius inspects bash command arguments to determine the
// blast radius. The command string is looked up in common argument keys.
func estimateBashRadius(args map[string]any) RadiusLevel {
	cmd := extractCommand(args)
	if cmd == "" {
		return RadiusPackage
	}
	lower := strings.ToLower(cmd)

	if strings.Contains(lower, "rm -rf") || strings.Contains(lower, "rm -r ") {
		return RadiusSystem
	}
	if strings.Contains(lower, "sudo") {
		return RadiusSystem
	}
	if strings.Contains(lower, "git push") {
		return RadiusSystem
	}
	if strings.Contains(lower, "git reset --hard") {
		return RadiusSystem
	}
	if strings.Contains(lower, "chmod 777") || strings.Contains(lower, "mkfs") {
		return RadiusSystem
	}
	if strings.Contains(lower, "dd if=") {
		return RadiusSystem
	}

	if strings.Contains(lower, "go build") || strings.Contains(lower, "go install") {
		return RadiusModule
	}
	if strings.Contains(lower, "make") && !strings.Contains(lower, "test") {
		return RadiusModule
	}
	if strings.Contains(lower, "npm install") || strings.Contains(lower, "npm ci") {
		return RadiusModule
	}
	if strings.Contains(lower, "pip install") {
		return RadiusModule
	}

	if strings.Contains(lower, "go test") || strings.Contains(lower, "go vet") {
		return RadiusPackage
	}
	if strings.Contains(lower, "npm test") || strings.Contains(lower, "npm run test") {
		return RadiusPackage
	}
	if strings.Contains(lower, "pytest") || strings.Contains(lower, "cargo test") {
		return RadiusPackage
	}

	return RadiusPackage
}

// extractCommand pulls the command string from common argument keys.
func extractCommand(args map[string]any) string {
	if args == nil {
		return ""
	}
	for _, key := range []string{"command", "cmd", "script", "args", "query", "input"} {
		if v, ok := args[key]; ok {
			if s := anyToString(v); s != "" {
				return s
			}
		}
	}
	return ""
}

// Description returns a human-readable description of the radius level.
func (b *BlastRadius) Description(level RadiusLevel) string {
	switch level {
	case RadiusNone:
		return "read-only, no side effects"
	case RadiusFile:
		return "affects a single file"
	case RadiusPackage:
		return "affects a package or directory"
	case RadiusModule:
		return "affects an entire module or repository"
	case RadiusSystem:
		return "affects the system or is irreversible"
	default:
		return "unknown blast radius"
	}
}

// Score maps a RadiusLevel to a 0.0–1.0 risk score. Higher radius = higher
// score.
func (b *BlastRadius) Score(level RadiusLevel) float64 {
	switch level {
	case RadiusNone:
		return 0.0
	case RadiusFile:
		return 0.2
	case RadiusPackage:
		return 0.4
	case RadiusModule:
		return 0.7
	case RadiusSystem:
		return 0.95
	default:
		return 0.5
	}
}

// ToRiskLevel converts a RadiusLevel to the corresponding RiskLevel used
// by the RiskClassifier. This is the integration point between blast
// radius and the existing risk classification system.
func (b *BlastRadius) ToRiskLevel(level RadiusLevel) RiskLevel {
	switch level {
	case RadiusNone:
		return RiskLow
	case RadiusFile:
		return RiskMedium
	case RadiusPackage:
		return RiskMedium
	case RadiusModule:
		return RiskHigh
	case RadiusSystem:
		return RiskCritical
	default:
		return RiskMedium
	}
}

// ClassifyWithBlast combines the existing RiskClassifier with the
// BlastRadius estimator, returning the higher of the two risk levels.
// This lets the permission engine upgrade allow → ask when a call has
// a large blast radius even if the tool name alone would be low-risk.
func ClassifyWithBlast(rc *RiskClassifier, br *BlastRadius, toolName string, args map[string]any) RiskLevel {
	if rc == nil && br == nil {
		return RiskMedium
	}
	var riskFromClassifier RiskLevel
	if rc != nil {
		riskFromClassifier = rc.Classify(toolName, args)
	}
	var riskFromBlast RiskLevel
	if br != nil {
		radius := br.Estimate(toolName, args)
		riskFromBlast = br.ToRiskLevel(radius)
	}
	if riskFromBlast > riskFromClassifier {
		return riskFromBlast
	}
	return riskFromClassifier
}

// ScoreWithBlast combines the blast-radius score with the risk-classifier
// score, returning the higher of the two as a 0.0–1.0 float.
func ScoreWithBlast(rc *RiskClassifier, br *BlastRadius, toolName string, args map[string]any) float64 {
	var classifierScore float64
	if rc != nil {
		classifierScore = riskLevelToScore(rc.Classify(toolName, args))
	}
	var blastScore float64
	if br != nil {
		blastScore = br.Score(br.Estimate(toolName, args))
	}
	if blastScore > classifierScore {
		return blastScore
	}
	return classifierScore
}

// riskLevelToScore maps a RiskLevel to a 0.0–1.0 score.
func riskLevelToScore(level RiskLevel) float64 {
	switch level {
	case RiskLow:
		return 0.1
	case RiskMedium:
		return 0.3
	case RiskHigh:
		return 0.7
	case RiskCritical:
		return 0.95
	default:
		return 0.5
	}
}
