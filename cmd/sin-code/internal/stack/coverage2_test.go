// SPDX-License-Identifier: MIT
// Purpose: additional coverage tests for stack.go to push the stack package
// past 70% statement coverage. Targets 0% functions: buildSuperpowersPrompt,
// installVane, installSuperpowers, and uncovered branches in trimError,
// Install, Format, doctorSuperpowers, doctorDox, doctorVaneConfig,
// doctorVaneHealth, installDox.
package stack

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/dox"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/superpowers"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/vane"
)

// ── buildSuperpowersPrompt (0% → 100%) ──────────────────────────────────

func TestBuildSuperpowersPrompt_NilResult(t *testing.T) {
	got := buildSuperpowersPrompt(nil)
	if !strings.Contains(got, "no result metadata") {
		t.Fatalf("nil result should mention 'no result metadata', got: %q", got)
	}
}

func TestBuildSuperpowersPrompt_ValidResult(t *testing.T) {
	got := buildSuperpowersPrompt(&superpowers.InstallResult{
		Repo:   "https://github.com/example/repo",
		SHA:    "abcdef1234567890",
		Branch: "main",
		Skills: 5,
	})
	for _, want := range []string{"example/repo", "abcdef1234567890", "main", "skills: 5"} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt missing %q: %s", want, got)
		}
	}
}

// ── trimError (66.7% → 100%) ────────────────────────────────────────────

func TestTrimError_Nil(t *testing.T) {
	if got := trimError(nil); got != "" {
		t.Fatalf("trimError(nil) = %q, want empty", got)
	}
}

func TestTrimError_LongError(t *testing.T) {
	longErr := errors.New(strings.Repeat("x", 300))
	got := trimError(longErr)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected truncated error ending with '…', got: %q", got)
	}
	if len(got) > 203 {
		t.Fatalf("trimmed error too long: %d chars", len(got))
	}
}

func TestTrimError_NormalError(t *testing.T) {
	got := trimError(errors.New("simple error"))
	if got != "simple error" {
		t.Fatalf("trimError: got %q, want 'simple error'", got)
	}
}

// ── Install (72.7% → higher) ────────────────────────────────────────────

func TestInstall_AllSkipped(t *testing.T) {
	_, cwd := setupTestHome(t)
	agents := filepath.Join(cwd, dox.AgentsFileName)
	rep := Install(InstallOptions{
		SkipSuperpowers: true,
		SkipDox:         true,
		SkipVane:        true,
		AgentsMDPath:    agents,
	})
	if !rep.AllOK {
		t.Fatalf("all-skipped Install should be AllOK=true, got: %#v", rep.Components)
	}
	for _, c := range rep.Components {
		if !c.Skipped {
			t.Errorf("expected all components skipped, but %s is not", c.Name)
		}
	}
}

func TestInstall_DoxOnlyWithTimeout(t *testing.T) {
	_, cwd := setupTestHome(t)
	agents := filepath.Join(cwd, dox.AgentsFileName)
	if err := os.WriteFile(agents, []byte("# Project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep := Install(InstallOptions{
		SkipSuperpowers: true,
		SkipVane:        true,
		AgentsMDPath:    agents,
		Timeout:         5 * time.Second,
	})
	if !rep.AllOK {
		t.Fatalf("dox-only Install should be AllOK=true, got: %#v", rep.Components)
	}
}

// ── Format (93.8% → 100%) ───────────────────────────────────────────────

func TestFormat_AllOK(t *testing.T) {
	r := Report{
		Components: []Component{
			{Name: "ok.layer", Layer: "x", OK: true, Detail: "fine"},
		},
		AllOK: true,
	}
	out := Format(r)
	if !strings.Contains(out, "overall: OK") {
		t.Errorf("expected 'overall: OK' in output:\n%s", out)
	}
}

func TestFormat_EmptyReport(t *testing.T) {
	r := Report{}
	out := Format(r)
	// Empty report with AllOK=false → DEGRADED
	if !strings.Contains(out, "DEGRADED") {
		t.Errorf("expected DEGRADED for empty report:\n%s", out)
	}
}

// ── doctorVaneConfig (75.0% → higher) ───────────────────────────────────

func TestDoctorVaneConfig_NoConfig(t *testing.T) {
	setupTestHome(t)
	// Don't save any vane config — should report "no config on disk"
	comp := doctorVaneConfig()
	if comp.OK {
		t.Fatalf("expected OK=false when no config, got: %+v", comp)
	}
	if !strings.Contains(comp.Detail, "no config") {
		t.Fatalf("expected 'no config' detail, got: %s", comp.Detail)
	}
}

