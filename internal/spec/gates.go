// SPDX-License-Identifier: MIT
// Purpose: SDLC quality gates for spec verification and Agent Loop integration.
// Implements verification hooks for ensuring spec compliance before execution (Phase 3).
// Docs: internal/spec/gates.go.doc.md
package spec

import (
	"fmt"
	"strings"
	"time"
)

// Gate is the interface for all quality gate implementations.
type Gate interface {
	Name() string
	Run(spec *Spec, context *VerificationContext) *GateResult
	Critical() bool // True if gate failure blocks execution
}

// VerificationContext provides context for gate execution.
type VerificationContext struct {
	Collection    *SpecCollection
	TokenBudget   int
	TimeLimit     time.Duration
	AllowWarnings bool // If true, warnings don't block
}

// GateName represents the name of a built-in gate.
type GateName string

const (
	GateNameTokenBudget    GateName = "token_budget"
	GateNameMarkdownSyntax GateName = "markdown_syntax"
	GateNameDependencies   GateName = "dependencies"
	GateNameRequiredFields GateName = "required_fields"
	GateNameStatus         GateName = "status"
	GateNameTypeCheck      GateName = "type_check"
)

// ─────────────────────────────────────────────────────────────────────
// Token Budget Gate
// ─────────────────────────────────────────────────────────────────────

type TokenBudgetGate struct {
	MaxTokens int
}

func (g *TokenBudgetGate) Name() string {
	return string(GateNameTokenBudget)
}

func (g *TokenBudgetGate) Critical() bool {
	return true
}

func (g *TokenBudgetGate) Run(spec *Spec, ctx *VerificationContext) *GateResult {
	result := &GateResult{
		GateName:  g.Name(),
		Timestamp: time.Now(),
		Details:   make(map[string]interface{}),
	}

	totalBudget := ctx.TokenBudget
	if totalBudget == 0 {
		totalBudget = g.MaxTokens
	}

	if spec.TokenEstimate > totalBudget {
		result.Passed = false
		result.Message = fmt.Sprintf("Token estimate (%d) exceeds budget (%d)",
			spec.TokenEstimate, totalBudget)
		result.Details["estimated"] = spec.TokenEstimate
		result.Details["budget"] = totalBudget
		result.Details["overage"] = spec.TokenEstimate - totalBudget
		return result
	}

	result.Passed = true
	result.Message = fmt.Sprintf("Token estimate: %d / %d (%.0f%%)",
		spec.TokenEstimate, totalBudget, 
		(float64(spec.TokenEstimate)/float64(totalBudget))*100)
	result.Details["estimated"] = spec.TokenEstimate
	result.Details["budget"] = totalBudget
	result.Details["usage_percent"] = (float64(spec.TokenEstimate) / float64(totalBudget)) * 100

	return result
}

// ─────────────────────────────────────────────────────────────────────
// Markdown Syntax Gate
// ─────────────────────────────────────────────────────────────────────

type MarkdownSyntaxGate struct{}

func (g *MarkdownSyntaxGate) Name() string {
	return string(GateNameMarkdownSyntax)
}

func (g *MarkdownSyntaxGate) Critical() bool {
	return false
}

func (g *MarkdownSyntaxGate) Run(spec *Spec, ctx *VerificationContext) *GateResult {
	result := &GateResult{
		GateName:  g.Name(),
		Timestamp: time.Now(),
		Details:   make(map[string]interface{}),
	}

	var issues []string

	// Check for balanced markdown code blocks
	fields := []struct {
		name  string
		value string
	}{
		{"description", spec.Description},
		{"goals", spec.Goals},
		{"constraints", spec.Constraints},
		{"input", spec.Input},
		{"output", spec.Output},
		{"examples", spec.Examples},
	}

	for _, field := range fields {
		if field.value == "" {
			continue
		}

		codeBlockCount := strings.Count(field.value, "```")
		if codeBlockCount%2 != 0 {
			issues = append(issues, fmt.Sprintf("%s has unbalanced code blocks", field.name))
		}

		// Check for unclosed markdown links
		openLinks := strings.Count(field.value, "[")
		closeLinks := strings.Count(field.value, "]")
		if openLinks != closeLinks {
			issues = append(issues, fmt.Sprintf("%s has unbalanced links", field.name))
		}
	}

	if len(issues) > 0 {
		result.Passed = false
		result.Message = fmt.Sprintf("Markdown syntax issues: %s", strings.Join(issues, "; "))
		result.Details["issues"] = issues
		result.Details["count"] = len(issues)
	} else {
		result.Passed = true
		result.Message = "Markdown syntax valid"
		result.Details["count"] = 0
	}

	return result
}

