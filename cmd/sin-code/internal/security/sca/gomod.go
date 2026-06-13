// SPDX-License-Identifier: MIT
// Purpose: native go.mod dependency parsing for the SCA scanner.
// Docs: sca.doc.md
package sca

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

// readFile is swappable for tests that exercise ParseGoMod.
var readFile = os.ReadFile

// ParseGoMod reads go.mod in projectPath and returns direct and indirect
// module dependencies as Go-ecosystem packages.
func ParseGoMod(projectPath string) ([]Package, error) {
	goMod := filepath.Join(projectPath, "go.mod")
	data, err := readFile(goMod)
	if err != nil {
		return nil, fmt.Errorf("read go.mod: %w", err)
	}
	return parseGoModBytes(data)
}

// parseGoModBytes parses go.mod content using golang.org/x/mod/modfile.
func parseGoModBytes(data []byte) ([]Package, error) {
	f, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return nil, fmt.Errorf("parse go.mod: %w", err)
	}

	out := make([]Package, 0, len(f.Require))
	seen := make(map[string]bool, len(f.Require))
	for _, r := range f.Require {
		if r == nil || r.Mod.Path == "" {
			continue
		}
		if seen[r.Mod.Path] {
			continue
		}
		seen[r.Mod.Path] = true
		out = append(out, Package{
			Name:      r.Mod.Path,
			Version:   r.Mod.Version,
			Ecosystem: "Go",
		})
	}
	return out, nil
}
