// SPDX-License-Identifier: MIT
// Purpose: language-agnostic verification check detection. Picks build/test/
// lint commands based on ecosystem marker files so the verify-gate works on
// polyglot repos, not just Go.
package orchestrator

import (
	"os"
	"path/filepath"
)

type ecosystem struct {
	marker string
	checks []Check
}

// detectors are evaluated in order; every matching ecosystem contributes its
// checks (monorepos may match several).
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
func DetectChecks(workspace string) []Check {
	var out []Check
	for _, d := range detectors {
		if fileExists(filepath.Join(workspace, d.marker)) {
			out = append(out, d.checks...)
		}
	}
	if len(out) == 0 {
		return DefaultGoChecks()
	}
	return out
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
