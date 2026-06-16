// SPDX-License-Identifier: MIT
// Purpose: validate a parsed Config. The validator returns a
// ValidationError for each problem it finds, with the field
// path. The CLI stops at the first error (one problem at a
// time is the operator-friendly default); a future CLI flag
// can fan out into "all problems".
package compiler

import (
	"fmt"
	"strings"
)

// ValidationError describes one problem in the config.
type ValidationError struct {
	Path    string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// ValidProjectTypes are the values accepted by project.type.
var ValidProjectTypes = map[string]bool{
	"go":        true,
	"python":    true,
	"rust":      true,
	"node":      true,
	"polyglot":  true,
}

// ValidVerifyModes are the values accepted by verify.mode.
var ValidVerifyModes = map[string]bool{
	"minimal":  true,
	"standard": true,
	"strict":   true,
}

// ValidVerifyModeValues is the sorted list, used in error
// messages so the operator sees the allowed values.
var ValidVerifyModeValues = sortedKeys(ValidVerifyModes)

// ValidProjectTypeValues is the sorted list.
var ValidProjectTypeValues = sortedKeys(ValidProjectTypes)

// Validate enforces the schema rules. Returns the first error
// found, or nil.
func Validate(c *Config) error {
	if c == nil {
		return &ValidationError{Path: "<root>", Message: "config is nil"}
	}
	// Version is required to be 1 (the only supported version).
	if c.Version == 0 {
		return &ValidationError{Path: "version", Message: "required (must be 1)"}
	}
	if c.Version != SchemaVersion {
		return &ValidationError{
			Path:    "version",
			Message: fmt.Sprintf("unsupported version %d (expected %d)", c.Version, SchemaVersion),
		}
	}
	// Project: type must be a known value.
	if c.Project.Type != "" && !ValidProjectTypes[c.Project.Type] {
		return &ValidationError{
			Path:    "project.type",
			Message: fmt.Sprintf("invalid value %q (expected one of %s)",
				c.Project.Type, strings.Join(ValidProjectTypeValues, ", ")),
		}
	}
	// Verify: mode must be a known value.
	if c.Verify.Mode != "" && !ValidVerifyModes[c.Verify.Mode] {
		return &ValidationError{
			Path:    "verify.mode",
			Message: fmt.Sprintf("invalid value %q (expected one of %s)",
				c.Verify.Mode, strings.Join(ValidVerifyModeValues, ", ")),
		}
	}
	// Verify: predicate names must be unique and non-empty.
	seen := map[string]bool{}
	for i, p := range c.Verify.Predicates {
		if p.Name == "" {
			return &ValidationError{
				Path:    fmt.Sprintf("verify.predicates[%d].name", i),
				Message: "required",
			}
		}
		if p.Command == "" {
			return &ValidationError{
				Path:    fmt.Sprintf("verify.predicates[%d].command", i),
				Message: "required",
			}
		}
		if seen[p.Name] {
			return &ValidationError{
				Path: fmt.Sprintf("verify.predicates[%d].name", i),
				Message: fmt.Sprintf("duplicate name %q", p.Name),
			}
		}
		seen[p.Name] = true
	}
	// Hooks: hook names must be unique across pre- and post-tool.
	hookSeen := map[string]bool{}
	checkHook := func(group string, hooks []Hook) error {
		for i, h := range hooks {
			if h.Name == "" {
				return &ValidationError{
					Path:    fmt.Sprintf("hooks.%s[%d].name", group, i),
					Message: "required",
				}
			}
			if h.When == "" {
				return &ValidationError{
					Path:    fmt.Sprintf("hooks.%s[%d].when", group, i),
					Message: "required (predicate or match expression)",
				}
			}
			if !h.Block && h.Run == "" {
				return &ValidationError{
					Path:    fmt.Sprintf("hooks.%s[%d]", group, i),
					Message: "either block: true or run: <command> must be set",
				}
			}
			if hookSeen[h.Name] {
				return &ValidationError{
					Path:    fmt.Sprintf("hooks.%s[%d].name", group, i),
					Message: fmt.Sprintf("duplicate hook name %q", h.Name),
				}
			}
			hookSeen[h.Name] = true
		}
		return nil
	}
	if err := checkHook("pre-tool", c.Hooks.PreTool); err != nil {
		return err
	}
	if err := checkHook("post-tool", c.Hooks.PostTool); err != nil {
		return err
	}
	// Permissions: each entry must be "Tool:pattern" or just
	// "pattern". The v0 check is structural only (contains a
	// colon OR is a glob). Real validation lands in v1.1 when
	// the engine reads the file.
	for i, e := range c.Permissions.Allow {
		if !validPermissionEntry(e) {
			return &ValidationError{
				Path:    fmt.Sprintf("permissions.allow[%d]", i),
				Message: fmt.Sprintf("invalid entry %q (expected 'Tool:pattern' or 'pattern')", e),
			}
		}
	}
	for i, e := range c.Permissions.Ask {
		if !validPermissionEntry(e) {
			return &ValidationError{
				Path:    fmt.Sprintf("permissions.ask[%d]", i),
				Message: fmt.Sprintf("invalid entry %q (expected 'Tool:pattern' or 'pattern')", e),
			}
		}
	}
	for i, e := range c.Permissions.Deny {
		if !validPermissionEntry(e) {
			return &ValidationError{
				Path:    fmt.Sprintf("permissions.deny[%d]", i),
				Message: fmt.Sprintf("invalid entry %q (expected 'Tool:pattern' or 'pattern')", e),
			}
		}
	}
	return nil
}

// validPermissionEntry is a cheap structural check. Real pattern
// parsing lands in v1.1.
func validPermissionEntry(s string) bool {
	if s == "" {
		return false
	}
	// Either "Tool:pattern" (must contain a colon) or just a
	// pattern (no colon). The check is lenient on purpose.
	if i := strings.Index(s, ":"); i >= 0 {
		return i > 0 && i < len(s)-1
	}
	// No colon: treat as a bare pattern. Must contain at least
	// one non-whitespace character, which is already guaranteed
	// by the empty check.
	return true
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// Inline sort to avoid importing "sort" for one call.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}
