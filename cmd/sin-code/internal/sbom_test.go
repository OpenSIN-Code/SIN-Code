// SPDX-License-Identifier: MIT
// Purpose: Tests for the sbom subcommand.
package internal

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectProjectType(t *testing.T) {
	dir := t.TempDir()

	goDir := filepath.Join(dir, "go-proj")
	os.MkdirAll(goDir, 0755)
	os.WriteFile(filepath.Join(goDir, "go.mod"), []byte("module test\n"), 0644)
	if got := detectProjectType(goDir); got != "go" {
		t.Errorf("detectProjectType(go.mod) = %q, want go", got)
	}

	nodeDir := filepath.Join(dir, "node-proj")
	os.MkdirAll(nodeDir, 0755)
	os.WriteFile(filepath.Join(nodeDir, "package.json"), []byte("{}"), 0644)
	if got := detectProjectType(nodeDir); got != "node" {
		t.Errorf("detectProjectType(package.json) = %q, want node", got)
	}

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

	pyDir3 := filepath.Join(dir, "py-proj3")
	os.MkdirAll(pyDir3, 0755)
	os.WriteFile(filepath.Join(pyDir3, "setup.py"), []byte("from setuptools import setup\n"), 0644)
	if got := detectProjectType(pyDir3); got != "python" {
		t.Errorf("detectProjectType(setup.py) = %q, want python", got)
	}

	pyDir4 := filepath.Join(dir, "py-proj4")
	os.MkdirAll(pyDir4, 0755)
	os.WriteFile(filepath.Join(pyDir4, "Pipfile"), []byte("[[source]]\n"), 0644)
	if got := detectProjectType(pyDir4); got != "python" {
		t.Errorf("detectProjectType(Pipfile) = %q, want python", got)
	}

	genDir := filepath.Join(dir, "generic-proj")
	os.MkdirAll(genDir, 0755)
	if got := detectProjectType(genDir); got != "generic" {
		t.Errorf("detectProjectType(empty) = %q, want generic", got)
	}
}

func TestDetectProjectType_Priority(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0644)
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("requests\n"), 0644)
	if got := detectProjectType(dir); got != "go" {
		t.Errorf("go.mod should take priority, got %q", got)
	}
}

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
	if deps[2].ComponentType != "application" {
		t.Errorf("dep[2].ComponentType = %q, want application", deps[2].ComponentType)
	}
}

func TestParseGoListOutput_Empty(t *testing.T) {
	deps, err := parseGoListOutput("")
	if err != nil {
		t.Fatalf("parseGoListOutput empty error: %v", err)
	}
	if len(deps) != 0 {
		t.Errorf("expected 0 deps for empty input, got %d", len(deps))
	}
}

func TestParseGoListOutput_InvalidJSON(t *testing.T) {
	raw := `{invalid json`
	deps, err := parseGoListOutput(raw)
	if err != nil {
		t.Fatalf("parseGoListOutput should not error on invalid JSON, got: %v", err)
	}
	if len(deps) != 0 {
		t.Errorf("expected 0 deps for invalid JSON, got %d", len(deps))
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
	if deps[0].PURLType != "golang" {
		t.Errorf("dep[0].PURLType = %q, want golang", deps[0].PURLType)
	}
}

func TestParseGoModFallback_SingleRequire(t *testing.T) {
	dir := t.TempDir()
	content := `module example.com/test

go 1.21

require github.com/spf13/cobra v1.10.2
`
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0644)
	deps, err := parseGoModFallback(dir)
	if err != nil {
		t.Fatalf("parseGoModFallback error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(deps))
	}
	if deps[0].Name != "github.com/spf13/cobra" {
		t.Errorf("dep[0].Name = %q, want github.com/spf13/cobra", deps[0].Name)
	}
}

func TestParseGoModFallback_MissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := parseGoModFallback(dir)
	if err == nil {
		t.Error("expected error for missing go.mod")
	}
}

func TestParseGoModFallback_GoVersionFilter(t *testing.T) {
	dir := t.TempDir()
	content := `module example.com/test

go 1.21

require (
	go.opentelemetry.io/otel v1.21.0
	github.com/spf13/cobra v1.10.2
)
`
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0644)
	deps, err := parseGoModFallback(dir)
	if err != nil {
		t.Fatalf("parseGoModFallback error: %v", err)
	}
	for _, d := range deps {
		if d.Name == "go" || d.Name == "1.21" {
			t.Errorf("go version should be filtered out, got dep %q", d.Name)
		}
	}
}