// ─────────────────────────────────────────────────────────────────────
// Dependencies Gate
// ─────────────────────────────────────────────────────────────────────

type DependenciesGate struct{}

func (g *DependenciesGate) Name() string {
	return string(GateNameDependencies)
}

func (g *DependenciesGate) Critical() bool {
	return true
}

func (g *DependenciesGate) Run(spec *Spec, ctx *VerificationContext) *GateResult {
	result := &GateResult{
		GateName:  g.Name(),
		Timestamp: time.Now(),
		Details:   make(map[string]interface{}),
	}

	var issues []string

	// Check all dependencies exist
	for _, depID := range spec.Dependencies {
		if _, ok := ctx.Collection.Specs[depID]; !ok {
			issues = append(issues, fmt.Sprintf("missing dependency: %s", depID))
		}
	}

	// Check for circular dependencies
	visited := make(map[string]bool)
	var hasCircle func(string) bool
	hasCircle = func(id string) bool {
		if visited[id] {
			return true
		}
		visited[id] = true

		for _, depID := range spec.Dependencies {
			if depID == spec.ID {
				return true
			}
		}

		return false
	}

	if hasCircle(spec.ID) {
		issues = append(issues, "circular dependency detected")
	}

	if len(issues) > 0 {
		result.Passed = false
		result.Message = fmt.Sprintf("Dependency issues: %s", strings.Join(issues, "; "))
		result.Details["issues"] = issues
		result.Details["count"] = len(issues)
	} else {
		result.Passed = true
		result.Message = fmt.Sprintf("Dependencies valid: %d dependency/ies", len(spec.Dependencies))
		result.Details["count"] = len(spec.Dependencies)
	}

	return result
}

// ─────────────────────────────────────────────────────────────────────
// Required Fields Gate
// ─────────────────────────────────────────────────────────────────────

type RequiredFieldsGate struct {
	RequiredFields []string
}

func (g *RequiredFieldsGate) Name() string {
	return string(GateNameRequiredFields)
}

func (g *RequiredFieldsGate) Critical() bool {
	return true
}

func (g *RequiredFieldsGate) Run(spec *Spec, ctx *VerificationContext) *GateResult {
	result := &GateResult{
		GateName:  g.Name(),
		Timestamp: time.Now(),
		Details:   make(map[string]interface{}),
	}

	var missing []string

	fieldMap := map[string]string{
		"title":       spec.Title,
		"description": spec.Description,
		"goals":       spec.Goals,
		"kind":        string(spec.Kind),
	}

	for _, field := range g.RequiredFields {
		if value, ok := fieldMap[field]; !ok || strings.TrimSpace(value) == "" {
			missing = append(missing, field)
		}
	}

	if len(missing) > 0 {
		result.Passed = false
		result.Message = fmt.Sprintf("Missing required fields: %s", strings.Join(missing, ", "))
		result.Details["missing"] = missing
		result.Details["count"] = len(missing)
	} else {
		result.Passed = true
		result.Message = "All required fields present"
		result.Details["count"] = 0
	}

	return result
}

// ─────────────────────────────────────────────────────────────────────
// Status Gate
// ─────────────────────────────────────────────────────────────────────

type StatusGate struct {
	AllowedStatuses map[SpecStatus]bool
}

func (g *StatusGate) Name() string {
	return string(GateNameStatus)
}

