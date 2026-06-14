// SPDX-License-Identifier: MIT
// Purpose: Additional coverage tests for poc.go, sckg.go, adw.go,
// agent_doctor_cmd.go, agent_edit_cmd.go, and agent_helpers.go.
// Test names match the run regex:
// TestPoc|TestSckg|TestAdw|TestAgentDoctor|TestAgentEdit|TestAgentHelpers|TestAgent
package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
)

// runAndCapture executes fn while capturing stdout, returning the output and
// the error from fn. It restores os.Stdout before returning.
func runAndCapture(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := fn()
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out), err
}

// runCmd captures stdout while running a cobra command's RunE.
func runCmd(t *testing.T, cmd *cobra.Command, args []string) (string, error) {
	t.Helper()
	return runAndCapture(t, func() error { return cmd.RunE(cmd, args) })
}

// ─────────────────────────────────────────────────────────────────────────────
// poc.go
// ─────────────────────────────────────────────────────────────────────────────

func TestPocCmd_MissingArgs(t *testing.T) {
	oldSpec, oldCode, oldFormat := pocSpec, pocCode, pocFormat
	pocSpec, pocCode, pocFormat = "", "", "text"
	defer func() { pocSpec, pocCode, pocFormat = oldSpec, oldCode, oldFormat }()

	_, err := runCmd(t, PocCmd, []string{})
	if err == nil {
		t.Fatal("expected error when --code and --spec are empty")
	}
}

func TestPocCmd_CodeTarget(t *testing.T) {
	dir := t.TempDir()
	codeFile := filepath.Join(dir, "code.go")
	os.WriteFile(codeFile, []byte("package main\nfunc Hello() {}\n"), 0o644)

	oldSpec, oldCode, oldFormat := pocSpec, pocCode, pocFormat
	pocSpec, pocCode, pocFormat = "", codeFile, "text"
	defer func() { pocSpec, pocCode, pocFormat = oldSpec, oldCode, oldFormat }()

	out, err := runCmd(t, PocCmd, []string{})
	if err != nil {
		t.Fatalf("PocCmd failed: %v", err)
	}
	if !strings.Contains(out, "Proof-of-Correctness") {
		t.Errorf("expected text output, got %q", out)
	}
}

func TestPocCmd_SpecTargetBackCompat(t *testing.T) {
	dir := t.TempDir()
	specFile := filepath.Join(dir, "spec.md")
	os.WriteFile(specFile, []byte("Hello() must exist\n"), 0o644)

	oldSpec, oldCode, oldFormat := pocSpec, pocCode, pocFormat
	pocSpec, pocCode, pocFormat = specFile, "", "text"
	defer func() { pocSpec, pocCode, pocFormat = oldSpec, oldCode, oldFormat }()

	out, err := runCmd(t, PocCmd, []string{})
	if err != nil {
		t.Fatalf("PocCmd failed: %v", err)
	}
	if !strings.Contains(out, "Proof-of-Correctness") {
		t.Errorf("expected text output, got %q", out)
	}
}

func TestPocCmd_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	codeFile := filepath.Join(dir, "code.go")
	os.WriteFile(codeFile, []byte("package main\nfunc Hello() {}\n"), 0o644)

	oldSpec, oldCode, oldFormat := pocSpec, pocCode, pocFormat
	pocSpec, pocCode, pocFormat = "", codeFile, "json"
	defer func() { pocSpec, pocCode, pocFormat = oldSpec, oldCode, oldFormat }()

	out, err := runCmd(t, PocCmd, []string{})
	if err != nil {
		t.Fatalf("PocCmd json failed: %v", err)
	}
	var result pocResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON output: %v\n%q", err, out)
	}
}

func TestPocVerifyCorrectness_SpecReadError(t *testing.T) {
	_, err := verifyCorrectness("/nonexistent/spec.md", "")
	if err == nil {
		t.Fatal("expected error for missing spec file")
	}
}

func TestPocVerifyCorrectness_CodePathError(t *testing.T) {
	_, err := verifyCorrectness("", "/nonexistent/code.go")
	if err == nil {
		t.Fatal("expected error for missing code path")
	}
}

func TestPocVerifyCorrectness_WalkError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("walk hook test is Unix-specific")
	}
	oldWalk := pocWalk
	pocWalk = func(root string, fn filepath.WalkFunc) error {
		return errors.New("walk error")
	}
	defer func() { pocWalk = oldWalk }()

	_, err := verifyCorrectness("", t.TempDir())
	if err == nil {
		t.Fatal("expected error from walk hook")
	}
}

func TestPocVerifyCorrectness_FileReadError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based read test is Unix-specific")
	}
	dir := t.TempDir()
	codeFile := filepath.Join(dir, "code.go")
	os.WriteFile(codeFile, []byte("package main\nfunc Hello() {}\n"), 0o644)
	os.Chmod(codeFile, 0o000)
	defer os.Chmod(codeFile, 0o644)

	res, err := verifyCorrectness("", dir)
	if err != nil {
		t.Fatalf("verifyCorrectness failed: %v", err)
	}
	if res.TotalChecks != 0 {
		t.Errorf("expected 0 checks for unreadable file, got %d", res.TotalChecks)
	}
}

func TestPocVerifyCorrectness_RequirementNotFound(t *testing.T) {
	dir := t.TempDir()
	specFile := filepath.Join(dir, "spec.md")
	codeFile := filepath.Join(dir, "code.go")
	os.WriteFile(specFile, []byte("MissingFunc() must exist\n"), 0o644)
	os.WriteFile(codeFile, []byte("package main\nfunc Hello() {}\n"), 0o644)

	res, err := verifyCorrectness(specFile, codeFile)
	if err != nil {
		t.Fatalf("verifyCorrectness failed: %v", err)
	}
	if res.Failed == 0 {
		t.Errorf("expected failed requirement, got checks: %+v", res.Checks)
	}
	if res.Coverage != 0 {
		t.Errorf("expected 0%% coverage, got %.1f%%", res.Coverage)
	}
}

func TestPocVerifyCorrectness_TODOForbidden(t *testing.T) {
	dir := t.TempDir()
	specFile := filepath.Join(dir, "spec.md")
	codeFile := filepath.Join(dir, "code.go")
	os.WriteFile(specFile, []byte("Hello() must exist\n"), 0o644)
	os.WriteFile(codeFile, []byte("package main\n// TODO: fix this\nfunc Hello() {}\n"), 0o644)

	res, err := verifyCorrectness(specFile, codeFile)
	if err != nil {
		t.Fatalf("verifyCorrectness failed: %v", err)
	}
	found := false
	for _, c := range res.Checks {
		if c.Name == "TODO" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected TODO forbidden check, got %v", res.Checks)
	}
}

func TestPocVerifyCorrectness_OsExitForbidden(t *testing.T) {
	dir := t.TempDir()
	codeFile := filepath.Join(dir, "lib.go")
	os.WriteFile(codeFile, []byte("package lib\nimport \"os\"\nfunc Stop() { os.Exit(1) }\n"), 0o644)

	res, err := verifyCorrectness("", codeFile)
	if err != nil {
		t.Fatalf("verifyCorrectness failed: %v", err)
	}
	found := false
	for _, c := range res.Checks {
		if c.Name == "os.Exit" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected os.Exit forbidden check, got %v", res.Checks)
	}
}

func TestPocVerifyCorrectness_Directory(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\nfunc Hello() {}\n"), 0o644)
	specFile := filepath.Join(dir, "spec.md")
	os.WriteFile(specFile, []byte("Hello() must exist\n"), 0o644)

	res, err := verifyCorrectness(specFile, dir)
	if err != nil {
		t.Fatalf("verifyCorrectness failed: %v", err)
	}
	if res.Coverage != 100 {
		t.Errorf("expected 100%% coverage for dir, got %.1f%%", res.Coverage)
	}
}

func TestPocVerifyCorrectness_NoSpec(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "code.go"), []byte("package main\nfunc Hello() {}\n"), 0o644)

	res, err := verifyCorrectness("", filepath.Join(dir, "code.go"))
	if err != nil {
		t.Fatalf("verifyCorrectness failed: %v", err)
	}
	if len(res.Checks) != 0 {
		t.Errorf("expected no checks without spec, got %d", len(res.Checks))
	}
}

func TestPocExtractRequirements_CodeBlock(t *testing.T) {
	spec := "See usage:\n```go\nhello()\nworld()\n```\n"
	reqs := extractRequirements(spec)
	foundHello, foundWorld := false, false
	for _, r := range reqs {
		if r.Name == "hello" {
			foundHello = true
		}
		if r.Name == "world" {
			foundWorld = true
		}
	}
	if !foundHello || !foundWorld {
		t.Errorf("expected hello and world from code block, got %v", reqs)
	}
}

func TestPocIsLikelyCodeName_Separators(t *testing.T) {
	if !isLikelyCodeName("hello_world") {
		t.Error("expected underscore name to be likely code")
	}
	if !isLikelyCodeName("hello-world") {
		t.Error("expected hyphen name to be likely code")
	}
	if !isLikelyCodeName("hello.world") {
		t.Error("expected dot name to be likely code")
	}
	if !isLikelyCodeName("HelloWorld") {
		t.Error("expected mixed-case name to be likely code")
	}
	if isLikelyCodeName("") {
		t.Error("expected empty name to be not likely code")
	}
	if isLikelyCodeName("hello") {
		t.Error("expected lowercase single word to be not likely code")
	}
}