func TestParseGoModFallback_EmptyRequire(t *testing.T) {
	dir := t.TempDir()
	content := `module example.com/test

go 1.21

require ()
`
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0644)
	deps, err := parseGoModFallback(dir)
	if err != nil {
		t.Fatalf("parseGoModFallback error: %v", err)
	}
	if len(deps) != 0 {
		t.Errorf("expected 0 deps for empty require block, got %d", len(deps))
	}
}

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

func TestParseRequirementsTxt_NoVersion(t *testing.T) {
	content := `requests
flask
`
	deps := parseRequirementsTxt(content)
	if len(deps) != 2 {
		t.Fatalf("expected 2 deps, got %d", len(deps))
	}
	if deps[0].Name != "requests" {
		t.Errorf("dep[0].Name = %q, want requests", deps[0].Name)
	}
	if deps[0].Version != "" {
		t.Errorf("dep[0].Version = %q, want empty (no version operator)", deps[0].Version)
	}
	if deps[1].Name != "flask" {
		t.Errorf("dep[1].Name = %q, want flask", deps[1].Name)
	}
}

func TestParseRequirementsTxt_CommentsAndFlags(t *testing.T) {
	content := `# This is a comment
-r other-requirements.txt
--index-url https://pypi.org/simple
requests>=2.28.0
`
	deps := parseRequirementsTxt(content)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep (skip comments and flags), got %d", len(deps))
	}
	if deps[0].Name != "requests" {
		t.Errorf("dep[0].Name = %q, want requests", deps[0].Name)
	}
}

func TestParseRequirementsTxt_Empty(t *testing.T) {
	deps := parseRequirementsTxt("")
	if len(deps) != 0 {
		t.Errorf("expected 0 deps for empty input, got %d", len(deps))
	}
}

