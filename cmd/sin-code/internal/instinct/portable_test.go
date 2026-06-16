// SPDX-License-Identifier: MIT
// Purpose: round-trip + malformed-line tests for the JSONL exchange format.
// Docs: portable_test.doc.md
package instinct

import (
	"bytes"
	"testing"
)

func TestJSONLRoundTrip(t *testing.T) {
	a := NewInstinct("when committing", "git", "run tests first", "obs", ScopeProject)
	a.ProjectID = "p1"
	a.Evidence = []string{"go test", "git commit"}
	for n := 0; n < 6; n++ {
		a.Reinforce()
	}
	b := NewInstinct("when validating", "security", "sanitize input", "obs", ScopeGlobal)

	var buf bytes.Buffer
	if err := ExportJSONL(&buf, []*Instinct{a, b}); err != nil {
		t.Fatalf("export: %v", err)
	}
	got, errs := ImportJSONL(&buf)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 instincts, got %d", len(got))
	}
	if got[0].ID != a.ID || got[0].Action != a.Action || got[0].Confidence != a.Confidence {
		t.Fatalf("roundtrip mismatch: %+v vs %+v", got[0], a)
	}
	if len(got[0].Evidence) != 2 {
		t.Fatalf("evidence lost: %v", got[0].Evidence)
	}
}

func TestImportJSONLSkipsMalformed(t *testing.T) {
	data := []byte(`{"id":"x","trigger":"t","domain":"d","action":"a","confidence":0.5,"scope":"global"}
not-json
{"id":"y","trigger":"t2","domain":"d","action":"a2","confidence":0.6,"scope":"global"}
`)
	got, errs := ImportJSONL(bytes.NewReader(data))
	if len(got) != 2 {
		t.Fatalf("expected 2 valid, got %d", len(got))
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 malformed line, got %d", len(errs))
	}
}
