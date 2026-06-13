// SPDX-License-Identifier: MIT
// Purpose: Centralised version metadata for the sin-code unified binary.
// All three variables are overridden at build time via -ldflags; defaults
// keep dev builds self-describing.
// Docs: version.go.doc.md
package internal

import (
	"fmt"
	"sync"

	"github.com/spf13/cobra"
)

var (
	Version = "dev"     // -X .../internal.Version=...
	commit  = "unknown" // -X .../internal.commit=...
	date    = "unknown" // -X .../internal.date=...
)

var (
	versionCmds []*cobra.Command
	versionMu    sync.Mutex
)

// RegisterVersionCmd registers a cobra command whose Version field should
// be kept in sync with the package-level Version, commit, and date variables.
// Called from each subcommand's init().
func RegisterVersionCmd(cmd *cobra.Command) {
	versionMu.Lock()
	defer versionMu.Unlock()
	cmd.Version = versionString()
	cmd.SetVersionTemplate("{{.Name}} {{.Version}}\n")
	versionCmds = append(versionCmds, cmd)
}

// versionString returns the full version string with commit and date.
func versionString() string {
	return fmt.Sprintf("%s (commit %s, built %s)", Version, commit, date)
}

// VersionLine returns the canonical "<name> v1.2.3 (commit abc1234, built ...)"
// string used by every --version printer.
func VersionLine(name string) string {
	return fmt.Sprintf("%s %s (commit %s, built %s)", name, Version, commit, date)
}

// SetVersion overwrites all three build-time variables and syncs the
// Version field on every registered cobra command. Safe to call
// multiple times; the last call wins.
func SetVersion(v, c, d string) {
	versionMu.Lock()
	defer versionMu.Unlock()
	if v != "" {
		Version = v
	}
	if c != "" {
		commit = c
	}
	if d != "" {
		date = d
	}
	vs := versionString()
	for _, cmd := range versionCmds {
		cmd.Version = vs
	}
}
