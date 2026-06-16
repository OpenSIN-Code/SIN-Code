// SPDX-License-Identifier: MIT
// Purpose: spec check tests — exercises the per-criterion verify runner
// including timeout, missing-command, and pass-through cases. Uses a
// `sh` script in t.TempDir() so the tests are hermetic.
// Docs: docs/SPEC-LAYER.md
package spec

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestCheck_AllPass(t *testing.T) {
	s := &Spec{
		Title: "demo",
		Criteria: []Criterion{
			{ID: "A1", Text: "always true", Verify: "true"},
			{ID: "A2", Text: "echo ok", Verify: "echo ok"},
		},
	}
	rep, err := s.Check(context.Background(), 5*time.Second)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rep.Passed != 2 || rep.Failed != 0 {
		t.Fatalf("expected 2/0, got %d/%d", rep.Passed, rep.Failed)
	}
	if rep.HasFailures() {
		t.Fatal("HasFailures should be false")
	}
}

func TestCheck_OneFails(t *testing.T) {
	s := &Spec{
		Title: "demo",
		Criteria: []Criterion{
			{ID: "A1", Text: "pass", Verify: "true"},
			{ID: "A2", Text: "fail", Verify: "false"},
			{ID: "A3", Text: "pass again", Verify: "true"},
		},
	}
	rep, _ := s.Check(context.Background(), 5*time.Second)
	if rep.Passed != 2 || rep.Failed != 1 {
		t.Fatalf("expected 2/1, got %d/%d", rep.Passed, rep.Failed)
	}
	if !rep.HasFailures() {
		t.Fatal("HasFailures should be true")
	}
}

func TestCheck_SkippedNoCommand(t *testing.T) {
	s := &Spec{
		Title: "demo",
		Criteria: []Criterion{
			{ID: "A1", Text: "no command", Verify: ""},
			{ID: "A2", Text: "pass", Verify: "true"},
		},
	}
	rep, _ := s.Check(context.Background(), 5*time.Second)
	if rep.Skipped != 1 || rep.Passed != 1 || rep.Failed != 0 {
		t.Fatalf("expected 1 skip / 1 pass / 0 fail, got %d/%d/%d",
			rep.Skipped, rep.Passed, rep.Failed)
	}
}

func TestCheck_Timeout(t *testing.T) {
	s := &Spec{
		Title: "demo",
		Criteria: []Criterion{
			{ID: "A1", Text: "sleep 5", Verify: "sleep 5"},
		},
	}
	rep, _ := s.Check(context.Background(), 200*time.Millisecond)
	if rep.Failed != 1 {
		t.Fatalf("expected timeout to be a failure, got %d failed", rep.Failed)
	}
	for _, r := range rep.Results {
		if r.ID == "A1" && r.ExitCode != -1 {
			t.Fatalf("expected exit code -1 on timeout, got %d", r.ExitCode)
		}
	}
}

func TestCheck_OutputTruncation(t *testing.T) {
	// Generate > 4KB of stdout.
	cmd := "head -c 8192 /dev/urandom | base64"
	s := &Spec{
		Title: "demo",
		Criteria: []Criterion{
			{ID: "A1", Text: "noisy", Verify: cmd},
		},
	}
	rep, _ := s.Check(context.Background(), 10*time.Second)
	if len(rep.Results[0].Output) > 8192 {
		t.Fatalf("output not truncated: %d bytes", len(rep.Results[0].Output))
	}
	if !strings.Contains(rep.Results[0].Output, "truncated") {
		t.Fatal("expected truncation marker")
	}
}
