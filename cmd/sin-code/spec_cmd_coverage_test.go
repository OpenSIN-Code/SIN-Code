// SPDX-License-Identifier: MIT
// Purpose: coverage tests for spec_cmd.go — exercises validate/show using
// package-level hooks so tests never need real *.spec.md files.
// Docs: spec_cmd.go
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/spec"
)

type specErrWriter struct{ err error }

func (e specErrWriter) Write(p []byte) (int, error) { return 0, e.err }

func saveSpecHooks(t *testing.T) {
	t.Helper()
	origLoad := specLoadHook
	origValidate := specValidateHook
	t.Cleanup(func() {
		specLoadHook = origLoad
		specValidateHook = origValidate
	})
}

func runSpecCmd(t *testing.T, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	cmd := NewSpecCmd()
	var out bytes.Buffer
	setOutAll(cmd, &out)
	cmd.SetArgs(args)
	return &out, cmd.Execute()
}

func TestSpecCmd_NewSpecCmd(t *testing.T) {
	cmd := NewSpecCmd()
	if cmd.Use != "spec" {
		t.Errorf("Use = %q, want spec", cmd.Use)
	}
	names := []string{}
	for _, c := range cmd.Commands() {
		names = append(names, c.Name())
	}
	joined := strings.Join(names, " ")
	for _, want := range []string{"validate", "show"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing subcommand %q in %q", want, joined)
		}
	}
}

func TestSpecCmd_Validate_LoadError(t *testing.T) {
	saveSpecHooks(t)
	specLoadHook = func(string) (*spec.Spec, error) { return nil, errors.New("load boom") }
	_, err := runSpecCmd(t, "validate", "feature.spec.md")
	if err == nil || !strings.Contains(err.Error(), "load boom") {
		t.Fatalf("expected load error, got %v", err)
	}
}

func TestSpecCmd_Validate_Fails(t *testing.T) {
	saveSpecHooks(t)
	specLoadHook = func(string) (*spec.Spec, error) {
		return &spec.Spec{Title: "Empty"}, nil
	}
	out, err := runSpecCmd(t, "validate", "feature.spec.md")
	if err == nil || !strings.Contains(err.Error(), "error(s)") {
		t.Fatalf("expected validation error, got %v", err)
	}
	if !strings.Contains(out.String(), "[error]") {
		t.Errorf("expected issue output, got %q", out.String())
	}
}

