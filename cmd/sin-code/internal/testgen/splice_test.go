// SPDX-License-Identifier: MIT
// Purpose: tests for LLM case splicing + JSON-to-Go literal rendering.
// Validates the round-trip from JSON LLM output to a Go test row.
package testgen

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJsonLiteral(t *testing.T) {
	cases := []struct {
		in  any
		out string
	}{
		{int(0), "0"},
		{int(2), "2"},
		{int(-7), "-7"},
		{float64(2.5), "2.5"},
		{float64(2), "2"},
		{true, "true"},
		{false, "false"},
		{"hello", `"hello"`},
		{nil, "nil"},
		{[]any{1.0, 2.0, 3.0}, "{1, 2, 3}"},
		{map[string]any{"a": 1.0, "b": 2.0}, `{"a": 1, "b": 2}`},
		{map[string]any{"b": 2.0, "a": 1.0}, `{"a": 1, "b": 2}`},
	}
	for _, c := range cases {
		got := jsonLiteral(c.in)
		if got != c.out {
			t.Errorf("jsonLiteral(%v) = %q, want %q", c.in, got, c.out)
		}
	}
}

func TestRenderCaseRow_SimpleArgsWants(t *testing.T) {
	fn := FuncInfo{
		Name: "Add", Args: []Param{{Name: "a", Type: "int"}, {Name: "b", Type: "int"}},
		Returns: []Param{{Name: "got", Type: "int"}},
	}
	tc := TestCase{
		Name:  "sum",
		Args:  map[string]any{"a": 2.0, "b": 3.0},
		Wants: map[string]any{"got": 5.0},
	}
	row := renderCaseRow(fn, tc)
	for _, want := range []string{`name: "sum"`, "a: 2,", "b: 3,", "wantGot: 5,", "wantErr: false"} {
		if !strings.Contains(row, want) {
			t.Errorf("renderCaseRow missing %q: %s", want, row)
		}
	}
}

func TestRenderCaseRow_StringArg(t *testing.T) {
	fn := FuncInfo{Name: "Reverse", Args: []Param{{Name: "s", Type: "string"}}, Returns: []Param{{Name: "got", Type: "string"}}}
	tc := TestCase{Name: "r", Args: map[string]any{"s": "abc"}, Wants: map[string]any{"got": "cba"}}
	row := renderCaseRow(fn, tc)
	if !strings.Contains(row, `s: "abc"`) {
		t.Errorf("missing s: %s", row)
	}
	if !strings.Contains(row, `wantGot: "cba"`) {
		t.Errorf("missing wantgot: %s", row)
	}
}

func TestRenderCaseRow_PartialFieldsFallback(t *testing.T) {
	fn := FuncInfo{Name: "Add", Args: []Param{{Name: "a", Type: "int"}, {Name: "b", Type: "int"}}, Returns: []Param{{Name: "got", Type: "int"}}}
	// missing b in args, missing got in wants -> zeros
	tc := TestCase{Name: "p", Args: map[string]any{"a": 4.0}, Wants: map[string]any{}, WantErr: false}
	row := renderCaseRow(fn, tc)
	for _, want := range []string{"a: 4,", "b: 0,", "wantGot: 0,"} {
		if !strings.Contains(row, want) {
			t.Errorf("partial render missing %q: %s", want, row)
		}
	}
}

func TestTestKey(t *testing.T) {
	if got := testKey(FuncInfo{Name: "Add"}); got != "Add" {
		t.Errorf("free func testKey: %q", got)
	}
	if got := testKey(FuncInfo{IsMethod: true, Receiver: "Calc", Name: "Sum"}); got != "Calc_Sum" {
		t.Errorf("method testKey: %q", got)
	}
}

// generateFallback-Splice test: feed real Go source + Cases map, ensure
// the LLM rows appear and the scaffold compiles.
func TestGenerateFallbackSplice(t *testing.T) {
	src := `package calc

func Add(a, b int) int { return a + b }
`
	tmp := t.TempDir()
	calcPath := filepath.Join(tmp, "calc.go")
	if err := os.WriteFile(calcPath, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := map[string][]TestCase{
		"Add": {
			{Name: "sum", Args: map[string]any{"a": 1.0, "b": 2.0}, Wants: map[string]any{"got": 3.0}},
			{Name: "negative", Args: map[string]any{"a": -1.0, "b": 1.0}, Wants: map[string]any{"got": 0.0}},
		},
	}
	got, err := generateFallback(context.Background(), calcPath, nil, cases)
	if err != nil {
		t.Fatalf("generateFallback: %v", err)
	}
	for _, want := range []string{
		`name: "sum"`, "a: 1,", "b: 2,",
		`name: "negative"`, "a: -1,", "b: 1,",
		"should NOT contain the basic case",
	} {
		// we use string contains for positives, explicit absence for the negative
		if want == "should NOT contain the basic case" {
			if strings.Contains(got, `name: "basic case"`) {
				t.Errorf("LLM cases present but scaffold still emitted basic case: %s", got)
			}
			continue
		}
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q: %s", want, got)
		}
	}
}

func TestGenerateFallbackNoCases_FallsBack(t *testing.T) {
	src := `package calc

func Add(a, b int) int { return a + b }
`
	tmp := t.TempDir()
	calcPath := filepath.Join(tmp, "calc.go")
	if err := os.WriteFile(calcPath, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := generateFallback(context.Background(), calcPath, nil, nil)
	if err != nil {
		t.Fatalf("generateFallback: %v", err)
	}
	if !strings.Contains(got, `name: "basic case"`) {
		t.Errorf("empty Cases should fall back to basic scaffold: %s", got)
	}
}
