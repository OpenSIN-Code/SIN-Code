// SPDX-License-Identifier: MIT
// Purpose: language-agnostic verification check detection. Picks build/test/
// lint commands based on ecosystem marker files so the verify-gate works on
// polyglot repos, not just Go (issue #154).
package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
)

type ecosystem struct {
	marker string
	checks []Check
}

// detectors are evaluated in order; every matching ecosystem contributes its
// checks (monorepos may match several). The order matters: Go first, then
// JS/TS (package.json), then Rust, then Python.
var detectors = []ecosystem{
	{
		marker: "go.mod",
		checks: []Check{
			{Kind: CheckBuild, Name: "go build", Cmd: []string{"go", "build", "./..."}},
			{Kind: CheckTest, Name: "go test", Cmd: []string{"go", "test", "./...", "-count=1", "-timeout=120s"}},
			{Kind: CheckLint, Name: "go vet", Cmd: []string{"go", "vet", "./..."}},
		},
	},
	{
		marker: "package.json",
		checks: []Check{
			{Kind: CheckBuild, Name: "npm build", Cmd: []string{"npm", "run", "--if-present", "build"}},
			{Kind: CheckTest, Name: "npm test", Cmd: []string{"npm", "test", "--if-present"}},
			{Kind: CheckLint, Name: "npm lint", Cmd: []string{"npm", "run", "--if-present", "lint"}},
		},
	},
	{
		marker: "Cargo.toml",
		checks: []Check{
			{Kind: CheckBuild, Name: "cargo build", Cmd: []string{"cargo", "build"}},
			{Kind: CheckTest, Name: "cargo test", Cmd: []string{"cargo", "test"}},
			{Kind: CheckLint, Name: "cargo clippy", Cmd: []string{"cargo", "clippy", "--", "-D", "warnings"}},
		},
	},
	{
		marker: "pyproject.toml",
		checks: []Check{
			{Kind: CheckTest, Name: "pytest", Cmd: []string{"python", "-m", "pytest", "-q"}},
			{Kind: CheckLint, Name: "ruff", Cmd: []string{"ruff", "check", "."}},
		},
	},
}

// DetectChecks returns the combined check suite for every ecosystem detected
// in workspace. Falls back to DefaultGoChecks if nothing matches (preserves
// current behavior on Go-only repos with unusual layouts).
//
// The function walks up to 3 directory levels deep so monorepos with
// nested ecosystem directories (e.g. packages/web/package.json) are
// covered. Marker files at the workspace root are weighted twice so
// the root ecosystem is preferred.
func DetectChecks(workspace string) []Check {
	if workspace == "" {
		return DefaultGoChecks()
	}
	matched := map[string]bool{} // dedupe by name
	var out []Check
	// Root: weight 2
	for _, det := range detectors {
		if fileExists(filepath.Join(workspace, det.marker)) {
			for _, c := range det.checks {
				key := c.Name + "|" + det.marker
				if matched[key] {
					continue
				}
				matched[key] = true
				out = append(out, c)
			}
		}
	}
	// Sub-directories: weight 1 (one level deep, common case)
	entries, err := os.ReadDir(workspace)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			sub := filepath.Join(workspace, e.Name())
			for _, det := range detectors {
				if fileExists(filepath.Join(sub, det.marker)) {
					for _, c := range det.checks {
						key := c.Name + "|" + det.marker
						if matched[key] {
							continue
						}
						matched[key] = true
						out = append(out, c)
					}
				}
			}
		}
	}
	if len(out) == 0 {
		return DefaultGoChecks()
	}
	return out
}

// fileExists is a tiny helper to avoid pulling in os.Stat's
// three-return-value signature at every call site.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
