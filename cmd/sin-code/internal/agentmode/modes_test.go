// SPDX-License-Identifier: MIT
// Purpose: tests for the agentmode package (issue #485).
package agentmode

import (
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
)

func TestGetMode_Valid(t *testing.T) {
	cases := []struct {
		input string
		want  Mode
	}{
		{"", ModeDefault},
		{"default", ModeDefault},
		{"architect", ModeArchitect},
		{"debug", ModeDebug},
		{"code", ModeCode},
		{"review", ModeReview},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := GetMode(tc.input)
			if err != nil {
				t.Fatalf("GetMode(%q): unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("GetMode(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestGetMode_Invalid(t *testing.T) {
	_, err := GetMode("nonexistent")
	if err == nil {
		t.Fatal("GetMode(\"nonexistent\"): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown agent mode") {
		t.Errorf("error should mention unknown mode, got: %v", err)
	}
}

func TestValidModes(t *testing.T) {
	got := ValidModes()
	for _, m := range []string{"default", "architect", "debug", "code", "review"} {
		if !strings.Contains(got, m) {
			t.Errorf("ValidModes(): missing %q in %q", m, got)
		}
	}
}

func TestSystemPrompt(t *testing.T) {
	t.Run("default is empty (non-breaking)", func(t *testing.T) {
		if m := ModeDefault.SystemPrompt(); m != "" {
			t.Errorf("default mode prompt should be empty, got %q", m)
		}
	})

	t.Run("architect has focus", func(t *testing.T) {
		p := ModeArchitect.SystemPrompt()
		if p == "" {
			t.Fatal("architect prompt should not be empty")
		}
		if !strings.Contains(strings.ToUpper(p), "ARCHITECT") {
			t.Errorf("architect prompt should mention ARCHITECT, got: %q", p)
		}
	})

	t.Run("debug has focus", func(t *testing.T) {
		p := ModeDebug.SystemPrompt()
		if p == "" {
			t.Fatal("debug prompt should not be empty")
		}
		if !strings.Contains(strings.ToUpper(p), "DEBUG") {
			t.Errorf("debug prompt should mention DEBUG, got: %q", p)
		}
	})

	t.Run("code has focus", func(t *testing.T) {
		p := ModeCode.SystemPrompt()
		if p == "" {
			t.Fatal("code prompt should not be empty")
		}
		if !strings.Contains(strings.ToUpper(p), "CODE") {
			t.Errorf("code prompt should mention CODE, got: %q", p)
		}
	})

	t.Run("review has focus", func(t *testing.T) {
		p := ModeReview.SystemPrompt()
		if p == "" {
			t.Fatal("review prompt should not be empty")
		}
		if !strings.Contains(strings.ToUpper(p), "REVIEW") {
			t.Errorf("review prompt should mention REVIEW, got: %q", p)
		}
	})

	t.Run("unknown mode returns empty", func(t *testing.T) {
		if p := Mode("garbage").SystemPrompt(); p != "" {
			t.Errorf("unknown mode prompt should be empty, got %q", p)
		}
	})
}

func TestIsToolAllowed(t *testing.T) {
	t.Run("default allows everything", func(t *testing.T) {
		for _, tool := range []string{"sin_write", "sin_edit", "sin_bash", "sin_git_commit", "sin_read", "sin_scout"} {
			if !ModeDefault.IsToolAllowed(tool) {
				t.Errorf("default mode should allow %s", tool)
			}
		}
	})

	t.Run("code allows everything", func(t *testing.T) {
		for _, tool := range []string{"sin_write", "sin_edit", "sin_bash", "sin_git_commit", "sin_read"} {
			if !ModeCode.IsToolAllowed(tool) {
				t.Errorf("code mode should allow %s", tool)
			}
		}
	})

	t.Run("architect blocks writes", func(t *testing.T) {
		blocked := []string{"sin_write", "sin_edit", "sin_bash", "sin_git_commit"}
		for _, tool := range blocked {
			if ModeArchitect.IsToolAllowed(tool) {
				t.Errorf("architect mode should block %s", tool)
			}
		}
	})

	t.Run("architect allows read-only", func(t *testing.T) {
		allowed := []string{"sin_read", "sin_scout", "sin_map", "sin_grasp", "sin_discover", "sin_sckg"}
		for _, tool := range allowed {
			if !ModeArchitect.IsToolAllowed(tool) {
				t.Errorf("architect mode should allow %s", tool)
			}
		}
	})

	t.Run("debug blocks writes but allows bash", func(t *testing.T) {
		blocked := []string{"sin_write", "sin_edit", "sin_git_commit"}
		for _, tool := range blocked {
			if ModeDebug.IsToolAllowed(tool) {
				t.Errorf("debug mode should block %s", tool)
			}
		}
		// Debug allows sin_bash for diagnostic commands (read-only usage)
		if !ModeDebug.IsToolAllowed("sin_bash") {
			t.Error("debug mode should allow sin_bash for diagnostics")
		}
	})

	t.Run("review blocks writes and bash", func(t *testing.T) {
		blocked := []string{"sin_write", "sin_edit", "sin_bash", "sin_git_commit"}
		for _, tool := range blocked {
			if ModeReview.IsToolAllowed(tool) {
				t.Errorf("review mode should block %s", tool)
			}
		}
	})

	t.Run("unknown mode allows everything", func(t *testing.T) {
		if !Mode("garbage").IsToolAllowed("sin_write") {
			t.Error("unknown mode should allow everything (safe fallback)")
		}
	})
}

func TestFilterTools(t *testing.T) {
	specs := []agentloop.ToolSpec{
		{Name: "sin_read"},
		{Name: "sin_write"},
		{Name: "sin_edit"},
		{Name: "sin_bash"},
		{Name: "sin_scout"},
		{Name: "sin_git_commit"},
		{Name: "sin_sckg"},
	}

	t.Run("default returns all", func(t *testing.T) {
		got := ModeDefault.FilterTools(specs)
		if len(got) != len(specs) {
			t.Errorf("default: got %d specs, want %d", len(got), len(specs))
		}
	})

	t.Run("code returns all", func(t *testing.T) {
		got := ModeCode.FilterTools(specs)
		if len(got) != len(specs) {
			t.Errorf("code: got %d specs, want %d", len(got), len(specs))
		}
	})

	t.Run("architect filters writes", func(t *testing.T) {
		got := ModeArchitect.FilterTools(specs)
		if len(got) != 3 {
			t.Errorf("architect: got %d specs, want 3 (sin_read, sin_scout, sin_sckg)", len(got))
		}
		names := make(map[string]bool, len(got))
		for _, s := range got {
			names[s.Name] = true
		}
		for _, blocked := range []string{"sin_write", "sin_edit", "sin_bash", "sin_git_commit"} {
			if names[blocked] {
				t.Errorf("architect: %s should be filtered out", blocked)
			}
		}
		if !names["sin_read"] {
			t.Error("architect: sin_read should be present")
		}
	})

	t.Run("debug filters writes but keeps bash", func(t *testing.T) {
		got := ModeDebug.FilterTools(specs)
		names := make(map[string]bool, len(got))
		for _, s := range got {
			names[s.Name] = true
		}
		for _, blocked := range []string{"sin_write", "sin_edit", "sin_git_commit"} {
			if names[blocked] {
				t.Errorf("debug: %s should be filtered out", blocked)
			}
		}
		if !names["sin_bash"] {
			t.Error("debug: sin_bash should be present")
		}
		if !names["sin_read"] {
			t.Error("debug: sin_read should be present")
		}
	})

	t.Run("review filters writes and bash", func(t *testing.T) {
		got := ModeReview.FilterTools(specs)
		if len(got) != 3 {
			t.Errorf("review: got %d specs, want 3", len(got))
		}
		names := make(map[string]bool, len(got))
		for _, s := range got {
			names[s.Name] = true
		}
		for _, blocked := range []string{"sin_write", "sin_edit", "sin_bash", "sin_git_commit"} {
			if names[blocked] {
				t.Errorf("review: %s should be filtered out", blocked)
			}
		}
	})

	t.Run("empty specs returns empty", func(t *testing.T) {
		got := ModeArchitect.FilterTools(nil)
		if len(got) != 0 {
			t.Errorf("nil input: got %d specs, want 0", len(got))
		}
	})

	t.Run("unknown mode returns all (safe fallback)", func(t *testing.T) {
		got := Mode("garbage").FilterTools(specs)
		if len(got) != len(specs) {
			t.Errorf("unknown mode: got %d specs, want %d", len(got), len(specs))
		}
	})
}

func TestIsRestricted(t *testing.T) {
	t.Run("default is not restricted", func(t *testing.T) {
		if ModeDefault.IsRestricted() {
			t.Error("default should not be restricted")
		}
	})

	t.Run("code is not restricted", func(t *testing.T) {
		if ModeCode.IsRestricted() {
			t.Error("code should not be restricted")
		}
	})

	t.Run("architect is restricted", func(t *testing.T) {
		if !ModeArchitect.IsRestricted() {
			t.Error("architect should be restricted")
		}
	})

	t.Run("debug is restricted", func(t *testing.T) {
		if !ModeDebug.IsRestricted() {
			t.Error("debug should be restricted")
		}
	})

	t.Run("review is restricted", func(t *testing.T) {
		if !ModeReview.IsRestricted() {
			t.Error("review should be restricted")
		}
	})
}

func TestModeString(t *testing.T) {
	cases := []struct {
		mode Mode
		want string
	}{
		{ModeDefault, "default"},
		{ModeArchitect, "architect"},
		{ModeDebug, "debug"},
		{ModeCode, "code"},
		{ModeReview, "review"},
	}
	for _, tc := range cases {
		if got := tc.mode.String(); got != tc.want {
			t.Errorf("%v.String() = %q, want %q", tc.mode, got, tc.want)
		}
	}
}

func TestFilterTools_PreservesOrder(t *testing.T) {
	specs := []agentloop.ToolSpec{
		{Name: "sin_read"},
		{Name: "sin_write"},
		{Name: "sin_scout"},
		{Name: "sin_edit"},
		{Name: "sin_sckg"},
	}
	got := ModeArchitect.FilterTools(specs)
	expected := []string{"sin_read", "sin_scout", "sin_sckg"}
	if len(got) != len(expected) {
		t.Fatalf("got %d specs, want %d", len(got), len(expected))
	}
	for i, name := range expected {
		if got[i].Name != name {
			t.Errorf("position %d: got %q, want %q", i, got[i].Name, name)
		}
	}
}
