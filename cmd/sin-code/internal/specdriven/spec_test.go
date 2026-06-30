// SPDX-License-Identifier: MIT
// Purpose: tests for the specdriven package (issue #480).
// Covers all four EARS formats, edge cases, and the architecture/code
// generation pipeline.
package specdriven

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseEARS_When(t *testing.T) {
	reqs, err := ParseEARS("When user clicks save, the system shall persist the form")
	if err != nil {
		t.Fatalf("ParseEARS: %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("expected 1 requirement, got %d", len(reqs))
	}
	r := reqs[0]
	if r.Type != "when" {
		t.Errorf("type: got %q want %q", r.Type, "when")
	}
	if r.Condition != "user clicks save" {
		t.Errorf("condition: got %q want %q", r.Condition, "user clicks save")
	}
	if r.Subject != "system" {
		t.Errorf("subject: got %q want %q", r.Subject, "system")
	}
	if r.Response != "persist the form" {
		t.Errorf("response: got %q want %q", r.Response, "persist the form")
	}
}

func TestParseEARS_While(t *testing.T) {
	reqs, err := ParseEARS("While loading, the system shall show a spinner")
	if err != nil {
		t.Fatalf("ParseEARS: %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("expected 1 requirement, got %d", len(reqs))
	}
	r := reqs[0]
	if r.Type != "while" {
		t.Errorf("type: got %q want %q", r.Type, "while")
	}
	if r.Condition != "loading" {
		t.Errorf("condition: got %q want %q", r.Condition, "loading")
	}
	if r.Subject != "system" {
		t.Errorf("subject: got %q want %q", r.Subject, "system")
	}
	if r.Response != "show a spinner" {
		t.Errorf("response: got %q want %q", r.Response, "show a spinner")
	}
}

func TestParseEARS_If(t *testing.T) {
	reqs, err := ParseEARS("If auth fails, then the system shall redirect to login")
	if err != nil {
		t.Fatalf("ParseEARS: %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("expected 1 requirement, got %d", len(reqs))
	}
	r := reqs[0]
	if r.Type != "if" {
		t.Errorf("type: got %q want %q", r.Type, "if")
	}
	if r.Condition != "auth fails" {
		t.Errorf("condition: got %q want %q", r.Condition, "auth fails")
	}
	if r.Subject != "system" {
		t.Errorf("subject: got %q want %q", r.Subject, "system")
	}
	if r.Response != "redirect to login" {
		t.Errorf("response: got %q want %q", r.Response, "redirect to login")
	}
}

func TestParseEARS_The(t *testing.T) {
	reqs, err := ParseEARS("The system shall log all errors")
	if err != nil {
		t.Fatalf("ParseEARS: %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("expected 1 requirement, got %d", len(reqs))
	}
	r := reqs[0]
	if r.Type != "the" {
		t.Errorf("type: got %q want %q", r.Type, "the")
	}
	if r.Condition != "always" {
		t.Errorf("condition: got %q want %q", r.Condition, "always")
	}
	if r.Subject != "system" {
		t.Errorf("subject: got %q want %q", r.Subject, "system")
	}
	if r.Response != "log all errors" {
		t.Errorf("response: got %q want %q", r.Response, "log all errors")
	}
}

func TestParseEARS_Multiple(t *testing.T) {
	text := `When user clicks save, the system shall persist the form
While loading, the system shall show a spinner
If auth fails, then the system shall redirect to login
The system shall log all errors
`
	reqs, err := ParseEARS(text)
	if err != nil {
		t.Fatalf("ParseEARS: %v", err)
	}
	if len(reqs) != 4 {
		t.Fatalf("expected 4 requirements, got %d", len(reqs))
	}
	wantTypes := []string{"when", "while", "if", "the"}
	for i, w := range wantTypes {
		if reqs[i].Type != w {
			t.Errorf("req[%d].type: got %q want %q", i, reqs[i].Type, w)
		}
	}
}

func TestParseEARS_Empty(t *testing.T) {
	reqs, err := ParseEARS("")
	if err != nil {
		t.Fatalf("ParseEARS: %v", err)
	}
	if len(reqs) != 0 {
		t.Fatalf("expected 0 requirements, got %d", len(reqs))
	}
}

func TestParseEARS_Comments(t *testing.T) {
	text := `# This is a comment
# Another comment
When user clicks save, the system shall persist the form
# Trailing comment
`
	reqs, err := ParseEARS(text)
	if err != nil {
		t.Fatalf("ParseEARS: %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("expected 1 requirement (comments skipped), got %d", len(reqs))
	}
	if reqs[0].Type != "when" {
		t.Errorf("type: got %q want %q", reqs[0].Type, "when")
	}
}

func TestParseEARS_Invalid(t *testing.T) {
	// Missing "shall" after "the" — should error.
	_, err := ParseEARS("The system must do something")
	if err == nil {
		t.Fatal("expected error for missing 'shall', got nil")
	}
}

func TestParseEARS_InvalidWhen(t *testing.T) {
	// "When" without ", the ... shall ..." — should error.
	_, err := ParseEARS("When something happens the system does things")
	if err == nil {
		t.Fatal("expected error for malformed 'when', got nil")
	}
}

func TestParseEARS_NonEARSLine(t *testing.T) {
	// Prose lines that don't start with an EARS keyword should be
	// silently ignored, not error.
	text := `This is just some prose.
When ready, the system shall start processing
More prose here.
`
	reqs, err := ParseEARS(text)
	if err != nil {
		t.Fatalf("ParseEARS: %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("expected 1 requirement (prose ignored), got %d", len(reqs))
	}
}

func TestParseShall(t *testing.T) {
	subject, response := parseShall("system shall persist the form")
	if subject != "system" {
		t.Errorf("subject: got %q want %q", subject, "system")
	}
	if response != "persist the form" {
		t.Errorf("response: got %q want %q", response, "persist the form")
	}
}

func TestParseShall_Missing(t *testing.T) {
	subject, response := parseShall("no keyword here")
	if subject != "" {
		t.Errorf("subject: got %q want empty", subject)
	}
	if response != "" {
		t.Errorf("response: got %q want empty", response)
	}
}

func TestGenerateArchitecture(t *testing.T) {
	spec := &Spec{
		Title: "Test",
		Requirements: []Requirement{
			{ID: "REQ-001", Type: "when", Subject: "system", Response: "persist the form", Condition: "user clicks save"},
			{ID: "REQ-002", Type: "the", Subject: "system", Response: "log all errors", Condition: "always"},
			{ID: "REQ-003", Type: "if", Subject: "auth service", Response: "redirect to login", Condition: "auth fails"},
		},
	}
	arch := GenerateArchitecture(spec)
	if len(arch.Components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(arch.Components))
	}
	// Sorted alphabetically: "auth service" < "system"
	if arch.Components[0].Name != "AuthService" {
		t.Errorf("component[0].name: got %q want %q", arch.Components[0].Name, "AuthService")
	}
	if arch.Components[1].Name != "System" {
		t.Errorf("component[1].name: got %q want %q", arch.Components[1].Name, "System")
	}
	// System has 2 responsibilities.
	if len(arch.Components[1].Responsibilities) != 2 {
		t.Fatalf("expected 2 responsibilities for System, got %d", len(arch.Components[1].Responsibilities))
	}
}

func TestGenerateCode(t *testing.T) {
	arch := &Architecture{
		Interfaces: []Interface{
			{
				Name: "Systemer",
				Methods: []Method{
					{Name: "PersistTheForm", Returns: "error"},
					{Name: "LogAllErrors", Returns: "error"},
				},
			},
		},
	}
	src, err := GenerateCode(arch, "myapp")
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	if !contains(src, "package myapp") {
		t.Error("missing package declaration")
	}
	if !contains(src, "type Systemer interface") {
		t.Error("missing interface declaration")
	}
	if !contains(src, "PersistTheForm() error") {
		t.Error("missing method declaration")
	}
}

func TestWriteCode(t *testing.T) {
	dir := t.TempDir()
	arch := &Architecture{
		Interfaces: []Interface{
			{
				Name: "Fooer",
				Methods: []Method{
					{Name: "DoBar", Returns: "error"},
				},
			},
		},
	}
	path, err := WriteCode(arch, "testpkg", dir)
	if err != nil {
		t.Fatalf("WriteCode: %v", err)
	}
	if filepath.Base(path) != "spec_generated.go" {
		t.Errorf("filename: got %q want %q", filepath.Base(path), "spec_generated.go")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated file: %v", err)
	}
	if !contains(string(data), "package testpkg") {
		t.Error("generated file missing package declaration")
	}
}

func TestLoadSpec(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.ears")
	content := `# Title: My Feature
# Description: A test feature
When user clicks save, the system shall persist the form
The system shall log all errors
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	spec, err := LoadSpec(path)
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	if spec.Title != "My Feature" {
		t.Errorf("title: got %q want %q", spec.Title, "My Feature")
	}
	if spec.Description != "A test feature" {
		t.Errorf("description: got %q want %q", spec.Description, "A test feature")
	}
	if len(spec.Requirements) != 2 {
		t.Fatalf("expected 2 requirements, got %d", len(spec.Requirements))
	}
}

func TestComponentName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"the system", "System"},
		{"the auth service", "AuthService"},
		{"system", "System"},
		{"the user repository", "UserRepository"},
		{"", "Component"},
	}
	for _, tt := range tests {
		got := componentName(tt.input)
		if got != tt.want {
			t.Errorf("componentName(%q): got %q want %q", tt.input, got, tt.want)
		}
	}
}

func TestMethodName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"persist the form", "PersistTheForm"},
		{"log all errors", "LogAllErrors"},
		{"do something", "DoSomething"},
		{"", "Do"},
	}
	for _, tt := range tests {
		got := methodName(tt.input)
		if got != tt.want {
			t.Errorf("methodName(%q): got %q want %q", tt.input, got, tt.want)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
