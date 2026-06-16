// SPDX-License-Identifier: MIT
// Purpose: PostToolUse hook that detects writes/edits to .go files and
// emits a coverage-driven test-generation request so the package can stay
// at 100% statement coverage. See cmd/sin-code/internal/coverdrohne.
// Docs: coverage.doc.md
package hooklife

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/coverdrohne"
)

// jsonMarshalIndentHook is swappable for tests that exercise the
// (theoretically unreachable) JSON marshal error branch.
var jsonMarshalIndentHook = json.MarshalIndent

// RequestDir is the project-local directory where auto-coverage requests
// are queued. It is relative to the working directory (ev.Workdir).
const RequestDir = ".sin-code/coverage-requests"

// CoverageGenerator is satisfied by coverdrohne.Generate.
type CoverageGenerator interface {
	Generate(root, packages, pkg string) error
}

// AutoCoverage is a PostToolUse hook. After a Write or Edit touches a .go
// file, it maps the file to its package import path and writes a JSON
// request to .sin-code/coverage-requests/<pkg>.json. The request can be
// consumed by `sin-code cover generate` or by an autonomous worker.
type AutoCoverage struct {
	// Enabled toggles the hook. Default false for privacy/performance.
	Enabled bool
	// RequestDir overrides the default queue directory.
	RequestDir string
	// PackagePath maps a root + file path to a Go import path.
	// In production this is coverdrohne.PackageImportPath.
	PackagePath func(root, file string) string
	// mkdirAll creates directories; swappable for tests.
	mkdirAll func(path string, perm os.FileMode) error
	// writeFile writes files; swappable for tests.
	writeFile func(name string, data []byte, perm os.FileMode) error
}

func (AutoCoverage) ID() string      { return "auto-coverage" }
func (AutoCoverage) Phases() []Phase { return []Phase{PostToolUse} }

func (a AutoCoverage) Run(_ context.Context, ev Event) Decision {
	if !a.Enabled || (ev.Tool != "Edit" && ev.Tool != "Write") {
		return Decision{Verdict: Allow}
	}
	path := ev.Args["path"]
	if !strings.HasSuffix(path, ".go") {
		return Decision{Verdict: Allow}
	}
	pkgPath := a.PackagePath
	if pkgPath == nil {
		pkgPath = coverdrohne.PackageImportPath
	}
	importPath := pkgPath(ev.Workdir, path)
	reqDir := coverageRequestDirWithOverride(ev.Workdir, a.RequestDir)
	mkdir := a.mkdirAll
	if mkdir == nil {
		mkdir = os.MkdirAll
	}
	write := a.writeFile
	if write == nil {
		write = os.WriteFile
	}
	if err := mkdir(reqDir, 0o755); err != nil {
		return Decision{Verdict: Warn, Message: "auto-coverage: cannot create request dir: " + err.Error()}
	}
	req := map[string]string{
		"package": importPath,
		"file":    path,
		"hint":    "auto-generated request after " + ev.Tool,
	}
	data, err := jsonMarshalIndentHook(req, "", "  ")
	if err != nil {
		return Decision{Verdict: Warn, Message: "auto-coverage: cannot marshal request: " + err.Error()}
	}
	fileName := strings.ReplaceAll(importPath, "/", "--") + ".json"
	dest := filepath.Join(reqDir, fileName)
	if err := write(dest, data, 0o644); err != nil {
		return Decision{Verdict: Warn, Message: "auto-coverage: cannot write request: " + err.Error()}
	}
	return Decision{Verdict: Warn, Message: "auto-coverage: queued request for " + importPath + " at " + dest}
}

// coverageRequestDirWithOverride returns the absolute queue directory,
// honouring the override if non-empty.
func coverageRequestDirWithOverride(workdir, override string) string {
	dir := override
	if dir == "" {
		dir = RequestDir
	}
	if workdir == "" {
		return dir
	}
	return filepath.Join(workdir, dir)
}
