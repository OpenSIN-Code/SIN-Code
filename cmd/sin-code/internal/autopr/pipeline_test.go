// SPDX-License-Identifier: MIT
// Purpose: tests for issue #158 — autopr pipeline report builder.
package autopr

import (
	"strings"
	"testing"
)

func TestNewReport_Empty(t *testing.T) {
	r := NewReport(".", nil)
	if r.WouldCreatePR {
		t.Error("empty issues must not create a PR")
	}
	if r.PRTitle != "" || r.PRBody != "" {
		t.Error("empty report must have empty PR fields")
	}
}

func TestNewReport_OnlyNonTrivial(t *testing.T) {
	in := []Issue{{ID: "x", Class: ClassNonTrivial, File: "a.go"}}
	r := NewReport(".", in)
	if r.WouldCreatePR {
		t.Error("non-trivial-only must not create a PR (reversible, no LLM)")
	}
	if len(r.AutoFixable) != 0 {
		t.Error("expected zero auto-fixable")
	}
	if len(r.RequiresHuman) != 1 {
		t.Error("expected one human-required")
	}
}

func TestNewReport_HasFixable(t *testing.T) {
	in := []Issue{
		{ID: "1", Class: ClassTrivial, Category: "format", File: "a.go",
			Note: "gofmt", Fix: "gofmt -w a.go"},
		{ID: "2", Class: ClassMechanical, Category: "test", File: "b.go",
			Note: "missing test", Fix: "echo stub > b_test.go"},
		{ID: "3", Class: ClassNonTrivial, File: "c.go", Note: "logic"},
	}
	r := NewReport("/ws", in)
	if !r.WouldCreatePR {
		t.Error("expected WouldCreatePR=true")
	}
	if len(r.AutoFixable) != 2 {
		t.Errorf("expected 2 auto-fixable, got %d", len(r.AutoFixable))
	}
	if len(r.RequiresHuman) != 1 {
		t.Errorf("expected 1 human, got %d", len(r.RequiresHuman))
	}
	if !strings.HasPrefix(r.PRTitle, "autopr:") {
		t.Errorf("PR title must start with 'autopr:', got %q", r.PRTitle)
	}
	if !strings.Contains(r.PRBody, "issue #158") {
		t.Error("PR body must reference issue #158")
	}
	if !strings.Contains(r.PRBody, "M3") {
		t.Error("PR body must reference the M3 verify-gate mandate")
	}
}

func TestRenderPRTitle_Stable(t *testing.T) {
	// Same input -> same title (acceptance criterion: deterministic).
	issues := []Issue{
		{Class: ClassTrivial, File: "a.go"},
		{Class: ClassTrivial, File: "b.go"},
	}
	r1 := NewReport(".", issues)
	r2 := NewReport(".", issues)
	if r1.PRTitle != r2.PRTitle {
		t.Errorf("titles must be stable: %q vs %q", r1.PRTitle, r2.PRTitle)
	}
}

func TestRenderCommands_Dedup(t *testing.T) {
	// Same Fix command twice -> only one in the list.
	issues := []Issue{
		{Class: ClassTrivial, Fix: "gofmt -w a.go"},
		{Class: ClassTrivial, Fix: "gofmt -w a.go"},
		{Class: ClassTrivial, Fix: "gofmt -w b.go"},
	}
	r := NewReport(".", issues)
	if len(r.CommandsToRun) != 2 {
		t.Errorf("expected 2 dedup'd commands, got %d: %v", len(r.CommandsToRun), r.CommandsToRun)
	}
}

func TestRenderCommands_Sorted(t *testing.T) {
	issues := []Issue{
		{Class: ClassTrivial, Fix: "gofmt -w z.go"},
		{Class: ClassTrivial, Fix: "gofmt -w a.go"},
	}
	r := NewReport(".", issues)
	if r.CommandsToRun[0] != "gofmt -w a.go" {
		t.Errorf("expected sorted commands, got %v", r.CommandsToRun)
	}
}
