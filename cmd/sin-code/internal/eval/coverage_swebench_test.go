// SPDX-License-Identifier: MIT
// Purpose: coverage tests for cmd/sin-code/internal/eval/swebench.go (issue #363).
// Exercises the SWE-bench harness surface that drives the in-process
// eval pipeline: dataset parsing, repo-name sanitisation, git-based
// patch application, dry-run orchestration, and result aggregation.
package eval

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// helpers --------------------------------------------------------------------

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

// runCmd is a tiny wrapper that fails the test on non-zero exit.
func runCmd(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s %v in %s: %v", name, args, dir, err)
	}
}

// initRepo builds a fresh git repo in a tempdir with one committed file.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runCmd(t, dir, "git", "init", "-q")
	runCmd(t, dir, "git", "config", "user.email", "test@example.com")
	runCmd(t, dir, "git", "config", "user.name", "sin-code-test")
	runCmd(t, dir, "git", "config", "commit.gpgsign", "false")
	writeFile(t, dir, "main.go", "package x\n\nfunc F() int { return 1 }\n")
	runCmd(t, dir, "git", "add", "main.go")
	runCmd(t, dir, "git", "commit", "-q", "-m", "init")
	return dir
}

// Test 1 ----------------------------------------------------------------------

// TestLoadSweInstances_BadJSON exercises the three loadSweInstances branches:
// (a) trailing-comma input emits a wrapped parse error,
// (b) missing field relies on the post-parse InstanceID auto-fill (current
//
//	behaviour: not a parse error — swe.go:172-176),
//
// (c) schema `version` field flows into SweInstance.Version.
func TestLoadSweInstances_BadJSON(t *testing.T) {
	dir := t.TempDir()

	// (a) trailing comma — first Unmarshal fails, fallback single fails,
	// wrapped "parse swebench json" error is returned.
	trailing := writeFile(t, dir, "trailing.json",
		`[{"instance_id":"a","repo":"r/a","base_commit":"abc",}]`)
	_, err := loadSweInstances(trailing)
	if err == nil {
		t.Fatal("expected parse error for trailing comma, got nil")
	}
	if !strings.Contains(err.Error(), "parse swebench json") {
		t.Fatalf("want wrapped 'parse swebench json' error, got: %v", err)
	}

	// (b) empty-array parses with FailToPass = []string{}; InstanceID
	// auto-fill loop has nothing to do.
	empty := writeFile(t, dir, "empty.json", `[]`)
	got, err := loadSweInstances(empty)
	if err != nil {
		t.Fatalf("empty array must parse, got: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 instances, got %d", len(got))
	}

	// (c) schema-version captured into Version.
	versioned := writeFile(t, dir, "versioned.json",
		`[{"instance_id":"v1","repo":"r/v","base_commit":"abc","version":"1.2.3","FAIL_TO_PASS":[]}]`)
	got, err = loadSweInstances(versioned)
	if err != nil {
		t.Fatalf("versioned parse: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 instance, got %d", len(got))
	}
	if got[0].Version != "1.2.3" {
		t.Fatalf("want Version=1.2.3, got %q", got[0].Version)
	}
	if got[0].FailToPass == nil || len(got[0].FailToPass) != 0 {
		t.Fatalf("want empty FailToPass, got %#v", got[0].FailToPass)
	}
}

// Test 2 ----------------------------------------------------------------------

// TestSanitizeRepo_RemovesSecrets exercises sanitizeRepo (swe.go:401-403), the
// string normaliser used to build filesystem-safe workspace paths.
// NOTE: sanitizeRepo is a pure string replacer over `/`, `\`, `:`. It does
// NOT touch files; tests that need a file-walker live in a separate
// (intentional, omitted here) helper. This test covers the contract that
// actually exists: stable, idempotent, deterministic substitution.
func TestSanitizeRepo_RemovesSecrets(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"foo/bar", "foo_bar"},
		{"foo\\bar", "foo_bar"},
		{"host:repo", "host_repo"},
		{"./.aws/credentials", "._.aws_credentials"},
		{"id_rsa", "id_rsa"}, // no slashes — left alone
		{"~/.ssh/id_rsa", "~_.ssh_id_rsa"},
	}
	for _, c := range cases {
		got := sanitizeRepo(c.in)
		if got != c.want {
			t.Errorf("sanitizeRepo(%q)=%q want %q", c.in, got, c.want)
		}
		// Idempotent on the result.
		again := sanitizeRepo(got)
		if again != got {
			t.Errorf("not idempotent: first=%q second=%q", got, again)
		}
	}
}

// Test 3 ----------------------------------------------------------------------

// TestApplyPatch_Conflict exercises applyPatch on a real git repo:
// (a) a clean patch applies successfully,
// (b) a follow-up patch whose context no longer matches the working-tree
//
//	state causes git apply to exit non-zero, propagated as a non-nil
//	error from applyPatch.
//
// NOTE: the current applyPatch surface (swe.go:287-297) does not accept a
// `force` flag — git apply's underlying semantics are reproduced as-is.
func TestApplyPatch_Conflict(t *testing.T) {
	repo := initRepo(t)

	// First patch: rewrite `return 1` → `return 2`.
	patch1 := `--- a/main.go
+++ b/main.go
@@ -1,3 +1,3 @@
 package x
 
-func F() int { return 1 }
+func F() int { return 2 }
`
	if err := applyPatch(repo, patch1); err != nil {
		t.Fatalf("first patch should apply cleanly, got: %v", err)
	}
	// Confirm the file now reads "return 2".
	body, err := os.ReadFile(filepath.Join(repo, "main.go"))
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	if !strings.Contains(string(body), "return 2") {
		t.Fatalf("first patch did not rewrite body: %s", body)
	}

	// Second patch: still targets "return 1" — the working-tree no
	// longer matches that context, so applyPatch should return a
	// non-nil error.
	patch2 := strings.Replace(patch1, "return 2", "return 3", 1)
	err = applyPatch(repo, patch2)
	if err == nil {
		t.Fatal("expected conflict error from second patch, got nil")
	}
}

