// SPDX-License-Identifier: MIT
// Purpose: tests for goal contracts (Definition-of-Done) — marshal round-trip,
// emptiness semantics, and source-priority resolution (AGENTS.md §8).
package goalcontract

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
)

func TestIsEmpty(t *testing.T) {
	if !(*GoalContract)(nil).IsEmpty() {
		t.Fatal("nil contract must be empty")
	}
	if !(&GoalContract{}).IsEmpty() {
		t.Fatal("zero contract must be empty")
	}
	if (&GoalContract{SemanticCriteria: []string{"x"}}).IsEmpty() {
		t.Fatal("contract with criteria is not empty")
	}
	if (&GoalContract{MaxFilesChanged: 3}).IsEmpty() {
		t.Fatal("contract with diff bound is not empty")
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	in := &GoalContract{
		GoalID:           "42",
		SemanticCriteria: []string{"docs updated", "no regressions"},
		MaxFilesChanged:  5,
	}
	s, err := in.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if s == "" {
		t.Fatal("non-empty contract marshaled to empty string")
	}
	out, err := Unmarshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.SemanticCriteria) != 2 || out.MaxFilesChanged != 5 || out.GoalID != "42" {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}

func TestMarshalEmptyIsBlank(t *testing.T) {
	s, err := (&GoalContract{}).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if s != "" {
		t.Fatalf("empty contract should marshal to blank, got %q", s)
	}
}

func TestUnmarshalBlankYieldsEmptyNonNil(t *testing.T) {
	c, err := Unmarshal("   ")
	if err != nil {
		t.Fatal(err)
	}
	if c == nil || !c.IsEmpty() {
		t.Fatal("blank string should yield empty non-nil contract")
	}
}

func TestUnmarshalInvalid(t *testing.T) {
	if _, err := Unmarshal("{not json"); err == nil {
		t.Fatal("expected error on invalid JSON")
	}
}

func TestResolveInlineCriteria(t *testing.T) {
	c, err := Resolve(ResolveOptions{
		Workspace: t.TempDir(),
		GoalID:    "g1",
		Criteria:  []string{"  fully addressed  ", "", "tests pass"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.SemanticCriteria) != 2 {
		t.Fatalf("blank criteria should be dropped, got %v", c.SemanticCriteria)
	}
	if c.SemanticCriteria[0] != "fully addressed" {
		t.Fatalf("criterion not trimmed: %q", c.SemanticCriteria[0])
	}
}

func TestResolveDoneWhenPredicate(t *testing.T) {
	c, err := Resolve(ResolveOptions{
		Workspace: t.TempDir(),
		DoneWhen:  "test -f done.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasCheckNamed(c.DeterministicChecks, "done-when") {
		t.Fatalf("expected done-when predicate, got %+v", c.DeterministicChecks)
	}
}

func TestResolveAutoDetectGoRepo(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Resolve(ResolveOptions{Workspace: ws, AutoDetect: true})
	if err != nil {
		t.Fatal(err)
	}
	// Expect the default Go checks plus the no-new-todos guard.
	if !hasCheckNamed(c.DeterministicChecks, "go build") {
		t.Fatal("expected go build check from auto-detect")
	}
	if !hasCheckNamed(c.DeterministicChecks, "no-new-todos") {
		t.Fatal("expected no-new-todos guard from auto-detect")
	}
}

func TestResolveVerifyCmdFallback(t *testing.T) {
	// No go.mod, no auto-detect: verify-cmd becomes the single check.
	c, err := Resolve(ResolveOptions{
		Workspace: t.TempDir(),
		VerifyCmd: "make check",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.DeterministicChecks) != 1 || c.DeterministicChecks[0].Name != "verify-cmd" {
		t.Fatalf("expected single verify-cmd fallback, got %+v", c.DeterministicChecks)
	}
	if c.DeterministicChecks[0].Kind != orchestrator.CheckPredicate {
		t.Fatal("verify-cmd should be a predicate check")
	}
}

func TestResolveContractFileMerges(t *testing.T) {
	ws := t.TempDir()
	file := filepath.Join(ws, "contract.json")
	raw := `{"semantic_criteria":["from file"],"max_files_changed":7}`
	if err := os.WriteFile(file, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Resolve(ResolveOptions{
		Workspace:    ws,
		ContractFile: "contract.json",
		Criteria:     []string{"from flag"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.SemanticCriteria) != 2 {
		t.Fatalf("file + flag criteria should merge, got %v", c.SemanticCriteria)
	}
	if c.MaxFilesChanged != 7 {
		t.Fatalf("max_files_changed from file lost: %d", c.MaxFilesChanged)
	}
}

func TestResolveMissingContractFile(t *testing.T) {
	if _, err := Resolve(ResolveOptions{
		Workspace:    t.TempDir(),
		ContractFile: "nope.json",
	}); err == nil {
		t.Fatal("expected error for missing contract file")
	}
}

func TestMarshalError(t *testing.T) {
	old := jsonMarshal
	jsonMarshal = func(v any) ([]byte, error) {
		return nil, errors.New("marshal boom")
	}
	defer func() { jsonMarshal = old }()
	c := &GoalContract{SemanticCriteria: []string{"x"}}
	if _, err := c.Marshal(); err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveContractFileMaxLinesChanged(t *testing.T) {
	ws := t.TempDir()
	file := filepath.Join(ws, "contract.json")
	raw := `{"semantic_criteria":["from file"],"max_files_changed":3,"max_lines_changed":99}`
	if err := os.WriteFile(file, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Resolve(ResolveOptions{
		Workspace:    ws,
		ContractFile: "contract.json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.MaxFilesChanged != 3 {
		t.Errorf("max_files_changed: %d", c.MaxFilesChanged)
	}
	if c.MaxLinesChanged != 99 {
		t.Errorf("max_lines_changed: %d", c.MaxLinesChanged)
	}
}

func TestResolveInvalidContractFile(t *testing.T) {
	ws := t.TempDir()
	file := filepath.Join(ws, "contract.json")
	if err := os.WriteFile(file, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(ResolveOptions{
		Workspace:    ws,
		ContractFile: "contract.json",
	}); err == nil {
		t.Fatal("expected error for invalid contract file")
	}
}
