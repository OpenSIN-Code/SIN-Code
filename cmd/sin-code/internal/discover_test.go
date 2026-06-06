// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the discover subcommand.
package internal

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverCmd_Flags(t *testing.T) {
	cmd := DiscoverCmd
	if cmd.Use != "discover [path]" {
		t.Errorf("expected Use 'discover [path]', got %q", cmd.Use)
	}
	flags := []string{"pattern", "sort_by", "format", "limit"}
	for _, f := range flags {
		if cmd.Flags().Lookup(f) == nil {
			t.Errorf("missing flag --%s", f)
		}
	}
}

func TestDiscoverCmd_DefaultValues(t *testing.T) {
	cmd := DiscoverCmd
	if v, _ := cmd.Flags().GetString("pattern"); v != "**/*" {
		t.Errorf("default pattern should be **/*, got %q", v)
	}
	if v, _ := cmd.Flags().GetString("sort_by"); v != "relevance" {
		t.Errorf("default sort_by should be relevance, got %q", v)
	}
	if v, _ := cmd.Flags().GetString("format"); v != "text" {
		t.Errorf("default format should be text, got %q", v)
	}
	if v, _ := cmd.Flags().GetInt("limit"); v != 100 {
		t.Errorf("default limit should be 100, got %d", v)
	}
}

func TestDiscoverCmd_RunWithValidPath(t *testing.T) {
	dir := t.TempDir()
	discoverFormat = "text"
	discoverPattern = "**/*"
	discoverSort = "relevance"
	discoverLimit = 10
	if err := DiscoverCmd.RunE(DiscoverCmd, []string{dir}); err != nil {
		t.Errorf("RunE failed: %v", err)
	}
	_ = filepath.Join(dir, "test.txt")
	_ = strings.Contains
}
