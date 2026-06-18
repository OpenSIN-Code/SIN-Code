// SPDX-License-Identifier: MIT
// Purpose: Unified edit policy that aligns chat sin_edit semantics with
// MCP sin_edit (issue #373). EditPolicy provides consistent validation and
// permission checking for edit operations across both surfaces. This
// prevents the divergence where chat's naive string replace and MCP's
// surgical editor had different rules.
package permission

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// EditPolicy is the unified policy for edit operations across chat and MCP.
// It is stateless and safe for concurrent use.
type EditPolicy struct {
	// WorkspaceRoot is the root directory that all edits must be within.
	// If empty, workspace boundary checks are skipped.
	WorkspaceRoot string
	// MaxLinesForAllow is the maximum diff size (in lines) that is
	// auto-allowed without asking. Edits larger than this require
	// confirmation. Default is 100.
	MaxLinesForAllow int
}

// EditOperation describes a single edit to validate and check.
type EditOperation struct {
	Type      string // "string_replace", "anchor", "symbol", "insert", "delete"
	FilePath  string
	OldString string
	NewString string
}

// NewEditPolicy creates an EditPolicy with the given workspace root and
// sensible defaults.
func NewEditPolicy(workspaceRoot string) *EditPolicy {
	return &EditPolicy{
		WorkspaceRoot:    workspaceRoot,
		MaxLinesForAllow: 100,
	}
}

// Validate checks that the operation is structurally valid. It verifies:
//   - FilePath is non-empty
//   - Type is a recognised edit type
//   - For string_replace: OldString is non-empty
//   - For insert: NewString is non-empty
//   - For delete: OldString is non-empty (the content to remove)
func (p *EditPolicy) Validate(op EditOperation) error {
	if op.FilePath == "" {
		return errors.New("edit: file path is required")
	}

	validTypes := map[string]bool{
		"string_replace": true,
		"anchor":         true,
		"symbol":         true,
		"insert":         true,
		"delete":         true,
	}
	if !validTypes[op.Type] {
		return fmt.Errorf("edit: unknown operation type %q", op.Type)
	}

	switch op.Type {
	case "string_replace":
		if op.OldString == "" {
			return errors.New("edit: string_replace requires non-empty OldString")
		}
	case "insert":
		if op.NewString == "" {
			return errors.New("edit: insert requires non-empty NewString")
		}
	case "delete":
		if op.OldString == "" {
			return errors.New("edit: delete requires non-empty OldString (content to remove)")
		}
	case "anchor", "symbol":
		// Anchor/symbol modes may have empty OldString if the anchor
		// alone is sufficient — validation is more relaxed here.
	}

	return nil
}

// CheckPermission returns "allow", "ask", or "deny" for the given operation.
// The decision tree (checked in order):
//  1. Deny if the file is a .env file or in a secrets directory.
//  2. Deny if the file path is outside the workspace root.
//  3. Ask if the diff is large (> MaxLinesForAllow lines).
//  4. Allow otherwise (small, in-workspace, non-secret edit).
//
// If the operation fails Validate, CheckPermission returns "deny".
func (p *EditPolicy) CheckPermission(op EditOperation) string {
	if err := p.Validate(op); err != nil {
		return "deny"
	}

	// Rule 1: deny on .env files and secret-looking paths.
	if isSecretFile(op.FilePath) {
		return "deny"
	}

	// Rule 2: deny on paths outside workspace.
	if p.WorkspaceRoot != "" {
		if !isWithinWorkspace(op.FilePath, p.WorkspaceRoot) {
			return "deny"
		}
	}

	// Rule 3: ask on large diffs.
	maxLines := p.MaxLinesForAllow
	if maxLines <= 0 {
		maxLines = 100
	}
	if diffLineCount(op.OldString, op.NewString) > maxLines {
		return "ask"
	}

	// Rule 4: allow small in-workspace edits.
	return "allow"
}

// isSecretFile returns true if the file path looks like a secrets file.
func isSecretFile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	if strings.HasPrefix(base, ".env") {
		return true
	}
	// Common secret file patterns.
	secretNames := []string{
		".env",
		".env.local",
		".env.production",
		".env.staging",
		".env.development",
		"credentials.json",
		"secrets.json",
		"secrets.yaml",
		"secrets.yml",
	}
	for _, s := range secretNames {
		if base == s {
			return true
		}
	}
	// Paths containing a .secrets or .env directory component.
	cleaned := filepath.ToSlash(path)
	parts := strings.Split(cleaned, "/")
	for _, part := range parts {
		if part == ".env" || part == ".secrets" || part == "secrets" {
			return true
		}
	}
	return false
}

// isWithinWorkspace checks whether the given file path is inside the
// workspace root. Both paths are cleaned and made absolute for comparison.
func isWithinWorkspace(filePath, workspaceRoot string) bool {
	cleanFile := filepath.Clean(filePath)
	cleanRoot := filepath.Clean(workspaceRoot)

	// If the file is already absolute, check prefix directly.
	if filepath.IsAbs(cleanFile) {
		rel, err := filepath.Rel(cleanRoot, cleanFile)
		if err != nil {
			return false
		}
		return !strings.HasPrefix(rel, "..")
	}

	// For relative paths, join with root and check.
	abs := filepath.Join(cleanRoot, cleanFile)
	rel, err := filepath.Rel(cleanRoot, abs)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..")
}

// diffLineCount returns the total number of lines in the old and new
// strings combined, which is a simple heuristic for diff size.
func diffLineCount(old, new string) int {
	count := 0
	if old != "" {
		count += strings.Count(old, "\n")
		if !strings.HasSuffix(old, "\n") {
			count++
		}
	}
	if new != "" {
		count += strings.Count(new, "\n")
		if !strings.HasSuffix(new, "\n") {
			count++
		}
	}
	return count
}
