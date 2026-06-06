// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the map, grasp, scout, harvest, orchestrate subcommands.
package internal

import (
	"testing"
)

func TestMapCmd_Flags(t *testing.T) {
	cmd := MapCmd
	if cmd.Use != "map [path]" {
		t.Errorf("expected Use 'map [path]', got %q", cmd.Use)
	}
	for _, f := range []string{"action", "format"} {
		if cmd.Flags().Lookup(f) == nil {
			t.Errorf("missing flag --%s", f)
		}
	}
}

func TestMapCmd_RunWithTempDir(t *testing.T) {
	dir := t.TempDir()
	mapPath = ""
	mapAction = "map"
	mapFormat = "text"
	if err := MapCmd.RunE(MapCmd, []string{dir}); err != nil {
		t.Errorf("RunE failed: %v", err)
	}
}

func TestGraspCmd_RequiresPath(t *testing.T) {
	graspPath = ""
	graspFormat = "text"
	err := GraspCmd.RunE(GraspCmd, []string{"/nonexistent/file.go"})
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestScoutCmd_RequiresQuery(t *testing.T) {
	scoutQuery = ""
	scoutPath = "/tmp"
	scoutType = "regex"
	scoutFormat = "text"
	err := ScoutCmd.RunE(ScoutCmd, []string{})
	if err == nil {
		t.Error("expected error when --query is missing")
	}
}

func TestHarvestCmd_RequiresURL(t *testing.T) {
	harvestURL = ""
	harvestFormat = "text"
	harvestMethod = "GET"
	harvestTimeout = 1
	err := HarvestCmd.RunE(HarvestCmd, []string{})
	if err == nil {
		t.Error("expected error when --url is missing")
	}
}

func TestOrchestrateCmd_AddTask(t *testing.T) {
	orchAction = "add"
	orchTitle = "Test task"
	orchFormat = "text"
	orchID = ""
	orchTags = ""
	if err := OrchestrateCmd.RunE(OrchestrateCmd, []string{}); err != nil {
		t.Errorf("RunE failed: %v", err)
	}
}