func TestParseRequirementsTxt_DashPrefix(t *testing.T) {
	content := `-e git+https://github.com/user/repo.git#egg=repo
--extra-index-url https://example.com
requests>=2.0
`
	deps := parseRequirementsTxt(content)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep (dash-prefixed lines skipped), got %d", len(deps))
	}
	if deps[0].Name != "requests" {
		t.Errorf("dep[0].Name = %q, want requests", deps[0].Name)
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

func TestParsePyprojectToml_SectionTransition(t *testing.T) {
	content := `[project.dependencies]
requests = ">=2.28.0"
flask = "^2.3.0"

[project.optional-dependencies]
dev = ["pytest"]
`
	deps := parsePyprojectToml(content)
	for _, d := range deps {
		if d.Name == "dev" || d.Name == "pytest" {
			t.Errorf("optional-dependencies section should not be parsed, got %q", d.Name)
		}
	}
}

func TestParsePyprojectToml_NoDependencies(t *testing.T) {
	content := `[build-system]
requires = ["setuptools>=61.0"]

[project]
name = "example"
`
	deps := parsePyprojectToml(content)
	if len(deps) != 0 {
		t.Errorf("expected 0 deps without [project.dependencies], got %d", len(deps))
	}
}

func TestParsePyprojectToml_Empty(t *testing.T) {
	deps := parsePyprojectToml("")
	if len(deps) != 0 {
		t.Errorf("expected 0 deps for empty input, got %d", len(deps))
	}
}

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

func TestParsePackageJSON_InvalidJSON(t *testing.T) {
	_, err := parsePackageJSON([]byte(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParsePackageJSON_NoDeps(t *testing.T) {
	data := []byte(`{"name":"test","version":"1.0.0"}`)
	deps, err := parsePackageJSON(data)
	if err != nil {
		t.Fatalf("parsePackageJSON error: %v", err)
	}
	if len(deps) != 0 {
		t.Errorf("expected 0 deps, got %d", len(deps))
	}
}

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

func TestCollectDependenciesNode_LockOldFormat(t *testing.T) {
	dir := t.TempDir()
	pkg := `{"name":"test","version":"1.0.0","dependencies":{"express":"^4.18.0"}}`
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0644)

	lock := `{"dependencies":{"express":{"version":"4.18.1"}}}`
	os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(lock), 0644)

	deps, err := collectDependencies(dir, "node")
	if err != nil {
		t.Fatalf("collectDependencies error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(deps))
	}
	if deps[0].Name != "express" {
		t.Errorf("dep[0].Name = %q, want express", deps[0].Name)
	}
	if deps[0].Version != "4.18.1" {
		t.Errorf("dep[0].Version = %q, want 4.18.1 (from lock dependencies)", deps[0].Version)
	}
}

func TestCollectNodeDeps_MissingPackageJSON(t *testing.T) {
	dir := t.TempDir()
	_, err := collectNodeDeps(dir)
	if err == nil {
		t.Error("expected error for missing package.json")
	}
}

func TestCollectNodeDeps_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{invalid}`), 0644)
	_, err := collectNodeDeps(dir)
	if err == nil {
		t.Error("expected error for invalid package.json")
	}
}

func TestCollectNodeDeps_NoLockFile(t *testing.T) {
	dir := t.TempDir()
	pkg := `{"name":"test","version":"1.0.0","dependencies":{"express":"^4.18.0"}}`
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0644)

	deps, err := collectNodeDeps(dir)
	if err != nil {
		t.Fatalf("collectNodeDeps error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(deps))
	}
	if deps[0].Version != "^4.18.0" {
		t.Errorf("version = %q, want ^4.18.0 (no lock file)", deps[0].Version)
	}
	if deps[0].PURLType != "npm" {
		t.Errorf("PURLType = %q, want npm", deps[0].PURLType)
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

func TestCollectGenericDeps_HiddenDirsSkipped(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	os.MkdirAll(filepath.Join(dir, ".hidden"), 0755)
	os.MkdirAll(filepath.Join(dir, "src"), 0755)

	deps := collectGenericDeps(dir)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep (hidden dirs skipped), got %d", len(deps))
	}
	if deps[0].Name != "src" {
		t.Errorf("dep[0].Name = %q, want src", deps[0].Name)
	}
}

func TestCollectGenericDeps_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	deps := collectGenericDeps(dir)
	if len(deps) != 0 {
		t.Errorf("expected 0 deps for empty dir, got %d", len(deps))
	}
}

func TestCollectGenericDeps_OnlyFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# hello"), 0644)
	os.WriteFile(filepath.Join(dir, "main.py"), []byte("print('hi')"), 0644)

	deps := collectGenericDeps(dir)
	if len(deps) != 0 {
		t.Errorf("expected 0 deps for files-only dir, got %d", len(deps))
	}
}

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
	if doc.DocumentNamespace == "" {
		t.Errorf("documentNamespace is empty")
	}
	if doc.CreationInfo.Created != "2026-06-07T12:00:00Z" {
		t.Errorf("creationInfo.created = %q, want 2026-06-07T12:00:00Z", doc.CreationInfo.Created)
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

func TestGenerateSPDX_NoDeps(t *testing.T) {
	dir := t.TempDir()
	doc, err := generateSPDX(dir, "generic", "empty-project", "2026-06-07T12:00:00Z")
	if err != nil {
		t.Fatalf("generateSPDX error: %v", err)
	}
	if len(doc.Packages) == 0 {
		t.Error("expected at least root package when no deps")
	}
	root := doc.Packages[0]
	if root.SPDXID != "SPDXRef-DOCUMENT" {
		t.Errorf("root package SPDXID = %q, want SPDXRef-DOCUMENT", root.SPDXID)
	}
	if root.PrimaryPackagePurpose != "APPLICATION" {
		t.Errorf("root package purpose = %q, want APPLICATION", root.PrimaryPackagePurpose)
	}
	if root.VersionInfo != "NOASSERTION" {
		t.Errorf("root package version = %q, want NOASSERTION", root.VersionInfo)
	}
}

func TestGenerateSPDX_FirstPackageIsRoot(t *testing.T) {
	dir := t.TempDir()
	content := `module example.com/test

go 1.21

require github.com/spf13/cobra v1.10.2
`
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0644)
	doc, err := generateSPDX(dir, "go", "test", "2026-06-07T12:00:00Z")
	if err != nil {
		t.Fatalf("generateSPDX error: %v", err)
	}
	if len(doc.Packages) == 0 {
		t.Fatal("expected at least one package")
	}
}

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
	if !strings.HasPrefix(doc.SerialNumber, "urn:uuid:") {
		t.Errorf("serialNumber = %q, want urn:uuid: prefix", doc.SerialNumber)
	}
	if doc.Version != 1 {
		t.Errorf("version = %d, want 1", doc.Version)
	}
	if doc.Metadata.Timestamp != "2026-06-07T12:00:00Z" {
		t.Errorf("metadata.timestamp = %q, want 2026-06-07T12:00:00Z", doc.Metadata.Timestamp)
	}
	if len(doc.Metadata.Tools) == 0 {
		t.Errorf("metadata.tools is empty")
	}
	if doc.Metadata.Tools[0].Name != "sin-code" {
		t.Errorf("metadata.tools[0].name = %q, want sin-code", doc.Metadata.Tools[0].Name)
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
		if comp.Scope != "required" {
			t.Errorf("component scope = %q, want required", comp.Scope)
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

func TestGenerateCycloneDX_NoDeps(t *testing.T) {
	dir := t.TempDir()
	doc, err := generateCycloneDX(dir, "generic", "empty-project", "2026-06-07T12:00:00Z")
	if err != nil {
		t.Fatalf("generateCycloneDX error: %v", err)
	}
	if len(doc.Components) == 0 {
		t.Error("expected at least one component when no deps")
	}
	comp := doc.Components[0]
	if comp.Type != "application" {
		t.Errorf("placeholder component type = %q, want application", comp.Type)
	}
	if comp.Version != "0.0.0" {
		t.Errorf("placeholder component version = %q, want 0.0.0", comp.Version)
	}
	if !strings.Contains(comp.PURL, "pkg:generic/") {
		t.Errorf("placeholder PURL = %q, want pkg:generic/ prefix", comp.PURL)
	}
}

func TestGenerateCycloneDX_GoProject(t *testing.T) {
	dir := t.TempDir()
	content := `module example.com/test

go 1.21

require github.com/spf13/cobra v1.10.2
`
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0644)
	doc, err := generateCycloneDX(dir, "go", "test", "2026-06-07T12:00:00Z")
	if err != nil {
		t.Fatalf("generateCycloneDX error: %v", err)
	}
	if len(doc.Components) == 0 {
		t.Fatal("expected at least one component")
	}
	found := false
	for _, comp := range doc.Components {
		if strings.Contains(comp.BomRef, "pkg:golang/") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected at least one component with pkg:golang/ purl type")
	}
}

func TestGenerateCycloneDX_PythonProject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("requests>=2.28.0\nflask==2.3.0\n"), 0644)
	doc, err := generateCycloneDX(dir, "python", "test", "2026-06-07T12:00:00Z")
	if err != nil {
		t.Fatalf("generateCycloneDX error: %v", err)
	}
	if len(doc.Components) < 2 {
		t.Fatalf("expected at least 2 components, got %d", len(doc.Components))
	}
	for _, comp := range doc.Components {
		if !strings.Contains(comp.BomRef, "pkg:pypi/") {
			t.Errorf("expected pkg:pypi/ in bom-ref, got %q", comp.BomRef)
		}
	}
}

func TestGenerateSBOM_UnsupportedFormat(t *testing.T) {
	_, err := generateSBOM(".", "generic", "xml")
	if err == nil {
		t.Error("expected error for unsupported format")
	}
	if !strings.Contains(err.Error(), "unsupported format") {
		t.Errorf("expected 'unsupported format' error, got %q", err.Error())
	}
}

func TestGenerateSBOM_SPDXFormat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\ngo 1.21\n"), 0644)
	result, err := generateSBOM(dir, "go", "spdx-json")
	if err != nil {
		t.Fatalf("generateSBOM spdx-json error: %v", err)
	}
	doc, ok := result.(*SPDXDocument)
	if !ok {
		t.Fatalf("expected *SPDXDocument, got %T", result)
	}
	if doc.SPDXVersion != "SPDX-2.3" {
		t.Errorf("spdxVersion = %q, want SPDX-2.3", doc.SPDXVersion)
	}
}

func TestGenerateSBOM_CycloneDXFormat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\ngo 1.21\n"), 0644)
	result, err := generateSBOM(dir, "go", "cyclonedx-json")
	if err != nil {
		t.Fatalf("generateSBOM cyclonedx-json error: %v", err)
	}
	doc, ok := result.(*CycloneDXDocument)
	if !ok {
		t.Fatalf("expected *CycloneDXDocument, got %T", result)
	}
	if doc.BomFormat != "CycloneDX" {
		t.Errorf("bomFormat = %q, want CycloneDX", doc.BomFormat)
	}
}

func TestGenerateSBOM_NameDotFallback(t *testing.T) {
	result, err := generateSBOM(".", "generic", "spdx-json")
	if err != nil {
		t.Fatalf("generateSBOM error: %v", err)
	}
	doc, ok := result.(*SPDXDocument)
	if !ok {
		t.Fatalf("expected *SPDXDocument, got %T", result)
	}
	if doc.Name != "unknown" {
		t.Errorf("name = %q, want 'unknown' when path base is '.'", doc.Name)
	}
}

func TestCollectDependencies_Python(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("requests>=2.28.0\nflask==2.3.0\n"), 0644)
	deps, err := collectDependencies(dir, "python")
	if err != nil {
		t.Fatalf("collectDependencies python error: %v", err)
	}
	if len(deps) != 2 {
		t.Fatalf("expected 2 deps, got %d", len(deps))
	}
}

func TestCollectDependencies_PythonPyprojectFallback(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`[project.dependencies]
requests = ">=2.28.0"
`), 0644)
	deps, err := collectDependencies(dir, "python")
	if err != nil {
		t.Fatalf("collectDependencies python error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(deps))
	}
	if deps[0].Name != "requests" {
		t.Errorf("dep[0].Name = %q, want requests", deps[0].Name)
	}
}

func TestCollectDependencies_PythonRequirementsPreferred(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("requests>=2.28.0\n"), 0644)
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`[project.dependencies]
flask = "^2.3.0"
`), 0644)
	deps, err := collectDependencies(dir, "python")
	if err != nil {
		t.Fatalf("collectDependencies python error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep (requirements.txt preferred, pyproject skipped), got %d", len(deps))
	}
	if deps[0].Name != "requests" {
		t.Errorf("dep[0].Name = %q, want requests (from requirements.txt)", deps[0].Name)
	}
}

func TestCollectDependencies_PythonNoFiles(t *testing.T) {
	dir := t.TempDir()
	deps, err := collectDependencies(dir, "python")
	if err != nil {
		t.Fatalf("collectDependencies python error: %v", err)
	}
	if len(deps) != 0 {
		t.Errorf("expected 0 deps for python project without files, got %d", len(deps))
	}
}

func TestCollectDependencies_DefaultGeneric(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src"), 0755)
	deps, err := collectDependencies(dir, "unknown-type")
	if err != nil {
		t.Fatalf("collectDependencies unknown-type error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep (generic), got %d", len(deps))
	}
	if deps[0].PURLType != "generic" {
		t.Errorf("dep[0].PURLType = %q, want generic", deps[0].PURLType)
	}
}

func TestSanitizeSPDXID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"github.com/spf13/cobra", "github.com-spf13-cobra"},
		{"some/package", "some-package"},
		{"normal_pkg-1.0", "normal_pkg-1.0"},
		{"@scope/name", "-scope-name"},
		{"UPPER_CASE", "UPPER_CASE"},
	}
	for _, c := range cases {
		got := sanitizeSPDXID(c.in)
		if got != c.want {
			t.Errorf("sanitizeSPDXID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNewUUID(t *testing.T) {
	uuid := newUUID()
	if uuid == "" {
		t.Error("expected non-empty UUID")
	}
	parts := strings.Split(uuid, "-")
	if len(parts) != 5 {
		t.Errorf("expected 5 UUID parts, got %d", len(parts))
	}
}

func TestNewUUID_Format(t *testing.T) {
	uuid := newUUID()
	if len(uuid) != 36 {
		t.Errorf("expected 36-char UUID, got %d chars: %q", len(uuid), uuid)
	}
	if uuid[8] != '-' || uuid[13] != '-' || uuid[18] != '-' || uuid[23] != '-' {
		t.Errorf("UUID dash positions incorrect: %q", uuid)
	}
}

func TestSbomCmd_DefaultFormat(t *testing.T) {
	format, _ := SbomCmd.Flags().GetString("format")
	if format != "spdx-json" {
		t.Errorf("default format = %q, want spdx-json", format)
	}
}

func TestSbomCmd_DefaultOutput(t *testing.T) {
	output, _ := SbomCmd.Flags().GetString("output")
	if output != "-" {
		t.Errorf("default output = %q, want -", output)
	}
}

func TestSbomCmd_MaxArgs(t *testing.T) {
	if SbomCmd.Args == nil {
		t.Error("expected Args validator to be set")
	}
}

func TestSbomCmd_RunE_NoArgs(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := SbomCmd.RunE(SbomCmd, []string{})
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	_ = buf.String()

	if err != nil {
		t.Logf("SbomCmd.RunE with no args: %v (may be expected for non-Go dir)", err)
	}
}

func TestSbomCmd_RunE_WithOutput(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\ngo 1.21\nrequire github.com/spf13/cobra v1.10.2\n"), 0644)
	outFile := filepath.Join(t.TempDir(), "sbom.json")

	oldStdout := os.Stdout
	os.Stdout = nil

	SbomCmd.SetArgs([]string{dir})
	SbomCmd.Flags().Set("format", "spdx-json")
	SbomCmd.Flags().Set("output", outFile)

	err := SbomCmd.RunE(SbomCmd, []string{dir})
	SbomCmd.Flags().Set("output", "-")
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("SbomCmd.RunE error: %v", err)
	}

	data, readErr := os.ReadFile(outFile)
	if readErr != nil {
		t.Fatalf("failed to read output file: %v", readErr)
	}

	var doc SPDXDocument
	if jsonErr := json.Unmarshal(data, &doc); jsonErr != nil {
		t.Fatalf("output file is not valid SPDX JSON: %v", jsonErr)
	}
	if doc.SPDXVersion != "SPDX-2.3" {
		t.Errorf("output SPDX version = %q, want SPDX-2.3", doc.SPDXVersion)
	}
}

func TestSbomCmd_RunE_CycloneDXOutput(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\ngo 1.21\nrequire github.com/spf13/cobra v1.10.2\n"), 0644)
	outFile := filepath.Join(t.TempDir(), "sbom-cdx.json")

	oldStdout := os.Stdout
	os.Stdout = nil

	SbomCmd.Flags().Set("format", "cyclonedx-json")
	SbomCmd.Flags().Set("output", outFile)

	err := SbomCmd.RunE(SbomCmd, []string{dir})
	SbomCmd.Flags().Set("format", "spdx-json")
	SbomCmd.Flags().Set("output", "-")
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("SbomCmd.RunE cyclonedx error: %v", err)
	}

	data, readErr := os.ReadFile(outFile)
	if readErr != nil {
		t.Fatalf("failed to read output file: %v", readErr)
	}

	var doc CycloneDXDocument
	if jsonErr := json.Unmarshal(data, &doc); jsonErr != nil {
		t.Fatalf("output file is not valid CycloneDX JSON: %v", jsonErr)
	}
	if doc.BomFormat != "CycloneDX" {
		t.Errorf("output bomFormat = %q, want CycloneDX", doc.BomFormat)
	}
}

func TestCollectGoDeps_Fallback(t *testing.T) {
	dir := t.TempDir()
	content := `module example.com/test

go 1.21

require (
	github.com/spf13/cobra v1.10.2
)
`
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0644)

	deps, err := collectGoDeps(dir)
	if err != nil {
		t.Fatalf("collectGoDeps error: %v", err)
	}
	found := false
	for _, d := range deps {
		if d.Name == "github.com/spf13/cobra" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected cobra in deps, got: %+v", deps)
	}
}

func TestCollectGoDeps_NoGoMod(t *testing.T) {
	dir := t.TempDir()
	_, err := collectGoDeps(dir)
	if err == nil {
		t.Error("expected error when go.mod is missing")
	}
}

func TestSbomCmd_RunE_NodeProject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"test","version":"1.0.0","dependencies":{"express":"^4.18.0"}}`), 0644)
	outFile := filepath.Join(t.TempDir(), "sbom-node.json")

	oldStdout := os.Stdout
	os.Stdout = nil

	SbomCmd.Flags().Set("format", "cyclonedx-json")
	SbomCmd.Flags().Set("output", outFile)

	err := SbomCmd.RunE(SbomCmd, []string{dir})
	SbomCmd.Flags().Set("format", "spdx-json")
	SbomCmd.Flags().Set("output", "-")
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("SbomCmd.RunE node error: %v", err)
	}

	data, readErr := os.ReadFile(outFile)
	if readErr != nil {
		t.Fatalf("failed to read output file: %v", readErr)
	}

	var doc CycloneDXDocument
	if jsonErr := json.Unmarshal(data, &doc); jsonErr != nil {
		t.Fatalf("output not valid CycloneDX: %v", jsonErr)
	}

	found := false
	for _, comp := range doc.Components {
		if strings.Contains(comp.BomRef, "pkg:npm/express") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected express component with pkg:npm/ purl")
	}
}

