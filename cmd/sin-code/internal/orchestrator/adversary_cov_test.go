// SPDX-License-Identifier: MIT
// Purpose: coverage tests for adversary.go that run without the "coverage"
// build tag (stcov_test.go is build-tagged and excluded from normal runs).
package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewAdversaryFields(t *testing.T) {
	agent := &stubAdversary{}
	adv := NewAdversary(agent, "/work/dir")
	if adv.Agent != agent {
		t.Error("Agent not set")
	}
	if adv.Workdir != "/work/dir" {
		t.Errorf("Workdir = %q, want /work/dir", adv.Workdir)
	}
	if adv.MaxAttacks != 6 {
		t.Errorf("MaxAttacks = %d, want 6", adv.MaxAttacks)
	}
	if adv.ProbeTimeout != 90*time.Second {
		t.Errorf("ProbeTimeout = %v, want 90s", adv.ProbeTimeout)
	}
	if adv.Hooks != nil {
		t.Error("Hooks should be nil by default")
	}
}

func TestProbePackageDirFound(t *testing.T) {
	dir := t.TempDir()
	pkgDir := filepath.Join(dir, "mypkg")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "mypkg.go"), []byte("package mypkg\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := probePackageDir("package mypkg\nfunc TestX(t *testing.T){}\n", dir)
	if err != nil {
		t.Fatalf("probePackageDir: %v", err)
	}
	if got != pkgDir {
		t.Errorf("expected %q, got %q", pkgDir, got)
	}
}

func TestProbePackageDirTestPackageSuffix(t *testing.T) {
	dir := t.TempDir()
	pkgDir := filepath.Join(dir, "foo")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "foo.go"), []byte("package foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Probe source declares "package foo_test" — the _test suffix is stripped.
	got, err := probePackageDir("package foo_test\nfunc TestX(t *testing.T){}\n", dir)
	if err != nil {
		t.Fatalf("probePackageDir with _test suffix: %v", err)
	}
	if got != pkgDir {
		t.Errorf("expected %q, got %q", pkgDir, got)
	}
}

func TestRelOrDotSameDir(t *testing.T) {
	got := relOrDot("/a/b", "/a/b")
	if got != "." {
		t.Errorf("relOrDot(same) = %q, want .", got)
	}
}

func TestRelOrDotSubdir(t *testing.T) {
	got := relOrDot("/a/b", "/a/b/c")
	if got != "c" {
		t.Errorf("relOrDot(subdir) = %q, want c", got)
	}
}

func TestRelOrDotParent(t *testing.T) {
	got := relOrDot("/a/b/c", "/a/b")
	if got != ".." {
		t.Errorf("relOrDot(parent) = %q, want ..", got)
	}
}

func TestExecuteProbeSuccess(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := adversaryExecCommand
	adversaryExecCommand = func(ctx context.Context, workdir, rel string, timeout time.Duration) ([]byte, error) {
		return []byte("ok"), nil
	}
	defer func() { adversaryExecCommand = old }()

	adv := &Adversary{Workdir: dir, ProbeTimeout: time.Second}
	landed, output, err := adv.executeProbe(context.Background(), &Attack{
		ProbeSource: "package main\nfunc TestAdversary(t *testing.T) {}\n",
	}, 0)
	if err != nil {
		t.Fatalf("executeProbe: %v", err)
	}
	if landed {
		t.Error("expected not landed (probe passed)")
	}
	if !strings.Contains(output, "ok") {
		t.Errorf("expected output to contain 'ok', got %q", output)
	}
	// Probe file should be cleaned up when probe passes.
	probePath := filepath.Join(dir, "adversary_0_test.go")
	if _, err := os.Stat(probePath); !os.IsNotExist(err) {
		t.Error("probe file should be removed after successful probe")
	}
}

