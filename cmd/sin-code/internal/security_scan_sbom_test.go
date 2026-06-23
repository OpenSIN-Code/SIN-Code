// SPDX-License-Identifier: MIT
// Purpose: unit tests for the SBOM generator integration.
// All tests are hermetic: no real network, no real vendored builds.
package internal

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// makeFakeSBOMGenerator writes a script that mimics the vendored SBOM generator:
// it reads -deps, writes a JSON stub to the output file specified by either
// -output-spdx or -output-cyclonedx, and prints a success line.
func makeFakeSBOMGenerator(t *testing.T, dir string) string {
	t.Helper()
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".bat"
	}
	bin := filepath.Join(dir, "sin-sbom-go"+ext)
	var script string
	if runtime.GOOS == "windows" {
		script = `@echo off
setlocal EnableDelayedExpansion
set "deps="
set "out="
set "fmt=spdx"
:loop
if "%~1"=="" goto done
if "%~1"=="-deps" (
    set "deps=%~2"
    shift
    shift
    goto loop
)
if "%~1"=="-output-spdx" (
    set "out=%~2"
    set "fmt=spdx"
    shift
    shift
    goto loop
)
if "%~1"=="-output-cyclonedx" (
    set "out=%~2"
    set "fmt=cyclonedx"
    shift
    shift
    goto loop
)
shift
goto loop
:done
echo {"format":"%fmt%","stub":true,"deps":%deps%} > "%out%"
echo OK
exit /b 0
`
	} else {
		script = `#!/bin/sh
deps=""
out=""
fmt="spdx"
while [ $# -gt 0 ]; do
  case "$1" in
    -deps) deps="$2"; shift 2 ;;
    -output-spdx) out="$2"; fmt="spdx"; shift 2 ;;
    -output-cyclonedx) out="$2"; fmt="cyclonedx"; shift 2 ;;
    *) shift ;;
  esac
done
printf '{"format":"%s","stub":true,"deps":"%s"}\n' "$fmt" "$deps" > "$out"
echo "OK"
exit 0
`
	}
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestSecurityScanSbomCmd_Help(t *testing.T) {
	cmd := NewSecurityScanSbomCmd()
	cmd.SetArgs([]string{"--help"})
	cmd.SetOut(new(strings.Builder))
	cmd.SetErr(new(strings.Builder))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("help command failed: %v", err)
	}
}

func TestFindSBOMGeneratorBinary_EnvVar(t *testing.T) {
	binDir := t.TempDir()
	bin := makeFakeSBOMGenerator(t, binDir)

	old := os.Getenv("SIN_SBOM_BIN")
	os.Setenv("SIN_SBOM_BIN", bin)
	defer os.Setenv("SIN_SBOM_BIN", old)

	found, err := findSBOMGeneratorBinary(".", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != bin {
		t.Errorf("expected %s, got %s", bin, found)
	}
}

func TestFindSBOMGeneratorBinary_Path(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	binDir := t.TempDir()
	makeFakeSBOMGenerator(t, binDir)
	os.Setenv("PATH", binDir)

	found, err := findSBOMGeneratorBinary(".", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filepath.Base(found) != "sin-sbom-go"+binaryExt() {
		t.Errorf("expected sin-sbom-go binary, got %s", found)
	}
}

func TestFindSBOMGeneratorBinary_NoBuild(t *testing.T) {
	old := os.Getenv("SIN_SBOM_BIN")
	os.Unsetenv("SIN_SBOM_BIN")
	defer os.Setenv("SIN_SBOM_BIN", old)

	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	defer os.Setenv("PATH", oldPath)

	oldCacheDir := sbomCacheDir
	tmpCache := t.TempDir()
	sbomCacheDir = func() (string, error) { return tmpCache, nil }
	defer func() { sbomCacheDir = oldCacheDir }()

	_, err := findSBOMGeneratorBinary(".", true)
	if err == nil {
		t.Fatal("expected error when binary missing and no-build set")
	}
	if !strings.Contains(err.Error(), "--no-build") {
		t.Errorf("expected --no-build in error, got %q", err.Error())
	}
}

func TestRunSBOMGenerator_SpdxJSON(t *testing.T) {
	binDir := t.TempDir()
	bin := makeFakeSBOMGenerator(t, binDir)

	out, err := runSBOMGenerator(t.TempDir(), bin, "spdx-json", "test-sbom", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), `"format":"spdx"`) {
		t.Errorf("expected spdx format in output, got %s", string(out))
	}
}

func TestRunSBOMGenerator_CycloneDXJSON(t *testing.T) {
	binDir := t.TempDir()
	bin := makeFakeSBOMGenerator(t, binDir)

	out, err := runSBOMGenerator(t.TempDir(), bin, "cyclonedx-json", "test-sbom", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), `"format":"cyclonedx"`) {
		t.Errorf("expected cyclonedx format in output, got %s", string(out))
	}
}

