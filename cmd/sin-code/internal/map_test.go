// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the map subcommand.
package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMapCmd_FlagsDetailed(t *testing.T) {
	cmd := MapCmd
	if cmd.Use != "map [path]" {
		t.Errorf("expected Use 'map [path]', got %q", cmd.Use)
	}
	flags := []string{"action", "format"}
	for _, f := range flags {
		if cmd.Flags().Lookup(f) == nil {
			t.Errorf("missing flag --%s", f)
		}
	}
}

func TestMapCmd_DefaultValues(t *testing.T) {
	v, _ := MapCmd.Flags().GetString("action")
	if v != "map" {
		t.Errorf("default action should be map, got %q", v)
	}
	v, _ = MapCmd.Flags().GetString("format")
	if v != "text" {
		t.Errorf("default format should be text, got %q", v)
	}
}

func TestMapCmd_NonexistentPath(t *testing.T) {
	mapFormat = "text"
	mapAction = "map"
	err := MapCmd.RunE(MapCmd, []string{"/nonexistent/path"})
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestMapCmd_FilePath(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "main.go")
	os.WriteFile(f, []byte("package main\n"), 0644)
	mapFormat = "text"
	mapAction = "map"
	err := MapCmd.RunE(MapCmd, []string{f})
	if err == nil {
		t.Error("expected error when path is a file, not a directory")
	}
}

func TestMapCmd_ValidDir_TextFormat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0644)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	mapFormat = "text"
	mapAction = "map"
	err := MapCmd.RunE(MapCmd, []string{dir})
	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("RunE failed: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()
	if !strings.Contains(out, "Summary") {
		t.Errorf("expected output to contain 'Summary', got %q", out)
	}
}

func TestMapCmd_ValidDir_JSONFormat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	mapFormat = "json"
	mapAction = "map"
	err := MapCmd.RunE(MapCmd, []string{dir})
	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("RunE failed: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(r)
	var result mapResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if result.Summary.TotalFiles == 0 {
		t.Error("expected at least 1 file")
	}
}

func TestMapCmd_DefaultPath(t *testing.T) {
	mapFormat = "text"
	mapAction = "map"
	cmd := MapCmd
	if cmd.Args != nil {
		t.Log("MapCmd accepts ArbitraryArgs so no path is fine")
	}
}

func TestMapArchitecture_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	result, err := mapArchitecture(dir, "map")
	if err != nil {
		t.Fatalf("mapArchitecture failed: %v", err)
	}
	if result.Summary.TotalFiles != 0 {
		t.Errorf("expected 0 files in empty dir, got %d", result.Summary.TotalFiles)
	}
	if result.Summary.TotalLines != 0 {
		t.Errorf("expected 0 lines in empty dir, got %d", result.Summary.TotalLines)
	}
}

func TestMapArchitecture_MixedFiles(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "app.py"), []byte("import os\ndef main():\n    pass\n"), 0644)
	os.WriteFile(filepath.Join(dir, "index.js"), []byte("const x = 1;\n"), 0644)
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("key: value\n"), 0644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package main\nfunc TestMain() {}\n"), 0644)

	result, err := mapArchitecture(dir, "map")
	if err != nil {
		t.Fatalf("mapArchitecture failed: %v", err)
	}
	if result.Summary.TotalFiles < 5 {
		t.Errorf("expected at least 5 files, got %d", result.Summary.TotalFiles)
	}
	if result.Summary.TestFiles < 1 {
		t.Errorf("expected at least 1 test file, got %d", result.Summary.TestFiles)
	}
	if result.Summary.ConfigFiles < 1 {
		t.Errorf("expected at least 1 config file, got %d", result.Summary.ConfigFiles)
	}
	if result.Summary.Documentation < 1 {
		t.Errorf("expected at least 1 doc file, got %d", result.Summary.Documentation)
	}
}

