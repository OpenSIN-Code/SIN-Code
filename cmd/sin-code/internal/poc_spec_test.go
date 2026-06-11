// SPDX-License-Identifier: MIT
// Purpose: Regression tests for dogfooding bug st-bug1 #3 — POC must not
// treat natural-language spec prose ("must", "Spec", "the") as required
// function names, and must recognize `Name()` call references.
package internal

import (
	"os"
	"path/filepath"
	"testing"
)

// The exact spec from the bug report: previously produced bogus required
// symbols "Spec" and "must".
const bugReportSpec = "# Hello Function Spec\nThe Hello() function must return the string \"hello\".\n"

func TestExtractRequirements_NoStopwordFalsePositives(t *testing.T) {
	reqs := extractRequirements(bugReportSpec)
	for _, r := range reqs {
		switch r.Name {
		case "must", "Spec", "the", "The", "return", "string", "function":
			t.Errorf("stopword %q extracted as requirement: %+v", r.Name, reqs)
		}
	}
}

func TestExtractRequirements_ParenCallReference(t *testing.T) {
	reqs := extractRequirements(bugReportSpec)
	found := false
	for _, r := range reqs {
		if r.Name == "Hello" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Hello' extracted from 'Hello()' reference, got %v", reqs)
	}
}

func TestExtractRequirements_StructuredReqLines(t *testing.T) {
	spec := "## Requirements\n- REQ-1: function hello() returns \"hello\"\n- REQ-2: must implement cleanup\n"
	reqs := extractRequirements(spec)
	want := map[string]bool{"hello": false, "cleanup": false}
	for _, r := range reqs {
		if _, ok := want[r.Name]; ok {
			want[r.Name] = true
		}
	}
	for name, ok := range want {
		if !ok {
			t.Errorf("expected requirement %q, got %v", name, reqs)
		}
	}
}

func TestExtractRequirements_ChainedKindKeyword(t *testing.T) {
	// "should define type Config" must extract "Config", not "type".
	reqs := extractRequirements("should define type Config for settings")
	foundConfig := false
	for _, r := range reqs {
		if r.Name == "type" {
			t.Errorf("'type' must not be extracted as a requirement: %v", reqs)
		}
		if r.Name == "Config" {
			foundConfig = true
		}
	}
	if !foundConfig {
		t.Errorf("expected 'Config' requirement, got %v", reqs)
	}
}

func TestVerifyCorrectness_BugReportSpecPasses(t *testing.T) {
	dir := t.TempDir()
	specFile := filepath.Join(dir, "spec.md")
	codeFile := filepath.Join(dir, "code.go")
	os.WriteFile(specFile, []byte(bugReportSpec), 0644)
	os.WriteFile(codeFile, []byte("package main\n\nfunc Hello() string {\n\treturn \"hello\"\n}\n\nfunc main() {\n\tprintln(Hello())\n}\n"), 0644)

	result, err := verifyCorrectness(specFile, codeFile)
	if err != nil {
		t.Fatalf("verifyCorrectness failed: %v", err)
	}
	if result.Failed != 0 {
		t.Errorf("expected 0 failed checks for satisfied natural-language spec, got %d: %+v", result.Failed, result.Checks)
	}
	if result.Coverage != 100.0 {
		t.Errorf("expected 100%% coverage, got %.1f%% (checks: %+v)", result.Coverage, result.Checks)
	}
}