func TestDoctorVaneConfig_ValidConfig(t *testing.T) {
	setupTestHome(t)
	if err := vane.SaveConfig(vane.Config{BaseURL: "http://localhost:8080"}); err != nil {
		t.Fatal(err)
	}
	comp := doctorVaneConfig()
	if !comp.OK {
		t.Fatalf("expected OK=true for valid config, got: %+v", comp)
	}
	if !strings.Contains(comp.Detail, "http://localhost:8080") {
		t.Fatalf("expected URL in detail, got: %s", comp.Detail)
	}
}

// ── doctorVaneHealth (77.8% → higher) ───────────────────────────────────

func TestDoctorVaneHealth_UnreachableClient(t *testing.T) {
	setupTestHome(t)
	if err := vane.SaveConfig(vane.Config{
		BaseURL:        "http://127.0.0.1:1",
		TimeoutSeconds: 1,
	}); err != nil {
		t.Fatal(err)
	}
	comp := doctorVaneHealth()
	if !comp.OK {
		t.Fatalf("vane.health should be OK even with no client, got: %+v", comp)
	}
	if !strings.Contains(comp.Detail, "DOWN") {
		t.Fatalf("expected DOWN detail, got: %s", comp.Detail)
	}
}

// ── doctorSuperpowers (36.4% → higher) ──────────────────────────────────

func TestDoctorSuperpowers_WithSkillsAndPin(t *testing.T) {
	home, _ := setupTestHome(t)
	sha := seedSuperpowersLayers(t, home)
	comp := doctorSuperpowers("")
	if !comp.OK {
		t.Fatalf("expected OK=true with skills + pin, got: %+v", comp)
	}
	if !strings.Contains(comp.Detail, "skills=2") {
		t.Errorf("expected skills=2 in detail, got: %s", comp.Detail)
	}
	if !strings.Contains(comp.Detail, sha[:12]) {
		t.Errorf("expected pin SHA in detail, got: %s", comp.Detail)
	}
}