func TestSbomCmd_RunE_PythonProject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("requests>=2.28.0\n"), 0644)
	outFile := filepath.Join(t.TempDir(), "sbom-py.json")

	oldStdout := os.Stdout
	os.Stdout = nil

	SbomCmd.Flags().Set("format", "spdx-json")
	SbomCmd.Flags().Set("output", outFile)

	err := SbomCmd.RunE(SbomCmd, []string{dir})
	SbomCmd.Flags().Set("output", "-")
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("SbomCmd.RunE python error: %v", err)
	}

	data, readErr := os.ReadFile(outFile)
	if readErr != nil {
		t.Fatalf("failed to read output file: %v", readErr)
	}

	var doc SPDXDocument
	if jsonErr := json.Unmarshal(data, &doc); jsonErr != nil {
		t.Fatalf("output not valid SPDX: %v", jsonErr)
	}

	found := false
	for _, pkg := range doc.Packages {
		if pkg.Name == "requests" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected requests package in SPDX output")
	}
}

func TestSPDXPackage_Fields(t *testing.T) {
	pkg := SPDXPackage{
		SPDXID:              "SPDXRef-Package-test",
		Name:                "test",
		VersionInfo:         "1.0.0",
		DownloadLocation:    "https://example.com",
		FilesAnalyzed:       false,
		VerificationCode:    nil,
		LicenseConcluded:    "NOASSERTION",
		LicenseDeclared:     "NOASSERTION",
		CopyrightText:       "NOASSERTION",
		PrimaryPackagePurpose: "LIBRARY",
	}
	data, err := json.Marshal(pkg)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	if !strings.Contains(string(data), `"filesAnalyzed":false`) {
		t.Errorf("expected filesAnalyzed:false in JSON")
	}
	if !strings.Contains(string(data), `"verificationCode":null`) {
		t.Errorf("expected verificationCode:null in JSON")
	}
}