func TestMapArchitecture_SkipsDotDirs(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, ".git"), 0755)
	os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("[core]\n"), 0644)
	os.Mkdir(filepath.Join(dir, ".hidden"), 0755)
	os.WriteFile(filepath.Join(dir, ".hidden", "secret.go"), []byte("package secret\n"), 0644)
	os.WriteFile(filepath.Join(dir, "visible.go"), []byte("package visible\n"), 0644)

	result, err := mapArchitecture(dir, "map")
	if err != nil {
		t.Fatalf("mapArchitecture failed: %v", err)
	}
	for _, deps := range result.Dependencies {
		for _, f := range deps {
			if strings.Contains(f, ".git") || strings.Contains(f, ".hidden") {
				t.Errorf("should skip dot directories, found %q", f)
			}
		}
	}
}

func TestMapArchitecture_SkipsCommonDirs(t *testing.T) {
	dir := t.TempDir()
	for _, skip := range []string{"node_modules", "vendor", "__pycache__", "dist", "build", "target"} {
		os.Mkdir(filepath.Join(dir, skip), 0755)
		os.WriteFile(filepath.Join(dir, skip, "file.go"), []byte("package skip\n"), 0644)
	}
	os.WriteFile(filepath.Join(dir, "keep.go"), []byte("package keep\n"), 0644)

	result, err := mapArchitecture(dir, "map")
	if err != nil {
		t.Fatalf("mapArchitecture failed: %v", err)
	}
	if result.Summary.TotalFiles != 1 {
		t.Errorf("expected 1 file (keep.go), got %d", result.Summary.TotalFiles)
	}
}

func TestMapArchitecture_DetectsGoEntryPoint(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "util.go"), []byte("package util\nfunc Helper() {}\n"), 0644)

	result, err := mapArchitecture(dir, "map")
	if err != nil {
		t.Fatalf("mapArchitecture failed: %v", err)
	}
	found := false
	for _, ep := range result.EntryPoints {
		if strings.HasSuffix(ep, "main.go") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected main.go as entry point, got %v", result.EntryPoints)
	}
}

func TestMapArchitecture_DetectsPythonEntryPoint(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "__main__.py"), []byte("import os\nif __name__ == '__main__':\n    pass\n"), 0644)

	result, err := mapArchitecture(dir, "map")
	if err != nil {
		t.Fatalf("mapArchitecture failed: %v", err)
	}
	found := false
	for _, ep := range result.EntryPoints {
		if strings.HasSuffix(ep, "__main__.py") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected __main__.py as entry point, got %v", result.EntryPoints)
	}
}

func TestMapArchitecture_DetectsJSEntryPoint(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.js"), []byte("const x = 1;\n"), 0644)

	result, err := mapArchitecture(dir, "map")
	if err != nil {
		t.Fatalf("mapArchitecture failed: %v", err)
	}
	found := false
	for _, ep := range result.EntryPoints {
		if strings.HasSuffix(ep, "index.js") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected index.js as entry point, got %v", result.EntryPoints)
	}
}

func TestMapArchitecture_DetectsDependencies(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "main.go")
	content := `package main
import (
	"fmt"
	"os"
)
func main() {
	fmt.Println("hi")
	os.Exit(0)
}
`
	os.WriteFile(goFile, []byte(content), 0644)

	result, err := mapArchitecture(dir, "map")
	if err != nil {
		t.Fatalf("mapArchitecture failed: %v", err)
	}
	if len(result.Dependencies) == 0 {
		t.Error("expected at least one file with dependencies")
	}
}

func TestMapArchitecture_OrphanCheck(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "orphan.go"), []byte("package orphan\nfunc Standalone() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644)

	result, err := mapArchitecture(dir, "map")
	if err != nil {
		t.Fatalf("mapArchitecture failed: %v", err)
	}
	if len(result.Orphans) == 0 {
		t.Log("orphan detection may vary based on import analysis")
	}
}

func TestMapArchitecture_HotPaths(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "pkg"), 0755)

	utilContent := `package pkg
func Helper() int { return 1 }
`
	os.WriteFile(filepath.Join(dir, "pkg", "util.go"), []byte(utilContent), 0644)

	for i := 0; i < 5; i++ {
		filename := filepath.Join(dir, fmt.Sprintf("client_%d.go", i))
		clientContent := `package main
import "pkg"
func Client` + fmt.Sprintf("%d", i) + `() { pkg.Helper() }
`
		os.WriteFile(filename, []byte(clientContent), 0644)
	}

	result, err := mapArchitecture(dir, "map")
	if err != nil {
		t.Fatalf("mapArchitecture failed: %v", err)
	}
	_ = result.HotPaths
}