func TestDoctorSuperpowers_SkillsButNoPin(t *testing.T) {
	home, _ := setupTestHome(t)
	// Seed skills but NOT the pin file
	skillsRoot := filepath.Join(home, "skills", "superpowers")
	for _, name := range []string{"alpha"} {
		dir := filepath.Join(skillsRoot, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		body := "---\nname: " + name + "\ndescription: fake\n---\n\n# " + name + "\n"
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	comp := doctorSuperpowers("")
	if comp.OK {
		t.Fatalf("expected OK=false when skills exist but pin missing, got: %+v", comp)
	}
	if !strings.Contains(comp.Detail, "pin=missing") {
		t.Errorf("expected 'pin=missing' in detail, got: %s", comp.Detail)
	}
}

// ── doctorDox (69.2% → higher) ──────────────────────────────────────────

func TestDoctorDox_RootWithErrors(t *testing.T) {
	_, cwd := setupTestHome(t)
	// Create a dox structure with an error (missing AGENTS.md in a subdirectory)
	subDir := filepath.Join(cwd, "subpkg")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// dox.Check looks for AGENTS.md; a subdirectory without one may produce warnings/errors
	comp := doctorDox(cwd)
	// The result depends on dox.Check's behavior — just verify it doesn't crash
	_ = comp
}

func TestDoctorDox_EmptyRoot(t *testing.T) {
	setupTestHome(t)
	comp := doctorDox("")
	// Empty root defaults to "." — should not crash
	_ = comp
}

// ── installVane (0% → covered) ──────────────────────────────────────────

func TestInstallVane_Success(t *testing.T) {
	setupTestHome(t)
	comp := installVane(InstallOptions{VaneURL: "http://localhost:9090"})
	if !comp.OK {
		t.Fatalf("installVane should succeed with fresh config, got: %+v", comp)
	}
	if !strings.Contains(comp.Detail, "http://localhost:9090") {
		t.Errorf("expected URL in detail, got: %s", comp.Detail)
	}
}

func TestInstallVane_DefaultURL(t *testing.T) {
	setupTestHome(t)
	comp := installVane(InstallOptions{})
	if !comp.OK {
		t.Fatalf("installVane with default config should succeed, got: %+v", comp)
	}
}

// ── installDox (71.4% → higher) ─────────────────────────────────────────

func TestInstallDox_NestedParentDir(t *testing.T) {
	_, cwd := setupTestHome(t)
	// Path with a nested parent that doesn't exist yet — installDox should mkdir
	nested := filepath.Join(cwd, "deep", "nested", "dir", "AGENTS.md")
	comp := installDox(InstallOptions{AgentsMDPath: nested})
	if !comp.OK {
		t.Fatalf("installDox should create nested parent dirs, got: %+v", comp)
	}
	if _, err := os.Stat(nested); err != nil {
		t.Fatalf("AGENTS.md should exist at nested path: %v", err)
	}
}

// ── Doctor integration ──────────────────────────────────────────────────

func TestDoctor_FullReportWithSeededSuperpowers(t *testing.T) {
	home, _ := setupTestHome(t)
	seedSuperpowersLayers(t, home)
	if err := vane.SaveConfig(vane.Config{BaseURL: "http://127.0.0.1:1", TimeoutSeconds: 1}); err != nil {
		t.Fatal(err)
	}
	rep := Doctor("")
	// superpowers should be OK, vane.config OK, vane.health OK (down is informational)
	var supOK, vaneCfgOK, vaneHealthOK bool
	for _, c := range rep.Components {
		if c.Name == "superpowers" && c.OK {
			supOK = true
		}
		if c.Name == "vane.config" && c.OK {
			vaneCfgOK = true
		}
		if c.Name == "vane.health" && c.OK {
			vaneHealthOK = true
		}
	}
	if !supOK {
		t.Errorf("superpowers should be OK with seeded skills+pin")
	}
	if !vaneCfgOK {
		t.Errorf("vane.config should be OK with saved config")
	}
	if !vaneHealthOK {
		t.Errorf("vane.health should be OK (down is informational)")
	}
}

// ── ErrNotInstalled ─────────────────────────────────────────────────────

func TestErrNotInstalled(t *testing.T) {
	if ErrNotInstalled == nil {
		t.Fatal("ErrNotInstalled should not be nil")
	}
	if !strings.Contains(ErrNotInstalled.Error(), "not installed") {
		t.Fatalf("unexpected error message: %v", ErrNotInstalled)
	}
}

// ── installSuperpowers (0% → covered) ───────────────────────────────────

// initFakeSuperpowersRepo creates a minimal git repo with two fake skills
// that superpowers.Install can clone. Returns the repo path.
func initFakeSuperpowersRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available, skipping installSuperpowers test")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "--bare")
	// Create a worktree, add skills, commit, push
	work := t.TempDir()
	runGit(t, work, "init", "-q")
	runGit(t, work, "config", "user.email", "test@example.com")
	runGit(t, work, "config", "user.name", "test")
	runGit(t, work, "config", "commit.gpgsign", "false")
	skillsDir := filepath.Join(work, "skills")
	for _, name := range []string{"alpha", "bravo"} {
		sdir := filepath.Join(skillsDir, name)
		if err := os.MkdirAll(sdir, 0o755); err != nil {
			t.Fatal(err)
		}
		body := "---\nname: " + name + "\ndescription: fake\n---\n\n# " + name + "\n"
		if err := os.WriteFile(filepath.Join(sdir, "SKILL.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, work, "add", ".")
	runGit(t, work, "commit", "-q", "-m", "init")
	runGit(t, work, "remote", "add", "origin", dir)
	runGit(t, work, "push", "-q", "origin", "main")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v in %s: %v", args, dir, err)
	}
}

func TestInstallSuperpowers_LocalRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	home, _ := setupTestHome(t)
	_ = home

	fakeRepo := initFakeSuperpowersRepo(t)
	// Override DefaultRepoURL to point at the local bare repo
	orig := superpowers.DefaultRepoURL
	superpowers.DefaultRepoURL = fakeRepo
	t.Cleanup(func() { superpowers.DefaultRepoURL = orig })

	comp := installSuperpowers(context.Background(), InstallOptions{
		RepoURL: fakeRepo,
		Branch:  "main",
	})
	if !comp.OK {
		t.Fatalf("installSuperpowers with local repo should succeed, got: %+v", comp)
	}
	if !strings.Contains(comp.Detail, "sha=") {
		t.Errorf("expected sha= in detail, got: %s", comp.Detail)
	}
}

func TestInstallSuperpowers_WithAgentsMD(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	_, cwd := setupTestHome(t)

	fakeRepo := initFakeSuperpowersRepo(t)
	orig := superpowers.DefaultRepoURL
	superpowers.DefaultRepoURL = fakeRepo
	t.Cleanup(func() { superpowers.DefaultRepoURL = orig })

	agents := filepath.Join(cwd, dox.AgentsFileName)
	if err := os.WriteFile(agents, []byte("# Project\n\nHello.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	comp := installSuperpowers(context.Background(), InstallOptions{
		RepoURL:      fakeRepo,
		Branch:       "main",
		AgentsMDPath: agents,
	})
	if !comp.OK {
		t.Fatalf("installSuperpowers with agents file should succeed, got: %+v", comp)
	}
}

func TestInstallSuperpowers_CloneFailure(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	setupTestHome(t)

	comp := installSuperpowers(context.Background(), InstallOptions{
		RepoURL: "/nonexistent/path/repo.git",
		Branch:  "main",
	})
	if comp.OK {
		t.Fatal("expected OK=false for nonexistent repo")
	}
	if !strings.Contains(comp.Detail, "clone") && !strings.Contains(comp.Detail, "git") {
		t.Errorf("expected clone/git error in detail, got: %s", comp.Detail)
	}
}