func TestPocOutputTextPOC(t *testing.T) {
	result := &pocResult{
		Spec:        "spec.md",
		Code:        "code.go",
		Coverage:    50.0,
		Passed:      1,
		Failed:      1,
		TotalChecks: 2,
		Checks: []pocCheck{
			{Name: "FoundIt", Type: "required", Status: "pass", File: "code.go", Line: 1},
			{Name: "MissingIt", Type: "required", Status: "fail"},
		},
		Summary: "Coverage: 50.0% (1/2 passed)",
	}
	out := runAndCaptureStdout(t, func() { outputTextPOC(result) })
	if !strings.Contains(out, "Proof-of-Correctness") {
		t.Errorf("expected header, got %q", out)
	}
	if !strings.Contains(out, "FoundIt") {
		t.Errorf("expected pass check, got %q", out)
	}
	if !strings.Contains(out, "MissingIt") {
		t.Errorf("expected fail check, got %q", out)
	}
}

// runAndCaptureStdout captures stdout for a function that returns nothing.
func runAndCaptureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out)
}

// ─────────────────────────────────────────────────────────────────────────────
// sckg.go
// ─────────────────────────────────────────────────────────────────────────────

func TestSckgCmd_AbsError(t *testing.T) {
	oldAbs := sckgAbs
	sckgAbs = func(path string) (string, error) { return "", errors.New("abs error") }
	defer func() { sckgAbs = oldAbs }()

	sckgAction, sckgQuery, sckgFormat = "build", "", "text"
	defer func() { sckgAction, sckgQuery, sckgFormat = "build", "", "text" }()

	_, err := runCmd(t, SckgCmd, []string{"."})
	if err == nil {
		t.Fatal("expected error from abs hook")
	}
}

func TestSckgCmd_PathNotFound(t *testing.T) {
	sckgAction, sckgQuery, sckgFormat = "build", "", "text"
	defer func() { sckgAction, sckgQuery, sckgFormat = "build", "", "text" }()

	_, err := runCmd(t, SckgCmd, []string{"/nonexistent/path/xyz"})
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
}

func TestSckgCmd_PathNotDir(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file.go")
	os.WriteFile(f, []byte("package main\n"), 0o644)

	sckgAction, sckgQuery, sckgFormat = "build", "", "text"
	defer func() { sckgAction, sckgQuery, sckgFormat = "build", "", "text" }()

	_, err := runCmd(t, SckgCmd, []string{f})
	if err == nil {
		t.Fatal("expected error for file path")
	}
}

func TestSckgCmd_BuildError(t *testing.T) {
	oldWalk := sckgWalk
	sckgWalk = func(root string, fn filepath.WalkFunc) error { return errors.New("walk error") }
	defer func() { sckgWalk = oldWalk }()

	sckgAction, sckgQuery, sckgFormat = "build", "", "text"
	defer func() { sckgAction, sckgQuery, sckgFormat = "build", "", "text" }()

	_, err := runCmd(t, SckgCmd, []string{t.TempDir()})
	if err == nil {
		t.Fatal("expected error from buildGraph")
	}
}

func TestSckgCmd_QueryJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc Hello() {}\n"), 0o644)

	sckgAction, sckgQuery, sckgFormat = "query", "hello", "json"
	defer func() { sckgAction, sckgQuery, sckgFormat = "build", "", "text" }()

	out, err := runCmd(t, SckgCmd, []string{dir})
	if err != nil {
		t.Fatalf("SckgCmd query json failed: %v", err)
	}
	var result queryResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON: %v\n%q", err, out)
	}
	if result.Query != "hello" {
		t.Errorf("expected query hello, got %q", result.Query)
	}
}

func TestSckgCmd_StatsJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc Hello() {}\n"), 0o644)

	sckgAction, sckgQuery, sckgFormat = "stats", "", "json"
	defer func() { sckgAction, sckgQuery, sckgFormat = "build", "", "text" }()

	out, err := runCmd(t, SckgCmd, []string{dir})
	if err != nil {
		t.Fatalf("SckgCmd stats json failed: %v", err)
	}
	var stats sckgStats
	if err := json.Unmarshal([]byte(out), &stats); err != nil {
		t.Fatalf("expected valid JSON: %v\n%q", err, out)
	}
}

func TestSckgBuildGraph_WalkError(t *testing.T) {
	oldWalk := sckgWalk
	sckgWalk = func(root string, fn filepath.WalkFunc) error { return errors.New("walk error") }
	defer func() { sckgWalk = oldWalk }()

	_, err := buildGraph(t.TempDir())
	if err == nil {
		t.Fatal("expected error from walk hook")
	}
}

func TestSckgBuildGraph_DuplicateNode(t *testing.T) {
	// Two files importing the same dependency should collapse the dep node.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\nimport \"fmt\"\nfunc A() {}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("package main\nimport \"fmt\"\nfunc B() {}\n"), 0o644)

	graph, err := buildGraph(dir)
	if err != nil {
		t.Fatalf("buildGraph failed: %v", err)
	}
	count := 0
	for _, n := range graph.Nodes {
		if n.Type == "module" && n.Name == "fmt" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected duplicate dep fmt collapsed to one node, got %d", count)
	}
}

func TestSckgBuildGraph_ChildSymbols(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\ntype MyStruct struct {\n\tfield int\n}\nfunc (m MyStruct) Method() {}\n"), 0o644)

	graph, err := buildGraph(dir)
	if err != nil {
		t.Fatalf("buildGraph failed: %v", err)
	}
	foundStruct := false
	for _, n := range graph.Nodes {
		if n.Name == "MyStruct" {
			foundStruct = true
		}
	}
	if !foundStruct {
		t.Errorf("expected struct node, got %v", graph.Nodes)
	}
}

func TestSckgBuildGraph_VarConst(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nconst Pi = 3.14\nvar Global = 1\n"), 0o644)

	graph, err := buildGraph(dir)
	if err != nil {
		t.Fatalf("buildGraph failed: %v", err)
	}
	foundPi, foundGlobal := false, false
	for _, n := range graph.Nodes {
		if n.Type == "variable" && n.Name == "Pi" {
			foundPi = true
		}
		if n.Type == "variable" && n.Name == "Global" {
			foundGlobal = true
		}
	}
	if !foundPi || !foundGlobal {
		t.Errorf("expected variable nodes, got %v", graph.Nodes)
	}
}

func TestSckgBuildGraph_FileReadError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod test is Unix-specific")
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "code.go")
	os.WriteFile(f, []byte("package main\n"), 0o644)
	os.Chmod(f, 0o000)
	defer os.Chmod(f, 0o644)

	graph, err := buildGraph(dir)
	if err != nil {
		t.Fatalf("buildGraph failed: %v", err)
	}
	if len(graph.Nodes) == 0 {
		t.Error("expected file node even for unreadable file")
	}
}

func TestSckgKind(t *testing.T) {
	cases := []struct {
		kind, want string
	}{
		{"func", "function"},
		{"method", "function"},
		{"struct", "type"},
		{"type", "type"},
		{"var", "variable"},
		{"const", "variable"},
		{"unknown", "unknown"},
	}
	for _, c := range cases {
		if got := sckgKind(c.kind); got != c.want {
			t.Errorf("sckgKind(%q) = %q, want %q", c.kind, got, c.want)
		}
	}
}

func TestSckgQueryGraph_RelatedByTarget(t *testing.T) {
	graph := &sckgGraph{
		Nodes: []sckgNode{
			{ID: "a", Name: "Alpha"},
			{ID: "b", Name: "Beta"},
		},
		Edges: []sckgEdge{{Source: "a", Target: "b", Type: "calls"}},
	}
	res := queryGraph(graph, "beta")
	if len(res.Matches) != 1 || res.Matches[0].ID != "b" {
		t.Errorf("expected match for Beta, got %+v", res.Matches)
	}
	if len(res.Related) != 1 || res.Related[0].ID != "a" {
		t.Errorf("expected related Alpha, got %+v", res.Related)
	}
}

func TestSckgGraphStats_MoreThanTenImports(t *testing.T) {
	graph := &sckgGraph{Nodes: []sckgNode{}, Edges: []sckgEdge{}}
	for i := 0; i < 15; i++ {
		depID := fmt.Sprintf("dep:%d", i)
		graph.Nodes = append(graph.Nodes, sckgNode{ID: depID, Type: "module", Name: depID})
		graph.Edges = append(graph.Edges, sckgEdge{Source: "file:a.go", Target: depID, Type: "imports"})
	}
	stats := graphStats(graph)
	if len(stats.TopImports) != 10 {
		t.Errorf("expected top 10 imports, got %d", len(stats.TopImports))
	}
}

