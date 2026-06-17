// SPDX-License-Identifier: MIT
// Purpose: tests for issue #323 — read-only todo MCP tools must default to
// "allow" (not "ask") so the daemon can read todos without --yolo. Mutating
// todo tools stay at "ask" per mandate M4.
package internal

import (
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
)

// allTodoReadOnlyTools lists the 8 read-only todo MCP tools that must be
// "allow" (issue #323 acceptance criteria).
var allTodoReadOnlyTools = []string{
	"sin_todo_list",
	"sin_todo_show",
	"sin_todo_ready",
	"sin_todo_blocked",
	"sin_todo_search",
	"sin_todo_prime",
	"sin_todo_stats",
	"sin_todo_deps",
}

// allTodoMutatingTools lists the 4 destructive todo MCP tools that must stay
// at "ask" (issue #323 acceptance criteria, mandate M4).
var allTodoMutatingTools = []string{
	"sin_todo_add",
	"sin_todo_complete",
	"sin_todo_claim",
	"sin_todo_dep_add",
}

// TestTodoReadOnlyTools_Allow verifies every read-only todo tool resolves to
// Allow under the default permission rules.
func TestTodoReadOnlyTools_Allow(t *testing.T) {
	engine := permission.New(DefaultPermissionRules())
	for _, tool := range allTodoReadOnlyTools {
		t.Run(tool, func(t *testing.T) {
			got := engine.Check(tool)
			if got != permission.Allow {
				t.Errorf("Check(%q) = %s, want allow (issue #323: read-only todo tools must be allow)", tool, got)
			}
		})
	}
}

// TestTodoMutatingTools_Ask verifies every destructive todo tool resolves to
// Ask (not Allow) under the default permission rules.
func TestTodoMutatingTools_Ask(t *testing.T) {
	engine := permission.New(DefaultPermissionRules())
	for _, tool := range allTodoMutatingTools {
		t.Run(tool, func(t *testing.T) {
			got := engine.Check(tool)
			if got != permission.Ask {
				t.Errorf("Check(%q) = %s, want ask (M4: mutating todo tools must require confirmation)", tool, got)
			}
		})
	}
}

// TestTodoReadOnlyTools_HeadlessNotDenied verifies that in headless mode
// (daemon), read-only todo tools are NOT denied — they resolve to Allow.
// This is the core bug from issue #323: the daemon could not read todos
// because "ask" resolved to "deny" in headless mode.
func TestTodoReadOnlyTools_HeadlessNotDenied(t *testing.T) {
	engine := permission.New(DefaultPermissionRules())
	engine.Headless = true
	for _, tool := range allTodoReadOnlyTools {
		got := engine.Check(tool)
		if got == permission.Deny {
			t.Errorf("Check(%q) = deny in headless mode — issue #323: read-only tools must be allow, not denied", tool)
		}
	}
}

// TestTodoMutatingTools_HeadlessDenied verifies that in headless mode
// (daemon), destructive todo tools ARE denied (ask → deny without --yolo).
// This confirms the daemon cannot mutate todos without explicit approval.
func TestTodoMutatingTools_HeadlessDenied(t *testing.T) {
	engine := permission.New(DefaultPermissionRules())
	engine.Headless = true
	for _, tool := range allTodoMutatingTools {
		got := engine.Check(tool)
		if got != permission.Deny {
			t.Errorf("Check(%q) = %s in headless mode, want deny (mutating tools must be denied without --yolo)", tool, got)
		}
	}
}

// TestTodoTools_YoloPromotesMutatingToAllow verifies that with --yolo,
// destructive todo tools are promoted to Allow (yolo bypasses ask).
func TestTodoTools_YoloPromotesMutatingToAllow(t *testing.T) {
	engine := permission.New(DefaultPermissionRules())
	engine.Yolo = true
	for _, tool := range allTodoMutatingTools {
		got := engine.Check(tool)
		if got != permission.Allow {
			t.Errorf("Check(%q) = %s with yolo, want allow (yolo bypasses ask)", tool, got)
		}
	}
}

// TestTodoTools_ComprehensiveTable is a table-driven test covering all 12
// todo tools in both default and headless modes, ensuring the policy matrix
// is exactly as specified by issue #323.
func TestTodoTools_ComprehensiveTable(t *testing.T) {
	defaultEngine := permission.New(DefaultPermissionRules())
	headlessEngine := permission.New(DefaultPermissionRules())
	headlessEngine.Headless = true

	cases := []struct {
		tool         string
		wantDefault  permission.Policy
		wantHeadless permission.Policy
	}{
		// Read-only → allow in both modes
		{"sin_todo_list", permission.Allow, permission.Allow},
		{"sin_todo_show", permission.Allow, permission.Allow},
		{"sin_todo_ready", permission.Allow, permission.Allow},
		{"sin_todo_blocked", permission.Allow, permission.Allow},
		{"sin_todo_search", permission.Allow, permission.Allow},
		{"sin_todo_prime", permission.Allow, permission.Allow},
		{"sin_todo_stats", permission.Allow, permission.Allow},
		{"sin_todo_deps", permission.Allow, permission.Allow},
		// Mutating → ask (default), deny (headless)
		{"sin_todo_add", permission.Ask, permission.Deny},
		{"sin_todo_complete", permission.Ask, permission.Deny},
		{"sin_todo_claim", permission.Ask, permission.Deny},
		{"sin_todo_dep_add", permission.Ask, permission.Deny},
	}

	for _, c := range cases {
		t.Run(c.tool, func(t *testing.T) {
			got := defaultEngine.Check(c.tool)
			if got != c.wantDefault {
				t.Errorf("default: Check(%q) = %s, want %s", c.tool, got, c.wantDefault)
			}
			gotH := headlessEngine.Check(c.tool)
			if gotH != c.wantHeadless {
				t.Errorf("headless: Check(%q) = %s, want %s", c.tool, gotH, c.wantHeadless)
			}
		})
	}
}

// TestTodoRules_ExplicitInDefaults verifies that all 12 todo tools have
// explicit entries in DefaultPermissionRules — none should fall through to
// the catch-all "*" rule. This guards against accidental removal.
func TestTodoRules_ExplicitInDefaults(t *testing.T) {
	rules := DefaultPermissionRules()
	allTools := append(append([]string{}, allTodoReadOnlyTools...), allTodoMutatingTools...)
	for _, tool := range allTools {
		found := false
		for _, r := range rules {
			if r.Tool == tool {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("tool %q not found in DefaultPermissionRules — must have an explicit entry (issue #323)", tool)
		}
	}
}

// TestTodoRules_NoWildcardGaps verifies there is no wildcard rule that would
// accidentally override the explicit todo rules (e.g. "sin_todo_*" → ask).
func TestTodoRules_NoWildcardGaps(t *testing.T) {
	rules := DefaultPermissionRules()
	for _, r := range rules {
		if strings.HasPrefix(r.Tool, "sin_todo_") && strings.Contains(r.Tool, "*") {
			t.Errorf("wildcard rule %q -> %s would shadow explicit todo rules (issue #323)", r.Tool, r.Policy)
		}
	}
}
