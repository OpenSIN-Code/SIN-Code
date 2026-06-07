// SPDX-License-Identifier: MIT
// Purpose: Tests for the sbom subcommand.
package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── detectProjectType ───────────────────────────────────────────────────────

func TestDetectProjectType(t *testing.T) {
	dir := t.TempDir()

	// Go project
	goDir := filepath.Join(dir, "go-proj")
	os.MkdirAll(goDir, 0755)
	os.WriteFile(filepath.Join(goDir, "go.mod"), []byte("module test\n"), 0644)
	if got := detectProjectType(goDir); got != "go" {
		t.Errorf("detectProjectType(go.mod) = %q, want go", got)
	}

	// Node project
	nodeDir := filepath.Join(dir, "node-proj")
	os.MkdirAll(nodeDir, 0755)
	os.WriteFile(filepath.Join(nodeDir, "package.json"), []byte("{}"), 0644)
	if got := detectProjectType(nodeDir); got != "node" {
		t.Errorf("detectProjectType(package.json) = %q, want node", got)
	}

	// Python project
	pyDir := filepath.Join(dir, "py-proj")
	os.MkdirAll(pyDir, 0755)
	os.WriteFile(filepath.Join(pyDir, "requirements.txt"), []byte("requests\n"), 0644)
	if got := detectProjectType(pyDir); got != "python" {
		t.Errorf("detectProjectType(requirements.txt) = %q, want python", got)
	}

	pyDir2 := filepath.Join(dir, "py-proj2")
	os.MkdirAll(pyDir2, 0755)
	os.WriteFile(filepath.Join(pyDir2, "pyproject.toml"), []byte("[project]\n"), 0644)
	if got := detectProjectType(pyDir2); got != "python" {
		t.Errorf("detectProjectType(pyproject.toml) = %q, want python", got)
	}

	// Generic project
	genDir := filepath.Join(dir, "generic-proj")
	os.MkdirAll(genDir, 0755)
	if got := detectProjectType(genDir); got != "generic" {
		t.Errorf("detectProjectType(empty) = %q, want generic", got)
	}
}

// ── Go module parsing ───────────────────────────────────────────────────────

func TestParseGoListOutput(t *testing.T) {
	raw := `{"Path": "github.com/spf13/cobra", "Version": "v1.10.2", "Main": false}
{"Path": "github.com/spf13/pflag", "Version": "v1.0.5", "Main": false}
{"Path": "example.com/mymod", "Main": true, "Version": "v0.0.0"}
`
	deps, err := parseGoListOutput(raw)
	if err != nil {
		t.Fatalf("parseGoListOutput error: %v", err)
	}
	if len(deps) != 3 {
		t.Fatalf("expected 3 deps, got %d", len(deps))
	}
	if deps[0].Name != "github.com/spf13/cobra" || deps[0].Version != "v1.10.2" {
		t.Errorf("dep[0] = %s@%s, want github.com/spf13/cobra@v1.10.2", deps[0].Name, deps[0].Version)
	}
	if deps[2].Purpose != "APPLICATION" {
		t.Errorf("dep[2].Purpose = %q, want APPLICATION", deps[2].Purpose)
	}
}

func TestParseGoModFallback(t *testing.T) {
	dir := t.TempDir()
	content := `module example.com/test

go 1.21

require (
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.5
)
`
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0644)
	deps, err := parseGoModFallback(dir)
	if err != nil {
		t.Fatalf("parseGoModFallback error: %v", err)
	}
	if len(deps) != 2 {
		t.Fatalf("expected 2 deps, got %d", len(deps))
	}
	if deps[0].Name != "github.com/spf13/cobra" || deps[0].Version != "v1.10.2" {
		t.Errorf("dep[0] = %s@%s", deps[0].Name, deps[0].Version)
	}
}

// ── Python parsing ────────────────────────────────────────────────────────────