func TestRunSBOMGenerator_UnsupportedFormat(t *testing.T) {
	binDir := t.TempDir()
	bin := makeFakeSBOMGenerator(t, binDir)

	_, err := runSBOMGenerator(t.TempDir(), bin, "xml", "test-sbom", 5)
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
	if !strings.Contains(err.Error(), "unsupported format") {
		t.Errorf("expected unsupported format error, got %q", err.Error())
	}
}

func TestRunSBOMGenerator_BinaryFailure(t *testing.T) {
	binDir := t.TempDir()
	var bin string
	if runtime.GOOS == "windows" {
		bin = filepath.Join(binDir, "sin-sbom-go.bat")
		_ = os.WriteFile(bin, []byte("@echo off\nexit /b 1\n"), 0o755)
	} else {
		bin = filepath.Join(binDir, "sin-sbom-go")
		_ = os.WriteFile(bin, []byte("#!/bin/sh\nexit 1\n"), 0o755)
	}

	_, err := runSBOMGenerator(t.TempDir(), bin, "spdx-json", "test-sbom", 5)
	if err == nil {
		t.Fatal("expected error when generator fails")
	}
}

func TestSecurityScanSbomCmd_ExecuteWithMock(t *testing.T) {
	oldLocator := sbomBinLocator
	defer func() { sbomBinLocator = oldLocator }()

	dir := t.TempDir()
	bin := makeFakeSBOMGenerator(t, dir)
	sbomBinLocator = func(scanPath string, noBuild bool) (string, error) {
		return bin, nil
	}

	cmd := NewSecurityScanSbomCmd()
	cmd.SetArgs([]string{dir, "--format", "spdx-json"})
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(new(strings.Builder))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if !strings.Contains(out.String(), `"format":"spdx"`) {
		t.Errorf("expected JSON output with spdx format, got %q", out.String())
	}
}