func TestSckgOutputTextStats_TopImportsAndOrphans(t *testing.T) {
	stats := &sckgStats{
		TotalNodes:  3,
		TotalEdges:  1,
		NodeTypes:   map[string]int{"file": 1, "function": 2},
		EdgeTypes:   map[string]int{"contains": 1},
		TopImports:  []importCount{{Name: "fmt", Count: 2}},
		OrphanNodes: []string{"OrphanFunc"},
	}
	out := runAndCaptureStdout(t, func() { outputTextSCKGStats(stats) })
	if !strings.Contains(out, "Top Imports") {
		t.Errorf("expected top imports output, got %q", out)
	}
	if !strings.Contains(out, "OrphanFunc") {
		t.Errorf("expected orphan output, got %q", out)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// adw.go
// ─────────────────────────────────────────────────────────────────────────────

func TestAdwCmd_AbsError(t *testing.T) {
	oldAbs := adwAbs
	adwAbs = func(path string) (string, error) { return "", errors.New("abs error") }
	defer func() { adwAbs = oldAbs }()

	adwFormat, adwStrict = "text", false
	defer func() { adwFormat, adwStrict = "text", false }()

	_, err := runCmd(t, AdwCmd, []string{"."})
	if err == nil {
		t.Fatal("expected error from abs hook")
	}
}

func TestAdwCmd_PathNotFound(t *testing.T) {
	adwFormat, adwStrict = "text", false
	defer func() { adwFormat, adwStrict = "text", false }()

	_, err := runCmd(t, AdwCmd, []string{"/nonexistent/adw/path"})
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
}

func TestAdwCmd_PathNotDir(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file.go")
	os.WriteFile(f, []byte("package main\n"), 0o644)

	adwFormat, adwStrict = "text", false
	defer func() { adwFormat, adwStrict = "text", false }()

	_, err := runCmd(t, AdwCmd, []string{f})
	if err == nil {
		t.Fatal("expected error for file path")
	}
}

func TestAdwCmd_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644)

	adwFormat, adwStrict = "json", false
	defer func() { adwFormat, adwStrict = "text", false }()

	out, err := runCmd(t, AdwCmd, []string{dir})
	if err != nil {
		t.Fatalf("AdwCmd json failed: %v", err)
	}
	var result adwResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON: %v\n%q", err, out)
	}
}

func TestAdwCmd_TextOutput(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644)

	adwFormat, adwStrict = "text", false
	defer func() { adwFormat, adwStrict = "text", false }()

	out, err := runCmd(t, AdwCmd, []string{dir})
	if err != nil {
		t.Fatalf("AdwCmd text failed: %v", err)
	}
	if !strings.Contains(out, "Architectural Debt Watchdogs") {
		t.Errorf("expected text output, got %q", out)
	}
}

func TestAdwScanDebt_WalkError(t *testing.T) {
	oldWalk := adwWalk
	adwWalk = func(root string, fn filepath.WalkFunc) error { return errors.New("walk error") }
	defer func() { adwWalk = oldWalk }()

	res := scanDebt(t.TempDir(), false)
	if res.Summary.FilesScanned != 0 {
		t.Errorf("expected 0 files scanned after walk error, got %d", res.Summary.FilesScanned)
	}
}

func TestAdwScanDebt_FileReadError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod test is Unix-specific")
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "code.go")
	os.WriteFile(f, []byte("package main\n"), 0o644)
	os.Chmod(f, 0o000)
	defer os.Chmod(f, 0o644)

	res := scanDebt(dir, false)
	if res.Summary.FilesScanned != 1 {
		t.Errorf("expected 1 file scanned, got %d", res.Summary.FilesScanned)
	}
}

func TestAdwScanDebt_LargeFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "big.go")
	os.WriteFile(f, []byte("package main\n"+strings.Repeat("// line\n", 510)), 0o644)

	res := scanDebt(dir, false)
	found := false
	for _, issue := range res.Issues {
		if issue.Type == "large_file" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected large_file issue, got %v", res.Issues)
	}
}

func TestAdwScanDebt_GodModule(t *testing.T) {
	dir := t.TempDir()
	imports := []string{"fmt", "os", "strings", "bytes", "io", "net/http", "encoding/json", "time", "path/filepath", "regexp", "sort", "strconv", "errors", "sync", "context", "math", "log"}
	content := "package main\nimport (\n"
	for _, imp := range imports {
		content += fmt.Sprintf("\t\"%s\"\n", imp)
	}
	content += ")\nfunc main() {}\n"
	os.WriteFile(filepath.Join(dir, "big.go"), []byte(content), 0o644)

	res := scanDebt(dir, false)
	found := false
	for _, issue := range res.Issues {
		if issue.Type == "god_module" && issue.Severity == "high" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected god_module issue, got %v", res.Issues)
	}
}

func TestAdwScanDebt_GoLongFunction(t *testing.T) {
	dir := t.TempDir()
	content := "package main\nfunc short() {}\nfunc longFunc() {\n"
	content += strings.Repeat("\tprintln(1)\n", 101)
	content += "}\n"
	os.WriteFile(filepath.Join(dir, "long.go"), []byte(content), 0o644)

	res := scanDebt(dir, false)
	found := false
	for _, issue := range res.Issues {
		if issue.Type == "long_function" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected long_function issue, got %v", res.Issues)
	}
}

func TestAdwScanDebt_PythonLongFunction(t *testing.T) {
	dir := t.TempDir()
	content := "def short():\n    pass\ndef long_func():\n"
	content += strings.Repeat("    pass\n", 101)
	os.WriteFile(filepath.Join(dir, "long.py"), []byte(content), 0o644)

	res := scanDebt(dir, false)
	found := false
	for _, issue := range res.Issues {
		if issue.Type == "long_function" && strings.Contains(issue.Message, "long_func") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected python long_function issue, got %v", res.Issues)
	}
}

func TestAdwScanDebt_JSLongFunction(t *testing.T) {
	dir := t.TempDir()
	content := "function short() { return 1; }\nfunction longFunc() {\n"
	content += strings.Repeat("  console.log(1);\n", 101)
	content += "}\n"
	os.WriteFile(filepath.Join(dir, "long.js"), []byte(content), 0o644)

	res := scanDebt(dir, false)
	found := false
	for _, issue := range res.Issues {
		if issue.Type == "long_function" && strings.Contains(issue.Message, "longFunc") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected JS long_function issue, got %v", res.Issues)
	}
}

func TestAdwScanDebt_TODO(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "todo.go"), []byte("package main\n// TODO: fix\nfunc main() {}\n"), 0o644)

	res := scanDebt(dir, false)
	found := false
	for _, issue := range res.Issues {
		if issue.Type == "todo" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected todo issue, got %v", res.Issues)
	}
}

func TestAdwScanDebt_CircularDeps(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\nimport \"b.go\"\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("package main\nimport \"a.go\"\n"), 0o644)

	res := scanDebt(dir, false)
	if res.Summary.Critical == 0 {
		t.Errorf("expected critical circular dependency issue, got %v", res.Issues)
	}
}

func TestAdwScanDebt_HighCoupling(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "util.go"), []byte("package main\nfunc Helper() {}\n"), 0o644)
	for i := 0; i < 12; i++ {
		content := fmt.Sprintf("package main\nimport \"fmt\"\nfunc Client%d() { fmt.Println(%d) }\n", i, i)
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("client_%d.go", i)), []byte(content), 0o644)
	}

	res := scanDebt(dir, false)
	found := false
	for _, issue := range res.Issues {
		if issue.Type == "high_coupling" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected high_coupling issue, got %v", res.Issues)
	}
}

func TestAdwScanDebt_ScoreAndGradeBoundaries(t *testing.T) {
	res := scanDebt(t.TempDir(), false)
	if res.Score != 100 || res.Grade != "A" {
		t.Errorf("expected empty scan to score 100 grade A, got %d %s", res.Score, res.Grade)
	}

	// Many critical circular-dependency issues should cap score at 0 and grade at F.
	dir := t.TempDir()
	for i := 0; i < 6; i++ {
		a := filepath.Join(dir, fmt.Sprintf("a%d.go", i))
		b := filepath.Join(dir, fmt.Sprintf("b%d.go", i))
		os.WriteFile(a, []byte(fmt.Sprintf("package main\nimport \"%s\"\n", filepath.Base(b))), 0o644)
		os.WriteFile(b, []byte(fmt.Sprintf("package main\nimport \"%s\"\n", filepath.Base(a))), 0o644)
	}
	res = scanDebt(dir, false)
	if res.Score > 0 || res.Grade != "F" {
		t.Errorf("expected score 0 grade F, got %d %s", res.Score, res.Grade)
	}
}

func TestAdwScanDebt_StrictExitCode(t *testing.T) {
	dir := t.TempDir()
	imports := []string{"fmt", "os", "strings", "bytes", "io", "net/http", "encoding/json", "time", "path/filepath", "regexp", "sort", "strconv", "errors", "sync", "context", "math", "log"}
	content := "package main\nimport (\n"
	for _, imp := range imports {
		content += fmt.Sprintf("\t\"%s\"\n", imp)
	}
	content += ")\nfunc main() {}\n"
	os.WriteFile(filepath.Join(dir, "bad.go"), []byte(content), 0o644)

	res := scanDebt(dir, true)
	if res.ExitCode != 1 {
		t.Errorf("expected strict exit code 1, got %d", res.ExitCode)
	}
}

func TestAdwOutputTextADW_Critical(t *testing.T) {
	result := &adwResult{
		Path: "/tmp/test",
		Summary: adwSummary{
			FilesScanned: 1,
			TotalIssues:  1,
			Critical:     1,
		},
		Score: 80,
		Grade: "B",
		Issues: []adwIssue{
			{Type: "circular_dependency", Severity: "critical", File: "a.go", Message: "cycle"},
		},
	}
	out := runAndCaptureStdout(t, func() { outputTextADW(result) })
	if !strings.Contains(out, "cycle") {
		t.Errorf("expected critical issue, got %q", out)
	}
}

func TestAdwOutputTextADW_High(t *testing.T) {
	result := &adwResult{
		Path: "/tmp/test",
		Summary: adwSummary{
			FilesScanned: 1,
			TotalIssues:  1,
			High:         1,
		},
		Score: 90,
		Grade: "B",
		Issues: []adwIssue{
			{Type: "god_module", Severity: "high", File: "a.go", Message: "16 imports", Metric: "16 imports"},
		},
	}
	out := runAndCaptureStdout(t, func() { outputTextADW(result) })
	if !strings.Contains(out, "16 imports") {
		t.Errorf("expected high issue, got %q", out)
	}
	if !strings.Contains(out, "metric:") {
		t.Errorf("expected metric display, got %q", out)
	}
}

