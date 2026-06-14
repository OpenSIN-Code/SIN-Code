// SPDX-License-Identifier: MIT
package main

import (
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/resource"
)

func TestDedupeRepos(t *testing.T) {
	got := dedupeRepos("/cwd", []string{"/cwd", "/a", "/b", "/a", ""})
	want := []string{"/cwd", "/a", "/b"}
	if len(got) != len(want) {
		t.Fatalf("dedupeRepos len = %d (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("dedupeRepos[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDedupeReposEmptyCwd(t *testing.T) {
	got := dedupeRepos("", []string{"/only"})
	if len(got) != 1 || got[0] != "/only" {
		t.Errorf("dedupeRepos with empty cwd = %v, want [/only]", got)
	}
}

func TestDiskOKNoFloor(t *testing.T) {
	// No floor configured -> always OK (fail-open).
	if !diskOK(resource.Limits{}) {
		t.Error("diskOK with zero limits should be true")
	}
}

func TestDiskOKFailOpenWhenUnavailable(t *testing.T) {
	// A huge floor: on unix the cwd almost certainly has less free
	// space than 1 EiB, so this should be false; on platforms where
	// DiskFree is unavailable it fails open (true). Either way the
	// call must not panic.
	_ = diskOK(resource.Limits{MinDiskBytes: 1 << 60})
}

func TestNewDaemonCmdFlags(t *testing.T) {
	cmd := NewDaemonCmd()
	for _, name := range []string{"poll", "lease", "verify-cmd", "max-turns", "concurrency", "repos", "max-memory", "max-procs", "min-disk"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("daemon command missing --%s flag", name)
		}
	}
}
