// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the poc (Proof-of-Correctness) subcommand.
package internal

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractRequirements_Empty(t *testing.T) {
	reqs := extractRequirements("")
	if len(reqs) != 0 {
		t.Errorf("expected 0 requirements for empty content, got %d", len(reqs))
	}
}

func TestExtractRequirements_FunctionKeyword(t *testing.T) {
	content := "The module must implement processOrder to handle orders."
	reqs := extractRequirements(content)
	if len(reqs) == 0 {
		t.Fatal("expected at least 1 requirement")
	}
	found := false
	for _, r := range reqs {
		if r.Name == "processOrder" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'processOrder' requirement, got %v", reqs)
	}
}

func TestExtractRequirements_ClassKeyword(t *testing.T) {
	content := "requires UserManager for auth"
	reqs := extractRequirements(content)
	if len(reqs) == 0 {
		t.Fatal("expected at least 1 requirement")
	}
	found := false
	for _, r := range reqs {
		if r.Name == "UserManager" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'UserManager' requirement, got %v", reqs)
	}
}

func TestExtractRequirements_MethodKeyword(t *testing.T) {
	content := "method authenticate should validate tokens"
	reqs := extractRequirements(content)
	if len(reqs) == 0 {
		t.Fatal("expected at least 1 requirement")
	}
	found := false
	for _, r := range reqs {
		if r.Name == "authenticate" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'authenticate' requirement, got %v", reqs)
	}
}

func TestExtractRequirements_InterfaceKeyword(t *testing.T) {
	content := "interface Handler for request routing"
	reqs := extractRequirements(content)
	if len(reqs) == 0 {
		t.Fatal("expected at least 1 requirement")
	}
	found := false
	for _, r := range reqs {
		if r.Name == "Handler" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'Handler' requirement, got %v", reqs)
	}
}

func TestExtractRequirements_Deduplicates(t *testing.T) {
	content := "must implement processOrder\nrequires processOrder"
	reqs := extractRequirements(content)
	count := 0
	for _, r := range reqs {
		if r.Name == "processOrder" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 requirement for duplicate name, got %d", count)
	}
}

func TestExtractRequirements_CodeBlock(t *testing.T) {
	content := "```python\nrequires hello\n    pass\n```\n"
	reqs := extractRequirements(content)
	if len(reqs) == 0 {
		t.Fatal("expected at least 1 requirement from code block")
	}
	found := false
	for _, r := range reqs {
		if r.Name == "hello" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'hello' from code block, got %v", reqs)
	}
}

func TestVerifyCorrectness_SingleFile(t *testing.T) {
	dir := t.TempDir()
	codeFile := filepath.Join(dir, "main.go")
	os.WriteFile(codeFile, []byte("package main\nfunc Add(a, b int) int { return a + b }\nfunc Subtract(a, b int) int { return a - b }\n"), 0644)

	result, err := verifyCorrectness("", codeFile)
	if err != nil {
		t.Fatalf("verifyCorrectness failed: %v", err)
	}
	if result.Code != codeFile {
		t.Errorf("expected code=%s, got %s", codeFile, result.Code)
	}
}

func TestVerifyCorrectness_SpecWithRequirements(t *testing.T) {
	dir := t.TempDir()
	specFile := filepath.Join(dir, "spec.md")
	codeFile := filepath.Join(dir, "main.go")

	os.WriteFile(specFile, []byte("# Spec\nThe system must implement Add and requires Subtract.\n"), 0644)
	os.WriteFile(codeFile, []byte("package main\nfunc Add(a, b int) int { return a + b }\nfunc Subtract(a, b int) int { return a - b }\n"), 0644)

	result, err := verifyCorrectness(specFile, codeFile)
	if err != nil {
		t.Fatalf("verifyCorrectness failed: %v", err)
	}
	if result.Passed == 0 {
		t.Error("expected at least some passed checks")
	}
	if result.Coverage == 0 {
		t.Error("expected non-zero coverage when requirements match")
	}
}

func TestVerifyCorrectness_MissingRequiredSymbol(t *testing.T) {
	dir := t.TempDir()
	specFile := filepath.Join(dir, "spec.md")
	codeFile := filepath.Join(dir, "main.go")

	os.WriteFile(specFile, []byte("must implement NonExistentFunction\n"), 0644)
	os.WriteFile(codeFile, []byte("package main\nfunc OtherFunc() {}\n"), 0644)

	result, err := verifyCorrectness(specFile, codeFile)
	if err != nil {
		t.Fatalf("verifyCorrectness failed: %v", err)
	}
	if result.Failed == 0 {
		t.Error("expected at least 1 failed check for missing required symbol")
	}
}