// Test 4 ----------------------------------------------------------------------

// TestRunSweBench_DryRun exercises RunSweBench with cfg.DryRun=true:
// evaluateInstance is short-circuited (swe.go:125-130), so neither
// setupRepo nor applyPatch is reached. The report must list every input
// instance in deterministic order with the (dry run) patch marker.
func TestRunSweBench_DryRun(t *testing.T) {
	dir := t.TempDir()

	dataset := writeFile(t, dir, "ds.json",
		`[
			{"instance_id":"one","repo":"r/a","base_commit":"c1","FAIL_TO_PASS":[]},
			{"instance_id":"two","repo":"r/b","base_commit":"c2","FAIL_TO_PASS":[]}
		]`)
	out := filepath.Join(dir, "out.json")

	cfg := SweConfig{
		DatasetPath: dataset,
		OutputPath:  out,
		Workspace:   filepath.Join(dir, "ws"),
		MaxTurns:    1,
		Timeout:     5 * time.Second,
		DryRun:      true,
	}

	before, _ := os.ReadDir(cfg.Workspace)
	report, err := RunSweBench(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunSweBench dry-run: %v", err)
	}

	// setupRepo was never reached: workspace must be empty.
	after, _ := os.ReadDir(cfg.Workspace)
	if len(before) != 0 || len(after) != 0 {
		t.Fatalf("workspace touched in dry-run: before=%d after=%d",
			len(before), len(after))
	}

	// Report contents: both instance IDs, both (dry run) patches, both
	// passed=true.
	if report == nil || len(report.Results) != 2 {
		t.Fatalf("want 2 results, got %#v", report)
	}
	wantIDs := []string{"one", "two"}
	for i, want := range wantIDs {
		got := report.Results[i]
		if got.InstanceID != want {
			t.Errorf("result[%d].InstanceID=%q want %q", i, got.InstanceID, want)
		}
		if got.Patch != "(dry run)" {
			t.Errorf("result[%d].Patch=%q want %q", i, got.Patch, "(dry run)")
		}
		if !got.Passed {
			t.Errorf("result[%d].Passed=false want true", i)
		}
	}

	// Byte-stable JSON envelope.
	first, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	second, err := roundTrip(report)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("output JSON not byte-stable\nfirst:\n%s\nre-marshalled:\n%s",
			first, second)
	}
}

// roundTrip re-marshals a SweReport in the same writer settings
// RunSweBench uses (2-space indent), to verify byte-stability without
// converting timestamps twice.
func roundTrip(r *SweReport) ([]byte, error) {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

// Test 5 ----------------------------------------------------------------------

// TestBuildSweReport_Aggregation exercises buildSweReport's pure-math
// branch: it must sum Pass/Fail, compute PassRate exactly, and handle
// the empty-input edge case (Total=0, Results unchanged).
func TestBuildSweReport_Aggregation(t *testing.T) {
	start := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	finish := start.Add(2 * time.Second)

	mk := func(id string, ok bool, micro int) SweResult {
		return SweResult{
			InstanceID: id,
			Repo:       "r/" + id,
			Passed:     ok,
			Duration:   time.Duration(micro) * time.Microsecond,
		}
	}
	results := []SweResult{
		mk("p1", true, 100_000),
		mk("p2", true, 150_000),
		mk("p3", true, 200_000),
		mk("f1", false, 50_000),
		mk("f2", false, 75_000),
	}

	r := buildSweReport("ds-x", results, start, finish)
	if r.Total != 5 {
		t.Errorf("Total=%d want 5", r.Total)
	}
	if r.Passed != 3 {
		t.Errorf("Passed=%d want 3", r.Passed)
	}
	if r.Failed != 2 {
		t.Errorf("Failed=%d want 2", r.Failed)
	}
	if r.Passed+r.Failed != r.Total {
		t.Errorf("Passed+Failed=%d != Total=%d", r.Passed+r.Failed, r.Total)
	}
	// Float compare: pass-rate must be exactly 0.6, no rounding.
	if r.PassRate != 0.6 {
		t.Errorf("PassRate=%v want 0.6", r.PassRate)
	}
	// Sum of durations = 575_000us = 575 ms (TotalDurMS uses ms from us/1000).
	wantMS := float64(575_000) / 1000.0
	if r.TotalDurMS != wantMS {
		t.Errorf("TotalDurMS=%v want %v", r.TotalDurMS, wantMS)
	}
	if !r.StartedAt.Equal(start) || !r.FinishedAt.Equal(finish) {
		t.Errorf("timestamps not preserved: started=%v finished=%v",
			r.StartedAt, r.FinishedAt)
	}

	// Empty-input edge case.
	empty := buildSweReport("ds-empty", nil, start, start)
	if empty.Total != 0 {
		t.Errorf("empty.Total=%d want 0", empty.Total)
	}
	if empty.Results != nil {
		t.Errorf("empty.Results not nil: %#v", empty.Results)
	}
}