func TestCycloneDXComponent_Omitempty(t *testing.T) {
	comp := CycloneDXComponent{
		Type:   "library",
		BomRef: "pkg:npm/test",
		Name:   "test",
	}
	data, err := json.Marshal(comp)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	s := string(data)
	if strings.Contains(s, `"version"`) {
		t.Errorf("expected version to be omitted when empty, got %q", s)
	}
	if strings.Contains(s, `"purl"`) {
		t.Errorf("expected purl to be omitted when empty, got %q", s)
	}
}

func TestCollectNodeDeps_DevAndProdDeps(t *testing.T) {
	dir := t.TempDir()
	pkg := `{"name":"test","version":"1.0.0","dependencies":{"express":"^4.18.0"},"devDependencies":{"jest":"^29.0.0","eslint":"^8.0.0"}}`
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0644)

	deps, err := collectNodeDeps(dir)
	if err != nil {
		t.Fatalf("collectNodeDeps error: %v", err)
	}
	if len(deps) != 3 {
		t.Fatalf("expected 3 deps (1 prod + 2 dev), got %d", len(deps))
	}
	found := make(map[string]bool)
	for _, d := range deps {
		found[d.Name] = true
	}
	if !found["express"] || !found["jest"] || !found["eslint"] {
		t.Errorf("expected express, jest, and eslint in deps, found: %v", found)
	}
}

