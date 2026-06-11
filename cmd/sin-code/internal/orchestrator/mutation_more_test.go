// SPDX-License-Identifier: MIT
// Purpose: tests for the mutation probe and its diff parser.

package orchestrator

import (
	"strings"
	"testing"
)

func TestParseAddedLines_EmptyDiff(t *testing.T) {
	if got := ParseAddedLines(""); len(got) != 0 {
		t.Fatalf("empty diff: %d lines", len(got))
	}
}

func TestParseAddedLines_ExtractsAddedLines(t *testing.T) {
	diff := `--- a/foo.go
+++ b/foo.go
@@ -1,3 +1,4 @@
 line1
+added1
+added2
 line3
`
	got := ParseAddedLines(diff)
	if len(got) != 2 {
		t.Fatalf("expected 2 added lines, got %d", len(got))
	}
	for _, l := range got {
		if l.File != "foo.go" {
			t.Errorf("file: %q", l.File)
		}
		if l.Line < 1 {
			t.Errorf("line: %d", l.Line)
		}
	}
}

func TestParseAddedLines_IgnoresRemovedLines(t *testing.T) {
	diff := `--- a/foo.go
+++ b/foo.go
@@ -1,3 +1,2 @@
-deleted
 line1
`
	got := ParseAddedLines(diff)
	if len(got) != 0 {
		t.Fatalf("removed lines must not be added, got %d", len(got))
	}
}

func TestProbeResult_Diagnosis(t *testing.T) {
	pr := &ProbeResult{
		Mutations: []Mutation{{File: "x.go", Line: 1}},
		Killed:    8,
		Survived:  2,
	}
	diag := pr.Diagnosis()
	if diag == "" {
		t.Fatal("empty diagnosis")
	}
	// diagnosis with survivors should mention them
	if !strings.Contains(diag, "x.go") {
		t.Errorf("diagnosis should reference the surviving file: %q", diag)
	}
}

func TestNewMutationProbe_NoPanic(t *testing.T) {
	mp := NewMutationProbe(t.TempDir(), []string{"true"})
	if mp == nil {
		t.Fatal("nil probe")
	}
}
