// SPDX-License-Identifier: MIT
package spec

import (
	"os"
	"path/filepath"
	"testing"
)

const sample = "# Faster JSON Encoder\n" +
	"\n" +
	"# Objective\n" +
	"Replace the reflection-based encoder with a code-generated one so the hot\n" +
	"path no longer allocates.\n" +
	"\n" +
	"# Requirements\n" +
	"- [must] R1: encode must be allocation-free on the steady-state path\n" +
	"- [should] support nested structs\n" +
	"- May: expose a streaming API\n" +
	"\n" +
	"# Acceptance Criteria\n" +
	"- A1: benchmark shows 0 allocs/op  `verify: go test -run=NONE -bench=Encode -benchmem ./...`\n" +
	"- A2: existing encoder tests still pass  `verify: go test ./encoder/...`\n" +
	"\n" +
	"# Invariants\n" +
	"- Public API of encoder package must not change\n" +
	"- No new third-party dependencies\n"

func TestParseFull(t *testing.T) {
	s, err := Parse(sample)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if s.Title != "Faster JSON Encoder" {
		t.Errorf("title = %q", s.Title)
	}
	if s.Objective == "" {
		t.Error("objective empty")
	}
	if len(s.Requirements) != 3 {
		t.Fatalf("requirements = %d, want 3", len(s.Requirements))
	}
	// R1 explicit id + must priority
	if s.Requirements[0].ID != "R1" || s.Requirements[0].Priority != Must {
		t.Errorf("req[0] = %+v", s.Requirements[0])
	}
	if s.Requirements[1].Priority != Should {
		t.Errorf("req[1] priority = %q, want should", s.Requirements[1].Priority)
	}
	if s.Requirements[2].Priority != May {
		t.Errorf("req[2] priority = %q, want may", s.Requirements[2].Priority)
	}
	// auto-assigned id for req without explicit id
	if s.Requirements[1].ID != "R2" {
		t.Errorf("req[1] id = %q, want R2", s.Requirements[1].ID)
	}
	if len(s.Criteria) != 2 {
		t.Fatalf("criteria = %d, want 2", len(s.Criteria))
	}
	if s.Criteria[0].ID != "A1" || s.Criteria[0].Verify == "" {
		t.Errorf("crit[0] = %+v", s.Criteria[0])
	}
	if s.Criteria[0].Text != "benchmark shows 0 allocs/op" {
		t.Errorf("crit[0] text = %q", s.Criteria[0].Text)
	}
	if len(s.Invariants) != 2 {
		t.Errorf("invariants = %d, want 2", len(s.Invariants))
	}
}

func TestValidateOK(t *testing.T) {
	s, _ := Parse(sample)
	res := Validate(s)
	if !res.OK() {
		t.Errorf("expected OK, got errors: %v", res.Errors())
	}
}

func TestValidateErrors(t *testing.T) {
	s, _ := Parse("# Just A Title\n\n# Objective\nDo a thing.\n")
	res := Validate(s)
	if res.OK() {
		t.Error("expected validation errors (no requirements, no criteria)")
	}
	// must flag both missing requirements and missing criteria
	var hasReq, hasAcc bool
	for _, i := range res.Errors() {
		if i.Field == "requirements" {
			hasReq = true
		}
		if i.Field == "acceptance" {
			hasAcc = true
		}
	}
	if !hasReq || !hasAcc {
		t.Errorf("missing expected errors: req=%v acc=%v", hasReq, hasAcc)
	}
}

func TestValidateDuplicateID(t *testing.T) {
	s, _ := Parse("# T\n# Objective\nx\n# Requirements\n- R1: a\n- R1: b\n# Acceptance Criteria\n- A1: c verify: true\n")
	res := Validate(s)
	if res.OK() {
		t.Error("expected duplicate-id error")
	}
}

func TestValidateNoVerifyWarning(t *testing.T) {
	s, _ := Parse("# T\n# Objective\nx\n# Requirements\n- do x\n# Acceptance Criteria\n- it works\n")
	res := Validate(s)
	if !res.OK() {
		t.Errorf("expected OK with only warnings, got: %v", res.Errors())
	}
	var warned bool
	for _, i := range res.Issues {
		if i.Severity == "warning" && i.Field == "acceptance" {
			warned = true
		}
	}
	if !warned {
		t.Error("expected a no-verify warning")
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.spec.md")
	if err := os.WriteFile(p, []byte(sample), 0644); err != nil {
		t.Fatal(err)
	}
	s, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Path != p {
		t.Errorf("path = %q, want %q", s.Path, p)
	}
}

func TestLoadMissing(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.spec.md")); err == nil {
		t.Error("expected error for missing file")
	}
}