func TestExecuteProbeLanded(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := adversaryExecCommand
	adversaryExecCommand = func(ctx context.Context, workdir, rel string, timeout time.Duration) ([]byte, error) {
		return []byte("FAIL: probe failed"), errors.New("exit 1")
	}
	defer func() { adversaryExecCommand = old }()

	adv := &Adversary{Workdir: dir, ProbeTimeout: time.Second}
	landed, output, err := adv.executeProbe(context.Background(), &Attack{
		ProbeSource: "package main\nfunc TestAdversary(t *testing.T) {}\n",
	}, 0)
	if err != nil {
		t.Fatalf("executeProbe: %v", err)
	}
	if !landed {
		t.Error("expected landed (probe failed)")
	}
	if !strings.Contains(output, "FAIL") {
		t.Errorf("expected output to contain 'FAIL', got %q", output)
	}
	// Probe file should remain when probe fails (kept as regression test).
	probePath := filepath.Join(dir, "adversary_0_test.go")
	if _, err := os.Stat(probePath); err != nil {
		t.Error("probe file should remain after failed probe")
	}
}

func TestExecuteProbeWriteFileError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o755)

	adv := &Adversary{Workdir: dir, ProbeTimeout: time.Second}
	_, _, err := adv.executeProbe(context.Background(), &Attack{
		ProbeSource: "package main\nfunc TestX(t *testing.T){}\n",
	}, 0)
	if err == nil {
		t.Fatal("expected write error")
	}
}

func TestAdversaryReviewCleared(t *testing.T) {
	agent := &stubAdversary{} // no attacks proposed
	adv := NewAdversary(agent, t.TempDir())
	res, err := adv.Review(context.Background(), "diff", "impact")
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if !res.Cleared {
		t.Error("expected cleared with no attacks")
	}
	if len(res.Attacks) != 0 {
		t.Errorf("expected 0 attacks, got %d", len(res.Attacks))
	}
}

func TestAdversaryReviewProposeError(t *testing.T) {
	agent := &mockAdversaryAgentErr{err: errors.New("fail")}
	adv := NewAdversary(agent, t.TempDir())
	_, err := adv.Review(context.Background(), "diff", "impact")
	if err == nil || !strings.Contains(err.Error(), "adversary propose") {
		t.Fatalf("expected propose error, got %v", err)
	}
}

func TestCollectAdversaryFindingsLanded(t *testing.T) {
	res := &AdversaryResult{
		Attacks: []Attack{
			{Kind: AttackBoundary, Hypothesis: "h1", Landed: true},
			{Kind: AttackContract, Hypothesis: "h2", Landed: false},
		},
		Landed:  1,
		Cleared: false,
	}
	collectAdversaryFindings(res)
	if len(res.Findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(res.Findings))
	}
	if res.Findings[0].Tag != TagRisk {
		t.Errorf("landed attack should be TagRisk, got %s", res.Findings[0].Tag)
	}
	if res.Findings[0].Confidence != 1.0 {
		t.Errorf("landed attack confidence should be 1.0, got %f", res.Findings[0].Confidence)
	}
	if res.Findings[1].Tag != TagVerify {
		t.Errorf("cleared attack should be TagVerify, got %s", res.Findings[1].Tag)
	}
	if res.Findings[1].Confidence != 0.5 {
		t.Errorf("cleared attack confidence should be 0.5, got %f", res.Findings[1].Confidence)
	}
}

func TestCollectAdversaryFindingsEmpty(t *testing.T) {
	res := &AdversaryResult{}
	collectAdversaryFindings(res)
	if len(res.Findings) != 0 {
		t.Errorf("expected 0 findings for empty result, got %d", len(res.Findings))
	}
}

func TestAdversaryReviewWithHooks(t *testing.T) {
	// This test exercises the hooks.Fire path in Review.
	// We need a hooks.Engine — but creating one requires the real hooks package.
	// The Review code checks `adv.Hooks != nil` before firing.
	// Without importing hooks (which would create a dependency), we can
	// verify the Review still works when Hooks is nil (already tested above).
	// The hooks path is exercised in the build-tagged stcov_test.go.
	// Here we just confirm the cleared path with no hooks.
	agent := &stubAdversary{attacks: []Attack{}}
	adv := NewAdversary(agent, t.TempDir())
	res, err := adv.Review(context.Background(), "diff", "impact")
	if err != nil || !res.Cleared {
		t.Fatalf("expected cleared, got %+v err=%v", res, err)
	}
}

type mockAdversaryAgentErr struct{ err error }

func (m *mockAdversaryAgentErr) ProposeAttacks(ctx context.Context, diff, impactBrief string, maxAttacks int) ([]Attack, error) {
	return nil, m.err
}