func TestAdwOutputTextADW_NoIssues(t *testing.T) {
	result := &adwResult{
		Path: "/tmp/test",
		Summary: adwSummary{
			FilesScanned: 1,
			TotalIssues:  0,
		},
		Score:  100,
		Grade:  "A",
		Issues: []adwIssue{},
	}
	out := runAndCaptureStdout(t, func() { outputTextADW(result) })
	if !strings.Contains(out, "No architectural debt detected") {
		t.Errorf("expected no-issues message, got %q", out)
	}
}

func TestAdwCheckLongFunctionsGo_Invalid(t *testing.T) {
	issues := checkLongFunctionsGo("bad.go", "bad.go", "not valid go")
	if len(issues) != 0 {
		t.Errorf("expected 0 issues for invalid Go, got %d", len(issues))
	}
}

func TestAdwCheckLongFunctionsPython_Short(t *testing.T) {
	issues := checkLongFunctionsPython("short.py", "short.py", "def short(): pass\n")
	if len(issues) != 0 {
		t.Errorf("expected 0 issues for short python, got %d", len(issues))
	}
}

func TestAdwCheckLongFunctionsJS_Short(t *testing.T) {
	issues := checkLongFunctionsJS("short.js", "short.js", "function short() { return 1; }\n")
	if len(issues) != 0 {
		t.Errorf("expected 0 issues for short JS, got %d", len(issues))
	}
}

func TestAdwFindCircularDeps_None(t *testing.T) {
	imports := map[string][]string{
		"a.go": {"b.go"},
		"b.go": {"c.go"},
		"c.go": {},
	}
	issues := findCircularDeps(imports)
	if len(issues) != 0 {
		t.Errorf("expected 0 circular deps, got %d", len(issues))
	}
}

func TestAdwIsTestFile(t *testing.T) {
	if !isTestFile("main_test.go") {
		t.Error("expected main_test.go to be test file")
	}
	if isTestFile("main.go") {
		t.Error("expected main.go not to be test file")
	}
}

func TestAdwIsConfigFile(t *testing.T) {
	if !isConfigFile("config.yaml") {
		t.Error("expected config.yaml to be config file")
	}
	if isConfigFile("main.go") {
		t.Error("expected main.go not to be config file")
	}
}

func TestAdwIsDocFile(t *testing.T) {
	if !isDocFile("README.md") {
		t.Error("expected README.md to be doc file")
	}
	if isDocFile("main.go") {
		t.Error("expected main.go not to be doc file")
	}
}

func TestAdwFindTestFile_Go(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package main\n"), 0o644)
	if !findTestFile(dir, "main.go", "go") {
		t.Error("expected to find Go test file")
	}
	if !findTestFile(dir, "main.go", "unknown") {
		t.Error("expected unknown language to return true (skip)")
	}
}

func TestAdwCheckTODOs_Quoted(t *testing.T) {
	content := "package main\nvar msg = \"TODO: not a real todo\"\n"
	issues := checkTODOs("main.go", content)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues for quoted TODO, got %d", len(issues))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// agent_doctor_cmd.go
// ─────────────────────────────────────────────────────────────────────────────

func TestAgentDoctorCmd_ShowError(t *testing.T) {
	withIsolatedAgentConfig(t)
	oldFormat := orch2Format
	orch2Format = "text"
	defer func() { orch2Format = oldFormat }()

	_, err := runCmd(t, OrchestratorAgentShowCmd, []string{"invalid/name"})
	if err == nil {
		t.Fatal("expected error for invalid agent name")
	}
}

func TestAgentDoctorCmd_ShowJSON(t *testing.T) {
	withIsolatedAgentConfig(t)
	oldFormat := orch2Format
	orch2Format = "json"
	defer func() { orch2Format = oldFormat }()

	out, err := runCmd(t, OrchestratorAgentShowCmd, []string{"coder"})
	if err != nil {
		t.Fatalf("agent-show json failed: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("expected valid JSON: %v\n%q", err, out)
	}
}

func TestAgentDoctorCmd_LoadAllError(t *testing.T) {
	withIsolatedAgentConfig(t)
	oldFormat := orch2Format
	orch2Format = "text"
	defer func() { orch2Format = oldFormat }()

	// Point config dir to a file so LoadUserAgents returns an error.
	oldCfg := os.Getenv("SIN_CODE_CONFIG_DIR")
	f := filepath.Join(t.TempDir(), "file")
	os.WriteFile(f, []byte("x"), 0o644)
	os.Setenv("SIN_CODE_CONFIG_DIR", f)
	defer os.Setenv("SIN_CODE_CONFIG_DIR", oldCfg)

	_, err := runCmd(t, OrchestratorAgentDoctorCmd, []string{})
	if err == nil {
		t.Fatal("expected error when loadAllEffectiveAgents fails")
	}
}

func TestAgentDoctorCmd_Success(t *testing.T) {
	withIsolatedAgentConfig(t)
	// Isolate from real user agent configs loaded via os.UserConfigDir.
	t.Setenv("HOME", t.TempDir())
	oldFormat := orch2Format
	oldOffline := agDoctorOffline
	orch2Format = "text"
	agDoctorOffline = true
	defer func() {
		orch2Format = oldFormat
		agDoctorOffline = oldOffline
	}()

	// Set API keys so the default agents pass in offline mode.
	t.Setenv("SIN_NIM_API_KEY", "key")
	t.Setenv("OPENAI_API_KEY", "key")
	t.Setenv("ANTHROPIC_API_KEY", "key")
	t.Setenv("GROQ_API_KEY", "key")
	t.Setenv("SIN_LLM_API_KEY", "key")

	_, err := runCmd(t, OrchestratorAgentDoctorCmd, []string{})
	if err != nil {
		t.Fatalf("agent-doctor expected success in offline mode, got: %v", err)
	}
}

func TestAgentDoctorCmd_JSON(t *testing.T) {
	withIsolatedAgentConfig(t)
	// Isolate from real user agent configs loaded via os.UserConfigDir.
	t.Setenv("HOME", t.TempDir())
	oldFormat := orch2Format
	oldOffline := agDoctorOffline
	orch2Format = "json"
	agDoctorOffline = true
	defer func() {
		orch2Format = oldFormat
		agDoctorOffline = oldOffline
	}()

	// Ensure no API keys are present so the default agent fails.
	keys := []string{"SIN_NIM_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GROQ_API_KEY", "SIN_LLM_API_KEY"}
	oldVals := make(map[string]string, len(keys))
	for _, k := range keys {
		oldVals[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	defer func() {
		for k, v := range oldVals {
			os.Setenv(k, v)
		}
	}()

	out, err := runCmd(t, OrchestratorAgentDoctorCmd, []string{"coder"})
	if err != nil {
		t.Fatalf("agent-doctor json expected no error, got: %v", err)
	}
	var reports []DoctorReport
	if err := json.Unmarshal([]byte(out), &reports); err != nil {
		t.Fatalf("expected valid JSON: %v\n%q", err, out)
	}
	if len(reports) != 1 || reports[0].OK {
		t.Errorf("expected JSON report with OK=false for coder, got %+v", reports)
	}
}

func TestAgentDoctorRunDoctor_ModelNotInList(t *testing.T) {
	oldKey := os.Getenv("OPENAI_API_KEY")
	os.Setenv("OPENAI_API_KEY", "test-key")
	defer os.Setenv("OPENAI_API_KEY", oldKey)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]string{{"id": "other-model"}},
		})
	}))
	defer srv.Close()

	cfg := orchestrator.AgentConfig{
		Name:     "mock-agent",
		Provider: "openai",
		BaseURL:  srv.URL,
		Model:    "missing-model",
	}
	rep := runDoctor([]orchestrator.AgentConfig{cfg}, false)
	if len(rep) != 1 || rep[0].OK {
		t.Fatalf("expected failing report for missing model, got %+v", rep[0])
	}
	if rep[0].Info["models_available"] != 1 {
		t.Errorf("expected models_available=1, got %v", rep[0].Info["models_available"])
	}
}

