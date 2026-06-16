// SPDX-License-Identifier: MIT
// Purpose: classify a spec/format/lint regression as trivial or
// non-trivial (issue #158). Trivial regressions are auto-fixable
// without an LLM call; non-trivial ones are deferred to the human.
// M3-mandated: the classifier is deterministic and pure (no I/O)
// so the verify gate stays race-free.
package autopr

import (
	"strings"
)

// Class is the severity of a single regression.
type Class string

const (
	// ClassTrivial is auto-fixable: gofmt, goimports, trailing
	// whitespace, generated test stub, missing import.
	ClassTrivial Class = "trivial"
	// ClassMechanical is auto-fixable but needs a deterministic
	// script: rename a symbol, regenerate a doc, add a missing
	// license header.
	ClassMechanical Class = "mechanical"
	// ClassNonTrivial requires an LLM (or a human): logic
	// changes, API design, behaviour drift.
	ClassNonTrivial Class = "non_trivial"
)

// Issue is one observed regression. The autopr pipeline reads
// these from the verify-gate report + a couple of static checks
// (gofmt diff, goimports diff, missing-spec-test scan).
type Issue struct {
	ID       string `json:"id"`       // stable id, e.g. "gofmt:cmd/sin-code/main.go"
	Class    Class  `json:"class"`    // ClassTrivial|ClassMechanical|ClassNonTrivial
	Category string `json:"category"` // "format" | "import" | "test" | "spec" | "lint"
	File     string `json:"file"`     // workspace-relative path
	Note     string `json:"note"`     // human-readable explanation
	// Fix is the command the pipeline would run for trivial /
	// mechanical classes. Empty for non_trivial.
	Fix string `json:"fix,omitempty"`
}

// TrivialAndMechanical returns only the issues that the pipeline
// can auto-fix (issue #158 acceptance criterion: "the auto-fix
// is reversible" — the human always sees the PR).
func TrivialAndMechanical(in []Issue) []Issue {
	var out []Issue
	for _, i := range in {
		if i.Class == ClassTrivial || i.Class == ClassMechanical {
			out = append(out, i)
		}
	}
	return out
}

// ClassifyGofmt reports whether the file's content is gofmt-clean.
// Returns a ClassTrivial issue with the Fix command set if not.
func ClassifyGofmt(file string, dirty bool) Issue {
	if !dirty {
		return Issue{}
	}
	return Issue{
		ID:       "gofmt:" + file,
		Class:    ClassTrivial,
		Category: "format",
		File:     file,
		Note:     "gofmt would reformat this file",
		Fix:      "gofmt -w " + file,
	}
}

// ClassifyMissingTest reports a spec criterion that has no
// associated test file. ClassMechanical because a test stub is
// generated deterministically.
func ClassifyMissingTest(spec, testFile string) Issue {
	return Issue{
		ID:       "missing-test:" + spec,
		Class:    ClassMechanical,
		Category: "test",
		File:     spec,
		Note:     "spec " + spec + " has no test file at " + testFile,
		Fix:      "echo placeholder >> " + testFile,
	}
}

// ClassifyImport returns a ClassTrivial issue when a Go file is
// missing an import the spec requires.
func ClassifyImport(file, missingImport string) Issue {
	return Issue{
		ID:       "import:" + file + ":" + missingImport,
		Class:    ClassTrivial,
		Category: "import",
		File:     file,
		Note:     "missing import " + missingImport,
		Fix:      "goimports -w " + file,
	}
}

// ClassifyNonTrivial is the catch-all for anything the pipeline
// cannot auto-fix.
func ClassifyNonTrivial(file, note string) Issue {
	return Issue{
		ID:       "non-trivial:" + file,
		Class:    ClassNonTrivial,
		Category: "spec",
		File:     file,
		Note:     note,
	}
}

// ClassFromString normalises a raw category string. Used by the
// report reader to keep the on-disk format human-friendly.
func ClassFromString(s string) Class {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "trivial":
		return ClassTrivial
	case "mechanical":
		return ClassMechanical
	case "non_trivial", "nontrivial", "non-trivial":
		return ClassNonTrivial
	default:
		return ClassNonTrivial // fail-closed
	}
}