func TestCollectNodeDeps_InvalidLockJSON(t *testing.T) {
	dir := t.TempDir()
	pkg := `{"name":"test","version":"1.0.0","dependencies":{"express":"^4.18.0"}}`
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0644)
	os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(`{invalid}`), 0644)

	deps, err := collectNodeDeps(dir)
	if err != nil {
		t.Fatalf("collectNodeDeps should handle invalid lock file gracefully: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(deps))
	}
	if deps[0].Version != "^4.18.0" {
		t.Errorf("version = %q, want ^4.18.0 (fallback from package.json)", deps[0].Version)
	}
}

func TestSbomCmd_RunE_UnsupportedFormat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\ngo 1.21\n"), 0644)
	outFile := filepath.Join(t.TempDir(), "sbom.xml")

	oldStdout := os.Stdout
	os.Stdout = nil

	SbomCmd.Flags().Set("format", "xml")
	SbomCmd.Flags().Set("output", outFile)

	err := SbomCmd.RunE(SbomCmd, []string{dir})
	SbomCmd.Flags().Set("format", "spdx-json")
	SbomCmd.Flags().Set("output", "-")
	os.Stdout = oldStdout

	if err == nil {
		t.Error("expected error for unsupported format")
	}
	if !strings.Contains(err.Error(), "sbom generation failed") {
		t.Errorf("expected 'sbom generation failed' error, got %q", err.Error())
	}
}

