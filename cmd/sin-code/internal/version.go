// SPDX-License-Identifier: MIT
// Purpose: Centralised version metadata for the sin-code unified binary.
// All three variables are overridden at build time via -ldflags; defaults
// keep dev builds self-describing.
// Docs: version.go.doc.md
package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	versionMu   sync.Mutex
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

// ── shared utilities ───────────────────────────────────────────────────

// osExit is a test hook so PrintError can be tested without killing the process.
var osExit = os.Exit

// PrintError prints an error to stderr in a consistent format and exits with code 1.
func PrintError(err error) {
	fmt.Fprintf(os.Stderr, "sin-code: %v\n", err)
	osExit(1)
}

// lookupStandalone finds a standalone SIN-Code tool binary in common locations.
// Returns the full path if found, or an error with installation instructions.
// Skips files that are copies of the current executable (prevents recursion
// when standalone binaries have been replaced with copies of sin-code).
func lookupStandalone(name string) (string, error) {
	selfPath, selfErr := osExecutable()
	if selfErr != nil {
		selfPath = ""
	}
	var selfInfo os.FileInfo
	if selfPath != "" {
		selfInfo, _ = os.Stat(selfPath)
	}

	candidates := []string{
		filepath.Join(os.Getenv("HOME"), ".local", "bin", name),
		filepath.Join("/usr", "local", "bin", name),
		filepath.Join("/opt", "homebrew", "bin", name),
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			if selfInfo != nil {
				if os.SameFile(selfInfo, info) {
					continue
				}
				if selfInfo.Size() == info.Size() && name != "sin-code" {
					continue
				}
			}
			return p, nil
		}
	}
	if path, err := exec.LookPath(name); err == nil {
		if info, err := os.Stat(path); err == nil && selfInfo != nil {
			if os.SameFile(selfInfo, info) || (selfInfo.Size() == info.Size() && name != "sin-code") {
				return "", fmt.Errorf(
					"standalone %s binary is a copy of sin-code (recursion prevented).\n"+
						"The unified sin-code binary replaces standalone binaries.\n"+
						"Use: sin-code %s [args] instead of %s [args]",
					name, name, name)
			}
		}
		return path, nil
	}
	return "", fmt.Errorf(
		"standalone %s binary not found in PATH or ~/.local/bin.\n"+
			"Install: go install github.com/OpenSIN-Code/SIN-Code-%s-Tool/cmd/%s@latest\n"+
			"Or:       clone and 'go build -o ~/.local/bin/%s'",
		name, capitalize(name), name, name)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]-32) + s[1:]
}