func TestVerifyCorrectness_Directory(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\nfunc FuncA() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("package main\nfunc FuncB() {}\n"), 0644)

	result, err := verifyCorrectness("", dir)
	if err != nil {
		t.Fatalf("verifyCorrectness failed: %v", err)
	}
	if result.Code != dir {
		t.Errorf("expected code=%s, got %s", dir, result.Code)
	}
}

func TestVerifyCorrectness_MissingCodePath(t *testing.T) {
	_, err := verifyCorrectness("", "/nonexistent/path")
	if err == nil {
		t.Error("expected error for missing code path")
	}
}

func TestVerifyCorrectness_MissingSpecPath(t *testing.T) {
	dir := t.TempDir()
	codeFile := filepath.Join(dir, "main.go")
	os.WriteFile(codeFile, []byte("package main\n"), 0644)

	_, err := verifyCorrectness("/nonexistent/spec.md", codeFile)
	if err == nil {
		t.Error("expected error for missing spec path")
	}
}

func TestVerifyCorrectness_OsExitWarning(t *testing.T) {
	dir := t.TempDir()
	codeFile := filepath.Join(dir, "lib.go")
	os.WriteFile(codeFile, []byte("package lib\nfunc Cleanup() { os.Exit(1) }\n"), 0644)

	result, err := verifyCorrectness("", codeFile)
	if err != nil {
		t.Fatalf("verifyCorrectness failed: %v", err)
	}
	found := false
	for _, check := range result.Checks {
		if check.Name == "os.Exit" && check.Type == "forbidden" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected os.Exit forbidden warning in library code, checks: %v", result.Checks)
	}
}

func TestVerifyCorrectness_OsExitAllowedInMain(t *testing.T) {
	dir := t.TempDir()
	codeFile := filepath.Join(dir, "main.go")
	os.WriteFile(codeFile, []byte("package main\nfunc main() { os.Exit(0) }\n"), 0644)

	result, err := verifyCorrectness("", codeFile)
	if err != nil {
		t.Fatalf("verifyCorrectness failed: %v", err)
	}
	for _, check := range result.Checks {
		if check.Name == "os.Exit" {
			t.Errorf("did not expect os.Exit warning in main file, got check: %v", check)
		}
	}
}

func TestVerifyCorrectness_TodoWarning(t *testing.T) {
	dir := t.TempDir()
	codeFile := filepath.Join(dir, "app.py")
	os.WriteFile(codeFile, []byte("def hello():\n    # TODO: implement properly\n    pass\n"), 0644)

	result, err := verifyCorrectness("", codeFile)
	if err != nil {
		t.Fatalf("verifyCorrectness failed: %v", err)
	}
	found := false
	for _, check := range result.Checks {
		if check.Name == "TODO" && check.Type == "forbidden" && check.Status == "warn" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected TODO forbidden warning, checks: %v", result.Checks)
	}
}

func TestVerifyCorrectness_FixmeWarning(t *testing.T) {
	dir := t.TempDir()
	codeFile := filepath.Join(dir, "app.go")
	os.WriteFile(codeFile, []byte("package main\n// FIXME: broken logic\nfunc hello() {}\n"), 0644)

	result, err := verifyCorrectness("", codeFile)
	if err != nil {
		t.Fatalf("verifyCorrectness failed: %v", err)
	}
	found := false
	for _, check := range result.Checks {
		if check.Name == "FIXME" && check.Type == "forbidden" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected FIXME forbidden warning, checks: %v", result.Checks)
	}
}

func TestVerifyCorrectness_NoTodoInTestFile(t *testing.T) {
	dir := t.TempDir()
	codeFile := filepath.Join(dir, "app_test.go")
	os.WriteFile(codeFile, []byte("package main\n// TODO: add more tests\nimport \"testing\"\nfunc TestApp(t *testing.T) {}\n"), 0644)

	result, err := verifyCorrectness("", codeFile)
	if err != nil {
		t.Fatalf("verifyCorrectness failed: %v", err)
	}
	for _, check := range result.Checks {
		if check.Name == "TODO" && check.Type == "forbidden" {
			t.Errorf("did not expect TODO warning in test file, got check: %v", check)
		}
	}
}

