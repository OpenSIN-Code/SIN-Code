// SPDX-License-Identifier: MIT
// Purpose: tests for LLM-driven case filling. All tests use the parse
// path; the network chat path is covered by llm/llm_test.go.
package testgen

import (
	"strings"
	"testing"
)

func TestParseCaseFillResponse_FencedJSON(t *testing.T) {
	in := "```json\n[{\"name\":\"a\",\"args\":{\"a\":1},\"wants\":{\"got\":2}}]\n```"
	cases, err := parseCaseFillResponse(in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cases) != 1 || cases[0].Name != "a" {
		t.Fatalf("unexpected cases: %+v", cases)
	}
}

func TestParseCaseFillResponse_BareJSON(t *testing.T) {
	in := `[{"name":"sum","args":{"a":2,"b":3},"wants":{"got":5,"err":false}}]`
	cases, err := parseCaseFillResponse(in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cases) != 1 || cases[0].Name != "sum" {
		t.Fatalf("unexpected cases: %+v", cases)
	}
}

func TestParseCaseFillResponse_ProseAroundJSON(t *testing.T) {
	in := "Here you go:\n```json\n[{\"name\":\"x\"}]\n```\nThanks!"
	cases, err := parseCaseFillResponse(in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cases) != 1 || cases[0].Name != "x" {
		t.Fatalf("unexpected cases: %+v", cases)
	}
}

func TestParseCaseFillResponse_Invalid(t *testing.T) {
	if _, err := parseCaseFillResponse("hello world"); err == nil {
		t.Fatal("expected error for no JSON array")
	}
	if _, err := parseCaseFillResponse("[]"); err == nil {
		t.Fatal("expected error for empty array")
	}
	if _, err := parseCaseFillResponse("not [json]"); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestBuildCaseFillPrompt_IncludesSignature(t *testing.T) {
	prompt := buildCaseFillPrompt(FuncInfo{
		Name: "Add", Args: []Param{{Name: "a", Type: "int"}, {Name: "b", Type: "int"}},
		Returns: []Param{{Name: "got", Type: "int"}},
		HasError: false,
	}, LLMOpts{MaxRepairIters: 0})
	for _, want := range []string{"Function: Add", "Args:", "a int", "b int", "Returns:", "got int"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q: %s", want, prompt)
		}
	}
}

func TestSystemPromptMentionsJSONArray(t *testing.T) {
	if !strings.Contains(systemPromptForTestCases(), "json") {
		t.Fatal("system prompt should mention JSON")
	}
	if !strings.Contains(systemPromptForTestCases(), "array") {
		t.Fatal("system prompt should mention array")
	}
}