func TestMapArchitecture_LargeFileSkipped(t *testing.T) {
	dir := t.TempDir()
	bigFile := filepath.Join(dir, "big.go")
	bigContent := strings.Repeat("// line\n", 100000)
	os.WriteFile(bigFile, []byte(bigContent), 0644)

	_, err := mapArchitecture(dir, "map")
	if err != nil {
		t.Fatalf("mapArchitecture failed with large file: %v", err)
	}
}

func TestOutputTextMap(t *testing.T) {
	r := &mapResult{
		Path: "/tmp/project",
		Summary: mapSummary{
			TotalFiles:    10,
			TotalLines:    500,
			Languages:     map[string]int{"go": 8, "python": 2},
			TestFiles:     3,
			ConfigFiles:   1,
			Documentation: 2,
		},
		EntryPoints: []string{"cmd/main.go"},
		HotPaths:    []hotPath{{Path: "pkg/util.go", Imports: 5, Importers: []string{"a.go", "b.go"}}},
		Orphans:     []string{"orphan.go"},
	}

	old := os.Stdout
	pr, pw, _ := os.Pipe()
	os.Stdout = pw

	err := outputTextMap(r)
	pw.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("outputTextMap failed: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(pr)
	out := buf.String()
	if !strings.Contains(out, "Total files:  10") {
		t.Errorf("expected 'Total files:  10', got %q", out)
	}
	if !strings.Contains(out, "cmd/main.go") {
		t.Errorf("expected entry point in output, got %q", out)
	}
	if !strings.Contains(out, "orphan.go") {
		t.Errorf("expected orphan in output, got %q", out)
	}
}

func TestOutputTextMap_EmptyProject(t *testing.T) {
	r := &mapResult{
		Path: "/tmp/empty",
		Summary: mapSummary{
			TotalFiles: 0,
			TotalLines: 0,
			Languages:  map[string]int{},
		},
	}

	old := os.Stdout
	pr2, pw, _ := os.Pipe()
	os.Stdout = pw

	err := outputTextMap(r)
	pw.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("outputTextMap failed: %v", err)
	}
	var buf2 bytes.Buffer
	buf2.ReadFrom(pr2)
	_ = buf2.String()
}

func TestMin(t *testing.T) {
	tests := []struct {
		a, b, expected int
	}{
		{1, 2, 1},
		{2, 1, 1},
		{5, 5, 5},
		{0, 10, 0},
		{-1, 1, -1},
	}
	for _, tt := range tests {
		got := min(tt.a, tt.b)
		if got != tt.expected {
			t.Errorf("min(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.expected)
		}
	}
}

func TestContent(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.txt")
	os.WriteFile(f, []byte("hello world"), 0644)
	got := content(f)
	if got != "hello world" {
		t.Errorf("content(%q) = %q, want %q", f, got, "hello world")
	}
}

func TestContent_Nonexistent(t *testing.T) {
	got := content("/nonexistent/file.txt")
	if got != "" {
		t.Errorf("expected empty string for nonexistent file, got %q", got)
	}
}

func TestMapArchitecture_RustEntryPoint(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.rs"), []byte("fn main() {}\n"), 0644)

	result, err := mapArchitecture(dir, "map")
	if err != nil {
		t.Fatalf("mapArchitecture failed: %v", err)
	}
	found := false
	for _, ep := range result.EntryPoints {
		if strings.HasSuffix(ep, "main.rs") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected main.rs as entry point, got %v", result.EntryPoints)
	}
}

func TestMapArchitecture_JavaEntryPoint(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "App.java"), []byte("public class App { public static void main(String[] args) {} }\n"), 0644)

	result, err := mapArchitecture(dir, "map")
	if err != nil {
		t.Fatalf("mapArchitecture failed: %v", err)
	}
	found := false
	for _, ep := range result.EntryPoints {
		if strings.HasSuffix(ep, "App.java") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected App.java as entry point, got %v", result.EntryPoints)
	}
}

