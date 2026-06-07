// SPDX-License-Identifier: MIT
// Purpose: Shared utilities for the sin-code unified binary (error printing,
// shared flags, output formatting). All subcommands import this package.
package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// PrintError prints an error to stderr in a consistent format and exits with code 1.
func PrintError(err error) {
	fmt.Fprintf(os.Stderr, "sin-code: %v\n", err)
	os.Exit(1)
}

// lookupStandalone finds a standalone SIN-Code tool binary in common locations.
// Returns the full path if found, or an error with installation instructions.
// Skips files that are copies of the current executable (prevents recursion
// when standalone binaries have been replaced with copies of sin-code).
func lookupStandalone(name string) (string, error) {
	selfPath, selfErr := os.Executable()
	if selfErr != nil {
		selfPath = ""
	}
	// Get self file info for comparison
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
			// Skip if same file (same inode) OR same size (likely a copy)
			if selfInfo != nil {
				if os.SameFile(selfInfo, info) {
					continue
				}
				// If same size and name is sin-code itself, it's likely a copy
				if selfInfo.Size() == info.Size() && name != "sin-code" {
					continue
				}
			}
			return p, nil
		}
	}
	// Also check PATH
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