func TestAgentDoctorFetchModels_NetworkError(t *testing.T) {
	_, err := fetchModels("http://127.0.0.1:1", "")
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestAgentDoctorFetchModels_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	if _, err := fetchModels(srv.URL, ""); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestAgentDoctorPrintDoctor_ModelsAvailable(t *testing.T) {
	out := runAndCaptureStdout(t, func() {
		printDoctor([]DoctorReport{
			{Agent: "mock", OK: true, Info: map[string]interface{}{
				"provider":         "openai",
				"base_url":         "http://x",
				"model":            "m",
				"models_available": 5,
			}},
		})
	})
	if !strings.Contains(out, "5 models") {
		t.Errorf("expected models available output, got %q", out)
	}
}

func TestAgentDoctorStringInList(t *testing.T) {
	if !stringInList([]string{"a", "b", "c"}, "b") {
		t.Error("expected to find b")
	}
	if stringInList([]string{"a", "b"}, "z") {
		t.Error("expected not to find z")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// agent_edit_cmd.go
// ─────────────────────────────────────────────────────────────────────────────

func TestAgentEditCmd_MissingAgent(t *testing.T) {
	oldAgent, oldSet := agEditAgent, agEditSet
	agEditAgent, agEditSet = "", nil
	defer func() { agEditAgent, agEditSet = oldAgent, oldSet }()

	_, err := runCmd(t, OrchestratorAgentEditCmd, []string{})
	if err == nil {
		t.Fatal("expected error for missing --agent")
	}
}

func TestAgentEditCmd_WithSet(t *testing.T) {
	withIsolatedAgentConfig(t)
	oldAgent, oldSet := agEditAgent, agEditSet
	agEditAgent, agEditSet = "test-edit-agent", []string{"model=gpt-4"}
	defer func() { agEditAgent, agEditSet = oldAgent, oldSet }()

	out, err := runCmd(t, OrchestratorAgentEditCmd, []string{})
	if err != nil {
		t.Fatalf("agent-edit --set failed: %v", err)
	}
	if !strings.Contains(out, "Updated") {
		t.Errorf("expected update message, got %q", out)
	}
}

func TestAgentEditCmd_OpenEditor(t *testing.T) {
	withIsolatedAgentConfig(t)
	t.Setenv("EDITOR", "true")
	oldAgent, oldSet := agEditAgent, agEditSet
	agEditAgent, agEditSet = "test-edit-agent", nil
	defer func() { agEditAgent, agEditSet = oldAgent, oldSet }()

	out, err := runCmd(t, OrchestratorAgentEditCmd, []string{})
	if err != nil {
		t.Fatalf("agent-edit editor failed: %v", err)
	}
	if !strings.Contains(out, "Seeded") && !strings.Contains(out, "true") {
		t.Errorf("expected editor or seed output, got %q", out)
	}
}

func TestAgentSetCmd_ArgsValidation(t *testing.T) {
	if err := OrchestratorAgentSetCmd.Args(OrchestratorAgentSetCmd, []string{"name"}); err == nil {
		t.Fatal("expected error for too few args")
	}
}

func TestAgentSetCmd_Success(t *testing.T) {
	withIsolatedAgentConfig(t)
	out, err := runCmd(t, OrchestratorAgentSetCmd, []string{"test-set-agent", "model=gpt-4", "provider=openai"})
	if err != nil {
		t.Fatalf("agent-set failed: %v", err)
	}
	if !strings.Contains(out, "Updated") {
		t.Errorf("expected update message, got %q", out)
	}
}

func TestAgentResetCmd_NoUserConfig(t *testing.T) {
	withIsolatedAgentConfig(t)
	out, err := runCmd(t, OrchestratorAgentResetCmd, []string{"no-such-agent"})
	if err != nil {
		t.Fatalf("agent-reset failed: %v", err)
	}
	if !strings.Contains(out, "nothing to reset") {
		t.Errorf("expected nothing-to-reset message, got %q", out)
	}
}

func TestAgentResetCmd_Success(t *testing.T) {
	withIsolatedAgentConfig(t)
	// Create a user config first.
	dir, _ := agentDir("reset-agent")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "agent.toml"), []byte("name = \"reset-agent\"\n"), 0o644)

	out, err := runCmd(t, OrchestratorAgentResetCmd, []string{"reset-agent"})
	if err != nil {
		t.Fatalf("agent-reset failed: %v", err)
	}
	if !strings.Contains(out, "Reset agent") {
		t.Errorf("expected reset message, got %q", out)
	}
}

func TestAgentEditOpenAgentInEditor_SeedWriteError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod test is Unix-specific")
	}
	withIsolatedAgentConfig(t)
	t.Setenv("EDITOR", "true")

	// Create a read-only agents directory so WriteFile fails.
	cfgDir := os.Getenv("SIN_CODE_CONFIG_DIR")
	roDir := filepath.Join(cfgDir, "sin-code", "agents", "seed-error-agent")
	os.MkdirAll(roDir, 0o755)
	os.Chmod(roDir, 0o555)
	defer os.Chmod(roDir, 0o755)

	err := openAgentInEditor("seed-error-agent")
	if err == nil {
		t.Fatal("expected error when seed write fails")
	}
}

func TestAgentEditOpenAgentInEditor_MkdirError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod test is Unix-specific")
	}
	withIsolatedAgentConfig(t)
	t.Setenv("EDITOR", "true")

	cfgDir := os.Getenv("SIN_CODE_CONFIG_DIR")
	os.Chmod(cfgDir, 0o555)
	defer os.Chmod(cfgDir, 0o755)

	err := openAgentInEditor("mkdir-error-agent")
	if err == nil {
		t.Fatal("expected error when mkdir fails")
	}
}

func TestAgentEditOpenAgentInEditor_EditorError(t *testing.T) {
	withIsolatedAgentConfig(t)
	t.Setenv("EDITOR", "nonexistent-binary-for-test-xyz")

	err := openAgentInEditor("editor-error-agent")
	if err == nil {
		t.Fatal("expected error for missing editor")
	}
}

func TestAgentEditBuildAgentSeed_NewAgent(t *testing.T) {
	seed := buildAgentSeed("new-agent-zzz")
	if seed == "" {
		t.Fatal("expected non-empty seed")
	}
	if !strings.Contains(seed, "new-agent-zzz") {
		t.Errorf("expected seed to contain name, got %q", seed)
	}
}

func TestAgentEditApplyAgentEdits_InvalidKV(t *testing.T) {
	withIsolatedAgentConfig(t)
	err := applyAgentEdits("kv-agent", []string{"notakeyvalue"})
	if err == nil {
		t.Fatal("expected error for invalid key=value")
	}
}

func TestAgentEditApplyAgentEdits_InvalidField(t *testing.T) {
	withIsolatedAgentConfig(t)
	err := applyAgentEdits("field-agent", []string{"unknown_field=value"})
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestAgentEditApplyAgentEdits_EncodeError(t *testing.T) {
	withIsolatedAgentConfig(t)
	oldEncoder := tomlNewEncoder
	tomlNewEncoder = func(w io.Writer) tomlEncoder {
		return &failEncoder{}
	}
	defer func() { tomlNewEncoder = oldEncoder }()

	err := applyAgentEdits("encode-agent", []string{"model=gpt-4"})
	if err == nil {
		t.Fatal("expected error from encoder")
	}
}

type failEncoder struct{}

func (f *failEncoder) Encode(v interface{}) error { return errors.New("encode error") }

func TestAgentEditSetAgentField_AllFields(t *testing.T) {
	cfg := &orchestrator.AgentConfig{}
	fields := map[string]string{
		"name":             "x",
		"description":      "desc",
		"type":             "code",
		"provider":         "openai",
		"base_url":         "http://x",
		"model":            "m",
		"max_tokens":       "100",
		"temperature":      "0.5",
		"system_file":      "sys.md",
		"max_context":      "200",
		"memory_namespace": "ns",
		"retention_days":   "7",
		"tools_allow":      "a,b",
		"tools_deny":       "c,d",
	}
	for k, v := range fields {
		if err := setAgentField(cfg, k, v); err != nil {
			t.Fatalf("setAgentField(%q, %q): %v", k, v, err)
		}
	}
	if cfg.Name != "x" || cfg.Description != "desc" || cfg.Type != "code" || cfg.Provider != "openai" ||
		cfg.BaseURL != "http://x" || cfg.Model != "m" || cfg.MaxTokens != 100 || cfg.Temperature != 0.5 ||
		cfg.SystemFile != "sys.md" || cfg.MaxContext != 200 || cfg.MemoryNS != "ns" || cfg.RetentionDays != 7 ||
		len(cfg.ToolsAllow) != 2 || len(cfg.ToolsDeny) != 2 {
		t.Errorf("unexpected cfg: %+v", cfg)
	}
}

func TestAgentEditSetAgentField_InvalidNumber(t *testing.T) {
	cfg := &orchestrator.AgentConfig{}
	if err := setAgentField(cfg, "max_tokens", "notanumber"); err == nil {
		t.Error("expected error for non-numeric max_tokens")
	}
	if err := setAgentField(cfg, "temperature", "notanumber"); err == nil {
		t.Error("expected error for non-numeric temperature")
	}
	if err := setAgentField(cfg, "max_context", "notanumber"); err == nil {
		t.Error("expected error for non-numeric max_context")
	}
	if err := setAgentField(cfg, "retention_days", "notanumber"); err == nil {
		t.Error("expected error for non-numeric retention_days")
	}
}

func TestAgentEditSplitKV(t *testing.T) {
	k, v, ok := splitKV("key=value")
	if !ok || k != "key" || v != "value" {
		t.Errorf("splitKV failed: %q %q %v", k, v, ok)
	}
	_, _, ok = splitKV("noequals")
	if ok {
		t.Error("expected no ok for missing =")
	}
}

func TestAgentEditSplitCSV(t *testing.T) {
	got := splitCSV("  a , b ,c  ")
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("splitCSV = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("splitCSV[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if splitCSV("") != nil {
		t.Error("expected nil for empty CSV")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// agent_helpers.go
// ─────────────────────────────────────────────────────────────────────────────

func TestAgentHelpersAgentDir_UserConfigDirError(t *testing.T) {
	old := osUserConfigDir
	osUserConfigDir = func() (string, error) { return "", errors.New("no config dir") }
	defer func() { osUserConfigDir = old }()

	// Ensure env vars are not set.
	oldSin := os.Getenv("SIN_CODE_CONFIG_DIR")
	oldXdg := os.Getenv("XDG_CONFIG_HOME")
	os.Unsetenv("SIN_CODE_CONFIG_DIR")
	os.Unsetenv("XDG_CONFIG_HOME")
	defer func() {
		os.Setenv("SIN_CODE_CONFIG_DIR", oldSin)
		os.Setenv("XDG_CONFIG_HOME", oldXdg)
	}()

	_, err := agentDir("coder")
	if err == nil {
		t.Fatal("expected error from UserConfigDir")
	}
}

func TestAgentHelpersLoadAllAgents_Error(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME-based config dir test is Unix-specific")
	}
	// Make os.UserConfigDir return a file path so ReadDir fails.
	oldHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	configPath := filepath.Join(tmpHome, "Library", "Application Support")
	os.MkdirAll(filepath.Dir(configPath), 0o755)
	os.WriteFile(configPath, []byte("x"), 0o644)
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", oldHome)

	_, err := loadAllEffectiveAgents()
	if err == nil {
		t.Fatal("expected error from LoadUserAgents")
	}
}

func TestAgentHelpersLoadEffectiveAgent_Default(t *testing.T) {
	withIsolatedAgentConfig(t)
	cfg, source, err := loadEffectiveAgent("coder")
	if err != nil {
		t.Fatalf("loadEffectiveAgent failed: %v", err)
	}
	if source != "default" {
		t.Errorf("expected default source, got %q", source)
	}
	if cfg.Name != "coder" {
		t.Errorf("expected coder, got %q", cfg.Name)
	}
}

func TestAgentHelpersLoadEffectiveAgent_UserOverrideDefault(t *testing.T) {
	withIsolatedAgentConfig(t)
	dir, _ := agentDir("coder")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "agent.toml"), []byte("model = \"override\"\n"), 0o644)

	cfg, source, err := loadEffectiveAgent("coder")
	if err != nil {
		t.Fatalf("loadEffectiveAgent failed: %v", err)
	}
	if source != "user (overrides default)" {
		t.Errorf("expected user override source, got %q", source)
	}
	if cfg.Model != "override" {
		t.Errorf("expected override model, got %q", cfg.Model)
	}
}

func TestAgentHelpersLoadEffectiveAgent_NewUserAgent(t *testing.T) {
	withIsolatedAgentConfig(t)
	dir, _ := agentDir("custom-agent-zzz")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "agent.toml"), []byte("name = \"custom-agent-zzz\"\nmodel = \"m\"\n"), 0o644)

	cfg, source, err := loadEffectiveAgent("custom-agent-zzz")
	if err != nil {
		t.Fatalf("loadEffectiveAgent failed: %v", err)
	}
	if source != "user (new agent)" {
		t.Errorf("expected new user agent source, got %q", source)
	}
	if cfg.Model != "m" {
		t.Errorf("expected model m, got %q", cfg.Model)
	}
}