func TestVerifyCorrectness_CoverageCalculation(t *testing.T) {
	dir := t.TempDir()
	specFile := filepath.Join(dir, "spec.md")
	codeFile := filepath.Join(dir, "main.go")

	os.WriteFile(specFile, []byte("must implement Add\nmust implement Subtract\nmust implement Multiply\n"), 0644)
	os.WriteFile(codeFile, []byte("package main\nfunc Add(a, b int) int { return a + b }\nfunc Subtract(a, b int) int { return a - b }\n"), 0644)

	result, err := verifyCorrectness(specFile, codeFile)
	if err != nil {
		t.Fatalf("verifyCorrectness failed: %v", err)
	}
	if result.Coverage < 66.0 || result.Coverage > 67.0 {
		t.Errorf("expected ~66.7%% coverage (2/3), got %.1f%%", result.Coverage)
	}
}

func TestOutputTextPOC(t *testing.T) {
	result := &pocResult{
		Spec:  "spec.md",
		Code:  "main.go",
		Passed: 2,
		Failed: 1,
		TotalChecks: 3,
		Coverage: 66.7,
		Checks: []pocCheck{
			{Name: "Add", Type: "required", Status: "pass", Message: "Found function 'Add'", File: "main.go", Line: 1},
			{Name: "Subtract", Type: "required", Status: "pass", Message: "Found function 'Subtract'", File: "main.go", Line: 2},
			{Name: "Multiply", Type: "required", Status: "fail", Message: "Required 'Multiply' not found"},
		},
		Summary: "Coverage: 66.7% (2/3 requirements, 3 checks, 2 passed, 1 failed, 0 warnings)",
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	if err := outputTextPOC(result); err != nil {
		t.Fatalf("outputTextPOC failed: %v", err)
	}
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if !strings.Contains(out, "Proof-of-Correctness") {
		t.Errorf("expected header in output, got %q", out)
	}
	if !strings.Contains(out, "66.7%") {
		t.Errorf("expected coverage in output, got %q", out)
	}
	if !strings.Contains(out, "Add") {
		t.Errorf("expected 'Add' in output, got %q", out)
	}
	if !strings.Contains(out, "Multiply") {
		t.Errorf("expected 'Multiply' in output, got %q", out)
	}
}

func TestOutputTextPOC_EmptyChecks(t *testing.T) {
	result := &pocResult{
		Spec:        "",
		Code:        "main.go",
		Passed:      0,
		Failed:      0,
		TotalChecks: 0,
		Coverage:    0,
		Checks:      []pocCheck{},
		Summary:     "Coverage: 0.0% (0/0 requirements)",
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	if err := outputTextPOC(result); err != nil {
		t.Fatalf("outputTextPOC failed: %v", err)
	}
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if !strings.Contains(out, "Proof-of-Correctness") {
		t.Errorf("expected header in output, got %q", out)
	}
}

func TestPocCmd_RequiresCode(t *testing.T) {
	pocSpec = ""
	pocCode = ""
	pocFormat = "text"
	err := PocCmd.RunE(PocCmd, []string{})
	if err == nil {
		t.Error("expected error when --spec and --code are missing")
	}
}

func TestPocCmd_UsesSpecAsFallback(t *testing.T) {
	dir := t.TempDir()
	specFile := filepath.Join(dir, "spec.md")
	os.WriteFile(specFile, []byte("# Spec\n"), 0644)

	pocSpec = specFile
	pocCode = ""
	pocFormat = "text"

	err := PocCmd.RunE(PocCmd, []string{})
	if err != nil {
		t.Errorf("expected no error when --spec provided as fallback, got %v", err)
	}
}

func TestPocCmd_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	codeFile := filepath.Join(dir, "main.go")
	os.WriteFile(codeFile, []byte("package main\nfunc Hello() {}\n"), 0644)

	pocSpec = ""
	pocCode = codeFile
	pocFormat = "json"

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := PocCmd.RunE(PocCmd, []string{})
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("PocCmd.RunE failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	var result pocResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON output, got parse error: %v", err)
	}
	if result.Code != codeFile {
		t.Errorf("expected code=%s, got %s", codeFile, result.Code)
	}
}
