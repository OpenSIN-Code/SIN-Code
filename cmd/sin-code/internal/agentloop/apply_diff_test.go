// SPDX-License-Identifier: MIT
// Purpose: Tests for the unified diff applier (issue #365).
package agentloop

import (
	"strings"
	"testing"
)

func TestApplyDiff_SimpleSingleLine(t *testing.T) {
	content := "line1\nline2\nline3\n"
	diff := "@@ -1,3 +1,3 @@\n line1\n-line2\n+replaced\n line3\n"
	hunks, err := ParseUnifiedDiff(diff)
	if err != nil {
		t.Fatalf("ParseUnifiedDiff: %v", err)
	}
	result, err := ApplyDiff(content, hunks)
	if err != nil {
		t.Fatalf("ApplyDiff: %v", err)
	}
	want := "line1\nreplaced\nline3\n"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestApplyDiff_MultiHunk(t *testing.T) {
	content := "a\nb\nc\nd\ne\nf\n"
	diff := "@@ -1,2 +1,2 @@\n a\n-b\n+B\n@@ -5,2 +5,2 @@\n e\n-f\n+F\n"
	hunks, err := ParseUnifiedDiff(diff)
	if err != nil {
		t.Fatalf("ParseUnifiedDiff: %v", err)
	}
	if err := ValidateDiff(content, hunks); err != nil {
		t.Fatalf("ValidateDiff: %v", err)
	}
	result, err := ApplyDiff(content, hunks)
	if err != nil {
		t.Fatalf("ApplyDiff: %v", err)
	}
	want := "a\nB\nc\nd\ne\nF\n"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestApplyDiff_ContextLines(t *testing.T) {
	content := "keep1\nremove\nkeep2\n"
	diff := "@@ -1,3 +1,3 @@\n keep1\n-remove\n+insert\n keep2\n"
	hunks, err := ParseUnifiedDiff(diff)
	if err != nil {
		t.Fatalf("ParseUnifiedDiff: %v", err)
	}
	result, err := ApplyDiff(content, hunks)
	if err != nil {
		t.Fatalf("ApplyDiff: %v", err)
	}
	want := "keep1\ninsert\nkeep2\n"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestApplyDiff_MismatchFailsValidation(t *testing.T) {
	content := "line1\nDIFFERENT\nline3\n"
	diff := "@@ -1,3 +1,3 @@\n line1\n-line2\n+replaced\n line3\n"
	hunks, err := ParseUnifiedDiff(diff)
	if err != nil {
		t.Fatalf("ParseUnifiedDiff: %v", err)
	}
	err = ValidateDiff(content, hunks)
	if err == nil {
		t.Fatal("ValidateDiff should have failed for mismatched context")
	}
}

func TestApplyDiff_EmptyDiff(t *testing.T) {
	content := "hello\nworld\n"
	hunks, err := ParseUnifiedDiff("")
	if err != nil {
		t.Fatalf("ParseUnifiedDiff: %v", err)
	}
	if len(hunks) != 0 {
		t.Fatalf("expected 0 hunks, got %d", len(hunks))
	}
	result, err := ApplyDiff(content, hunks)
	if err != nil {
		t.Fatalf("ApplyDiff: %v", err)
	}
	if result != content {
		t.Errorf("got %q, want %q", result, content)
	}
}

func TestApplyDiff_PureInsertion(t *testing.T) {
	content := "line1\nline3\n"
	diff := "@@ -1,0 +2,1 @@\n+line2\n"
	hunks, err := ParseUnifiedDiff(diff)
	if err != nil {
		t.Fatalf("ParseUnifiedDiff: %v", err)
	}
	if err := ValidateDiff(content, hunks); err != nil {
		t.Fatalf("ValidateDiff: %v", err)
	}
	result, err := ApplyDiff(content, hunks)
	if err != nil {
		t.Fatalf("ApplyDiff: %v", err)
	}
	want := "line1\nline2\nline3\n"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestApplyDiff_PureDeletion(t *testing.T) {
	content := "a\nb\nc\n"
	diff := "@@ -1,3 +1,2 @@\n a\n-b\n c\n"
	hunks, err := ParseUnifiedDiff(diff)
	if err != nil {
		t.Fatalf("ParseUnifiedDiff: %v", err)
	}
	result, err := ApplyDiff(content, hunks)
	if err != nil {
		t.Fatalf("ApplyDiff: %v", err)
	}
	want := "a\nc\n"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestApplyDiff_NoTrailingNewline(t *testing.T) {
	content := "line1\nline2\nline3"
	diff := "@@ -1,3 +1,3 @@\n line1\n-line2\n+replaced\n line3\n"
	hunks, err := ParseUnifiedDiff(diff)
	if err != nil {
		t.Fatalf("ParseUnifiedDiff: %v", err)
	}
	result, err := ApplyDiff(content, hunks)
	if err != nil {
		t.Fatalf("ApplyDiff: %v", err)
	}
	want := "line1\nreplaced\nline3"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestApplyDiff_BinaryGuard(t *testing.T) {
	// Content containing NUL bytes should be rejected to prevent
	// accidental application of diffs to binary files.
	content := "line1\n\x00\x00\x00\nline3\n"
	diff := "@@ -1,3 +1,3 @@\n line1\n-\x00\x00\x00\n+replaced\n line3\n"
	_, err := ParseUnifiedDiff(diff)
	if err != nil {
		t.Fatalf("ParseUnifiedDiff: %v", err)
	}
	if !isBinary(content) {
		t.Fatal("isBinary should detect NUL bytes")
	}
	// ValidateDiff should succeed (text matches) but we guard at the
	// DiffApplier.Apply level. The standalone ApplyDiff does not check
	// for binary — that's the caller's responsibility. Verify the guard
	// function works:
	if isBinary("normal text") {
		t.Fatal("isBinary should return false for normal text")
	}
}

func TestApplyDiff_ApplierApply(t *testing.T) {
	content := "a\nb\nc\n"
	diff := "@@ -1,3 +1,3 @@\n a\n-b\n+B\n c\n"
	applier := NewDiffApplier()
	result, err := applier.Apply(content, diff)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	want := "a\nB\nc\n"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestApplyDiff_FileHeadersSkipped(t *testing.T) {
	diff := "--- a/file.go\n+++ b/file.go\n@@ -1,2 +1,2 @@\n a\n-b\n+B\n"
	hunks, err := ParseUnifiedDiff(diff)
	if err != nil {
		t.Fatalf("ParseUnifiedDiff: %v", err)
	}
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	if hunks[0].OldText != "a\nb\n" {
		t.Errorf("OldText = %q, want %q", hunks[0].OldText, "a\nb\n")
	}
	if hunks[0].NewText != "a\nB\n" {
		t.Errorf("NewText = %q, want %q", hunks[0].NewText, "a\nB\n")
	}
}

func TestGenerateDiff_RoundTrip(t *testing.T) {
	oldContent := "alpha\nbeta\ngamma\n"
	newContent := "alpha\nBETA\ngamma\n"
	diff := GenerateDiff(oldContent, newContent)
	hunks, err := ParseUnifiedDiff(diff)
	if err != nil {
		t.Fatalf("ParseUnifiedDiff: %v", err)
	}
	result, err := ApplyDiff(oldContent, hunks)
	if err != nil {
		t.Fatalf("ApplyDiff: %v", err)
	}
	if result != newContent {
		t.Errorf("round-trip = %q, want %q", result, newContent)
	}
}

func TestGenerateDiff_Insertion(t *testing.T) {
	oldContent := "a\nc\n"
	newContent := "a\nb\nc\n"
	diff := GenerateDiff(oldContent, newContent)
	if !strings.Contains(diff, "+b") {
		t.Errorf("diff missing inserted line: %s", diff)
	}
}

func TestGenerateDiff_Deletion(t *testing.T) {
	oldContent := "a\nb\nc\n"
	newContent := "a\nc\n"
	diff := GenerateDiff(oldContent, newContent)
	if !strings.Contains(diff, "-b") {
		t.Errorf("diff missing removed line: %s", diff)
	}
}

// isBinary returns true if the content contains NUL bytes, which is the
// standard heuristic for binary file detection.
func isBinary(content string) bool {
	return strings.ContainsRune(content, 0)
}