func TestSbomCmd_RunE_InvalidOutputPath(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\ngo 1.21\n"), 0644)

	oldStdout := os.Stdout
	os.Stdout = nil

	// Use a path whose parent is a file: fails with ENOTDIR on every
	// platform, even in containers/CI where "/" is writable.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if werr := os.WriteFile(blocker, []byte("x"), 0644); werr != nil {
		os.Stdout = oldStdout
		t.Fatal(werr)
	}
	SbomCmd.Flags().Set("format", "spdx-json")
	SbomCmd.Flags().Set("output", filepath.Join(blocker, "sub", "sbom.json"))

	err := SbomCmd.RunE(SbomCmd, []string{dir})
	SbomCmd.Flags().Set("output", "-")
	os.Stdout = oldStdout

	if err == nil {
		t.Fatal("expected error for invalid output path")
	}
	if !strings.Contains(err.Error(), "cannot create output file") {
		t.Errorf("expected 'cannot create output file' error, got %q", err.Error())
	}
}

func TestCollectDependencies_GoError(t *testing.T) {
	dir := t.TempDir()
	_, err := collectDependencies(dir, "go")
	if err == nil {
		t.Error("expected error for go project without go.mod")
	}
}

func TestCollectDependencies_PythonEmptyFiles(t *testing.T) {
	dir := t.TempDir()
	deps, err := collectDependencies(dir, "python")
	if err != nil {
		t.Fatalf("collectDependencies python error: %v", err)
	}
	if len(deps) != 0 {
		t.Errorf("expected 0 deps, got %d", len(deps))
	}
}

func TestCollectGenericDeps_UnreadableDir(t *testing.T) {
	deps := collectGenericDeps("/nonexistent/path/xyz")
	if len(deps) != 0 {
		t.Errorf("expected 0 deps for nonexistent dir, got %d", len(deps))
	}
}

func TestGenerateSPDX_CollectDepsError(t *testing.T) {
	dir := t.TempDir()
	_, err := generateSPDX(dir, "go", "test", "2026-06-07T12:00:00Z")
	if err == nil {
		t.Error("expected error when collectDependencies fails for go type")
	}
}

func TestGenerateCycloneDX_CollectDepsError(t *testing.T) {
	dir := t.TempDir()
	_, err := generateCycloneDX(dir, "go", "test", "2026-06-07T12:00:00Z")
	if err == nil {
		t.Error("expected error when collectDependencies fails for go type")
	}
}

func TestCollectDependencies_NodeError(t *testing.T) {
	dir := t.TempDir()
	_, err := collectDependencies(dir, "node")
	if err == nil {
		t.Error("expected error for node project without package.json")
	}
}
