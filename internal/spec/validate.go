// SPDX-License-Identifier: MIT
// Purpose: Spec validation rules and error handling for the Spec Layer.
// All validation is deterministic and synchronous — no LLM calls.
// Docs: internal/spec/validate.go.doc.md
package spec

import (
	"fmt"
	"strings"
)

// ValidationError represents a single validation failure.
type ValidationError struct {
	SpecID  string
	Field   string
	Message string
}

// ValidationResult holds the outcome of spec validation.
type ValidationResult struct {
	Valid  bool
	Errors []ValidationError
}

// ValidationRules defines the strict rules for spec content.
var ValidationRules = struct {
	MinTitleLength    int
	MaxTitleLength    int
	MinDescriptionLen int
	RequiredFields    []string
}{
	MinTitleLength:    3,
	MaxTitleLength:    200,
	MinDescriptionLen: 10,
	RequiredFields:    []string{"title", "description", "goals"},
}

// ValidateSpec performs comprehensive validation on a spec.
// Returns a ValidationResult with all errors found (non-blocking).
func ValidateSpec(s *Spec) ValidationResult {
	var errors []ValidationError

	// Validate ID
	if strings.TrimSpace(s.ID) == "" {
		errors = append(errors, ValidationError{
			SpecID:  s.ID,
			Field:   "id",
			Message: "ID cannot be empty",
		})
	}

	// Validate Title
	if len(s.Title) < ValidationRules.MinTitleLength {
		errors = append(errors, ValidationError{
			SpecID:  s.ID,
			Field:   "title",
			Message: fmt.Sprintf("Title must be at least %d characters", ValidationRules.MinTitleLength),
		})
	}
	if len(s.Title) > ValidationRules.MaxTitleLength {
		errors = append(errors, ValidationError{
			SpecID:  s.ID,
			Field:   "title",
			Message: fmt.Sprintf("Title must not exceed %d characters", ValidationRules.MaxTitleLength),
		})
	}

	// Validate Kind
	validKinds := map[SpecKind]bool{
		SpecKindGoal:        true,
		SpecKindProcess:     true,
		SpecKindConstraint:  true,
		SpecKindComponent:   true,
		SpecKindIntegration: true,
	}
	if !validKinds[s.Kind] {
		errors = append(errors, ValidationError{
			SpecID:  s.ID,
			Field:   "kind",
			Message: fmt.Sprintf("Invalid spec kind: %s", s.Kind),
		})
	}

	// Validate Description
	if len(s.Description) < ValidationRules.MinDescriptionLen {
		errors = append(errors, ValidationError{
			SpecID:  s.ID,
			Field:   "description",
			Message: fmt.Sprintf("Description must be at least %d characters", ValidationRules.MinDescriptionLen),
		})
	}

	// Validate Goals (required for active specs)
	if s.Status == SpecStatusActive && strings.TrimSpace(s.Goals) == "" {
		errors = append(errors, ValidationError{
			SpecID:  s.ID,
			Field:   "goals",
			Message: "Goals are required for active specs",
		})
	}

	// Validate Status
	validStatuses := map[SpecStatus]bool{
		SpecStatusDraft:    true,
		SpecStatusActive:   true,
		SpecStatusArchived: true,
	}
	if !validStatuses[s.Status] {
		errors = append(errors, ValidationError{
			SpecID:  s.ID,
			Field:   "status",
			Message: fmt.Sprintf("Invalid status: %s", s.Status),
		})
	}

	return ValidationResult{
		Valid:  len(errors) == 0,
		Errors: errors,
	}
}

// ValidateDependencies checks for cycles and missing dependencies in a collection.
func ValidateDependencies(collection *SpecCollection) ValidationResult {
	var errors []ValidationError

	// Build adjacency map for cycle detection
	adjMap := make(map[string][]string)
	for id, spec := range collection.Specs {
		adjMap[id] = spec.Dependencies
	}

	// DFS-based cycle detection
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var hasCycle func(string) bool
	hasCycle = func(id string) bool {
		visited[id] = true
		recStack[id] = true

		for _, dep := range adjMap[id] {
			if !visited[dep] {
				if hasCycle(dep) {
					return true
				}
			} else if recStack[dep] {
				return true
			}
		}

		recStack[id] = false
		return false
	}

	// Check each spec for cycles
	for id := range collection.Specs {
		if !visited[id] {
			if hasCycle(id) {
				errors = append(errors, ValidationError{
					SpecID:  id,
					Field:   "dependencies",
					Message: "Cycle detected in dependency graph",
				})
			}
		}
	}

	// Check for missing dependencies
	for id, spec := range collection.Specs {
		for _, dep := range spec.Dependencies {
			if _, exists := collection.Specs[dep]; !exists {
				errors = append(errors, ValidationError{
					SpecID:  id,
					Field:   "dependencies",
					Message: fmt.Sprintf("Missing dependency: %s", dep),
				})
			}
		}
	}

	return ValidationResult{
		Valid:  len(errors) == 0,
		Errors: errors,
	}
}

// ValidateTokenBudget checks if total token estimate is within acceptable range.
func ValidateTokenBudget(collection *SpecCollection, maxTotal int) ValidationResult {
	var errors []ValidationError

	if collection.Statistics.TotalTokenEstimate > maxTotal {
		errors = append(errors, ValidationError{
			SpecID:  collection.ID,
			Field:   "token_budget",
			Message: fmt.Sprintf("Total token estimate (%d) exceeds budget (%d)",
				collection.Statistics.TotalTokenEstimate, maxTotal),
		})
	}

	return ValidationResult{
		Valid:  len(errors) == 0,
		Errors: errors,
	}
}

// ValidateMarkdown checks markdown fields for basic syntax.
func ValidateMarkdown(s *Spec) ValidationResult {
	var errors []ValidationError

	fields := map[string]string{
		"description": s.Description,
		"goals":       s.Goals,
		"constraints": s.Constraints,
		"input":       s.Input,
		"output":      s.Output,
		"examples":    s.Examples,
	}

	for field, content := range fields {
		if content == "" {
			continue // Skip empty fields
		}

		// Basic checks: ensure headers are present for structured fields
		if field == "examples" && !strings.Contains(content, "```") {
			// Examples should ideally have code blocks
			errors = append(errors, ValidationError{
				SpecID:  s.ID,
				Field:   field,
				Message: fmt.Sprintf("%s field should contain code examples (missing ``` blocks)", field),
			})
		}
	}

	return ValidationResult{
		Valid:  len(errors) == 0,
		Errors: errors,
	}
}

// String returns a formatted error message for ValidationError.
func (ve ValidationError) String() string {
	return fmt.Sprintf("[%s] %s: %s", ve.SpecID, ve.Field, ve.Message)
}

// Summary returns a concise text summary of validation errors.
func (vr ValidationResult) Summary() string {
	if vr.Valid {
		return "✓ All validations passed"
	}
	return fmt.Sprintf("✗ %d validation error(s)", len(vr.Errors))
}

// Details returns a detailed multi-line error report.
func (vr ValidationResult) Details() string {
	var sb strings.Builder
	sb.WriteString(vr.Summary())
	sb.WriteString("\n")
	for _, err := range vr.Errors {
		sb.WriteString(fmt.Sprintf("  • %s\n", err.String()))
	}
	return sb.String()
}