func TestSpecCmd_Validate_QuietFails(t *testing.T) {
	saveSpecHooks(t)
	specLoadHook = func(string) (*spec.Spec, error) {
		return &spec.Spec{Title: "Empty"}, nil
	}
	cmd := NewSpecCmd()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	var out bytes.Buffer
	setOutAll(cmd, &out)
	cmd.SetArgs([]string{"validate", "-q", "feature.spec.md"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "error(s)") {
		t.Fatalf("expected validation error, got %v", err)
	}
	if out.String() != "" {
		t.Errorf("expected quiet output, got %q", out.String())
	}
}

func TestSpecCmd_Validate_OK(t *testing.T) {
	saveSpecHooks(t)
	specLoadHook = func(string) (*spec.Spec, error) {
		return &spec.Spec{
			Title:        "Feature",
			Objective:    "test coverage",
			Requirements: []spec.Requirement{{ID: "R1", Text: "do it", Priority: spec.Must}},
			Criteria:     []spec.Criterion{{ID: "A1", Text: "check it", Verify: "true"}},
		}, nil
	}
	out, err := runSpecCmd(t, "validate", "feature.spec.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "OK") {
		t.Errorf("expected OK output, got %q", out.String())
	}
}

func TestSpecCmd_Validate_QuietOK(t *testing.T) {
	saveSpecHooks(t)
	specLoadHook = func(string) (*spec.Spec, error) {
		return &spec.Spec{
			Title:        "Feature",
			Objective:    "test coverage",
			Requirements: []spec.Requirement{{ID: "R1", Text: "do it", Priority: spec.Must}},
			Criteria:     []spec.Criterion{{ID: "A1", Text: "check it", Verify: "true"}},
		}, nil
	}
	cmd := NewSpecCmd()
	var out bytes.Buffer
	setOutAll(cmd, &out)
	cmd.SetArgs([]string{"validate", "-q", "feature.spec.md"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if out.String() != "" {
		t.Errorf("expected quiet output, got %q", out.String())
	}
}

func TestSpecCmd_Show_LoadError(t *testing.T) {
	saveSpecHooks(t)
	specLoadHook = func(string) (*spec.Spec, error) { return nil, errors.New("load boom") }
	_, err := runSpecCmd(t, "show", "feature.spec.md")
	if err == nil || !strings.Contains(err.Error(), "load boom") {
		t.Fatalf("expected load error, got %v", err)
	}
}

func TestSpecCmd_Show_JSON(t *testing.T) {
	saveSpecHooks(t)
	specLoadHook = func(string) (*spec.Spec, error) {
		return &spec.Spec{
			Title:        "Feature",
			Objective:    "test coverage\nmore",
			Requirements: []spec.Requirement{{ID: "R1", Text: "do it", Priority: spec.Must}},
			Criteria: []spec.Criterion{
				{ID: "A1", Text: "check it", Verify: "true"},
				{ID: "A2", Text: "check again"},
			},
			Invariants: []string{"do not break API"},
		}, nil
	}
	out, err := runSpecCmd(t, "show", "--json", "feature.spec.md")
	if err != nil {
		t.Fatal(err)
	}
	var got spec.Spec
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json decode: %v: %q", err, out.String())
	}
	if got.Title != "Feature" {
		t.Errorf("title = %q, want Feature", got.Title)
	}
}

func TestSpecCmd_Show_JSONEncodeError(t *testing.T) {
	saveSpecHooks(t)
	specLoadHook = func(string) (*spec.Spec, error) { return &spec.Spec{}, nil }
	cmd := NewSpecCmd()
	cmd.SetArgs([]string{"show", "--json", "feature.spec.md"})
	setOutAll(cmd, specErrWriter{err: errors.New("encode boom")})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "encode boom") {
		t.Fatalf("expected encode error, got %v", err)
	}
}

func TestSpecCmd_Show_Text(t *testing.T) {
	saveSpecHooks(t)
	specLoadHook = func(string) (*spec.Spec, error) {
		return &spec.Spec{
			Title:     "Feature",
			Objective: "test coverage\nmore",
			Requirements: []spec.Requirement{
				{ID: "R1", Text: "do it", Priority: spec.Should},
			},
			Criteria: []spec.Criterion{
				{ID: "A1", Text: "check it", Verify: "true"},
				{ID: "A2", Text: "check again"},
			},
			Invariants: []string{"do not break API"},
		}, nil
	}
	out, err := runSpecCmd(t, "show", "feature.spec.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Feature") {
		t.Errorf("expected title, got %q", out.String())
	}
	if !strings.Contains(out.String(), "test coverage") {
		t.Errorf("expected objective, got %q", out.String())
	}
	if !strings.Contains(out.String(), "R1 [should] do it") {
		t.Errorf("expected requirement, got %q", out.String())
	}
	if !strings.Contains(out.String(), "A1 check it  (verify: true)") {
		t.Errorf("expected criterion with verify, got %q", out.String())
	}
	if !strings.Contains(out.String(), "A2 check again") {
		t.Errorf("expected criterion without verify, got %q", out.String())
	}
	if !strings.Contains(out.String(), "invariants:   1") {
		t.Errorf("expected invariants count, got %q", out.String())
	}
}

func TestSpecCmd_Show_UntitledAndNoObjective(t *testing.T) {
	saveSpecHooks(t)
	specLoadHook = func(string) (*spec.Spec, error) {
		return &spec.Spec{
			Requirements: []spec.Requirement{{ID: "R1", Text: "do it", Priority: spec.May}},
			Criteria:     []spec.Criterion{{ID: "A1", Text: "check it"}},
		}, nil
	}
	out, err := runSpecCmd(t, "show", "feature.spec.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "(untitled spec)") {
		t.Errorf("expected untitled, got %q", out.String())
	}
	if !strings.Contains(out.String(), "(none)") {
		t.Errorf("expected no objective, got %q", out.String())
	}
}

func TestSpecCmd_FirstLine(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"a\nb", "a"},
		{"", "(none)"},
		{"single", "single"},
	}
	for _, c := range cases {
		if got := firstLine(c.in); got != c.want {
			t.Errorf("firstLine(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