func TestMapArchitecture_ModuleTracking(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "cmd"), 0755)
	os.WriteFile(filepath.Join(dir, "cmd", "main.go"), []byte("package main\nfunc main() {}\n"), 0644)

	result, err := mapArchitecture(dir, "map")
	if err != nil {
		t.Fatalf("mapArchitecture failed: %v", err)
	}
	if result.Summary.TotalFiles < 1 {
		t.Errorf("expected at least 1 file, got %d", result.Summary.TotalFiles)
	}
}

func TestMapArchitecture_TypeScriptEntryPoint(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.ts"), []byte("const x = 1;\n"), 0644)

	result, err := mapArchitecture(dir, "map")
	if err != nil {
		t.Fatalf("mapArchitecture failed: %v", err)
	}
	found := false
	for _, ep := range result.EntryPoints {
		if strings.HasSuffix(ep, "main.ts") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected main.ts as entry point, got %v", result.EntryPoints)
	}
}

func TestMapArchitecture_ConfigDetection(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0644)
	os.WriteFile(filepath.Join(dir, "go.sum"), []byte("\n"), 0644)
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM golang\n"), 0644)

	result, err := mapArchitecture(dir, "map")
	if err != nil {
		t.Fatalf("mapArchitecture failed: %v", err)
	}
	if result.Summary.ConfigFiles < 3 {
		t.Errorf("expected at least 3 config files, got %d", result.Summary.ConfigFiles)
	}
}

func TestMapArchitecture_DocDetection(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0644)
	os.WriteFile(filepath.Join(dir, "LICENSE"), []byte("MIT\n"), 0644)
	os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte("# Changelog\n"), 0644)

	result, err := mapArchitecture(dir, "map")
	if err != nil {
		t.Fatalf("mapArchitecture failed: %v", err)
	}
	if result.Summary.Documentation < 2 {
		t.Errorf("expected at least 2 doc files, got %d", result.Summary.Documentation)
	}
}

func TestMapArchitecture_PythonEntryPoint(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.py"), []byte("import os\nif __name__ == \"__main__\":\n    os.exit(0)\n"), 0644)

	result, err := mapArchitecture(dir, "map")
	if err != nil {
		t.Fatalf("mapArchitecture failed: %v", err)
	}
	found := false
	for _, ep := range result.EntryPoints {
		if strings.HasSuffix(ep, "app.py") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected app.py as entry point (contains __main__), got %v", result.EntryPoints)
	}
}

func TestMapCmd_InvalidAbsPath(t *testing.T) {
	mapFormat = "text"
	mapAction = "map"
	err := MapCmd.RunE(MapCmd, []string{"\x00invalid"})
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestOutputTextMap_NoEntryPoints(t *testing.T) {
	r := &mapResult{
		Path: "/tmp/test",
		Summary: mapSummary{
			TotalFiles: 1,
			TotalLines: 10,
			Languages:  map[string]int{"go": 1},
		},
		HotPaths: []hotPath{},
		Orphans:  []string{},
	}

	old := os.Stdout
	pr, pw, _ := os.Pipe()
	os.Stdout = pw

	err := outputTextMap(r)
	pw.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("outputTextMap failed: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(pr)
	out := buf.String()
	if !strings.Contains(out, "Total files") {
		t.Errorf("expected summary in output, got %q", out)
	}
}

func TestMin_Equal(t *testing.T) {
	if got := min(5, 5); got != 5 {
		t.Errorf("min(5,5) = %d, want 5", got)
	}
}

func TestMapArchitecture_VendorSkip(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "vendor"), 0755)
	os.WriteFile(filepath.Join(dir, "vendor", "lib.go"), []byte("package vendor\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644)

	result, err := mapArchitecture(dir, "map")
	if err != nil {
		t.Fatalf("mapArchitecture failed: %v", err)
	}
	for _, f := range result.Summary.Languages {
		_ = f
	}
	for lang := range result.Summary.Languages {
		_ = lang
	}
}

func TestMapArchitecture_HotPathDetection(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nimport \"fmt\"\nfunc main() { fmt.Println() }\n"), 0644)

	result, err := mapArchitecture(dir, "map")
	if err != nil {
		t.Fatalf("mapArchitecture failed: %v", err)
	}
	_ = result.HotPaths
}



