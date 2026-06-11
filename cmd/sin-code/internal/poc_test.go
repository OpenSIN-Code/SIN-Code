// SPDX-License-Identifier: MIT

package internal

import (
	"strings"
	"testing"
)

// TestPocExtractRequirements_NaturalLanguage verifies that extractRequirements
// handles natural-language specs (where the identifier precedes the keyword)
// without false positives on common English words. (st-bug1, Bug 3)
func TestPocExtractRequirements_NaturalLanguage(t *testing.T) {
	content := `# Hello Function Spec
The ` + "`Hello`" + ` function must return the string "hello".
The function should also be defined in the main package.`

	reqs := extractRequirements(content)

	// Must find "Hello" (the real identifier)
	found := false
	for _, r := range reqs {
		if r.Name == "Hello" {
			found = true
		}
		// Should not include common English words
		lower := strings.ToLower(r.Name)
		if lower == "must" || lower == "should" || lower == "return" || lower == "string" || lower == "function" {
			t.Errorf("extractRequirements extracted common English word %q as a requirement", r.Name)
		}
	}
	if !found {
		t.Errorf("expected to extract 'Hello' as requirement, got: %+v", reqs)
	}
}

// TestPocExtractRequirements_StructuredSpec verifies that structured specs
// (with the keyword preceding the identifier) still work. (st-bug1, Bug 3)
func TestPocExtractRequirements_StructuredSpec(t *testing.T) {
	content := `function ` + "`Hello`" + ` must return "hello"
function ` + "`World`" + ` must exist`

	reqs := extractRequirements(content)
	if len(reqs) != 2 {
		t.Errorf("expected 2 requirements, got %d: %+v", len(reqs), reqs)
	}
	found := map[string]bool{}
	for _, r := range reqs {
		found[r.Name] = true
	}
	if !found["Hello"] {
		t.Error("expected to find 'Hello' requirement")
	}
	if !found["World"] {
		t.Error("expected to find 'World' requirement")
	}
}

// TestPocExtractRequirements_RejectsCommonWords verifies that bare "must" /
// "should" / etc. don't extract a denylisted word as a requirement.
// (st-bug1, Bug 3)
func TestPocExtractRequirements_RejectsCommonWords(t *testing.T) {
	content := `The function must return the value.`

	reqs := extractRequirements(content)
	for _, r := range reqs {
		lower := strings.ToLower(r.Name)
		if denylistedRequirementWords[lower] {
			t.Errorf("extractRequirements extracted denylisted word %q", r.Name)
		}
	}
}

// TestPocExtractRequirements_EmptyAndShort verifies edge cases. (st-bug1, Bug 3)
func TestPocExtractRequirements_EmptyAndShort(t *testing.T) {
	if reqs := extractRequirements(""); len(reqs) != 0 {
		t.Errorf("expected 0 requirements for empty spec, got %d", len(reqs))
	}
	if reqs := extractRequirements("just plain text"); len(reqs) != 0 {
		t.Errorf("expected 0 requirements for plain text without keywords, got %d", len(reqs))
	}
}
