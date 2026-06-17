// SPDX-License-Identifier: MIT
package tui

import (
	"testing"
)

func TestFuzzySubsequenceMatchExact(t *testing.T) {
	ok, indices := fuzzySubsequenceMatch("beta", "be")
	if !ok {
		t.Fatal("expected match for 'be' in 'beta'")
	}
	if len(indices) != 2 || indices[0] != 0 || indices[1] != 1 {
		t.Errorf("expected indices [0,1], got %v", indices)
	}
}

func TestFuzzySubsequenceMatchCaseInsensitive(t *testing.T) {
	ok, _ := fuzzySubsequenceMatch("BETA", "be")
	if !ok {
		t.Fatal("expected case-insensitive match")
	}
	ok, _ = fuzzySubsequenceMatch("beta", "BE")
	if !ok {
		t.Fatal("expected case-insensitive match")
	}
}

func TestFuzzySubsequenceMatchSubsequence(t *testing.T) {
	ok, indices := fuzzySubsequenceMatch("view: sessions", "vs")
	if !ok {
		t.Fatal("expected 'vs' to match 'view: sessions'")
	}
	if len(indices) != 2 {
		t.Errorf("expected 2 indices, got %d", len(indices))
	}
}

func TestFuzzySubsequenceMatchNoMatch(t *testing.T) {
	ok, _ := fuzzySubsequenceMatch("alpha", "xyz")
	if ok {
		t.Fatal("expected no match for 'xyz' in 'alpha'")
	}
}

func TestFuzzySubsequenceMatchEmptyQuery(t *testing.T) {
	ok, indices := fuzzySubsequenceMatch("anything", "")
	if !ok {
		t.Fatal("empty query should always match")
	}
	if indices != nil {
		t.Error("empty query should return nil indices")
	}
}

func TestFuzzySubsequenceMatchOutOfOrder(t *testing.T) {
	ok, _ := fuzzySubsequenceMatch("beta", "tb")
	if ok {
		t.Fatal("expected no match for 'tb' in 'beta' (out of order)")
	}
}

func TestFuzzyScoreConsecutiveHigher(t *testing.T) {
	target := "beta"
	_, indConsec := fuzzySubsequenceMatch(target, "be")
	_, indSpread := fuzzySubsequenceMatch(target, "ba")

	scoreConsec := fuzzyScore(target, indConsec)
	scoreSpread := fuzzyScore(target, indSpread)

	if scoreConsec <= scoreSpread {
		t.Errorf("consecutive match (%d) should score higher than spread (%d)", scoreConsec, scoreSpread)
	}
}

func TestFuzzyScoreStartBonus(t *testing.T) {
	target := "alpha"
	_, indStart := fuzzySubsequenceMatch(target, "a")
	scoreStart := fuzzyScore(target, indStart)

	target2 := "gamma"
	_, indEnd := fuzzySubsequenceMatch(target2, "a")
	scoreEnd := fuzzyScore(target2, indEnd)

	if scoreStart <= scoreEnd {
		t.Errorf("match at start (%d) should score higher than match in middle (%d)", scoreStart, scoreEnd)
	}
}

func TestFuzzyFilterSortedByScore(t *testing.T) {
	items := []string{"view: sessions", "security", "scout", "serve", "sbom"}
	matches := fuzzyFilter(items, "s")

	if len(matches) != 5 {
		t.Fatalf("expected 5 matches, got %d", len(matches))
	}

	for i := 1; i < len(matches); i++ {
		if matches[i-1].Score < matches[i].Score {
			t.Errorf("matches not sorted by score: %d before %d", matches[i-1].Score, matches[i].Score)
		}
	}
}

func TestFuzzyFilterNoMatches(t *testing.T) {
	items := []string{"alpha", "beta", "gamma"}
	matches := fuzzyFilter(items, "xyz")
	if len(matches) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matches))
	}
}

func TestFuzzyFilterEmptyQuery(t *testing.T) {
	items := []string{"alpha", "beta"}
	matches := fuzzyFilter(items, "")
	if len(matches) != 2 {
		t.Errorf("expected 2 matches for empty query, got %d", len(matches))
	}
}

func TestFuzzyFilterSubsequenceMatching(t *testing.T) {
	items := []string{"view: config", "view: chat", "security", "alpha"}
	matches := fuzzyFilter(items, "vc")

	if len(matches) < 1 {
		t.Fatalf("expected at least 1 match for 'vc', got %d", len(matches))
	}
	hasConfig := false
	for _, m := range matches {
		ok, _ := fuzzySubsequenceMatch(m.Item, "vc")
		if !ok {
			t.Errorf("result %q does not actually match 'vc'", m.Item)
		}
		if m.Item == "view: config" {
			hasConfig = true
		}
	}
	if !hasConfig {
		t.Error("expected 'view: config' in results")
	}
}

func TestFuzzyHighlightMarksMatchedChars(t *testing.T) {
	_, indices := fuzzySubsequenceMatch("beta", "be")
	highlighted := fuzzyHighlight("beta", indices, "", "\x1b[1m")
	if !contains(highlighted, "\x1b[1mb") {
		t.Errorf("expected 'b' to be highlighted, got %q", highlighted)
	}
	if !contains(highlighted, "\x1b[1me") {
		t.Errorf("expected 'e' to be highlighted, got %q", highlighted)
	}
}

func TestFuzzyHighlightNoIndices(t *testing.T) {
	result := fuzzyHighlight("beta", nil, "", "")
	if result != "beta" {
		t.Errorf("expected unchanged string, got %q", result)
	}
}

func TestFilterPaletteFuzzySubsequence(t *testing.T) {
	m := NewModel()
	m.OpenPalette()
	m.filterPalette("vs")
	found := false
	for _, item := range m.Palette.Filter {
		if item == "view: sessions" {
			found = true
		}
	}
	if !found {
		t.Error("fuzzy filter should match 'vs' -> 'view: sessions'")
	}
}

func TestFilterPaletteFuzzySortedByRelevance(t *testing.T) {
	m := NewModel()
	m.OpenPalette()
	m.filterPalette("s")
	if len(m.Palette.Filter) == 0 {
		t.Fatal("expected matches for 's'")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