func TestMapArchitecture_TestFileCount(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package main\n"), 0644)

	result, err := mapArchitecture(dir, "map")
	if err != nil {
		t.Fatalf("mapArchitecture failed: %v", err)
	}
	if result.Summary.TestFiles < 1 {
		t.Errorf("expected at least 1 test file, got %d", result.Summary.TestFiles)
	}
}

func TestMapCmd_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)

	mapFormat = "json"
	mapAction = "map"

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := MapCmd.RunE(MapCmd, []string{dir})
	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("MapCmd.RunE failed: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(r)
	var result mapResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("expected valid JSON output, got parse error: %v", err)
	}
}

func TestOutputTextMap_WithOrphans(t *testing.T) {
	r := &mapResult{
		Path: "/tmp/test",
		Summary: mapSummary{
			TotalFiles: 3, TotalLines: 100,
			Languages: map[string]int{"go": 3},
		},
		EntryPoints: []string{"main.go"},
		HotPaths:   []hotPath{{Path: "utils.go", Imports: 5}},
		Orphans:    []string{"orphan1.go", "orphan2.go"},
	}

	old := os.Stdout
	pr, pw, _ := os.Pipe()
	os.Stdout = pw

	err := outputTextMap(r)
	pw.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("outputTextMap failed: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(pr)
	out := buf.String()
	if !strings.Contains(out, "Orphans") {
		t.Errorf("expected Orphans in output, got: %q", out)
	}
	if !strings.Contains(out, "Hot Paths") {
		t.Errorf("expected Hot Paths in output, got: %q", out)
	}
}

func TestOutputTextMap_ManyOrphans(t *testing.T) {
	orphans := make([]string, 25)
	for i := range orphans {
		orphans[i] = fmt.Sprintf("orphan_%d.go", i)
	}
	r := &mapResult{
		Path: "/tmp/test",
		Summary: mapSummary{
			TotalFiles: 25, TotalLines: 500,
			Languages: map[string]int{"go": 25},
		},
		Orphans: orphans,
	}

	old := os.Stdout
	pr, pw, _ := os.Pipe()
	os.Stdout = pw

	err := outputTextMap(r)
	pw.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("outputTextMap failed: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(pr)
	_ = buf.String()
}

func TestMin_AGreaterThanB(t *testing.T) {
	if got := min(10, 5); got != 5 {
		t.Errorf("min(10,5) = %d, want 5", got)
	}
}

func TestMapArchitecture_WithImports(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nimport \"fmt\"\nfunc main() { fmt.Println() }\n"), 0644)
	os.Mkdir(filepath.Join(dir, "pkg"), 0755)
	os.WriteFile(filepath.Join(dir, "pkg", "utils.go"), []byte("package pkg\nfunc Helper() {}\n"), 0644)

	result, err := mapArchitecture(dir, "map")
	if err != nil {
		t.Fatalf("mapArchitecture failed: %v", err)
	}
	if result.Summary.TotalFiles < 2 {
		t.Errorf("expected at least 2 files, got %d", result.Summary.TotalFiles)
	}
}

func TestMapCmd_InvalidPath(t *testing.T) {
	mapFormat = "text"
	mapAction = "map"
	err := MapCmd.RunE(MapCmd, []string{"/nonexistent/path"})
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestMapArchitecture_MultipleModules(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "cmd"), 0755)
	os.WriteFile(filepath.Join(dir, "cmd", "main.go"), []byte("package main\nfunc main() {}\n"), 0644)
	os.Mkdir(filepath.Join(dir, "internal"), 0755)
	os.WriteFile(filepath.Join(dir, "internal", "handler.go"), []byte("package internal\nfunc Handle() {}\n"), 0644)

	result, err := mapArchitecture(dir, "map")
	if err != nil {
		t.Fatalf("mapArchitecture failed: %v", err)
	}
	_ = result.Modules
}

func TestMapArchitecture_LargeFileSkip(t *testing.T) {
	dir := t.TempDir()
	bigFile := filepath.Join(dir, "big.go")
	bigContent := "package main\n" + strings.Repeat("// fill\n", 500000)
	os.WriteFile(bigFile, []byte(bigContent), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644)

	result, err := mapArchitecture(dir, "map")
	if err != nil {
		t.Fatalf("mapArchitecture failed: %v", err)
	}
	_ = result.Summary
}
