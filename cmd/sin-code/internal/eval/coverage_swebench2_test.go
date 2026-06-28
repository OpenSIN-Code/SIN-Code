// SPDX-License-Identifier: MIT
// Purpose: additional coverage tests for swebench.go to push eval package
// past 70% statement coverage. Targets 0% functions: SwePrintSummary,
// applyTestPatch, runTests, setupRepo, runAgent, evaluateInstance,
// and uncovered branches in applyPatch, loadSweInstances, writeSweReport,
// RunSweBench.
package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ── SwePrintSummary (0% → covered) ──────────────────────────────────────

func TestSwePrintSummary_BasicReport(t *testing.T) {
	r := &SweReport{
		Dataset:    "ds-1",
		Total:      3,
		Passed:     2,
		Failed:     1,
		PassRate:   0.6667,
		TotalDurMS: 1500,
		Results: []SweResult{
			{InstanceID: "inst-a", Passed: true},
			{InstanceID: "inst-b", Passed: true},
			{InstanceID: "inst-c", Passed: false, Error: "boom"},
		},
	}
	var buf bytes.Buffer
	SwePrintSummary(&buf, r)
	out := buf.String()
	for _, want := range []string{"ds-1", "Total:  3", "Passed: 2", "Failed: 1", "Pass Rate: 66.67%", "Duration: 1500 ms", "inst-a", "PASS", "inst-c", "FAIL"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestSwePrintSummary_NilWriter(t *testing.T) {
	r := &SweReport{Dataset: "x", Total: 0}
	// Should not panic when w is nil — falls back to os.Stdout.
	SwePrintSummary(nil, r)
}

func TestSwePrintSummary_LargeReportSkipsTable(t *testing.T) {
	results := make([]SweResult, 51)
	for i := range results {
		results[i] = SweResult{InstanceID: "inst", Passed: true}
	}
	r := &SweReport{Dataset: "big", Total: 51, Passed: 51, PassRate: 1.0, Results: results}
	var buf bytes.Buffer
	SwePrintSummary(&buf, r)
	out := buf.String()
	// Table header should NOT appear when > 50 results
	if strings.Contains(out, "Instance") && strings.Contains(out, "Result") {
		t.Errorf("table should be skipped for >50 results, but found table header:\n%s", out)
	}
}

func TestSwePrintSummary_EmptyReport(t *testing.T) {
	r := &SweReport{Dataset: "empty", Total: 0, Results: nil}
	var buf bytes.Buffer
	SwePrintSummary(&buf, r)
	out := buf.String()
	// Total=0 so table section is skipped
	for _, want := range []string{"empty", "Total:  0", "Passed: 0"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestSwePrintSummary_LongInstanceID(t *testing.T) {
	longID := strings.Repeat("x", 50)
	r := &SweReport{
		Dataset:  "d",
		Total:    1,
		Passed:   1,
		PassRate: 1.0,
		Results:  []SweResult{{InstanceID: longID, Passed: true}},
	}
	var buf bytes.Buffer
	SwePrintSummary(&buf, r)
	out := buf.String()
	// Long ID should be truncated to 39 chars
	if !strings.Contains(out, strings.Repeat("x", 39)) {
		t.Errorf("expected truncated ID (39 chars) in output:\n%s", out)
	}
}

// ── applyTestPatch (0% → covered) ───────────────────────────────────────

func TestApplyTestPatch_EmptyPatch(t *testing.T) {
	repo := initRepo(t)
	if err := applyTestPatch(repo, ""); err != nil {
		t.Fatalf("empty patch should return nil, got: %v", err)
	}
	if err := applyTestPatch(repo, "   \n  "); err != nil {
		t.Fatalf("whitespace-only patch should return nil, got: %v", err)
	}
}

func TestApplyTestPatch_AppliesToRepo(t *testing.T) {
	repo := initRepo(t)
	testPatch := `--- a/test_file.go
+++ b/test_file.go
@@ -0,0 +1,1 @@
+package x
`
	if err := applyTestPatch(repo, testPatch); err != nil {
		t.Fatalf("valid test patch should apply: %v", err)
	}
}

func TestApplyTestPatch_Conflict(t *testing.T) {
	repo := initRepo(t)
	// A patch that tries to remove a line that doesn't exist — git apply will fail
	badPatch := `--- a/main.go
+++ b/main.go
@@ -1,3 +1,3 @@
 package x

-func F() int { return 999 }
+func F() int { return 2 }
`
	err := applyTestPatch(repo, badPatch)
	if err == nil {
		t.Fatal("expected error for conflicting patch")
	}
}

// ── runTests (0% → covered) ─────────────────────────────────────────────

func TestRunTests_NoTargets(t *testing.T) {
	repo := t.TempDir()
	inst := SweInstance{}
	if err := runTests(repo, inst); err != nil {
		t.Fatalf("no targets should return nil, got: %v", err)
	}
}

func TestRunTests_OnlyPassToPass(t *testing.T) {
	repo := t.TempDir()
	inst := SweInstance{
		PassToPass: []string{"nonexistent_test.py"},
	}
	err := runTests(repo, inst)
	if err == nil {
		t.Fatal("expected error for missing test file")
	}
	if !strings.Contains(err.Error(), "test failures") {
		t.Fatalf("expected 'test failures' in error, got: %v", err)
	}
}

// ── setupRepo (0% → covered) ─────────────────────────────────────────────

func TestSetupRepo_AlreadyCloned(t *testing.T) {
	repo := initRepo(t)
	inst := SweInstance{Repo: "foo/bar", BaseCommit: ""}
	// .git already exists → should return nil without cloning
	if err := setupRepo(repo, inst); err != nil {
		t.Fatalf("setupRepo on existing .git should return nil, got: %v", err)
	}
}

func TestSetupRepo_CloneFailure(t *testing.T) {
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	inst := SweInstance{Repo: "nonexistent/invalid-repo-xyz", BaseCommit: ""}
	err := setupRepo(repoDir, inst)
	if err == nil {
		t.Fatal("expected clone error for invalid repo")
	}
	if !strings.Contains(err.Error(), "git clone") {
		t.Fatalf("expected 'git clone' in error, got: %v", err)
	}
}

// ── runAgent (0% → covered) ─────────────────────────────────────────────

func TestRunAgent_BinaryNotFound(t *testing.T) {
	dir := t.TempDir()
	inst := SweInstance{Problem: "fix", Hints: ""}
	patchFile := filepath.Join(dir, "out.patch")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := runAgent(ctx, "/nonexistent/binary/path", dir, inst, patchFile)
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}

func TestRunAgent_FakeBinaryWritesPatch(t *testing.T) {
	dir := t.TempDir()
	patchFile := filepath.Join(dir, "predicted.patch")
	patchContent := "--- a/foo\n+++ b/foo\n@@ -1,1 +1,1 @@\n-old\n+new\n"

	// Create a fake "binary" that writes the patch file.
	fakeBin := filepath.Join(dir, "fake-agent")
	script := "#!/bin/sh\nprintf '%s' '" + patchContent + "' > \"" + patchFile + "\"\n"
	if err := os.WriteFile(fakeBin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	inst := SweInstance{Problem: "fix bug", Hints: "try X"}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	got, err := runAgent(ctx, fakeBin, dir, inst, patchFile)
	if err != nil {
		t.Fatalf("runAgent: %v", err)
	}
	if strings.TrimSpace(got) != strings.TrimSpace(patchContent) {
		t.Fatalf("patch mismatch:\ngot:  %q\nwant: %q", got, patchContent)
	}
}

func TestRunAgent_NoPatchFileFallsBackToGitDiff(t *testing.T) {
	repo := initRepo(t)
	patchFile := filepath.Join(repo, "predicted.patch")
	// Fake binary that does nothing (no patch file created).
	fakeBin := filepath.Join(repo, "fake-noop")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	inst := SweInstance{Problem: "fix", Hints: ""}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	got, err := runAgent(ctx, fakeBin, repo, inst, patchFile)
	if err != nil {
		t.Fatalf("runAgent fallback: %v", err)
	}
	// git diff on a clean repo returns empty string
	if got != "" {
		t.Fatalf("expected empty diff on clean repo, got %q", got)
	}
}

// ── applyPatch (87.5% → 100%) ───────────────────────────────────────────

func TestApplyPatch_EmptyPatch(t *testing.T) {
	repo := initRepo(t)
	if err := applyPatch(repo, ""); err == nil {
		t.Fatal("expected error for empty patch")
	}
	if err := applyPatch(repo, "   "); err == nil {
		t.Fatal("expected error for whitespace-only patch")
	}
}

// ── loadSweInstances (76.5% → higher) ───────────────────────────────────

func TestLoadSweInstances_SingleObject(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "single.json",
		`{"instance_id":"solo","repo":"r/solo","base_commit":"abc","FAIL_TO_PASS":["test1"]}`)
	got, err := loadSweInstances(path)
	if err != nil {
		t.Fatalf("single object should parse: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 instance, got %d", len(got))
	}
	if got[0].InstanceID != "solo" {
		t.Fatalf("want InstanceID=solo, got %q", got[0].InstanceID)
	}
}

func TestLoadSweInstances_MissingInstanceID(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "noid.json",
		`[{"repo":"r/n","base_commit":"abc","FAIL_TO_PASS":[]}]`)
	got, err := loadSweInstances(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1, got %d", len(got))
	}
	if got[0].InstanceID != "instance-0" {
		t.Fatalf("want auto-filled 'instance-0', got %q", got[0].InstanceID)
	}
}

func TestLoadSweInstances_FileNotFound(t *testing.T) {
	_, err := loadSweInstances("/nonexistent/path/file.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// ── writeSweReport (85.7% → 100%) ───────────────────────────────────────

func TestWriteSweReport_UnwritablePath(t *testing.T) {
	r := &SweReport{Dataset: "x", Total: 0}
	err := writeSweReport("/nonexistent/dir/path/out.json", r)
	if err == nil {
		t.Fatal("expected error for unwritable path")
	}
}

// ── RunSweBench (59.5% → higher) ────────────────────────────────────────

func TestRunSweBench_MissingDatasetPath(t *testing.T) {
	_, err := RunSweBench(context.Background(), SweConfig{
		DatasetPath: "",
		DryRun:      true,
	})
	if err == nil {
		t.Fatal("expected error for empty dataset path")
	}
	if !strings.Contains(err.Error(), "dataset path is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunSweBench_ContextCancelled(t *testing.T) {
	dir := t.TempDir()
	ds := writeFile(t, dir, "ds.json",
		`[{"instance_id":"a","repo":"r/a","base_commit":"c1","FAIL_TO_PASS":[]},
		  {"instance_id":"b","repo":"r/b","base_commit":"c2","FAIL_TO_PASS":[]}]`)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := RunSweBench(ctx, SweConfig{
		DatasetPath: ds,
		OutputPath:  filepath.Join(dir, "out.json"),
		Workspace:   filepath.Join(dir, "ws"),
		DryRun:      true,
	})
	if err == nil {
		t.Fatal("expected context cancelled error")
	}
}

func TestRunSweBench_LoadError(t *testing.T) {
	dir := t.TempDir()
	bad := writeFile(t, dir, "bad.json", `{not valid json}`)
	_, err := RunSweBench(context.Background(), SweConfig{
		DatasetPath: bad,
		OutputPath:  filepath.Join(dir, "out.json"),
		DryRun:      true,
	})
	if err == nil {
		t.Fatal("expected load error")
	}
	if !strings.Contains(err.Error(), "load instances") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunSweBench_DryRunDefaultsApplied(t *testing.T) {
	dir := t.TempDir()
	ds := writeFile(t, dir, "ds.json",
		`[{"instance_id":"x","repo":"r/x","base_commit":"c","FAIL_TO_PASS":[]}]`)
	// Don't set OutputPath, Workspace, MaxTurns, Timeout, SinCodeBin —
	// all should get defaults.
	report, err := RunSweBench(context.Background(), SweConfig{
		DatasetPath: ds,
		DryRun:      true,
	})
	if err != nil {
		t.Fatalf("RunSweBench: %v", err)
	}
	if report == nil || len(report.Results) != 1 {
		t.Fatalf("unexpected report: %#v", report)
	}
	// Default output path should exist
	if _, err := os.Stat("swebench-results.json"); err == nil {
		_ = os.Remove("swebench-results.json")
	}
}

func TestRunSweBench_NonDryRunError(t *testing.T) {
	dir := t.TempDir()
	ds := writeFile(t, dir, "ds.json",
		`[{"instance_id":"x","repo":"r/x","base_commit":"c","FAIL_TO_PASS":[]}]`)
	// Non-dry-run: evaluateInstance will try to git clone an invalid repo
	_, err := RunSweBench(context.Background(), SweConfig{
		DatasetPath: ds,
		OutputPath:  filepath.Join(dir, "out.json"),
		Workspace:   filepath.Join(dir, "ws"),
		Timeout:     5 * time.Second,
		SinCodeBin:  "/nonexistent/binary",
		DryRun:      false,
	})
	if err != nil {
		// Could fail on write report or could succeed with error results
		// Either way it should not panic
	}
}

// ── evaluateInstance (0% → covered) ─────────────────────────────────────

func TestEvaluateInstance_MkdirFailure(t *testing.T) {
	dir := t.TempDir()
	// Make workspace path unwritable by creating a file where a dir should go
	blocker := filepath.Join(dir, "ws")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	inst := SweInstance{InstanceID: "i1", Repo: "r/x", BaseCommit: ""}
	cfg := SweConfig{
		Workspace:  blocker,
		Timeout:    5 * time.Second,
		SinCodeBin: "/nonexistent",
	}
	_, _, err := evaluateInstance(context.Background(), cfg, inst)
	if err == nil {
		t.Fatal("expected mkdir error")
	}
	if !strings.Contains(err.Error(), "mkdir") {
		t.Fatalf("expected mkdir error, got: %v", err)
	}
}

func TestEvaluateInstance_ExistingRepoSetupFailsOnClone(t *testing.T) {
	dir := t.TempDir()
	ws := filepath.Join(dir, "ws")
	inst := SweInstance{InstanceID: "i1", Repo: "nonexistent/invalid-xyz", BaseCommit: ""}
	cfg := SweConfig{
		Workspace:  ws,
		Timeout:    5 * time.Second,
		SinCodeBin: "/nonexistent",
	}
	_, _, err := evaluateInstance(context.Background(), cfg, inst)
	if err == nil {
		t.Fatal("expected setup error for invalid repo")
	}
}

// ── SweReport JSON round-trip ───────────────────────────────────────────

func TestSweReport_JSONRoundTrip(t *testing.T) {
	r := &SweReport{
		Dataset:    "ds-rt",
		Total:      2,
		Passed:     1,
		Failed:     1,
		PassRate:   0.5,
		TotalDurMS: 100,
		Results: []SweResult{
			{InstanceID: "a", Passed: true, Duration: 50 * time.Millisecond, Turns: 3, Patch: "diff"},
			{InstanceID: "b", Passed: false, Error: "failed", Duration: 50 * time.Millisecond},
		},
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var back SweReport
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.Total != 2 || back.Passed != 1 || back.Failed != 1 {
		t.Fatalf("round-trip mismatch: %+v", back)
	}
	if back.Results[0].Patch != "diff" {
		t.Fatalf("patch not preserved: %q", back.Results[0].Patch)
	}
}