func TestCollectSBOMDependencies_GoMod(t *testing.T) {
	root := t.TempDir()
	goMod := filepath.Join(root, "go.mod")
	goModContent := `module example.com/test

go 1.21

require (
	github.com/spf13/cobra v1.8.0
	github.com/stretchr/testify v1.8.4
)
`
	if err := os.WriteFile(goMod, []byte(goModContent), 0o644); err != nil {
		t.Fatal(err)
	}

	deps, sourceType, err := collectSBOMDependencies(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sourceType != "go" {
		t.Errorf("expected source type go, got %q", sourceType)
	}
	if len(deps) != 2 {
		t.Fatalf("expected 2 deps, got %d: %v", len(deps), deps)
	}
	found := make(map[string]bool)
	for _, d := range deps {
		found[d.Name] = true
		if d.PURL == "" {
			t.Errorf("expected PURL for %s", d.Name)
		}
	}
	if !found["github.com/spf13/cobra"] || !found["github.com/stretchr/testify"] {
		t.Errorf("expected both deps, got %v", found)
	}
}

func TestCollectSBOMDependencies_PackageJSON(t *testing.T) {
	root := t.TempDir()
	pj := filepath.Join(root, "package.json")
	content := `{
  "name": "demo",
  "version": "1.0.0",
  "dependencies": {
    "lodash": "^4.17.21",
    "express": "~4.18.0"
  },
  "devDependencies": {
    "jest": "29.0.0"
  }
}
`
	if err := os.WriteFile(pj, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	deps, sourceType, err := collectSBOMDependencies(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sourceType != "npm" {
		t.Errorf("expected source type npm, got %q", sourceType)
	}
	if len(deps) != 3 {
		t.Fatalf("expected 3 deps, got %d: %v", len(deps), deps)
	}
	found := make(map[string]string)
	for _, d := range deps {
		found[d.Name] = d.Version
	}
	if found["lodash"] != "4.17.21" {
		t.Errorf("expected lodash version 4.17.21, got %q", found["lodash"])
	}
	if found["express"] != "4.18.0" {
		t.Errorf("expected express version 4.18.0, got %q", found["express"])
	}
	if found["jest"] != "29.0.0" {
		t.Errorf("expected jest version 29.0.0, got %q", found["jest"])
	}
}

func TestCollectSBOMDependencies_RequirementsTxt(t *testing.T) {
	root := t.TempDir()
	req := filepath.Join(root, "requirements.txt")
	content := `# core
requests==2.31.0
flask>=2.0.0

# ignored
pytest
`
	if err := os.WriteFile(req, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	deps, sourceType, err := collectSBOMDependencies(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sourceType != "pypi" {
		t.Errorf("expected source type pypi, got %q", sourceType)
	}
	if len(deps) != 2 {
		t.Fatalf("expected 2 deps, got %d: %v", len(deps), deps)
	}
	found := make(map[string]string)
	for _, d := range deps {
		found[d.Name] = d.Version
	}
	if found["requests"] != "2.31.0" {
		t.Errorf("expected requests version 2.31.0, got %q", found["requests"])
	}
	if found["flask"] != "2.0.0" {
		t.Errorf("expected flask version 2.0.0, got %q", found["flask"])
	}
}

func TestCollectSBOMDependencies_Generic(t *testing.T) {
	root := t.TempDir()
	deps, sourceType, err := collectSBOMDependencies(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sourceType != "generic" {
		t.Errorf("expected source type generic, got %q", sourceType)
	}
	if len(deps) != 0 {
		t.Errorf("expected 0 deps, got %d", len(deps))
	}
}

func TestFindSBOMModuleRoot(t *testing.T) {
	root := t.TempDir()
	moduleDir := filepath.Join(root, "SIN-Code-SBOM-Generator-Go")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "go.mod"), []byte("module test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "cmd", "sin-code"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := findSBOMModuleRoot(filepath.Join(root, "cmd", "sin-code"))
	if got != moduleDir {
		t.Errorf("expected module root %q, got %q", moduleDir, got)
	}
}

func TestFindSBOMModuleRoot_NotFound(t *testing.T) {
	if got := findSBOMModuleRoot(t.TempDir()); got != "" {
		t.Errorf("expected empty root, got %q", got)
	}
}

func TestSecurityScanCmd_TreeIncludesSbom(t *testing.T) {
	cmd := NewSecurityScanCmd()
	found := false
	for _, c := range cmd.Commands() {
		if c.Name() == "sbom" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'sbom' subcommand under security scan")
	}
}

func TestCachedSBOMBinaryPath(t *testing.T) {
	oldCacheDir := sbomCacheDir
	tmpCache := t.TempDir()
	sbomCacheDir = func() (string, error) { return tmpCache, nil }
	defer func() { sbomCacheDir = oldCacheDir }()

	want := filepath.Join(tmpCache, "sin-code", "sin-sbom-go"+binaryExt())
	if got := cachedSBOMBinaryPath(); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestSecurityScanSbomCmd_NameFlag(t *testing.T) {
	oldLocator := sbomBinLocator
	defer func() { sbomBinLocator = oldLocator }()

	dir := t.TempDir()
	bin := makeFakeSBOMGenerator(t, dir)

	sbomBinLocator = func(scanPath string, noBuild bool) (string, error) {
		return bin, nil
	}

	cmd := NewSecurityScanSbomCmd()
	cmd.SetArgs([]string{dir, "--format", "spdx-json", "--name", "custom-doc"})
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(new(strings.Builder))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if !strings.Contains(out.String(), `"format":"spdx"`) {
		t.Errorf("expected JSON output, got %q", out.String())
	}
}

func TestParseRequirementsTxtDeps_Invalid(t *testing.T) {
	root := t.TempDir()
	req := filepath.Join(root, "requirements.txt")
	if err := os.WriteFile(req, []byte("pytest\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deps, err := parseRequirementsTxtDeps(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deps) != 0 {
		t.Errorf("expected 0 versioned deps, got %d", len(deps))
	}
}

func TestParsePackageJSONDeps_InvalidJSON(t *testing.T) {
	root := t.TempDir()
	pj := filepath.Join(root, "package.json")
	if err := os.WriteFile(pj, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := parsePackageJSONDeps(root)
	if err == nil {
		t.Fatal("expected error for invalid package.json")
	}
}

func TestSbomBinLocator_Override(t *testing.T) {
	oldLocator := sbomBinLocator
	defer func() { sbomBinLocator = oldLocator }()

	sbomBinLocator = func(string, bool) (string, error) {
		return "/fake/sin-sbom-go", nil
	}
	bin, err := sbomBinLocator(".", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bin != "/fake/sin-sbom-go" {
		t.Errorf("expected override path, got %q", bin)
	}
}

func TestWriteSBOMDepsFile(t *testing.T) {
	deps := []sbomDep{{Name: "a", Version: "1.0.0", Type: "library"}}
	path, err := writeSBOMDepsFile(t.TempDir(), deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"name": "a"`) {
		t.Errorf("expected deps JSON, got %s", string(data))
	}
}

func TestFindSBOMGeneratorBinary_EnvVarMissing(t *testing.T) {
	old := os.Getenv("SIN_SBOM_BIN")
	os.Setenv("SIN_SBOM_BIN", "/nonexistent/sin-sbom-go")
	defer os.Setenv("SIN_SBOM_BIN", old)

	_, err := findSBOMGeneratorBinary(".", false)
	if err == nil {
		t.Fatal("expected error for missing env binary")
	}
	if !strings.Contains(err.Error(), "SIN_SBOM_BIN") {
		t.Errorf("expected SIN_SBOM_BIN error, got %q", err.Error())
	}
}

func TestRunSBOMGenerator_UsesDocumentName(t *testing.T) {
	binDir := t.TempDir()
	bin := makeFakeSBOMGenerator(t, binDir)

	_, err := runSBOMGenerator(t.TempDir(), bin, "spdx-json", "my-doc", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