func TestAgentHelpersLoadEffectiveAgent_DecodeError(t *testing.T) {
	withIsolatedAgentConfig(t)
	dir, _ := agentDir("coder")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "agent.toml"), []byte("not valid toml = ="), 0o644)

	_, _, err := loadEffectiveAgent("coder")
	if err == nil {
		t.Fatal("expected error for invalid toml")
	}
}

func TestAgentHelpersLoadEffectiveAgent_NotFound(t *testing.T) {
	withIsolatedAgentConfig(t)
	_, _, err := loadEffectiveAgent("nonexistent-agent-xyz")
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
}

func TestAgentHelpersMergeAgentConfig(t *testing.T) {
	base := orchestrator.AgentConfig{Name: "base", Model: "m1", MaxTokens: 100}
	override := orchestrator.AgentConfig{Model: "m2", MaxTokens: 200, Provider: "openai"}
	merged := mergeAgentConfig(base, override)
	if merged.Model != "m2" || merged.MaxTokens != 200 || merged.Provider != "openai" || merged.Name != "base" {
		t.Errorf("unexpected merge: %+v", merged)
	}
}

func TestAgentHelpersOrDash(t *testing.T) {
	if orDash("") != "-" {
		t.Error("expected empty to be -")
	}
	if orDash("x") != "x" {
		t.Error("expected non-empty to be unchanged")
	}
}

func TestAgentHelpersSanitizeName(t *testing.T) {
	if sanitizeName("a/b") != "ab" {
		t.Errorf("sanitize failed: %q", sanitizeName("a/b"))
	}
	if sanitizeName("a b") != "ab" {
		t.Errorf("sanitize failed: %q", sanitizeName("a b"))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Additional coverage for remaining branches
// ─────────────────────────────────────────────────────────────────────────────

func TestPocCmd_VerifyError(t *testing.T) {
	oldSpec, oldCode, oldFormat := pocSpec, pocCode, pocFormat
	pocSpec, pocCode, pocFormat = "/nonexistent/spec.md", "", "text"
	defer func() { pocSpec, pocCode, pocFormat = oldSpec, oldCode, oldFormat }()

	_, err := runCmd(t, PocCmd, []string{})
	if err == nil {
		t.Fatal("expected error from verifyCorrectness")
	}
}

func TestPocOutputTextPOC_Warn(t *testing.T) {
	result := &pocResult{
		Spec:        "spec.md",
		Code:        "code.go",
		Coverage:    100,
		Passed:      1,
		Failed:      0,
		TotalChecks: 1,
		Checks: []pocCheck{
			{Name: "TODO", Type: "forbidden", Status: "warn", Message: "TODO found"},
		},
		Summary: "Coverage: 100.0%",
	}
	out := runAndCaptureStdout(t, func() { outputTextPOC(result) })
	if !strings.Contains(out, "▲") {
		t.Errorf("expected warn icon, got %q", out)
	}
}

func TestPocExtractRequirements_CodeBlockNewRequirement(t *testing.T) {
	// The code block must introduce a requirement that is NOT already extracted
	// by the outer regexes. A bare, quoted identifier before a keyword on the
	// same line inside the code block is seen by both; use a multi-line pattern
	// where the outer preRe cannot match across the newline but the inner call
	// still sees the whole block. Actually, the same regex is applied to the
	// block content, so we instead rely on the outer regexes missing the pattern
	// by placing it inside a code block language fence that the outer callRe
	// does not see as a function call. Use a backtick-quoted identifier before
	// a keyword on one line inside the code block.
	spec := "See code:\n```go\n`MyStruct` type\n```\n"
	reqs := extractRequirements(spec)
	found := false
	for _, r := range reqs {
		if r.Name == "MyStruct" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected MyStruct from code block, got %v", reqs)
	}
}

func TestSckgCmd_QueryError(t *testing.T) {
	oldWalk := sckgWalk
	sckgWalk = func(root string, fn filepath.WalkFunc) error { return errors.New("walk error") }
	defer func() { sckgWalk = oldWalk }()

	sckgAction, sckgQuery, sckgFormat = "query", "x", "text"
	defer func() { sckgAction, sckgQuery, sckgFormat = "build", "", "text" }()

	_, err := runCmd(t, SckgCmd, []string{t.TempDir()})
	if err == nil {
		t.Fatal("expected error from query buildGraph")
	}
}

func TestSckgCmd_StatsError(t *testing.T) {
	oldWalk := sckgWalk
	sckgWalk = func(root string, fn filepath.WalkFunc) error { return errors.New("walk error") }
	defer func() { sckgWalk = oldWalk }()

	sckgAction, sckgQuery, sckgFormat = "stats", "", "text"
	defer func() { sckgAction, sckgQuery, sckgFormat = "build", "", "text" }()

	_, err := runCmd(t, SckgCmd, []string{t.TempDir()})
	if err == nil {
		t.Fatal("expected error from stats buildGraph")
	}
}

func TestSckgCmd_ExportError(t *testing.T) {
	oldWalk := sckgWalk
	sckgWalk = func(root string, fn filepath.WalkFunc) error { return errors.New("walk error") }
	defer func() { sckgWalk = oldWalk }()

	sckgAction, sckgQuery, sckgFormat = "export", "", "text"
	defer func() { sckgAction, sckgQuery, sckgFormat = "build", "", "text" }()

	_, err := runCmd(t, SckgCmd, []string{t.TempDir()})
	if err == nil {
		t.Fatal("expected error from export buildGraph")
	}
}

func TestSckgBuildGraph_SkipsNonCodeFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Hello"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("notes"), 0o644)

	graph, err := buildGraph(dir)
	if err != nil {
		t.Fatalf("buildGraph failed: %v", err)
	}
	for _, n := range graph.Nodes {
		if n.Type == "file" {
			t.Errorf("expected no file nodes for md/txt, found: %v", n)
		}
	}
}

func TestSckgQueryGraph_RelatedBySource(t *testing.T) {
	graph := &sckgGraph{
		Nodes: []sckgNode{
			{ID: "a", Name: "Alpha"},
			{ID: "b", Name: "Beta"},
		},
		Edges: []sckgEdge{{Source: "a", Target: "b", Type: "calls"}},
	}
	res := queryGraph(graph, "alpha")
	if len(res.Matches) != 1 || res.Matches[0].ID != "a" {
		t.Errorf("expected match for Alpha, got %+v", res.Matches)
	}
	if len(res.Related) != 1 || res.Related[0].ID != "b" {
		t.Errorf("expected related Beta, got %+v", res.Related)
	}
}

func TestSckgGraphStats_OrphanNodes(t *testing.T) {
	graph := &sckgGraph{
		Nodes: []sckgNode{
			{ID: "file:a.go", Type: "file", Name: "a.go"},
			{ID: "func:a.go:Hello", Type: "function", Name: "Hello"},
			{ID: "func:a.go:Orphan", Type: "function", Name: "Orphan"},
		},
		Edges: []sckgEdge{
			{Source: "file:a.go", Target: "func:a.go:Hello", Type: "contains"},
		},
	}
	stats := graphStats(graph)
	found := false
	for _, o := range stats.OrphanNodes {
		if o == "Orphan" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected Orphan in orphan nodes, got %v", stats.OrphanNodes)
	}
}

func TestSckgBuildGraph_DirectoryWalkError(t *testing.T) {
	oldWalk := sckgWalk
	sckgWalk = func(root string, fn filepath.WalkFunc) error {
		info := &fakeDirInfo{name: "bad", mode: os.ModeDir}
		return fn(filepath.Join(root, "bad"), info, errors.New("dir error"))
	}
	defer func() { sckgWalk = oldWalk }()

	graph, err := buildGraph(t.TempDir())
	if err != nil {
		t.Fatalf("buildGraph failed: %v", err)
	}
	if len(graph.Nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(graph.Nodes))
	}
}

