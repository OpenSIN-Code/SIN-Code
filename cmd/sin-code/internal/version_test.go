// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the per-subcommand --version flag (issue #38).
// Regression guard for issue #38 — per-subcommand --version flag.
// These tests fail if the per-subcommand Version field is removed.
package internal

import (
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

type SubcommandCase struct {
	SubName string
	Cmd     *cobra.Command
}

func versionSubcommands() []SubcommandCase {
	return []SubcommandCase{
		{"discover", DiscoverCmd},
		{"execute", ExecuteCmd},
		{"map", MapCmd},
		{"grasp", GraspCmd},
		{"scout", ScoutCmd},
		{"harvest", HarvestCmd},
		{"orchestrate", OrchestrateCmd},
	}
}

func advancedSubcommands() []SubcommandCase {
	return []SubcommandCase{
		{"ibd", IbdCmd},
		{"poc", PocCmd},
		{"sckg", SckgCmd},
		{"adw", AdwCmd},
		{"oracle", OracleCmd},
		{"efm", EfmCmd},
	}
}

func allVersionSubcommands() []SubcommandCase {
	return append(versionSubcommands(), advancedSubcommands()...)
}

func TestVersionSubcommands_HaveVersionField(t *testing.T) {
	for _, sc := range allVersionSubcommands() {
		if sc.Cmd.Version == "" {
			t.Errorf("%s: Version field is empty — cobra will not auto-add --version", sc.SubName)
		}
	}
}

func TestVersionSubcommands_ExposeVersionFlag(t *testing.T) {
	for _, sc := range allVersionSubcommands() {
		sc.Cmd.InitDefaultVersionFlag()
		if sc.Cmd.Flags().Lookup("version") == nil {
			t.Errorf("%s: --version flag not registered (cobra should auto-add when Version is set)", sc.SubName)
		}
		if sc.Cmd.Flags().ShorthandLookup("v") == nil {
			t.Errorf("%s: -v shorthand not registered (cobra should auto-add when Version is set)", sc.SubName)
		}
	}
}

func TestVersionSubcommands_OutputFormat(t *testing.T) {
	SetVersion("v9.9.9-test", "deadbeef", "2026-06-12T15:04:05Z")
	defer SetVersion("dev", "unknown", "unknown")

	for _, sc := range versionSubcommands() {
		t.Run(sc.SubName, func(t *testing.T) {
			restore := captureStdout(t)
			sc.Cmd.SetArgs([]string{"--version"})
			if err := sc.Cmd.Execute(); err != nil {
				t.Fatalf("%s --version returned error: %v", sc.SubName, err)
			}
			out := restore()
			expected := fmt.Sprintf("%s v9.9.9-test (commit deadbeef, built 2026-06-12T15:04:05Z)", sc.SubName)
			if !strings.Contains(out, expected) {
				t.Errorf("%s --version output mismatch:\n got:  %q\n want substring: %q", sc.SubName, out, expected)
			}
		})
	}
}

func TestAdvancedSubcommands_OutputFormat(t *testing.T) {
	SetVersion("v9.9.9-test", "deadbeef", "2026-06-12T15:04:05Z")
	defer SetVersion("dev", "unknown", "unknown")

	for _, sc := range advancedSubcommands() {
		t.Run(sc.SubName, func(t *testing.T) {
			restore := captureStdout(t)
			sc.Cmd.SetArgs([]string{"--version"})
			if err := sc.Cmd.Execute(); err != nil {
				t.Fatalf("%s --version returned error: %v", sc.SubName, err)
			}
			out := restore()
			expected := fmt.Sprintf("%s v9.9.9-test (commit deadbeef, built 2026-06-12T15:04:05Z)", sc.SubName)
			if !strings.Contains(out, expected) {
				t.Errorf("%s --version output mismatch:\n got:  %q\n want substring: %q", sc.SubName, out, expected)
			}
		})
	}
}

func TestVersionSubcommands_NoDoubleNewline(t *testing.T) {
	for _, sc := range allVersionSubcommands() {
		t.Run(sc.SubName, func(t *testing.T) {
			restore := captureStdout(t)
			sc.Cmd.SetArgs([]string{"--version"})
			_ = sc.Cmd.Execute()
			out := restore()
			if strings.HasSuffix(out, "\n\n") {
				t.Errorf("%s --version ends with double newline: %q", sc.SubName, out)
			}
		})
	}
}

func TestVersionSubcommands_RunENotCalled(t *testing.T) {
	SetVersion("v1.0.0-test", "abc1234", "2026-01-01T00:00:00Z")
	defer SetVersion("dev", "unknown", "unknown")

	restore := captureStdout(t)
	ExecuteCmd.SetArgs([]string{"--version"})
	if err := ExecuteCmd.Execute(); err != nil {
		t.Fatalf("execute --version returned error: %v", err)
	}
	out := restore()
	if strings.Contains(out, "--command is required") {
		t.Errorf("execute --version should not invoke RunE; got: %q", out)
	}
}

func TestVersionLine(t *testing.T) {
	SetVersion("v1.2.3", "abc1234", "2026-06-13T10:00:00Z")
	defer SetVersion("dev", "unknown", "unknown")

	got := VersionLine("discover")
	want := "discover v1.2.3 (commit abc1234, built 2026-06-13T10:00:00Z)"
	if got != want {
		t.Errorf("VersionLine(discover) = %q, want %q", got, want)
	}
}

func TestSetVersion(t *testing.T) {
	SetVersion("v0.0.1-set", "setcommit", "setdate")
	if Version != "v0.0.1-set" {
		t.Errorf("Version = %q, want %q", Version, "v0.0.1-set")
	}
	SetVersion("", "newcommit", "")
	if Version != "v0.0.1-set" {
		t.Errorf("Version should not change on empty string: %q", Version)
	}
	SetVersion("dev", "unknown", "unknown")
}

func TestSetVersionSyncsCmdFields(t *testing.T) {
	SetVersion("v9.9.9-sync", "synccommit", "syncdate")
	defer SetVersion("dev", "unknown", "unknown")

	for _, sc := range allVersionSubcommands() {
		if !strings.Contains(sc.Cmd.Version, "v9.9.9-sync") {
			t.Errorf("%s: cmd.Version not synced after SetVersion, got %q", sc.SubName, sc.Cmd.Version)
		}
	}
}
