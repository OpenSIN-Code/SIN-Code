// SPDX-License-Identifier: MIT
// Purpose: tests for the PR 4 additions to the Spec-Layer: method-on-
// receiver Go signatures and strict-mode JSON matching.
// Docs: docs/spec-layer.md §"Drift detection (the hardening)"
package spec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDrift_GoMethodOnReceiver(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "s.go", `package x

type S struct{ v int }

// (S) Method: value receiver.
func (S) Method() {}

// (*S) Set: pointer receiver.
func (*S) Set(v int) { s := S{}; s.v = v }
`)

	s := &Spec{
		Title: "demo",
		Requirements: []Requirement{
			{ID: "R1", Text: "`S.Method()` is in the public API", Priority: Must},
			{ID: "R2", Text: "`*S.Set(v int)` is in the public API", Priority: Must},
			{ID: "R3", Text: "`Missing()` is in the public API (DRIFT)", Priority: Must},
		},
	}
	rep, _ := s.DetectSignatureDrift(root)
	if len(rep.Hits) != 3 {
		t.Fatalf("expected 3 hits, got %d", len(rep.Hits))
	}
	if !rep.Hits[0].Match {
		t.Errorf("R1 (value receiver) should match: %s", rep.Hits[0].Note)
	}
	if !rep.Hits[1].Match {
		t.Errorf("R2 (pointer receiver) should match: %s", rep.Hits[1].Note)
	}
	if rep.Hits[2].Match {
		t.Errorf("R3 (missing) should NOT match")
	}
}

func TestDrift_GoMethodParamMismatch(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "s.go", `package x

type S struct{}
func (S) Set(v int) {}
`)

	s := &Spec{
		Title: "demo",
		Requirements: []Requirement{
			{ID: "R1", Text: "`S.Set(v string)` is in the public API (DRIFT)", Priority: Must},
		},
	}
	rep, _ := s.DetectSignatureDrift(root)
	if len(rep.Hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(rep.Hits))
	}
	if rep.Hits[0].Match {
		t.Fatal("expected drift (param type differs)")
	}
}

func TestDrift_GoGenericReceiver(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "g.go", `package x

type Box[T any] struct{ v T }
func (Box[T]) Get() T { return b.v }
`)

	s := &Spec{
		Title: "demo",
		Requirements: []Requirement{
			{ID: "R1", Text: "`Box[T].Get() T` is in the public API", Priority: Must},
		},
	}
	rep, _ := s.DetectSignatureDrift(root)
	// The generic receiver renders as "Box[T]"; the spec uses the
	// same form. v0: we accept the canonical string match.
	if len(rep.Hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(rep.Hits))
	}
	if !rep.Hits[0].Match {
		t.Errorf("R1 (generic receiver) should match: %s", rep.Hits[0].Note)
	}
}

func TestDrift_JSONStrictModePass(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "config.json"),
		[]byte(`{"name": "sin-code", "id": 42}`),
		0o644); err != nil {
		t.Fatal(err)
	}
	// Strict mode: the JSON has exactly the spec's keys.
	s := &Spec{
		Title: "demo",
		Requirements: []Requirement{
			{ID: "R1", Text: "shape `{\"name\": \"str\", \"id\": \"int\"} strict!` is the schema", Priority: Must},
		},
	}
	rep, _ := s.DetectSignatureDrift(root)
	if len(rep.JSON) != 1 {
		t.Fatalf("expected 1 JSON hit, got %d", len(rep.JSON))
	}
	if !rep.JSON[0].Match {
		t.Errorf("expected strict-mode match, got drift: %s", rep.JSON[0].Note)
	}
}

func TestDrift_JSONStrictModeFailExtras(t *testing.T) {
	root := t.TempDir()
	// JSON has an extra "extra" key that the spec doesn't list.
	if err := os.WriteFile(filepath.Join(root, "config.json"),
		[]byte(`{"name": "sin-code", "id": 42, "extra": "oops"}`),
		0o644); err != nil {
		t.Fatal(err)
	}
	s := &Spec{
		Title: "demo",
		Requirements: []Requirement{
			{ID: "R1", Text: "shape `{\"name\": \"str\", \"id\": \"int\"} strict!` is the schema", Priority: Must},
		},
	}
	rep, _ := s.DetectSignatureDrift(root)
	if len(rep.JSON) != 1 {
		t.Fatalf("expected 1 JSON hit, got %d", len(rep.JSON))
	}
	if rep.JSON[0].Match {
		t.Fatal("expected strict-mode drift (extras forbidden)")
	}
	if !strings.Contains(rep.JSON[0].Note, "extra") {
		t.Fatalf("expected note to mention 'extra', got: %q", rep.JSON[0].Note)
	}
}

func TestDrift_JSONNonStrictIgnoresExtras(t *testing.T) {
	root := t.TempDir()
	// Non-strict (default) should ignore the extra key.
	if err := os.WriteFile(filepath.Join(root, "config.json"),
		[]byte(`{"name": "sin-code", "id": 42, "extra": "ignored"}`),
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
	if rep.JSON[0].Match != true {
		t.Errorf("non-strict should ignore extras, got drift: %s", rep.JSON[0].Note)
	}
}

func TestExtractJSONShapes_StrictMarker(t *testing.T) {
	cases := []struct {
		text       string
		wantKeys   int // how many shapes to extract
		wantStrict bool
	}{
		{`{"name": "str", "id": "int"} strict!`, 1, true},
		{`{"name": "str", "id": "int"}`, 1, false},
		{`{"x": "int"}`, 1, false},
		{`not a shape`, 0, false},
	}
	for _, c := range cases {
		shapes := extractJSONShapes("`" + c.text + "`")
		if len(shapes) != c.wantKeys {
			t.Errorf("text=%q: expected %d shapes, got %d", c.text, c.wantKeys, len(shapes))
			continue
		}
		if c.wantKeys > 0 && shapes[0].Strict != c.wantStrict {
			t.Errorf("text=%q: expected strict=%v, got %v", c.text, c.wantStrict, shapes[0].Strict)
		}
	}
}

// make sure the JSON helper still works after the strict change
func TestJSONMatch_NonStrictBackwardCompat(t *testing.T) {
	doc := `{"a": 1, "b": "x"}`
	var v any
	if err := json.Unmarshal([]byte(doc), &v); err != nil {
		t.Fatal(err)
	}
	ok, msg := jsonMatch(map[string]string{"a": "int"}, v, false)
	if !ok {
		t.Errorf("non-strict should accept extra key, got: %s", msg)
	}
	ok, msg = jsonMatch(map[string]string{"a": "int"}, v, true)
	if ok {
		t.Errorf("strict should reject extra key, got ok=true")
	} else if !strings.Contains(msg, "b") {
		t.Errorf("expected msg to mention 'b', got: %q", msg)
	}
}
