// SPDX-License-Identifier: MIT
// Purpose: tests for Python and JSON signature drift. Skips the
// Python tests if python3 is not on PATH (CI may not have it).
// Docs: docs/SPEC-LAYER.md §"Drift detection (the hardening)" (Python, JSON)
package spec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// python3OnPath is checked once per test run. If python3 is missing,
// the Python-specific tests are skipped (the typeCompatible and
// jsonExtract tests below don't need python).
func python3OnPath(t *testing.T) string {
	t.Helper()
	for _, p := range []string{"python3", "python"} {
		if path, err := lookPath(p); err == nil {
			return path
		}
	}
	t.Skip("python3 not on PATH; skipping Python drift test")
	return ""
}

// LookPath is a tiny indirection so the test file doesn't depend on
// os/exec at the top of the file (we only need it for the skip).
func lookPath(p string) (string, error) {
	for _, dir := range strings.Split(os.Getenv("PATH"), string(os.PathListSeparator)) {
		if dir == "" {
			continue
		}
		full := filepath.Join(dir, p)
		if _, err := os.Stat(full); err == nil {
			return full, nil
		}
	}
	return "", os.ErrNotExist
}

func TestDrift_PythonMatch(t *testing.T) {
	python3 := python3OnPath(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.py"),
		[]byte("def foo(x: int, y: str) -> str:\n    return y\n"),
		0o644); err != nil {
		t.Fatal(err)
	}
	s := &Spec{
		Title: "demo",
		Requirements: []Requirement{
			{ID: "R1", Text: "`def foo(x: int, y: str) -> str` is in the public API", Priority: Must},
		},
	}
	rep, err := s.DetectSignatureDriftWithPython(root, python3)
	if err != nil {
		t.Fatalf("DetectSignatureDrift: %v", err)
	}
	if len(rep.Hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(rep.Hits))
	}
	h := rep.Hits[0]
	if h.Kind != "python" {
		t.Errorf("Kind: got %q, want 'python'", h.Kind)
	}
	if !h.Match {
		t.Errorf("expected match, got drift: %s", h.Note)
	}
}

func TestDrift_PythonMissing(t *testing.T) {
	python3 := python3OnPath(t)
	root := t.TempDir()
	// Empty Python tree.
	if err := os.WriteFile(filepath.Join(root, "a.py"),
		[]byte("# empty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Spec{
		Title: "demo",
		Requirements: []Requirement{
			{ID: "R1", Text: "`def foo() -> str` is in the public API", Priority: Must},
		},
	}
	rep, _ := s.DetectSignatureDriftWithPython(root, python3)
	if len(rep.Hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(rep.Hits))
	}
	if rep.Hits[0].Match {
		t.Fatal("expected drift (function missing)")
	}
	if !strings.Contains(rep.Hits[0].Note, "not found") {
		t.Fatalf("expected 'not found' note, got: %q", rep.Hits[0].Note)
	}
}

func TestDrift_PythonBinaryMissing(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.py"),
		[]byte("def foo() -> str:\n    return ''\n"),
		0o644); err != nil {
		t.Fatal(err)
	}
	s := &Spec{
		Title: "demo",
		Requirements: []Requirement{
			{ID: "R1", Text: "`def foo() -> str` is in the public API", Priority: Must},
		},
	}
	// Pass a non-existent binary; the drift code should surface a
	// "python3 not available" note rather than crashing.
	rep, _ := s.DetectSignatureDriftWithPython(root, "/nonexistent/python3-binary")
	if len(rep.Hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(rep.Hits))
	}
	if rep.Hits[0].Match {
		t.Fatal("expected non-match when python3 missing")
	}
	if !strings.Contains(rep.Hits[0].Note, "python3 not available") {
		t.Fatalf("expected 'python3 not available' note, got: %q", rep.Hits[0].Note)
	}
}

func TestDrift_GoAndPythonInSameSpec(t *testing.T) {
	python3 := python3OnPath(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"),
		[]byte("package x\nfunc GoFunc() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.py"),
		[]byte("def py_func() -> str:\n    return ''\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Spec{
		Title: "demo",
		Requirements: []Requirement{
			{ID: "R1", Text: "`GoFunc()` is in the Go API", Priority: Must},
			{ID: "R2", Text: "`def py_func() -> str` is in the Python API", Priority: Must},
		},
	}
	rep, err := s.DetectSignatureDriftWithPython(root, python3)
	if err != nil {
		t.Fatalf("DetectSignatureDrift: %v", err)
	}
	if len(rep.Hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(rep.Hits))
	}
	// Hits are sorted by Kind first ("go" < "python").
	if rep.Hits[0].Kind != "go" || rep.Hits[1].Kind != "python" {
		t.Errorf("Kind order wrong: %s, %s", rep.Hits[0].Kind, rep.Hits[1].Kind)
	}
	for _, h := range rep.Hits {
		if !h.Match {
			t.Errorf("%s: expected match, got drift: %s", h.RequirementID, h.Note)
		}
	}
}