func (g *StatusGate) Critical() bool {
	return true
}

func (g *StatusGate) Run(spec *Spec, ctx *VerificationContext) *GateResult {
	result := &GateResult{
		GateName:  g.Name(),
		Timestamp: time.Now(),
		Details:   make(map[string]interface{}),
	}

	if len(g.AllowedStatuses) == 0 {
		// Default: only active specs are allowed
		g.AllowedStatuses = map[SpecStatus]bool{SpecStatusActive: true}
	}

	if g.AllowedStatuses[spec.Status] {
		result.Passed = true
		result.Message = fmt.Sprintf("Status valid: %s", spec.Status)
		result.Details["status"] = string(spec.Status)
	} else {
		result.Passed = false
		result.Message = fmt.Sprintf("Status not allowed: %s", spec.Status)
		result.Details["status"] = string(spec.Status)
		result.Details["allowed"] = make([]string, 0)
		for status := range g.AllowedStatuses {
			result.Details["allowed"] = append(result.Details["allowed"].([]string), string(status))
		}
	}

	return result
}

// ─────────────────────────────────────────────────────────────────────
// Gate Registry & Runner
// ─────────────────────────────────────────────────────────────────────

// GateRegistry holds all registered gates for a collection.
type GateRegistry struct {
	gates map[string]Gate
}

// NewGateRegistry creates a new gate registry with default gates.
func NewGateRegistry() *GateRegistry {
	registry := &GateRegistry{
		gates: make(map[string]Gate),
	}

	// Register default gates
	registry.Register(&TokenBudgetGate{MaxTokens: 100000})
	registry.Register(&MarkdownSyntaxGate{})
	registry.Register(&DependenciesGate{})
	registry.Register(&RequiredFieldsGate{
		RequiredFields: []string{"title", "description", "goals"},
	})
	registry.Register(&StatusGate{
		AllowedStatuses: map[SpecStatus]bool{SpecStatusActive: true},
	})

	return registry
}

// Register adds a gate to the registry.
func (gr *GateRegistry) Register(gate Gate) {
	gr.gates[gate.Name()] = gate
}

// Run executes all registered gates for a spec.
func (gr *GateRegistry) Run(spec *Spec, ctx *VerificationContext) VerificationResults {
	results := VerificationResults{
		SpecID:    spec.ID,
		Timestamp: time.Now(),
		Results:   make(map[string]*GateResult),
	}

	for name, gate := range gr.gates {
		gateResult := gate.Run(spec, ctx)
		results.Results[name] = gateResult

		if !gateResult.Passed && gate.Critical() {
			results.HasCriticalFailure = true
		}
	}

	results.Passed = !results.HasCriticalFailure

	return results
}

// ─────────────────────────────────────────────────────────────────────
// Verification Results
// ─────────────────────────────────────────────────────────────────────

// VerificationResults holds the results of all gates for a spec.
type VerificationResults struct {
	SpecID              string
	Timestamp           time.Time
	Results             map[string]*GateResult
	Passed              bool
	HasCriticalFailure  bool
}

// Summary returns a brief summary of verification results.
func (vr VerificationResults) Summary() string {
	passed := 0
	failed := 0

	for _, result := range vr.Results {
		if result.Passed {
			passed++
		} else {
			failed++
		}
	}

	if vr.Passed {
		return fmt.Sprintf("✓ %s: All %d gates passed", vr.SpecID, passed)
	}
	return fmt.Sprintf("✗ %s: %d passed, %d failed", vr.SpecID, passed, failed)
}

// Details returns a detailed report of all gate results.
func (vr VerificationResults) Details() string {
	var result strings.Builder
	result.WriteString(vr.Summary())
	result.WriteString("\n")

	for name, gateResult := range vr.Results {
		marker := "✓"
		if !gateResult.Passed {
			marker = "✗"
		}
		result.WriteString(fmt.Sprintf("  %s [%s] %s\n", marker, name, gateResult.Message))
	}

	return result.String()
}
