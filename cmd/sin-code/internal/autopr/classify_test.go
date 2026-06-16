// SPDX-License-Identifier: MIT
// Purpose: tests for issue #158 — trivial vs. non-trivial classifier.
package autopr

import "testing"

func TestTrivialAndMechanical(t *testing.T) {
	in := []Issue{
		{ID: "1", Class: ClassTrivial},
		{ID: "2", Class: ClassMechanical},
		{ID: "3", Class: ClassNonTrivial},
		{ID: "4", Class: ClassTrivial},
	}
	out := TrivialAndMechanical(in)
	if len(out) != 3 {
		t.Fatalf("expected 3 auto-fixable, got %d", len(out))
	}
	if out[0].ID != "1" || out[1].ID != "2" || out[2].ID != "4" {
		t.Errorf("unexpected ids: %s %s %s", out[0].ID, out[1].ID, out[2].ID)
	}
}

func TestClassifyGofmt(t *testing.T) {
	clean := ClassifyGofmt("main.go", false)
	if clean.ID != "" {
		t.Errorf("expected empty id for clean file, got %q", clean.ID)
	}
	dirty := ClassifyGofmt("main.go", true)
	if dirty.Class != ClassTrivial {
		t.Errorf("expected ClassTrivial, got %q", dirty.Class)
	}
	if dirty.Fix != "gofmt -w main.go" {
		t.Errorf("expected fix command, got %q", dirty.Fix)
	}
}

func TestClassifyMissingTest(t *testing.T) {
	i := ClassifyMissingTest("foo.spec.md", "foo_test.go")
	if i.Class != ClassMechanical {
		t.Errorf("expected ClassMechanical, got %q", i.Class)
	}
	if i.Fix == "" {
		t.Error("expected a non-empty Fix command")
	}
}

func TestClassifyImport(t *testing.T) {
	i := ClassifyImport("a.go", "fmt")
	if i.Class != ClassTrivial {
		t.Errorf("expected ClassTrivial, got %q", i.Class)
	}
	if i.Category != "import" {
		t.Errorf("expected category=import, got %q", i.Category)
	}
}

func TestClassifyNonTrivial(t *testing.T) {
	i := ClassifyNonTrivial("a.go", "logic drift")
	if i.Class != ClassNonTrivial {
		t.Errorf("expected ClassNonTrivial, got %q", i.Class)
	}
}

func TestClassFromString(t *testing.T) {
	cases := map[string]Class{
		"trivial":      ClassTrivial,
		"TRIVIAL":      ClassTrivial,
		"mechanical":   ClassMechanical,
		"non_trivial":  ClassNonTrivial,
		"nontrivial":   ClassNonTrivial,
		"non-trivial":  ClassNonTrivial,
		"bogus":        ClassNonTrivial, // fail-closed
		"":             ClassNonTrivial,
	}
	for in, want := range cases {
		if got := ClassFromString(in); got != want {
			t.Errorf("ClassFromString(%q) = %q, want %q", in, got, want)
		}
	}
}