func TestDrift_JSONMatch(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "config.json"),
		[]byte(`{"name": "sin-code", "id": 42, "enabled": true}`),
		0o644); err != nil {
		t.Fatal(err)
	}
	s := &Spec{
		Title: "demo",
		Requirements: []Requirement{
			{ID: "R1", Text: `config shape ` + "`{\"name\": \"str\", \"id\": \"int\", \"enabled\": \"bool\"}`" + ` is documented`, Priority: Must},
		},
	}
	rep, err := s.DetectSignatureDrift(root)
	if err != nil {
		t.Fatalf("DetectSignatureDrift: %v", err)
	}
	if len(rep.JSON) != 1 {
		t.Fatalf("expected 1 JSON hit, got %d", len(rep.JSON))
	}
	if !rep.JSON[0].Match {
		t.Errorf("expected match, got drift: %s", rep.JSON[0].Note)
	}
}

func TestDrift_JSONMissingKey(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "config.json"),
		[]byte(`{"name": "sin-code"}`),
		0o644); err != nil {
		t.Fatal(err)
	}
	s := &Spec{
		Title: "demo",
		Requirements: []Requirement{
			{ID: "R1", Text: "shape `{\"name\": \"str\", \"id\": \"int\"}` is the schema", Priority: Must},
		},
	}
	rep, _ := s.DetectSignatureDrift(root)
	if len(rep.JSON) != 1 {
		t.Fatalf("expected 1 JSON hit, got %d", len(rep.JSON))
	}
	if rep.JSON[0].Match {
		t.Fatal("expected drift (missing key)")
	}
	if !strings.Contains(rep.JSON[0].Note, "id") {
		t.Fatalf("expected note to mention 'id', got: %q", rep.JSON[0].Note)
	}
}

func TestDrift_JSONTypeMismatch(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "config.json"),
		[]byte(`{"id": "not-a-number"}`),
		0o644); err != nil {
		t.Fatal(err)
	}
	s := &Spec{
		Title: "demo",
		Requirements: []Requirement{
			{ID: "R1", Text: "shape `{\"id\": \"int\"}` is the schema", Priority: Must},
		},
	}
	rep, _ := s.DetectSignatureDrift(root)
	if rep.JSON[0].Match {
		t.Fatal("expected drift (type mismatch)")
	}
}

func TestDrift_ExtractJSONShapes(t *testing.T) {
	// Direct test of the parser helper. All values are JSON strings
	// (the type annotation); the parser does not interpret them.
	cases := []struct {
		text string
		want map[string]string
	}{
		{`{"name": "str", "id": "int"}`, map[string]string{"name": "str", "id": "int"}},
		{`{"tags": "[]str"}`, map[string]string{"tags": "[]str"}},
		{`{"meta": "{}"}`, map[string]string{"meta": "{}"}},
	}
	for _, c := range cases {
		got := extractJSONShapes("`" + c.text + "`")
		if len(got) != 1 {
			t.Errorf("text=%q: expected 1 shape, got %d", c.text, len(got))
			continue
		}
		if !mapsEqual(got[0].Shape, c.want) {
			t.Errorf("text=%q: got %+v, want %+v", c.text, got[0].Shape, c.want)
		}
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestJSONMatch_TypeVariants(t *testing.T) {
	// Pure unit test of the type comparator.
	cases := []struct {
		doc  string
		want string
		ok   bool
	}{
		{`"hello"`, "str", true},
		{`"hello"`, "string", true},
		{`42`, "int", true},
		{`42`, "number", true},
		{`true`, "bool", true},
		{`[1, 2, 3]`, "[]int", true},
		{`{"a": 1}`, "object", true},
		{`"hello"`, "int", false},
		{`"hello"`, "[]str", false},
	}
	for _, c := range cases {
		var v any
		if err := json.Unmarshal([]byte(c.doc), &v); err != nil {
			t.Fatalf("unmarshal %q: %v", c.doc, err)
		}
		ok, _ := jsonMatch(map[string]string{"k": c.want}, map[string]any{"k": v}, false)
		if ok != c.ok {
			t.Errorf("doc=%s want=%s: got ok=%v, want %v", c.doc, c.want, ok, c.ok)
		}
	}
}
