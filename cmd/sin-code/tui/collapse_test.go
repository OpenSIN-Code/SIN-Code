// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

var testHintStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

func TestCollapseOutput_Short(t *testing.T) {
	output := "line1\nline2\nline3"
	got := CollapseOutput(output, 5, false, testHintStyle)
	if got != output {
		t.Errorf("CollapseOutput short = %q, want %q (unchanged)", got, output)
	}
}

func TestCollapseOutput_Long(t *testing.T) {
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, "line"+strings.Repeat("x", i))
	}
	output := strings.Join(lines, "\n")
	got := CollapseOutput(output, 5, false, testHintStyle)

	if strings.Contains(got, "linexxxxx") {
		t.Errorf("CollapseOutput should not contain line index 5+ (truncated at 5), got %q", got)
	}
	if !strings.Contains(got, "line\n") || !strings.Contains(got, "linexxxx") {
		t.Errorf("CollapseOutput should contain first 5 lines, got %q", got)
	}
	if !strings.Contains(got, "+15 more lines") {
		t.Errorf("CollapseOutput should contain hint with remaining count, got %q", got)
	}
	if !strings.Contains(got, "press Tab to expand") {
		t.Errorf("CollapseOutput hint should mention Tab, got %q", got)
	}
}

func TestCollapseOutput_Expanded(t *testing.T) {
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, "line"+strings.Repeat("x", i))
	}
	output := strings.Join(lines, "\n")
	got := CollapseOutput(output, 5, true, testHintStyle)
	if got != output {
		t.Errorf("CollapseOutput expanded = %q, want %q (full output)", got, output)
	}
}

func TestCollapseOutput_ExactLines(t *testing.T) {
	output := "line1\nline2\nline3\nline4\nline5"
	got := CollapseOutput(output, 5, false, testHintStyle)
	if got != output {
		t.Errorf("CollapseOutput exact lines = %q, want %q (unchanged)", got, output)
	}
}

func TestCollapseOutput_ZeroMaxLines(t *testing.T) {
	output := "line1\nline2"
	got := CollapseOutput(output, 0, false, testHintStyle)
	if got != output {
		t.Errorf("CollapseOutput maxLines=0 = %q, want %q (unchanged)", got, output)
	}
}

func TestCollapseOutput_Empty(t *testing.T) {
	got := CollapseOutput("", 5, false, testHintStyle)
	if got != "" {
		t.Errorf("CollapseOutput empty = %q, want empty", got)
	}
}