func TestParseRequirementsTxt(t *testing.T) {
	content := `requests>=2.28.0
flask==2.3.0
# comment
pytest~=7.0.0
-e git+https://github.com/user/repo.git#egg=repo
`
	deps := parseRequirementsTxt(content)
	if len(deps) != 3 {
		t.Fatalf("expected 3 deps, got %d: %+v", len(deps), deps)
	}
	expected := []struct{ name, version string }{
		{"requests", ">=2.28.0"},
		{"flask", "==2.3.0"},
		{"pytest", "~=7.0.0"},
	}
	for i, exp := range expected {
		if deps[i].Name != exp.name {
			t.Errorf("dep[%d].Name = %q, want %q", i, deps[i].Name, exp.name)
		}
		if deps[i].Version != exp.version {
			t.Errorf("dep[%d].Version = %q, want %q", i, deps[i].Version, exp.version)
		}
	}
}

func TestParsePyprojectToml(t *testing.T) {
	content := `[build-system]
requires = ["setuptools>=61.0"]

[project]
name = "example"
version = "1.0.0"

[project.dependencies]
requests = ">=2.28.0"
flask = "^2.3.0"
`
	deps := parsePyprojectToml(content)
	if len(deps) < 2 {
		t.Fatalf("expected at least 2 deps, got %d: %+v", len(deps), deps)
	}
	found := make(map[string]bool)
	for _, d := range deps {
		found[d.Name] = true
	}
	if !found["requests"] {
		t.Errorf("expected 'requests' in deps")
	}
	if !found["flask"] {
		t.Errorf("expected 'flask' in deps")
	}
}

// ── Node.js parsing ───────────────────────────────────────────────────────────

func TestParsePackageJSON(t *testing.T) {
	data := []byte(`{
  "name": "test-app",
  "version": "1.0.0",
  "dependencies": {
    "express": "^4.18.0",
    "lodash": "^4.17.21"
  },
  "devDependencies": {
    "jest": "^29.0.0"
  }
}`)
	deps, err := parsePackageJSON(data)
	if err != nil {
		t.Fatalf("parsePackageJSON error: %v", err)
	}
	if len(deps) != 3 {
		t.Fatalf("expected 3 deps, got %d", len(deps))
	}
	found := make(map[string]string)
	for _, d := range deps {
		found[d.Name] = d.Version
	}
	if found["express"] != "^4.18.0" {
		t.Errorf("express version = %q, want ^4.18.0", found["express"])
	}
	if found["lodash"] != "^4.17.21" {
		t.Errorf("lodash version = %q, want ^4.17.21", found["lodash"])
	}
	if found["jest"] != "^29.0.0" {
		t.Errorf("jest version = %q, want ^29.0.0", found["jest"])
	}
}

// ── SPDX format validation ──────────────────────────────────────────────────

func TestGenerateSPDXFormat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0644)
	doc, err := generateSPDX(dir, "go", "test-project", "2026-06-07T12:00:00Z")
	if err != nil {
		t.Fatalf("generateSPDX error: %v", err)
	}

	if doc.SPDXVersion != "SPDX-2.3" {
		t.Errorf("spdxVersion = %q, want SPDX-2.3", doc.SPDXVersion)
	}
	if doc.SPDXID != "SPDXRef-DOCUMENT" {
		t.Errorf("SPDXID = %q, want SPDXRef-DOCUMENT", doc.SPDXID)
	}
	if doc.Name != "test-project" {
		t.Errorf("name = %q, want test-project", doc.Name)
	}
	if doc.CreationInfo.Created == "" {
		t.Errorf("creationInfo.created is empty")
	}
	if len(doc.CreationInfo.Creators) == 0 {
		t.Errorf("creationInfo.creators is empty")
	}
	if len(doc.Packages) == 0 {
		t.Errorf("packages is empty")
	}

	for _, pkg := range doc.Packages {
		if pkg.SPDXID == "" {
			t.Errorf("package SPDXID is empty")
		}
		if pkg.Name == "" {
			t.Errorf("package name is empty")
		}
		if pkg.LicenseConcluded == "" {
			t.Errorf("package licenseConcluded is empty")
		}
		if pkg.PrimaryPackagePurpose == "" {
			t.Errorf("package primaryPackagePurpose is empty")
		}
	}

	// Verify JSON marshalling
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	if !strings.Contains(string(out), `"spdxVersion"`) {
		t.Errorf("JSON missing spdxVersion field")
	}
	if !strings.Contains(string(out), `"packages"`) {
		t.Errorf("JSON missing packages field")
	}
}

