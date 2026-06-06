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
func lookupStandalone(name string) (string, error) {
	candidates := []string{
		filepath.Join(os.Getenv("HOME"), ".local", "bin", name),
		filepath.Join("/usr", "local", "bin", name),
		filepath.Join("/opt", "homebrew", "bin", name),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	// Also check PATH
	if path, err := exec.LookPath(name); err == nil {
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