type fakeDirInfo struct {
	name string
	mode os.FileMode
}

func (f fakeDirInfo) Name() string       { return f.name }
func (f fakeDirInfo) Size() int64        { return 0 }
func (f fakeDirInfo) Mode() os.FileMode  { return f.mode }
func (f fakeDirInfo) ModTime() time.Time { return time.Now() }
func (f fakeDirInfo) IsDir() bool        { return f.mode&os.ModeDir != 0 }
func (f fakeDirInfo) Sys() interface{}   { return nil }

type fakeFileInfo struct {
	name string
	mode os.FileMode
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Now() }
func (f fakeFileInfo) IsDir() bool        { return f.mode&os.ModeDir != 0 }
func (f fakeFileInfo) Sys() interface{}   { return nil }

func TestAdwScanDebt_ScoreCap(t *testing.T) {
	oldScore := adwInitialScore
	adwInitialScore = 200
	defer func() { adwInitialScore = oldScore }()

	res := scanDebt(t.TempDir(), false)
	if res.Score != 100 {
		t.Errorf("expected score capped at 100, got %d", res.Score)
	}
}

func TestAdwCheckTODOs_SkipsAdwFile(t *testing.T) {
	issues := checkTODOs("adw.go", "// TODO: should be ignored\n")
	if len(issues) != 0 {
		t.Errorf("expected 0 issues for adw.go, got %d", len(issues))
	}
	issues = checkTODOs("adw_test.go", "// TODO: should be ignored\n")
	if len(issues) != 0 {
		t.Errorf("expected 0 issues for adw_test.go, got %d", len(issues))
	}
}

func TestAdwCheckTODOs_SkipsRawString(t *testing.T) {
	content := "package main\nvar hint = `TODO: inside\n"
	issues := checkTODOs("main.go", content)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues for raw string, got %d", len(issues))
	}
}

func TestAdwCheckTODOs_SkipsRegexpCompile(t *testing.T) {
	content := "package main\nre := regexp.MustCompile(`TODO:.*`)\n"
	issues := checkTODOs("main.go", content)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues for regexp pattern, got %d", len(issues))
	}
}

func TestAdwCheckTODOs_SkipsBullet(t *testing.T) {
	content := "  - TODO/FIXME comments\n"
	issues := checkTODOs("main.go", content)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues for bullet, got %d", len(issues))
	}
}

func TestAdwCheckTODOs_MediumSeverity(t *testing.T) {
	content := "package main\n// FIXME: urgent\n// BUG: crash\n"
	issues := checkTODOs("main.go", content)
	if len(issues) == 0 {
		t.Fatal("expected issues")
	}
	for _, issue := range issues {
		if issue.Severity != "medium" {
			t.Errorf("expected medium severity, got %q", issue.Severity)
		}
	}
}

func TestAdwFindTestFile_Rust(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "lib.rs"), []byte("fn func() {}"), 0o644)
	os.WriteFile(filepath.Join(dir, "lib_test.rs"), []byte("fn test() {}"), 0o644)
	if !findTestFile(dir, "lib.rs", "rust") {
		t.Error("expected to find lib_test.rs")
	}
}

func TestAdwFindTestFile_Java(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "App.java"), []byte("class App {}"), 0o644)
	os.WriteFile(filepath.Join(dir, "AppTest.java"), []byte("class AppTest {}"), 0o644)
	if !findTestFile(dir, "App.java", "java") {
		t.Error("expected to find AppTest.java")
	}
}

func TestAdwOutputTextADW_MediumAndLine(t *testing.T) {
	result := &adwResult{
		Path: "/tmp/test",
		Summary: adwSummary{
			FilesScanned: 1,
			TotalIssues:  2,
			Medium:       1,
			Low:          1,
		},
		Score: 90,
		Grade: "B",
		Issues: []adwIssue{
			{Type: "todo", Severity: "medium", File: "main.go", Line: 5, Message: "FIXME: x"},
			{Type: "todo", Severity: "low", File: "other.go", Line: 0, Message: "TODO: y"},
		},
	}
	out := runAndCaptureStdout(t, func() { outputTextADW(result) })
	if !strings.Contains(out, "main.go:5") {
		t.Errorf("expected line in location, got %q", out)
	}
	if !strings.Contains(out, "▲") {
		t.Errorf("expected medium icon, got %q", out)
	}
	if !strings.Contains(out, "○") {
		t.Errorf("expected low icon, got %q", out)
	}
}

func TestAdwScanDebt_DirectoryWalkError(t *testing.T) {
	oldWalk := adwWalk
	adwWalk = func(root string, fn filepath.WalkFunc) error {
		info := &fakeDirInfo{name: "bad", mode: os.ModeDir}
		return fn(filepath.Join(root, "bad"), info, errors.New("dir error"))
	}
	defer func() { adwWalk = oldWalk }()

	res := scanDebt(t.TempDir(), false)
	if res.Summary.FilesScanned != 0 {
		t.Errorf("expected 0 files scanned, got %d", res.Summary.FilesScanned)
	}
}

func TestAdwScanDebt_UnknownLanguageSkip(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "data.json"), []byte("{}"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Hello"), 0o644)
	os.WriteFile(filepath.Join(dir, "plain.txt"), []byte("hello"), 0o644)

	res := scanDebt(dir, false)
	if res.Summary.FilesScanned != 0 {
		t.Errorf("expected 0 files scanned for non-code files, got %d", res.Summary.FilesScanned)
	}
}

func TestAgentDoctorCmd_LoadAllError_Isolated(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME-based config dir test is Unix-specific")
	}
	withIsolatedAgentConfig(t)
	oldFormat := orch2Format
	orch2Format = "text"
	defer func() { orch2Format = oldFormat }()

	oldHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	configPath := filepath.Join(tmpHome, "Library", "Application Support")
	os.MkdirAll(filepath.Dir(configPath), 0o755)
	os.WriteFile(configPath, []byte("x"), 0o644)
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", oldHome)

	_, err := runCmd(t, OrchestratorAgentDoctorCmd, []string{})
	if err == nil {
		t.Fatal("expected error when loadAllEffectiveAgents fails")
	}
}

func TestAgentDoctorRunDoctor_MissingBaseURL(t *testing.T) {
	cfg := orchestrator.AgentConfig{Name: "x", Provider: "custom"}
	rep := runDoctor([]orchestrator.AgentConfig{cfg}, true)
	if len(rep) != 1 || rep[0].OK {
		t.Fatalf("expected failing report for missing base URL, got %+v", rep[0])
	}
	found := false
	for _, issue := range rep[0].Issues {
		if issue == "no base_url configured" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected missing base_url issue, got %v", rep[0].Issues)
	}
}

func TestAgentDoctorRunDoctor_FetchModelsError(t *testing.T) {
	oldKey := os.Getenv("OPENAI_API_KEY")
	os.Setenv("OPENAI_API_KEY", "test-key")
	defer os.Setenv("OPENAI_API_KEY", oldKey)

	cfg := orchestrator.AgentConfig{
		Name:     "x",
		Provider: "openai",
		BaseURL:  "http://127.0.0.1:1",
		Model:    "gpt-4o",
	}
	rep := runDoctor([]orchestrator.AgentConfig{cfg}, false)
	if len(rep) != 1 {
		t.Fatalf("expected 1 report, got %d", len(rep))
	}
	found := false
	for _, issue := range rep[0].Issues {
		if strings.Contains(issue, "could not fetch /v1/models") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected fetch models issue, got %v", rep[0].Issues)
	}
}

func TestAgentEditResetCmd_InvalidName(t *testing.T) {
	_, err := runCmd(t, OrchestratorAgentResetCmd, []string{"invalid/name"})
	if err == nil {
		t.Fatal("expected error for invalid agent name")
	}
}

func TestAgentEditOpenAgentInEditor_InvalidName(t *testing.T) {
	if err := openAgentInEditor("invalid/name"); err == nil {
		t.Fatal("expected error for invalid agent name")
	}
}

func TestAgentEditOpenAgentInEditor_DefaultEditor(t *testing.T) {
	withIsolatedAgentConfig(t)
	t.Setenv("EDITOR", "")
	binDir := t.TempDir()
	fakeVim := filepath.Join(binDir, "vim")
	os.WriteFile(fakeVim, []byte("#!/bin/sh\nexit 0\n"), 0o755)
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir)
	defer os.Setenv("PATH", oldPath)

	if err := openAgentInEditor("default-editor-agent"); err != nil {
		t.Fatalf("openAgentInEditor with default editor failed: %v", err)
	}
}

func TestAgentEditBuildAgentSeed_DefaultAgent(t *testing.T) {
	seed := buildAgentSeed("coder")
	if seed == "" {
		t.Fatal("empty seed for default agent")
	}
	if !strings.Contains(seed, "coder") {
		t.Errorf("expected seed to contain coder, got %q", seed)
	}
}