// ── CycloneDX format validation ───────────────────────────────────────────────

func TestGenerateCycloneDXFormat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"test","version":"1.0.0","dependencies":{"express":"^4.18.0"}}`), 0644)
	doc, err := generateCycloneDX(dir, "node", "test-project", "2026-06-07T12:00:00Z")
	if err != nil {
		t.Fatalf("generateCycloneDX error: %v", err)
	}

	if doc.BomFormat != "CycloneDX" {
		t.Errorf("bomFormat = %q, want CycloneDX", doc.BomFormat)
	}
	if doc.SpecVersion != "1.5" {
		t.Errorf("specVersion = %q, want 1.5", doc.SpecVersion)
	}
	if doc.SerialNumber == "" {
		t.Errorf("serialNumber is empty")
	}
	if doc.Version != 1 {
		t.Errorf("version = %d, want 1", doc.Version)
	}
	if doc.Metadata.Timestamp == "" {
		t.Errorf("metadata.timestamp is empty")
	}
	if len(doc.Metadata.Tools) == 0 {
		t.Errorf("metadata.tools is empty")
	}
	if len(doc.Components) == 0 {
		t.Errorf("components is empty")
	}

	for _, comp := range doc.Components {
		if comp.Type == "" {
			t.Errorf("component type is empty")
		}
		if comp.BomRef == "" {
			t.Errorf("component bom-ref is empty")
		}
		if comp.Name == "" {
			t.Errorf("component name is empty")
		}
	}

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	if !strings.Contains(string(out), `"bomFormat"`) {
		t.Errorf("JSON missing bomFormat field")
	}
	if !strings.Contains(string(out), `"components"`) {
		t.Errorf("JSON missing components field")
	}
}

// ── Integration: collectDependencies ───────────────────────────────────────────

func TestCollectDependenciesNode(t *testing.T) {
	dir := t.TempDir()
	pkg := `{"name":"test","version":"1.0.0","dependencies":{"express":"^4.18.0","lodash":"^4.17.21"},"devDependencies":{"jest":"^29.0.0"}}`
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0644)

	lock := `{"packages":{"node_modules/express":{"version":"4.18.2"},"node_modules/lodash":{"version":"4.17.21"}}}`
	os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(lock), 0644)

	deps, err := collectDependencies(dir, "node")
	if err != nil {
		t.Fatalf("collectDependencies error: %v", err)
	}
	if len(deps) != 3 {
		t.Fatalf("expected 3 deps, got %d", len(deps))
	}

	found := make(map[string]string)
	for _, d := range deps {
		found[d.Name] = d.Version
	}
	if found["express"] != "4.18.2" {
		t.Errorf("express version = %q, want 4.18.2", found["express"])
	}
	if found["lodash"] != "4.17.21" {
		t.Errorf("lodash version = %q, want 4.17.21", found["lodash"])
	}
}

func TestCollectDependenciesGeneric(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src"), 0755)
	os.MkdirAll(filepath.Join(dir, "tests"), 0755)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# hello"), 0644)

	deps := collectGenericDeps(dir)
	if len(deps) != 2 {
		t.Fatalf("expected 2 deps, got %d: %+v", len(deps), deps)
	}
	found := make(map[string]bool)
	for _, d := range deps {
		found[d.Name] = true
	}
	if !found["src"] {
		t.Errorf("expected 'src' in generic deps")
	}
	if !found["tests"] {
		t.Errorf("expected 'tests' in generic deps")
	}
}

// ── sanitizeSPDXID ────────────────────────────────────────────────────────────

func TestSanitizeSPDXID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"github.com/spf13/cobra", "github.com-spf13-cobra"},
		{"some/package", "some-package"},
		{"normal_pkg-1.0", "normal_pkg-1.0"},
	}
	for _, c := range cases {
		got := sanitizeSPDXID(c.in)
		if got != c.want {
			t.Errorf("sanitizeSPDXID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
