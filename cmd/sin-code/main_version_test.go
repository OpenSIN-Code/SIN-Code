// SPDX-License-Identifier: MIT
// Purpose: Subprocess-level integration test for `sin-code <sub> --version`
// on each of the 7 core Go-tool subcommands (issue #38). Forking is
// required because cobra's flag parsing mutates global state on the
// rootCmd and we want a hermetic per-subcommand probe.
package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestMain_VersionFlagOnAllSubcommands(t *testing.T) {
	subs := []string{
		"discover", "execute", "map", "grasp",
		"scout", "harvest", "orchestrate",
	}
	for _, sub := range subs {
		t.Run(sub, func(t *testing.T) {
			envKey := "TEST_MAIN_VERSION_" + strings.ToUpper(sub)
			if os.Getenv(envKey) == "1" {
				os.Args = []string{"sin-code", sub, "--version"}
				main()
				return
			}
			cmd := exec.Command(os.Args[0], "-test.run=TestMain_VersionFlagOnAllSubcommands/"+sub)
			cmd.Env = append(os.Environ(),
				envKey+"=1",
				"HOME=/tmp",
				"SIN_CODE_NO_UPDATE_CHECK=1",
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s --version: subprocess failed: %v\noutput: %s", sub, err, output)
			}
			out := string(output)
			if !strings.Contains(out, sub) {
				t.Errorf("%s --version: output should contain subcommand name, got: %q", sub, out)
			}
			if !strings.Contains(out, "commit") {
				t.Errorf("%s --version: output should contain 'commit' (issue example format), got: %q", sub, out)
			}
			if strings.Contains(out, "Usage:") {
				t.Errorf("%s --version: output contains 'Usage' — looks like help, not version: %q", sub, out)
			}
		})
	}
}

func TestMain_VersionSymlinkRouting(t *testing.T) {
	if os.Getenv("TEST_MAIN_VERSION_SYMLINK") == "1" {
		os.Args = []string{"discover", "--version"}
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMain_VersionSymlinkRouting")
	cmd.Env = append(os.Environ(),
		"TEST_MAIN_VERSION_SYMLINK=1",
		"HOME=/tmp",
		"SIN_CODE_NO_UPDATE_CHECK=1",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("symlink --version: subprocess failed: %v\noutput: %s", err, output)
	}
	out := string(output)
	if !strings.Contains(out, "discover") {
		t.Errorf("symlink discover --version: output should contain 'discover', got: %q", out)
	}
	if !strings.Contains(out, "commit") {
		t.Errorf("symlink discover --version: output should contain 'commit', got: %q", out)
	}
}