func TestAgentEditApplyAgentEdits_InvalidName(t *testing.T) {
	if err := applyAgentEdits("invalid/name", []string{"model=gpt-4"}); err == nil {
		t.Fatal("expected error for invalid agent name")
	}
}

func TestAgentEditApplyAgentEdits_MkdirError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod test is Unix-specific")
	}
	withIsolatedAgentConfig(t)
	cfgDir := os.Getenv("SIN_CODE_CONFIG_DIR")
	os.Chmod(cfgDir, 0o555)
	defer os.Chmod(cfgDir, 0o755)

	if err := applyAgentEdits("mkdir-error-agent", []string{"model=gpt-4"}); err == nil {
		t.Fatal("expected error when mkdir fails")
	}
}

func TestAgentEditApplyAgentEdits_DecodeError(t *testing.T) {
	withIsolatedAgentConfig(t)
	dir, _ := agentDir("decode-agent")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "agent.toml"), []byte("not valid toml = ="), 0o644)

	if err := applyAgentEdits("decode-agent", []string{"model=gpt-4"}); err == nil {
		t.Fatal("expected error for invalid toml")
	}
}

func TestAgentEditApplyAgentEdits_CreateError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod test is Unix-specific")
	}
	withIsolatedAgentConfig(t)
	dir, _ := agentDir("create-error-agent")
	os.MkdirAll(dir, 0o755)
	os.Chmod(dir, 0o555)
	defer os.Chmod(dir, 0o755)

	if err := applyAgentEdits("create-error-agent", []string{"model=gpt-4"}); err == nil {
		t.Fatal("expected error when create fails")
	}
}

func TestAgentEditApplyAgentEdits_NameFallback(t *testing.T) {
	withIsolatedAgentConfig(t)
	dir, _ := agentDir("name-fallback-agent")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "agent.toml"), []byte("model = \"m\"\n"), 0o644)

	if err := applyAgentEdits("name-fallback-agent", []string{"provider=openai"}); err != nil {
		t.Fatalf("applyAgentEdits failed: %v", err)
	}
	var cfg orchestrator.AgentConfig
	_, _ = toml.DecodeFile(filepath.Join(dir, "agent.toml"), &cfg)
	if cfg.Name != "name-fallback-agent" {
		t.Errorf("expected name fallback, got %q", cfg.Name)
	}
}

func TestAgentEditSetAgentField_UnknownField(t *testing.T) {
	cfg := &orchestrator.AgentConfig{}
	if err := setAgentField(cfg, "unknown", "x"); err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestAgentHelpersMergeAgentConfig_AllFields(t *testing.T) {
	base := orchestrator.AgentConfig{Name: "base"}
	override := orchestrator.AgentConfig{
		Description:   "desc",
		Type:          "code",
		Provider:      "p",
		BaseURL:       "http://x",
		Model:         "m2",
		MaxTokens:     2000,
		Temperature:   0.5,
		SystemFile:    "sys",
		MaxContext:    4000,
		ToolsAllow:    []string{"a"},
		ToolsDeny:     []string{"b"},
		MemoryNS:      "ns",
		RetentionDays: 7,
	}
	merged := mergeAgentConfig(base, override)
	if merged.Description != "desc" || merged.Type != "code" || merged.Provider != "p" ||
		merged.BaseURL != "http://x" || merged.Model != "m2" || merged.MaxTokens != 2000 ||
		merged.Temperature != 0.5 || merged.SystemFile != "sys" || merged.MaxContext != 4000 ||
		merged.MemoryNS != "ns" || merged.RetentionDays != 7 ||
		len(merged.ToolsAllow) != 1 || len(merged.ToolsDeny) != 1 || merged.Name != "base" {
		t.Errorf("unexpected merge: %+v", merged)
	}
}

func TestAgentHelpersLoadAllAgents_NewUserAgent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME-based config dir test is Unix-specific")
	}
	withIsolatedAgentConfig(t)
	oldHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	agentsDir := filepath.Join(tmpHome, "Library", "Application Support", "sin-code", "agents")
	os.MkdirAll(filepath.Join(agentsDir, "custom-user-agent"), 0o755)
	os.WriteFile(filepath.Join(agentsDir, "custom-user-agent", "agent.toml"), []byte("name = \"custom-user-agent\"\nmodel = \"m\"\n"), 0o644)
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", oldHome)

	agents, err := loadAllEffectiveAgents()
	if err != nil {
		t.Fatalf("loadAllEffectiveAgents failed: %v", err)
	}
	found := false
	for _, a := range agents {
		if a.Name == "custom-user-agent" && a.Model == "m" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected custom-user-agent in agents, got %v", agents)
	}
}

func TestAgentHelpersLoadAllAgents_MergeDefault(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME-based config dir test is Unix-specific")
	}
	withIsolatedAgentConfig(t)
	oldHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	agentsDir := filepath.Join(tmpHome, "Library", "Application Support", "sin-code", "agents")
	os.MkdirAll(filepath.Join(agentsDir, "coder"), 0o755)
	os.WriteFile(filepath.Join(agentsDir, "coder", "agent.toml"), []byte("name = \"coder\"\nmodel = \"user-model\"\n"), 0o644)
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", oldHome)

	agents, err := loadAllEffectiveAgents()
	if err != nil {
		t.Fatalf("loadAllEffectiveAgents failed: %v", err)
	}
	found := false
	for _, a := range agents {
		if a.Name == "coder" && a.Model == "user-model" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected merged coder agent in agents, got %v", agents)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Branch-coverage tests for the remaining uncovered lines.
// ─────────────────────────────────────────────────────────────────────────────

func TestPocExtractRequirements_CodeBlockUnique(t *testing.T) {
	old := pocExtractRequirementsCodeBlock
	defer func() { pocExtractRequirementsCodeBlock = old }()
	pocExtractRequirementsCodeBlock = func(content string) []requirement {
		return []requirement{{Name: "UniqueCodeBlockReq", Type: "symbol", Description: "from block"}}
	}
	reqs := extractRequirements("Some prose.\n```go\nanything\n```\n")
	if len(reqs) != 1 || reqs[0].Name != "UniqueCodeBlockReq" {
		t.Errorf("expected unique code-block requirement, got %v", reqs)
	}
}

func TestSckgBuildGraph_SkipDir(t *testing.T) {
	oldWalk := sckgWalk
	defer func() { sckgWalk = oldWalk }()
	root := t.TempDir()
	sckgWalk = func(r string, walkFn filepath.WalkFunc) error {
		_ = walkFn(filepath.Join(r, "node_modules"), &fakeDirInfo{name: "node_modules", mode: os.ModeDir}, nil)
		return walkFn(filepath.Join(r, "main.go"), &fakeDirInfo{name: "main.go"}, nil)
	}
	g, err := buildGraph(root)
	if err != nil {
		t.Fatalf("buildGraph failed: %v", err)
	}
	if len(g.Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(g.Nodes))
	}
}

func TestAdwScanDebt_SkipDir(t *testing.T) {
	oldWalk := adwWalk
	defer func() { adwWalk = oldWalk }()
	root := t.TempDir()
	adwWalk = func(r string, walkFn filepath.WalkFunc) error {
		_ = walkFn(filepath.Join(r, ".git"), &fakeDirInfo{name: ".git", mode: os.ModeDir}, nil)
		return walkFn(filepath.Join(r, "main.go"), &fakeDirInfo{name: "main.go"}, nil)
	}
	res := scanDebt(root, false)
	if res.Summary.FilesScanned != 1 {
		t.Errorf("expected 1 file scanned, got %d", res.Summary.FilesScanned)
	}
}

func TestAgentDoctorRunDoctor_InvalidBaseURL(t *testing.T) {
	cfg := orchestrator.AgentConfig{
		Name:     "x",
		Provider: "openai",
		BaseURL:  "http://invalid url with spaces",
		Model:    "gpt-4o",
	}
	rep := runDoctor([]orchestrator.AgentConfig{cfg}, false)
	if len(rep) != 1 {
		t.Fatalf("expected 1 report, got %d", len(rep))
	}
	found := false
	for _, issue := range rep[0].Issues {
		if strings.Contains(issue, "could not fetch /v1/models") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected fetch models issue, got %v", rep[0].Issues)
	}
}

func TestAgentResetCmd_RemoveAllError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod test is Unix-specific")
	}
	withIsolatedAgentConfig(t)
	dir, _ := agentDir("reset-readonly-agent")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "agent.toml"), []byte("name = \"reset-readonly-agent\"\n"), 0o644)
	os.Chmod(dir, 0o555)
	defer os.Chmod(dir, 0o755)
	_, err := runCmd(t, OrchestratorAgentResetCmd, []string{"reset-readonly-agent"})
	if err == nil {
		t.Fatal("expected error removing read-only agent dir")
	}
}

func TestAgentEditApplyAgentEdits_NameEmpty(t *testing.T) {
	withIsolatedAgentConfig(t)
	dir, _ := agentDir("empty-name-agent")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "agent.toml"), []byte("name = \"\"\ndescription = \"d\"\n"), 0o644)
	if err := applyAgentEdits("empty-name-agent", []string{"model=m1"}); err != nil {
		t.Fatalf("applyAgentEdits failed: %v", err)
	}
	var cfg orchestrator.AgentConfig
	if _, err := toml.DecodeFile(filepath.Join(dir, "agent.toml"), &cfg); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if cfg.Name != "empty-name-agent" {
		t.Errorf("expected name empty-name-agent, got %q", cfg.Name)
	}
}
