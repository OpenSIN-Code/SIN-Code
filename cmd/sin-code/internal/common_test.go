// SPDX-License-Identifier: MIT
// Purpose: Unit tests for common.go (PrintError, lookupStandalone, capitalize).
package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestMain(m *testing.M) {
	if os.Getenv("TEST_PRINT_ERROR") == "1" {
		PrintError(fmt.Errorf("test error"))
		return
	}
	if os.Getenv("SIN_CODE_SUBPROCESS") == "1" {
		SetCurrentVersion("test")
		root := &cobra.Command{Use: "sin-code", Version: "test"}
		root.AddCommand(DiscoverCmd, ExecuteCmd, MapCmd, GraspCmd, ScoutCmd,
			HarvestCmd, OrchestrateCmd, IbdCmd, PocCmd, SckgCmd, AdwCmd,
			OracleCmd, EfmCmd, ServeCmd, SecurityCmd, SbomCmd, ConfigCmd,
			SelfUpdateCmd)
		root.SetArgs(os.Args[1:])
		if err := root.Execute(); err != nil {
			os.Exit(1)
		}
		return
	}
	os.Exit(m.Run())
}

func TestPrintError(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestPrintError")
	cmd.Env = append(os.Environ(), "TEST_PRINT_ERROR=1")
	output, err := cmd.CombinedOutput()
	if e, ok := err.(*exec.ExitError); !ok || e.ExitCode() != 1 {
		t.Fatalf("expected exit code 1, got %v", err)
	}
	expected := "sin-code: test error\n"
	got := string(output)
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestLookupStandalone_FoundInHome(t *testing.T) {
	tmpDir := t.TempDir()
	binDir := filepath.Join(tmpDir, ".local", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	fakeBin := filepath.Join(binDir, "discover")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\necho discover"), 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", tmpDir)
	t.Setenv("PATH", tmpDir)

	path, err := lookupStandalone("discover")
	if err != nil {
		t.Fatalf("lookupStandalone(discover): %v", err)
	}
	if path != fakeBin {
		t.Errorf("expected %q, got %q", fakeBin, path)
	}
}

func TestLookupStandalone_FoundInPath(t *testing.T) {
	tmpDir := t.TempDir()
	fakeBin := filepath.Join(tmpDir, "grasp")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\necho grasp"), 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", "/nonexistent")
	t.Setenv("PATH", tmpDir)

	path, err := lookupStandalone("grasp")
	if err != nil {
		t.Fatalf("lookupStandalone(grasp): %v", err)
	}
	if path != fakeBin {
		t.Errorf("expected %q, got %q", fakeBin, path)
	}
}

func TestLookupStandalone_NotFound(t *testing.T) {
	t.Setenv("HOME", "/nonexistent")
	t.Setenv("PATH", "/nonexistent")

	_, err := lookupStandalone("nonexistent-tool-xyz")
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
	if fmt.Sprintf("%v", err) == "" {
		t.Error("error should not be empty")
	}
}

func TestLookupStandalone_RecursionPrevention(t *testing.T) {
	tmpDir := t.TempDir()
	binDir := filepath.Join(tmpDir, ".local", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}

	selfPath, err := os.Executable()
	if err != nil {
		t.Skipf("cannot get self path: %v", err)
	}
	selfData, err := os.ReadFile(selfPath)
	if err != nil {
		t.Skipf("cannot read self: %v", err)
	}

	fakeBin := filepath.Join(binDir, "discover")
	if err := os.WriteFile(fakeBin, selfData, 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", tmpDir)
	t.Setenv("PATH", tmpDir)

	_, err = lookupStandalone("discover")
	if err == nil {
		t.Error("expected error for recursive copy")
	}
}

func TestLookupStandalone_SameFileSizeRecursion(t *testing.T) {
	tmpDir := t.TempDir()
	binDir := filepath.Join(tmpDir, ".local", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}

	selfPath, _ := os.Executable()
	selfInfo, _ := os.Stat(selfPath)

	fakeBin := filepath.Join(binDir, "discover")
	fakeData := make([]byte, selfInfo.Size())
	if err := os.WriteFile(fakeBin, fakeData, 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", tmpDir)
	t.Setenv("PATH", tmpDir)

	_, err := lookupStandalone("discover")
	if err == nil {
		t.Error("expected recursion-prevention error for same-size file")
	}
}

func TestCapitalize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"discover", "Discover"},
		{"map", "Map"},
		{"a", "A"},
		{"", ""},
		{"z", "Z"},
		{"execute", "Execute"},
	}
	for _, tt := range tests {
		got := capitalize(tt.input)
		if got != tt.want {
			t.Errorf("capitalize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
